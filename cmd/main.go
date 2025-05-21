package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
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

	request, err := parseRequest(bufio.NewReader(conn))
	if err != nil {
		fmt.Println("failed to parse request:", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n"))
		conn.Write([]byte("Content-Type: text/plain\r\n"))
		conn.Write([]byte("Content-Length: 0\r\n"))
		conn.Write([]byte("\r\n"))
		return
	}

	if request.Method != "GET" {
		fmt.Println("unsupported method:", request.Method)
		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n"))
		conn.Write([]byte("Content-Type: text/plain\r\n"))
		conn.Write([]byte("Content-Length: 0\r\n"))
		conn.Write([]byte("\r\n"))
		return
	}

	body := []byte("Hello, World!")
	conn.Write([]byte("HTTP/1.1 200 OK\r\n"))
	conn.Write([]byte("Content-Type: text/plain\r\n"))
	conn.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(body))))
	conn.Write([]byte("\r\n"))
	conn.Write(body)

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
	if request.Method != "GET" {
		return nil, fmt.Errorf("unsupported method: %s", request.Method)
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
			_, err := reader.Read(body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %s", err.Error())
			}
			request.Body = body
		}
	}

	return &request, nil
}
