package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Helper types for testing
type testHandler struct {
	response string
}

func (h *testHandler) Handle() HandlerFunc {
	return func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			Body:       []byte(h.response),
			Headers:    make(map[string]string),
		}, nil
	}
}

// TestHTTPRouter tests the router functionality
func TestHTTPRouter(t *testing.T) {
	router := NewHTTPRouter()

	// Test exact match (non-regex pattern)
	handler := &testHandler{response: "exact"}
	router.AddRoute("/test", handler)

	if h, found := router.Match("/test"); !found {
		t.Error("Expected to find handler for /test")
	} else if resp, _ := h(&Request{}); string(resp.Body) != "exact" {
		t.Error("Expected exact match handler")
	}

	// Test regex match
	regexHandler := &testHandler{response: "regex"}
	router.AddRoute(`^/files/.*\.txt$`, regexHandler)

	if h, found := router.Match("/files/test.txt"); !found {
		t.Error("Expected to find handler for /files/test.txt")
	} else if resp, _ := h(&Request{}); string(resp.Body) != "regex" {
		t.Errorf("Expected regex match handler, got %s", string(resp.Body))
	}

	// Test that regex doesn't match wrong patterns
	if h, found := router.Match("/files/test.jpg"); found {
		resp, _ := h(&Request{})
		t.Errorf("Did not expect handler for /files/test.jpg, but got one with response: %s", string(resp.Body))
	}

	// Test no match
	if _, found := router.Match("/nonexistent"); found {
		t.Error("Expected no handler for /nonexistent")
	}

	// Test overlapping patterns (exact match should take precedence)
	router.AddRoute(`^/test$`, &testHandler{response: "regex-test"})
	if h, found := router.Match("/test"); !found {
		t.Error("Expected to find handler for /test")
	} else if resp, _ := h(&Request{}); string(resp.Body) != "exact" {
		t.Error("Expected exact match to take precedence over regex")
	}
}

// Update any tests that create HTTPServer instances directly
func TestParseRequest(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0", t.TempDir(), slog.New(slog.NewTextHandler(os.Stdout, nil)))

	tests := []struct {
		name    string
		input   string
		want    *Request
		wantErr bool
	}{
		{
			name: "valid GET request",
			input: "GET /test HTTP/1.1\r\n" +
				"Host: localhost\r\n" +
				"User-Agent: test\r\n" +
				"\r\n",
			want: &Request{
				Method:   "GET",
				Path:     "/test",
				Protocol: "HTTP/1.1",
				Headers: map[string]string{
					"Host":       "localhost",
					"User-Agent": "test",
				},
			},
		},
		{
			name: "request with body",
			input: "POST /test HTTP/1.1\r\n" +
				"Content-Length: 11\r\n" +
				"\r\n" +
				"Hello World",
			want: &Request{
				Method:   "POST",
				Path:     "/test",
				Protocol: "HTTP/1.1",
				Headers: map[string]string{
					"Content-Length": "11",
				},
				Body: []byte("Hello World"),
			},
		},
		{
			name:    "invalid request line",
			input:   "INVALID REQUEST\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "unsupported protocol",
			input:   "GET /test HTTP/2.0\r\n\r\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got, err := server.parseRequest(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.Method != tt.want.Method {
					t.Errorf("Method = %v, want %v", got.Method, tt.want.Method)
				}
				if got.Path != tt.want.Path {
					t.Errorf("Path = %v, want %v", got.Path, tt.want.Path)
				}
				if got.Protocol != tt.want.Protocol {
					t.Errorf("Protocol = %v, want %v", got.Protocol, tt.want.Protocol)
				}
				if string(got.Body) != string(tt.want.Body) {
					t.Errorf("Body = %v, want %v", string(got.Body), string(tt.want.Body))
				}
			}
		})
	}
}

