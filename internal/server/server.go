package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultResponseHeaders defines the default headers for all responses
var DefaultResponseHeaders = map[string]string{
	"Accept-Ranges":    "bytes",
	"Cache-Control":    "no-cache",
	"Connection":       "keep-alive",
	"Content-Encoding": "identity",
	"Content-Type":     "text/plain; charset=utf-8",
	"Server":           "tiny-http/0.1",
}

// Router defines the interface for HTTP request routing
type Router interface {
	Match(path string) (Handler, bool)
	AddRoute(pattern string, handler Handler)
}

// HTTPRouter implements the Router interface using regex pattern matching
type HTTPRouter struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	patterns []*compiledPattern
}

type compiledPattern struct {
	pattern string
	regex   *regexp.Regexp
	handler Handler
}

// NewHTTPRouter creates a new HTTP router
func NewHTTPRouter() *HTTPRouter {
	return &HTTPRouter{
		handlers: make(map[string]Handler),
		patterns: make([]*compiledPattern, 0),
	}
}

// AddRoute adds a new route to the router
func (r *HTTPRouter) AddRoute(pattern string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if it looks like a regex pattern (contains regex metacharacters)
	isRegexPattern := strings.ContainsAny(pattern, "^$.*+?{}[]|()")

	if isRegexPattern {
		// Try to compile as regex
		if re, err := regexp.Compile(pattern); err == nil {
			r.patterns = append(r.patterns, &compiledPattern{
				pattern: pattern,
				regex:   re,
				handler: handler,
			})
		} else {
			// Invalid regex, store as exact match
			r.handlers[pattern] = handler
		}
	} else {
		// Plain string, store as exact match
		r.handlers[pattern] = handler
	}
}

// Match finds a handler for the given path
func (r *HTTPRouter) Match(path string) (HandlerFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check exact matches first
	if handler, ok := r.handlers[path]; ok {
		return handler.Handle(), true
	}

	// Check regex patterns
	for _, cp := range r.patterns {
		if cp.regex.MatchString(path) {
			return cp.handler.Handle(), true
		}
	}

	return nil, false
}

// Server defines the interface for HTTP servers
type Server interface {
	ListenAndServe(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// HTTPServer implements a simple HTTP server
type HTTPServer struct {
	Addr          string
	Router        *HTTPRouter
	Middlewares   []Middleware
	Logger        *slog.Logger
	FileDirectory string

	mu       sync.Mutex
	listener net.Listener
	wg       sync.WaitGroup
	shutdown bool
	ctx      context.Context // Add this field
}

// NewHTTPServer creates a new HTTP server instance
func NewHTTPServer(addr string, fileDirectory string, logger *slog.Logger) *HTTPServer {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	router := NewHTTPRouter()

	// Add file handler for all paths
	router.AddRoute(`^/.*$`, &FileHandler{
		FileDirectory: fileDirectory,
		Logger:        logger,
	})

	return &HTTPServer{
		Addr:          addr,
		Router:        router,
		FileDirectory: fileDirectory,
		Middlewares: []Middleware{
			BaseMiddleware,
			LoggingMiddleware(logger),
			GzipMiddleware,
		},
		Logger: logger,
		ctx:    context.Background(), // Initialize with background context
	}
}

// ListenAndServe starts the HTTP server and blocks until shutdown
func (s *HTTPServer) ListenAndServe(ctx context.Context) error {
	// Store context for use in handleConnection
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Addr, err)
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.Logger.Info("Server starting", "address", listener.Addr().String())

	// Channel to collect connection handling errors
	connErrors := make(chan error, 100)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Handle connections in a goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					// Context cancelled, stop accepting connections
					s.Logger.Info("Listener closed due to shutdown")
					return
				default:
					connErrors <- fmt.Errorf("failed to accept connection: %w", err)
					continue
				}
			}

			// Handle connection in a goroutine
			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				s.handleConnection(c)
			}(conn)
		}
	}()

	// Wait for shutdown signal or fatal error
	select {
	case <-ctx.Done():
		s.Logger.Info("Shutdown signal received")
		// Create a new context with timeout for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-connErrors:
		return err
	}
}

