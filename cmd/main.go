package main

import (
	"fmt"
	"net"
	"os"
)

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

	body := []byte("Hello, World!")
	conn.Write([]byte("HTTP/1.1 200 OK\r\n"))
	conn.Write([]byte("Content-Type: text/plain\r\n"))
	conn.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(body))))
	conn.Write([]byte("\r\n"))
	conn.Write(body)
}