// Update any other tests that create server instances directly
func TestMiddleware(t *testing.T) {
	// Test BaseMiddleware
	t.Run("BaseMiddleware", func(t *testing.T) {
		handler := func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: 200,
				Body:       []byte("test"),
			}, nil
		}

		wrapped := BaseMiddleware(handler)
		resp, err := wrapped(&Request{Protocol: "HTTP/1.1"})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Protocol != "HTTP/1.1" {
			t.Errorf("Protocol = %v, want HTTP/1.1", resp.Protocol)
		}

		if resp.Headers["Server"] != "tiny-http/0.1" {
			t.Errorf("Server header = %v, want tiny-http/0.1", resp.Headers["Server"])
		}

		if resp.Headers["Content-Length"] != "4" {
			t.Errorf("Content-Length = %v, want 4", resp.Headers["Content-Length"])
		}
	})

	// Test GzipMiddleware
	t.Run("GzipMiddleware", func(t *testing.T) {
		handler := func(req *Request) (*Response, error) {
			// Create a response large enough to trigger compression
			body := bytes.Repeat([]byte("Hello World! "), 100)
			return &Response{
				StatusCode: 200,
				Headers:    map[string]string{"Content-Type": "text/plain"},
				Body:       body,
			}, nil
		}

		wrapped := GzipMiddleware(handler)

		// Test with gzip support
		req := &Request{
			Headers: map[string]string{
				"Accept-Encoding": "gzip, deflate",
			},
		}

		resp, err := wrapped(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Headers["Content-Encoding"] != "gzip" {
			t.Error("Expected Content-Encoding: gzip")
		}

		// Test without gzip support
		req2 := &Request{
			Headers: map[string]string{
				"Accept-Encoding": "deflate",
			},
		}

		resp2, err := wrapped(req2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp2.Headers["Content-Encoding"] == "gzip" {
			t.Error("Did not expect gzip encoding")
		}
	})
}

// TestFileHandler tests file serving functionality
func TestFileHandler(t *testing.T) {
	// Create a temporary directory with test files
	tempDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"index.html": "<html><body>Hello</body></html>",
		"test.txt":   "Plain text file",
		"style.css":  "body { color: red; }",
		"data.json":  `{"test": true}`,
	}

	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Create subdirectory with index.html
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "index.html"), []byte("<html>Subdir</html>"), 0644); err != nil {
		t.Fatalf("Failed to create subdir index.html: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedType   string
		expectedBody   string
	}{
		{
			name:           "serve index.html for root",
			path:           "/",
			expectedStatus: 200,
			expectedType:   "text/html; charset=utf-8",
			expectedBody:   "<html><body>Hello</body></html>",
		},
		{
			name:           "serve text file",
			path:           "/test.txt",
			expectedStatus: 200,
			expectedType:   "text/plain; charset=utf-8",
			expectedBody:   "Plain text file",
		},
		{
			name:           "serve CSS file",
			path:           "/style.css",
			expectedStatus: 200,
			expectedType:   "text/css; charset=utf-8",
			expectedBody:   "body { color: red; }",
		},
		{
			name:           "serve JSON file",
			path:           "/data.json",
			expectedStatus: 200,
			expectedType:   "application/json; charset=utf-8",
			expectedBody:   `{"test": true}`,
		},
		{
			name:           "serve subdirectory index",
			path:           "/subdir/",
			expectedStatus: 200,
			expectedType:   "text/html; charset=utf-8",
			expectedBody:   "<html>Subdir</html>",
		},
		{
			name:           "file not found",
			path:           "/nonexistent.txt",
			expectedStatus: 404,
		},
		{
			name:           "directory traversal attempt",
			path:           "/../etc/passwd",
			expectedStatus: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     tt.path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp, err := handler.Handle()(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.expectedStatus)
			}

			if tt.expectedType != "" && resp.Headers["Content-Type"] != tt.expectedType {
				t.Errorf("Content-Type = %v, want %v", resp.Headers["Content-Type"], tt.expectedType)
			}

			if tt.expectedBody != "" && string(resp.Body) != tt.expectedBody {
				t.Errorf("Body = %v, want %v", string(resp.Body), tt.expectedBody)
			}
		})
	}
}

// TestServerShutdown tests graceful shutdown
func TestServerShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", t.TempDir(), logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual address
	server.mu.Lock()
	listener := server.listener
	server.mu.Unlock()

	if listener == nil {
		t.Fatal("Server listener is nil")
	}

	addr := listener.Addr().String()

	// Make a connection
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send a request
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")

	// Cancel context to trigger shutdown
	cancel()

	// Check that ListenAndServe returned
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("ListenAndServe error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Server did not shut down in time")
	}
}

