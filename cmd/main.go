package main

import (
	"flag"
	"fmt"
	"os"

	tiny "github.com/marcocampos/tiny-http/internal/server"
)

func main() {
	directory := flag.String("directory", "", "Directory to serve files from")
	hostname := flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
	port := flag.String("port", "8080", "Port to listen on")
	flag.Parse()

	if *directory == "" {
		fmt.Println("error: directory flag is required")
		flag.Usage()
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", *hostname, *port)
	server := tiny.NewHTTPServer(addr, *directory)
	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
