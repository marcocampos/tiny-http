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

var (
	Version       = "n/a"
	CommitHash    = "n/a"
	BuildTimetamp = time.Now().Unix()
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

	isRegexPattern := strings.ContainsAny(pattern, "^$.*+?{}[]|()")

	if isRegexPattern {
		if re, err := regexp.Compile(pattern); err == nil {
			r.patterns = append(r.patterns, &compiledPattern{
				pattern: pattern,
				regex:   re,
				handler: handler,
			})
		} else {
			r.handlers[pattern] = handler
		}
	} else {
		r.handlers[pattern] = handler
	}
}

// Match finds a handler for the given path
func (r *HTTPRouter) Match(path string) (HandlerFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if handler, ok := r.handlers[path]; ok {
		return handler.Handle(), true
	}

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
	ctx      context.Context
}

// NewHTTPServer creates a new HTTP server instance
func NewHTTPServer(addr string, fileDirectory string, logger *slog.Logger) *HTTPServer {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	router := NewHTTPRouter()

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
		ctx:    context.Background(),
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

	s.Logger.Info("tinny-http server", "version", Version, "commit_hash", CommitHash, "build_timestamp", BuildTimetamp)
	s.Logger.Info("listening", "address", listener.Addr().String(), "directory", s.FileDirectory)

	connErrors := make(chan error, 100)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					s.Logger.Info("Listener closed due to shutdown")
					return
				default:
					connErrors <- fmt.Errorf("failed to accept connection: %w", err)
					continue
				}
			}

			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				s.handleConnection(c)
			}(conn)
		}
	}()

	select {
	case <-ctx.Done():
		s.Logger.Info("Shutdown signal received")
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
		s.Logger.Info("closing listener")
		if err := listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
	}

	done := make(chan struct{})
	go func() {
		s.Logger.Info("waiting for active connections to finish")
		s.wg.Wait()
		s.Logger.Info("all connections finished")
		close(done)
	}()

	select {
	case <-done:
		s.Logger.Info("all connections closed gracefully")
		s.Logger.Info("shutting down complete")
		return nil
	case <-ctx.Done():
		s.Logger.Warn("shutdown timeout reached, forcing close")
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

		req, err := s.parseRequest(reader)
		if err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			if !isConnectionClosedError(err) {
				s.Logger.Error("failed to parse request", "error", err)
			}
			return
		}

		resp := s.handleRequest(req)

		if err := s.writeResponse(writer, resp); err != nil {
			if s.ctx != nil && s.ctx.Err() != nil {
				return
			}
			s.Logger.Error("failed to write response", "error", err)
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
			break
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
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
	_, err := fmt.Fprintf(writer, "%s %d %s\r\n", response.Protocol, response.StatusCode, response.StatusText)
	if err != nil {
		return err
	}

	for key, value := range response.Headers {
		_, err := fmt.Fprintf(writer, "%s: %s\r\n", key, value)
		if err != nil {
			return err
		}
	}

	_, err = writer.WriteString("\r\n")
	if err != nil {
		return err
	}

	if err = writer.Flush(); err != nil {
		return err
	}

	if response.Reader != nil {
		defer response.Reader.Close()

		_, err := io.Copy(writer, response.Reader)
		if err != nil {
			return err
		}

		return writer.Flush()
	} else if len(response.Body) > 0 {
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