// Shutdown waits for active connections to finish and closes the listener
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdown = true
	listener := s.listener
	s.mu.Unlock()

	if listener != nil {
		s.Logger.Info("Closing listener")
		if err := listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
	}

	// Wait for active connections to finish with timeout
	done := make(chan struct{})
	go func() {
		s.Logger.Info("Waiting for active connections to finish")
		s.wg.Wait()
		s.Logger.Info("All connections finished")
		close(done)
	}()

	select {
	case <-done:
		s.Logger.Info("All connections closed gracefully")
		return nil
	case <-ctx.Done():
		s.Logger.Warn("Shutdown timeout reached, forcing close")
		return ctx.Err()
	}
}

// handleConnection processes incoming connections and supports keep-alive
func (s *HTTPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		if s.ctx != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
			}
		}

		// Parse the request
		req, err := s.parseRequest(reader)
		if err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			if !isConnectionClosedError(err) {
				s.Logger.Error("Failed to parse request", "error", err)
			}
			return
		}

		resp := s.handleRequest(req)

		if err := s.writeResponse(writer, resp); err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			s.Logger.Error("Failed to write response", "error", err)
			return
		}

		if strings.ToLower(req.Headers["Connection"]) == "close" {
			return
		}

		conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
}

// Helper function to check if error is due to connection being closed
func isConnectionClosedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF")
}

func (s *HTTPServer) handleRequest(request *Request) *Response {
	if request.Method != "GET" && request.Method != "HEAD" {
		s.Logger.Warn("unsupported method", "method", request.Method)
		return HTTP405MethodNotAllowed()
	}

	handler, found := s.Router.Match(request.Path)
	if !found {
		s.Logger.Warn("no handler found", "path", request.Path)
		return HTTP404NotFound()
	}

	handlerPipeline := handler
	for i := len(s.Middlewares) - 1; i >= 0; i-- {
		handlerPipeline = s.Middlewares[i](handlerPipeline)
	}

	response, err := handlerPipeline(request)
	if err != nil {
		s.Logger.Error("handler error", "error", err, "path", request.Path)
		return HTTP500InternalServerError()
	}

	if request.Method == "HEAD" {
		response.Body = nil
		if response.Reader != nil {
			response.Reader.Close()
			response.Reader = nil
		}
	}

	return response
}

func (s *HTTPServer) parseRequest(reader *bufio.Reader) (*Request, error) {
	startLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read request line: %w", err)
	}

	startLine = strings.TrimRight(startLine, "\r\n")
	parts := strings.Split(startLine, " ")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid request line: %s", startLine)
	}

	request := &Request{
		Method:   parts[0],
		Path:     parts[1],
		Protocol: parts[2],
		Headers:  make(map[string]string),
	}

	if request.Protocol != "HTTP/1.1" && request.Protocol != "HTTP/1.0" {
		return nil, fmt.Errorf("unsupported protocol: %s", request.Protocol)
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // End of headers
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue // Skip malformed headers
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		request.Headers[key] = value
	}

	if clHeader, ok := request.Headers["Content-Length"]; ok {
		cl, err := strconv.Atoi(clHeader)
		if err != nil || cl < 0 {
			return nil, fmt.Errorf("invalid content-length: %s", clHeader)
		}

		if cl > 0 {
			body := make([]byte, cl)
			_, err := io.ReadFull(reader, body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %w", err)
			}
			request.Body = body
		}
	}

	return request, nil
}

// writeResponse writes an HTTP response to a writer
func (s *HTTPServer) writeResponse(writer *bufio.Writer, response *Response) error {
	// Write status line
	_, err := fmt.Fprintf(writer, "%s %d %s\r\n", response.Protocol, response.StatusCode, response.StatusText)
	if err != nil {
		return err
	}

	// Write headers
	for key, value := range response.Headers {
		_, err := fmt.Fprintf(writer, "%s: %s\r\n", key, value)
		if err != nil {
			return err
		}
	}

	// End headers
	_, err = writer.WriteString("\r\n")
	if err != nil {
		return err
	}

	// Flush headers first
	if err = writer.Flush(); err != nil {
		return err
	}

	// Write body
	if response.Reader != nil {
		// Stream from reader for large files
		defer response.Reader.Close()

		// Stream directly to the writer (which is already *bufio.Writer)
		// Copy in chunks to avoid loading entire file into memory
		_, err := io.Copy(writer, response.Reader)
		if err != nil {
			return err
		}

		// Flush the writer
		return writer.Flush()
	} else if len(response.Body) > 0 {
		// Write body from memory for small files
		_, err = writer.Write(response.Body)
		if err != nil {
			return err
		}
	}

	return writer.Flush()
}

// copyHeaders creates a copy of a header map
func copyHeaders(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
