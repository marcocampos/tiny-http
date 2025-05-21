package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Request struct {
	Method   string
	Path     string
	Protocol string
	Headers  map[string]string
	Body     []byte
}

type Response struct {
	StatusCode int
	StatusText string
	Protocol   string
	Headers    map[string]string
	Body       []byte
}

var DefaultHeaders = map[string]string{
	"Server":         "tinny-http/0.1",
	"Content-Type":   "text/plain",
	"Content-Length": "0",
}

type HandlerFunc func(*Request) (*Response, error)

type Middleware func(next HandlerFunc) HandlerFunc

type Router map[string]HandlerFunc

var DefaultMiddlewares = []Middleware{
	baseMiddleware,
}

var ServerRouter = Router{
	`^\/$`: rootHandler,
}

func main() {
	fmt.Println("tinny-http: a simple HTTP server")
	listen, err := net.Listen("tcp4", ":8080")
	if err != nil {
		fmt.Printf("failed to listen on port 8080: %v\n", err)
		os.Exit(1)
	}
	defer listen.Close()
	fmt.Println("listening on port 8080...")

	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Printf("failed to accept connection: %v\n", err)
			continue
		}
		handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Println("accepted connection from", conn.RemoteAddr())

	var request *Request
	var response *Response
	var handler HandlerFunc
	var err error

	request, err = parseRequest(bufio.NewReader(conn))
	if err != nil {
		response = &Response{
			StatusCode: 400,
			StatusText: "Bad Request",
			Protocol:   "HTTP/1.1",
			Headers:    DefaultHeaders,
			Body:       []byte{},
		}
		conn.Write(marshalResponse(response))
		fmt.Println("failed to parse request:", err)
		fmt.Println("response: ", response.StatusCode, response.StatusText)
		return
	}

	fmt.Println("parsed request:", request.Method, request.Path, request.Protocol)

	if request.Method != "GET" {
		response = &Response{
			StatusCode: 405,
			StatusText: "Method Not Allowed",
			Protocol:   "HTTP/1.1",
			Headers:    DefaultHeaders,
			Body:       []byte{},
		}
		conn.Write(marshalResponse(response))
		fmt.Println("unsupported method:", request.Method)
		fmt.Println("response: ", response.StatusCode, response.StatusText)
		return
	}

	handler, found := matchRoute(request.Path)
	if !found {
		response = &Response{
			StatusCode: 404,
			StatusText: "Not Found",
			Protocol:   "HTTP/1.1",
			Headers:    DefaultHeaders,
			Body:       []byte{},
		}
		conn.Write(marshalResponse(response))
		fmt.Println("no handler found for path: ", request.Path)
		fmt.Println("response: ", response.StatusCode, response.StatusText)
		return
	}

	handlerPipeline := handler
	for _, middleware := range DefaultMiddlewares {
		handlerPipeline = middleware(handlerPipeline)
	}

	response, err = handlerPipeline(request)
	if err != nil {
		response = &Response{
			StatusCode: 500,
			StatusText: "Internal Server Error",
			Protocol:   "HTTP/1.1",
			Headers:    DefaultHeaders,
			Body:       []byte{},
		}
		conn.Write(marshalResponse(response))
		fmt.Println("failed to handle request:", err)
		return
	}
	conn.Write(marshalResponse(response))
	fmt.Println("sent response: ", response.StatusCode, response.StatusText)
}

func parseRequest(reader *bufio.Reader) (*Request, error) {
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

func marshalResponse(response *Response) []byte {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %d %s\r\n", response.Protocol, response.StatusCode, response.StatusText))
	for key, value := range response.Headers {
		sb.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	sb.WriteString("\r\n")
	sb.Write(response.Body)
	return []byte(sb.String())
}

func matchRoute(path string) (HandlerFunc, bool) {
	if handler, ok := ServerRouter[path]; ok {
		return handler, true
	}

	for pattern, handler := range ServerRouter {
		if pattern == path {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(path) {
			return handler, true
		}
	}
	return nil, false
}

func rootHandler(request *Request) (*Response, error) {
	body := []byte("Hello, World!")
	return &Response{
		StatusCode: 200,
		StatusText: "OK",
		Protocol:   "HTTP/1.1",
		Body:       body,
	}, nil
}

func baseMiddleware(next HandlerFunc) HandlerFunc {
	return func(request *Request) (*Response, error) {
		response, err := next(request)
		if err != nil {
			return nil, err
		}

		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		for key, value := range DefaultHeaders {
			if _, ok := response.Headers[key]; !ok {
				response.Headers[key] = value
			}
		}

		response.Headers["Content-Length"] = strconv.Itoa(len(response.Body))
		response.Protocol = "HTTP/1.1"
		return response, nil
	}
}
