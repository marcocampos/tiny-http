# tiny-http

A lightweight HTTP server written in Go that serves static files.

<!--toc:start-->
- [tiny-http](#tiny-http)
  - [Features](#features)
  - [Installation](#installation)
  - [Usage](#usage)
    - [Command Line](#command-line)
    - [Command Line Options](#command-line-options)
    - [Example](#example)
  - [Architecture](#architecture)
    - [Project Structure](#project-structure)
    - [Key Components](#key-components)
    - [Middleware](#middleware)
  - [Security Features](#security-features)
  - [Performance Optimizations](#performance-optimizations)
  - [Testing](#testing)
  - [Docker Support](#docker-support)
  - [Contributing](#contributing)
  - [License](#license)

## Features

- **Static File Serving**: Efficiently serves static files from a specified directory
- **MIME Type Detection**: Automatically detects and sets appropriate Content-Type headers
- **Directory Index**: Automatically serves `index.html` for directory requests
- **Gzip Compression**: Compresses responses when supported by the client
- **Security Headers**: Implements security best practices with proper headers
- **Graceful Shutdown**: Handles shutdown signals gracefully with connection draining
- **Request Logging**: Structured logging with configurable log levels
- **Connection Keep-Alive**: Supports HTTP/1.1 persistent connections
- **Regex-based Routing**: Flexible routing system with regex pattern support
- **Middleware Pipeline**: Extensible middleware system for request/response processing
- **Concurrent Request Handling**: Handles multiple connections concurrently
- **Request Timeouts**: Prevents resource exhaustion with connection timeouts

## Installation

```bash
go get github.com/marcocampos/tiny-http
```

## Usage

### Command Line

```bash
# Build the server
go build ./cmd/main.go

# Run the server
./main -directory ./static -port 8080 -hostname 0.0.0.0 -log-level info
```

### Command Line Options

- `-directory` (required): Directory to serve files from
- `-hostname`: Hostname or IP address to bind to (default: "0.0.0.0")
- `-port`: Port to listen on (default: "8080")
- `-log-level`: Log level - debug, info, warn, error (default: "info")

### Example

```bash
# Serve files from the current directory on port 3000
./main -directory . -port 3000

# Serve with debug logging
./main -directory ./public -log-level debug
```

## Architecture

### Project Structure

```
tiny-http/
├── cmd/
│   ├── main.go                 # Entry point with CLI argument parsing and server setup
│   └── main_test.go           # Integration tests for the main application
├── internal/
│   └── server/
│       ├── handlers.go         # File serving handler with MIME type detection and security
│       ├── handlers_test.go    # Tests for file handlers and static content serving
│       ├── http.go            # HTTP request/response types and router implementation
│       ├── http_test.go       # Tests for HTTP parsing and router functionality
│       ├── middleware.go      # Request/response middleware (logging, gzip, security)
│       ├── middleware_test.go # Tests for middleware components and pipeline
│       ├── server.go          # Core HTTP server with connection handling and shutdown
│       └── server_test.go     # Tests for server lifecycle and connection management
├── static/
│   └── index.html            # Default static content for testing and demonstration
├── Dockerfile                # Container configuration for deployment
├── go.mod                   # Go module dependencies and version management
└── README.md               # Project documentation and usage instructions
```

### Key Components

1. **HTTPServer**: Main server struct that manages connections and request handling
2. **HTTPRouter**: Flexible router supporting both exact matches and regex patterns
3. **FileHandler**: Handles static file serving with security checks
4. **Middleware System**: Pluggable middleware for cross-cutting concerns

### Middleware

The server includes several built-in middleware:

- **BaseMiddleware**: Adds default headers and ensures proper response structure
- **LoggingMiddleware**: Logs all requests and responses with timing information
- **GzipMiddleware**: Compresses responses for supported clients
- **SecurityMiddleware**: Adds security headers (can be enabled)
- **CORSMiddleware**: Handles cross-origin requests (can be enabled)

## Security Features

- Path traversal protection
- Secure default headers
- Content-Type sniffing prevention
- XSS protection headers
- Configurable CORS support

## Performance Optimizations

- Concurrent connection handling with goroutines
- Efficient file reading with proper buffer management
- Smart gzip compression (skips already compressed formats)
- Regex patterns compiled once and reused

## Testing

Run the comprehensive test suite:

```bash
go test ./internal/server -v
```

The test suite covers:

- Router functionality (exact and regex matching)
- HTTP request parsing
- Middleware pipeline
- File serving with various scenarios
- Graceful shutdown behavior

## Docker Support

Build and run with Docker:

```bash
# Build the image
docker build -t tiny-http .

# Run the container
docker run -p 8080:8080 -v $(pwd)/static:/static tiny-http
```

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass
2. Code follows Go best practices
3. New features include appropriate tests
4. Documentation is updated

## License

This project is open source and available under the MIT License.

---
Happy Hacking!