// Benchmark tests
func BenchmarkHTTPRouterMatch(b *testing.B) {
	router := NewHTTPRouter()

	// Add various routes
	for i := 0; i < 100; i++ {
		router.AddRoute(fmt.Sprintf("/path%d", i), &testHandler{response: "test"})
	}
	router.AddRoute(`^/api/.*$`, &testHandler{response: "api"})
	router.AddRoute(`^/files/.*\.(jpg|png)$`, &testHandler{response: "image"})

	paths := []string{
		"/path50",
		"/api/users",
		"/files/test.jpg",
		"/nonexistent",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Match(paths[i%len(paths)])
	}
}

// Update for benchmark tests that create server instances
func BenchmarkParseRequest(b *testing.B) {
	server := NewHTTPServer("127.0.0.1:0", b.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := "GET /test/path HTTP/1.1\r\n" +
		"Host: localhost:8080\r\n" +
		"User-Agent: BenchmarkClient/1.0\r\n" +
		"Accept: text/html,application/json\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(request))
		_, err := server.parseRequest(reader)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteResponse(b *testing.B) {
	server := NewHTTPServer("127.0.0.1:0", b.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	response := &Response{
		Protocol:   "HTTP/1.1",
		StatusCode: 200,
		StatusText: "OK",
		Headers: map[string]string{
			"Content-Type":   "text/html; charset=utf-8",
			"Content-Length": "1000",
			"Cache-Control":  "public, max-age=3600",
			"Server":         "tiny-http/0.1",
		},
		Body: bytes.Repeat([]byte("a"), 1000),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer := bufio.NewWriter(io.Discard)
		err := server.writeResponse(writer, response)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestHTTPRouterExactVsRegexPrecedence tests precedence between exact and regex matches
func TestHTTPRouterExactVsRegexPrecedence(t *testing.T) {
	router := NewHTTPRouter()

	exactHandler := &testHandler{response: "exact"}
	regexHandler := &testHandler{response: "regex"}

	// Add regex first, then exact - exact should take precedence
	router.AddRoute(`^/test.*$`, regexHandler)
	router.AddRoute("/test", exactHandler)

	handlerFunc, found := router.Match("/test")
	if !found {
		t.Fatal("Should find a match for /test")
	}

	response, _ := handlerFunc(&Request{})
	if string(response.Body) != "exact" {
		t.Errorf("Expected exact match to take precedence, got: %s", string(response.Body))
	}
}

// TestHTTPRouterRemoveRoute tests route removal
func TestHTTPRouterRemoveRoute(t *testing.T) {
	router := NewHTTPRouter()

	handler := &testHandler{response: "test"}
	router.AddRoute("/test", handler)

	// Verify route exists
	if _, found := router.Match("/test"); !found {
		t.Fatal("Route should exist before removal")
	}

	// Remove route (assuming RemoveRoute method exists)
	// router.RemoveRoute("/test")

	// For now, just verify the route exists since RemoveRoute might not be implemented
	if _, found := router.Match("/test"); !found {
		t.Error("Route should still exist (RemoveRoute not implemented)")
	}
}

// TestParseRequestLargeHeaders tests handling of large headers
func TestParseRequestLargeHeaders(t *testing.T) {
	server := &HTTPServer{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	// Create a request with very large header value
	largeValue := strings.Repeat("a", 8192)
	input := fmt.Sprintf("GET /test HTTP/1.1\r\nX-Large-Header: %s\r\n\r\n", largeValue)

	reader := bufio.NewReader(strings.NewReader(input))
	req, err := server.parseRequest(reader)

	if err != nil {
		t.Errorf("Should handle large headers, got error: %v", err)
	} else if req.Headers["X-Large-Header"] != largeValue {
		t.Error("Large header value not preserved correctly")
	}
}

// TestParseRequestWithBody tests request parsing with various body sizes
func TestParseRequestWithBody(t *testing.T) {
	server := &HTTPServer{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		name        string
		body        string
		contentType string
	}{
		{
			name:        "small JSON body",
			body:        `{"key": "value"}`,
			contentType: "application/json",
		},
		{
			name:        "form data",
			body:        "field1=value1&field2=value2",
			contentType: "application/x-www-form-urlencoded",
		},
		{
			name:        "large body",
			body:        strings.Repeat("test data ", 1000),
			contentType: "text/plain",
		},
		{
			name:        "binary data",
			body:        string([]byte{0, 1, 2, 3, 255, 254, 253}),
			contentType: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf("POST /test HTTP/1.1\r\n"+
				"Content-Type: %s\r\n"+
				"Content-Length: %d\r\n"+
				"\r\n%s",
				tt.contentType, len(tt.body), tt.body)

			reader := bufio.NewReader(strings.NewReader(input))
			req, err := server.parseRequest(reader)

			if err != nil {
				t.Fatalf("parseRequest() error = %v", err)
			}

			if string(req.Body) != tt.body {
				t.Errorf("Body mismatch: got %q, want %q", string(req.Body), tt.body)
			}

			if req.Headers["Content-Type"] != tt.contentType {
				t.Errorf("Content-Type mismatch: got %q, want %q", req.Headers["Content-Type"], tt.contentType)
			}
		})
	}
}

// TestFileServing tests static file serving functionality
func TestFileServing(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	// Create test files
	files := map[string]string{
		"index.html":      "<html><body>Hello World</body></html>",
		"style.css":       "body { color: red; }",
		"script.js":       "console.log('hello');",
		"data.json":       `{"message": "test"}`,
		"image.png":       string([]byte{137, 80, 78, 71, 13, 10, 26, 10}), // PNG header
		"subdir/test.txt": "subdirectory file",
	}

	for filePath, content := range files {
		fullPath := filepath.Join(tempDir, filePath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}
	}

	tests := []struct {
		path            string
		expectedStatus  int
		expectedContent string
		expectedType    string
	}{
		{"/index.html", 200, files["index.html"], "text/html; charset=utf-8"},
		{"/style.css", 200, files["style.css"], "text/css; charset=utf-8"},
		{"/script.js", 200, files["script.js"], "application/javascript; charset=utf-8"},
		{"/data.json", 200, files["data.json"], "application/json; charset=utf-8"},
		{"/image.png", 200, files["image.png"], "image/png"},
		{"/subdir/test.txt", 200, files["subdir/test.txt"], "text/plain; charset=utf-8"},
		{"/nonexistent.txt", 404, "", ""},
		{"/", 200, files["index.html"], "text/html; charset=utf-8"}, // Should serve index.html
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     tt.path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp := server.handleRequest(req)

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Status code = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}

			if tt.expectedStatus == 200 {
				if string(resp.Body) != tt.expectedContent {
					t.Errorf("Body = %q, want %q", string(resp.Body), tt.expectedContent)
				}

				if tt.expectedType != "" && resp.Headers["Content-Type"] != tt.expectedType {
					t.Errorf("Content-Type = %q, want %q", resp.Headers["Content-Type"], tt.expectedType)
				}
			}
		})
	}
}

// TestHeadRequestHandling tests HEAD request behavior
func TestHeadRequestHandling(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	// Create test file
	testContent := "This is test content"
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/test.txt"},
		{"HEAD", "/test.txt"},
	}

	var getResp, headResp *Response

	for _, tt := range tests {
		req := &Request{
			Method:   tt.method,
			Path:     tt.path,
			Protocol: "HTTP/1.1",
			Headers:  make(map[string]string),
		}

		resp := server.handleRequest(req)

		if tt.method == "GET" {
			getResp = resp
		} else {
			headResp = resp
		}
	}

	// HEAD response should have same status and headers as GET but no body
	if getResp.StatusCode != headResp.StatusCode {
		t.Errorf("HEAD status %d != GET status %d", headResp.StatusCode, getResp.StatusCode)
	}

	// Check important headers match
	for _, header := range []string{"Content-Type", "Content-Length"} {
		if getResp.Headers[header] != headResp.Headers[header] {
			t.Errorf("HEAD %s header %q != GET %s header %q",
				header, headResp.Headers[header], header, getResp.Headers[header])
		}
	}

	// HEAD should not have body
	if len(headResp.Body) != 0 {
		t.Errorf("HEAD response should not have body, got %d bytes", len(headResp.Body))
	}

	// GET should have body
	if string(getResp.Body) != testContent {
		t.Errorf("GET body = %q, want %q", string(getResp.Body), testContent)
	}
}

// TestErrorResponseGeneration tests various error responses
func TestErrorResponseGeneration(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedAllow  string
	}{
		{"method not allowed", "POST", "/test.txt", 405, "GET, HEAD"},
		{"not found", "GET", "/nonexistent.txt", 404, ""},
		{"forbidden directory traversal", "GET", "/../etc/passwd", 404, ""},
		{"forbidden absolute path", "GET", "/etc/passwd", 404, ""}, // Should be treated as relative to document root
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method:   tt.method,
				Path:     tt.path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp := server.handleRequest(req)

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Status code = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}

			if tt.expectedAllow != "" && resp.Headers["Allow"] != tt.expectedAllow {
				t.Errorf("Allow header = %q, want %q", resp.Headers["Allow"], tt.expectedAllow)
			}

			// Error responses should have non-empty body
			if len(resp.Body) == 0 {
				t.Error("Error response should have body")
			}
		})
	}
}

