package server

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var DefaulResponsetHeaders = map[string]string{
	"Accept-Ranges":    "bytes",
	"Cache-Control":    "no-cache",
	"Connection":       "keep-alive",
	"Content-Encoding": "identity",
	"Content-Type":     "text/plain; charset=utf-8",
	"Server":           "tinny-http/0.1",
}

type Router interface {
	Match(path string) (Handler, bool)
	AddRoute(pattern string, handler Handler)
}

type HttpRouter struct {
	handlers map[string]Handler
}

func (r *HttpRouter) AddRoute(pattern string, handler Handler) {
	if _, ok := r.handlers[pattern]; ok {
		return
	}
	r.handlers[pattern] = handler
}

func (r *HttpRouter) Match(path string) (HandlerFunc, bool) {
	if handler, ok := r.handlers[path]; ok {
		return handler.Handle(), true
	}

	for pattern, handler := range r.handlers {
		if pattern == path {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(path) {
			return handler.Handle(), true
		}
	}
	return nil, false
}

type Server interface {
	ListenAndServe() error
}

type HttpServer struct {
	Addr          string
	Router        *HttpRouter
	Middlewares   []Middleware
	Logger        *slog.Logger
	FileDirectory string
}

func NewHttpRouter() *HttpRouter {
	return &HttpRouter{
		handlers: make(map[string]Handler),
	}
}

func NewHTTPServer(addr string, fileDirectory string) *HttpServer {
	router := NewHttpRouter()
	router.AddRoute(`^\/[^\/]+$`, &FileHandler{
		FileDirectory: fileDirectory,
	})

	return &HttpServer{
		Addr:   addr,
		Router: router,
		Middlewares: []Middleware{
			BaseMiddleware,
			GzipMiddleware,
		},
		Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

func (s *HttpServer) ListenAndServe() error {
	s.Logger.Info("tinny-http: a simple HTTP server")
	s.Logger.Info("listening on", "addr", s.Addr)
	s.Logger.Info("serving files from", "directory", s.FileDirectory)

	listen, err := net.Listen("tcp4", s.Addr)
	if err != nil {
		s.Logger.Error("failed to listen", "addr", s.Addr, "error", err)
		return fmt.Errorf("failed to listen on port %s: %v", s.Addr, err)
	}
	defer listen.Close()
	s.Logger.Info("listening", "addr", s.Addr)

	for {
		conn, err := listen.Accept()
		if err != nil {
			s.Logger.Error("failed to accept connection", "error", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *HttpServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	s.Logger.Info("accepted connection", "remote", conn.RemoteAddr().String())

	var request *Request
	var response *Response
	var handler HandlerFunc
	var err error

	request, err = s.parseRequest(bufio.NewReader(conn))
	if err != nil {
		response = Http400BadRequest()
		conn.Write(s.marshalResponse(response))
		s.Logger.Error("failed to parse request", "error", err, "response", response.StatusCode)
		return
	}

	s.Logger.Info("parsed request",
		"method", request.Method,
		"path", request.Path,
		"protocol", request.Protocol,
	)

	if request.Method != "GET" {
		response = Http405MethodNotAllowed()
		conn.Write(s.marshalResponse(response))
		s.Logger.Warn("unsupported method", "method", request.Method, "response", response.StatusCode)
		return
	}

	handler, found := s.Router.Match(request.Path)
	if !found {
		response = Http404NotFound()
		conn.Write(s.marshalResponse(response))
		s.Logger.Warn("no handler found", "path", request.Path, "response", response.StatusCode)
		return
	}

	handlerPipeline := handler
	for _, middleware := range s.Middlewares {
		handlerPipeline = middleware(handlerPipeline)
	}

	response, err = handlerPipeline(request)
	if err != nil {
		response = Http500InternalServerError()
		conn.Write(s.marshalResponse(response))
		s.Logger.Error("failed to handle request", "error", err, "response", response.StatusCode)
		return
	}
	conn.Write(s.marshalResponse(response))
	s.Logger.Info("sent response", "status", response.StatusCode, "text", response.StatusText)
}

func (s *HttpServer) parseRequest(reader *bufio.Reader) (*Request, error) {
	startLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read request: %s", err.Error())
	}

	var request Request
	total, err := fmt.Sscanf(startLine, "%s %s %s", &request.Method, &request.Path, &request.Protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request line: %s", err.Error())
	}

	if total != 3 {
		return nil, fmt.Errorf("invalid request start line: %s", startLine)
	}

	if request.Protocol != "HTTP/1.1" {
		return nil, fmt.Errorf("unsupported protocol: %s", request.Protocol)
	}

	request.Headers = make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %s", err.Error())
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

	if clStr, ok := request.Headers["Content-Length"]; ok {
		cl, err := strconv.Atoi(clStr)
		if err != nil || cl < 0 {
			return nil, fmt.Errorf("invalid content-length")
		}
		if cl > 0 {
			body := make([]byte, cl)
			_, err := io.ReadFull(reader, body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %s", err.Error())
			}
			request.Body = body
		}
	}

	return &request, nil
}

func (s *HttpServer) marshalResponse(response *Response) []byte {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %d %s\r\n", response.Protocol, response.StatusCode, response.StatusText))
	for key, value := range response.Headers {
		sb.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	sb.WriteString("\r\n")
	sb.Write(response.Body)
	return []byte(sb.String())
}

func copyHeaders(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