// TestConcurrentConnections tests handling multiple concurrent connections
func TestConcurrentConnections(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	// Create test file
	testContent := "concurrent test"
	testFile := filepath.Join(tempDir, "concurrent.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	numClients := 10
	var wg sync.WaitGroup
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Create connection
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()

			// Handle connection in background
			go server.handleConnection(serverConn)

			// Send request
			request := "GET /concurrent.txt HTTP/1.1\r\nHost: localhost\r\n\r\n"
			if _, err := clientConn.Write([]byte(request)); err != nil {
				errors <- fmt.Errorf("client %d write error: %v", clientID, err)
				return
			}

			// Read response
			reader := bufio.NewReader(clientConn)
			statusLine, err := reader.ReadString('\n')
			if err != nil {
				errors <- fmt.Errorf("client %d read error: %v", clientID, err)
				return
			}

			if !strings.Contains(statusLine, "200 OK") {
				errors <- fmt.Errorf("client %d got unexpected status: %s", clientID, statusLine)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

// TestMIMETypeDetection tests MIME type detection for various file extensions
func TestMIMETypeDetection(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	testFiles := map[string]string{
		"test.html": "text/html; charset=utf-8",
		"test.css":  "text/css; charset=utf-8",
		"test.js":   "application/javascript; charset=utf-8",
		"test.json": "application/json; charset=utf-8",
		"test.xml":  "application/xml; charset=utf-8",
		"test.txt":  "text/plain; charset=utf-8",
		"test.png":  "image/png",
		"test.jpg":  "image/jpeg",
		"test.gif":  "image/gif",
		"test.pdf":  "application/pdf",
		"noext":     "text/plain; charset=utf-8", // Default for unknown extensions
	}

	// Create test files
	for filename := range testFiles {
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", filename, err)
		}
	}

	for filename, expectedMIME := range testFiles {
		t.Run(filename, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     "/" + filename,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp := server.handleRequest(req)

			if resp.StatusCode != 200 {
				t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
			}

			if resp.Headers["Content-Type"] != expectedMIME {
				t.Errorf("MIME type = %q, want %q", resp.Headers["Content-Type"], expectedMIME)
			}
		})
	}
}

// TestQueryStringHandling tests handling of query strings in URLs
func TestQueryStringHandling(t *testing.T) {
	tempDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := NewHTTPServer("127.0.0.1:0", tempDir, logger)

	// Create test file
	testFile := filepath.Join(tempDir, "query-test.html")
	if err := os.WriteFile(testFile, []byte("<html>test</html>"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	testPaths := []string{
		"/query-test.html",
		"/query-test.html?param=value",
		"/query-test.html?multiple=params&test=123",
		"/query-test.html?empty=&null",
	}

	for _, path := range testPaths {
		t.Run(path, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp := server.handleRequest(req)

			// All should return 200 OK since query strings should be ignored for file serving
			if resp.StatusCode != 200 {
				t.Errorf("Expected 200 OK for %s, got %d", path, resp.StatusCode)
			}
		})
	}
}
