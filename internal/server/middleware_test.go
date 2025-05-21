package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSecurityMiddleware(t *testing.T) {
	handler := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
			},
			Body: []byte("<html><body>Test</body></html>"),
		}, nil
	}

	wrapped := SecurityMiddleware(handler)

	req := &Request{
		Method:   "GET",
		Path:     "/test",
		Protocol: "HTTP/1.1",
		Headers:  make(map[string]string),
	}

	resp, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check security headers
	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		if resp.Headers[header] != expected {
			t.Errorf("Header %s = %v, want %v", header, resp.Headers[header], expected)
		}
	}

	// Check CSP for HTML responses
	if !strings.Contains(resp.Headers["Content-Security-Policy"], "default-src 'self'") {
		t.Error("Expected Content-Security-Policy header for HTML response")
	}
}

func TestSecurityMiddlewareNonHTML(t *testing.T) {
	handler := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: []byte(`{"test": true}`),
		}, nil
	}

	wrapped := SecurityMiddleware(handler)

	req := &Request{
		Method:   "GET",
		Path:     "/api/test",
		Protocol: "HTTP/1.1",
		Headers:  make(map[string]string),
	}

	resp, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should not have CSP for non-HTML responses
	if _, exists := resp.Headers["Content-Security-Policy"]; exists {
		t.Error("Should not have Content-Security-Policy header for non-HTML response")
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers:    make(map[string]string),
			Body:       []byte("test"),
		}, nil
	}

	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		expectCORS     bool
		expectedOrigin string
	}{
		{
			name:           "allow all origins",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://example.com",
			expectCORS:     true,
			expectedOrigin: "https://example.com",
		},
		{
			name:           "allow specific origin",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			requestOrigin:  "https://example.com",
			expectCORS:     true,
			expectedOrigin: "https://example.com",
		},
		{
			name:           "deny unlisted origin",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://evil.com",
			expectCORS:     false,
		},
		{
			name:           "no origin header",
			allowedOrigins: []string{"*"},
			requestOrigin:  "",
			expectCORS:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := CORSMiddleware(tt.allowedOrigins)(handler)

			req := &Request{
				Method:   "GET",
				Path:     "/test",
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			if tt.requestOrigin != "" {
				req.Headers["Origin"] = tt.requestOrigin
			}

			resp, err := wrapped(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectCORS {
				if resp.Headers["Access-Control-Allow-Origin"] != tt.expectedOrigin {
					t.Errorf("Access-Control-Allow-Origin = %v, want %v",
						resp.Headers["Access-Control-Allow-Origin"], tt.expectedOrigin)
				}

				if resp.Headers["Access-Control-Allow-Methods"] != "GET, HEAD, OPTIONS" {
					t.Error("Incorrect Access-Control-Allow-Methods header")
				}

				if resp.Headers["Access-Control-Allow-Headers"] != "Content-Type, Accept" {
					t.Error("Incorrect Access-Control-Allow-Headers header")
				}

				if resp.Headers["Access-Control-Max-Age"] != "86400" {
					t.Error("Incorrect Access-Control-Max-Age header")
				}
			} else {
				if _, exists := resp.Headers["Access-Control-Allow-Origin"]; exists {
					t.Error("Should not have CORS headers")
				}
			}
		})
	}
}

func TestGzipMiddlewareCompression(t *testing.T) {
	// Create a response with compressible content
	largeText := strings.Repeat("This is a test string that should compress well. ", 50)

	handler := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type": "text/plain",
			},
			Body: []byte(largeText),
		}, nil
	}

	wrapped := GzipMiddleware(handler)

	req := &Request{
		Method:   "GET",
		Path:     "/test",
		Protocol: "HTTP/1.1",
		Headers: map[string]string{
			"Accept-Encoding": "gzip, deflate",
		},
	}

	resp, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that response was compressed
	if resp.Headers["Content-Encoding"] != "gzip" {
		t.Error("Expected Content-Encoding: gzip")
	}

	// Check Vary header
	if !strings.Contains(resp.Headers["Vary"], "Accept-Encoding") {
		t.Error("Expected Vary header to include Accept-Encoding")
	}

	// Decompress and verify content
	reader, err := gzip.NewReader(bytes.NewReader(resp.Body))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	if string(decompressed) != largeText {
		t.Error("Decompressed content doesn't match original")
	}

	// Verify compression actually reduced size
	if len(resp.Body) >= len(largeText) {
		t.Error("Compression didn't reduce size")
	}
}

func TestGzipMiddlewareNoCompression(t *testing.T) {
	tests := []struct {
		name           string
		acceptHeader   string
		contentType    string
		bodySize       int
		shouldCompress bool
	}{
		{
			name:           "no gzip support",
			acceptHeader:   "deflate",
			contentType:    "text/plain",
			bodySize:       2000,
			shouldCompress: false,
		},
		{
			name:           "small response",
			acceptHeader:   "gzip",
			contentType:    "text/plain",
			bodySize:       500, // Less than 1KB
			shouldCompress: false,
		},
		{
			name:           "already compressed format",
			acceptHeader:   "gzip",
			contentType:    "image/jpeg",
			bodySize:       2000,
			shouldCompress: false,
		},
		{
			name:           "zip file",
			acceptHeader:   "gzip",
			contentType:    "application/zip",
			bodySize:       2000,
			shouldCompress: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := make([]byte, tt.bodySize)
			for i := range body {
				body[i] = byte(i % 256)
			}

			handler := func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					StatusText: "OK",
					Headers: map[string]string{
						"Content-Type": tt.contentType,
					},
					Body: body,
				}, nil
			}

			wrapped := GzipMiddleware(handler)

			req := &Request{
				Method:   "GET",
				Path:     "/test",
				Protocol: "HTTP/1.1",
				Headers: map[string]string{
					"Accept-Encoding": tt.acceptHeader,
				},
			}

			resp, err := wrapped(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.shouldCompress {
				if resp.Headers["Content-Encoding"] != "gzip" {
					t.Error("Expected Content-Encoding: gzip")
				}
			} else {
				if resp.Headers["Content-Encoding"] == "gzip" {
					t.Error("Should not have compressed response")
				}
				// Body should be unchanged
				if !bytes.Equal(resp.Body, body) {
					t.Error("Body was modified when it shouldn't have been")
				}
			}
		})
	}
}

func TestGzipMiddlewareExistingVaryHeader(t *testing.T) {
	handler := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type": "text/plain",
				"Vary":         "User-Agent",
			},
			Body: bytes.Repeat([]byte("test"), 500), // Large enough to compress
		}, nil
	}

	wrapped := GzipMiddleware(handler)

	req := &Request{
		Method:   "GET",
		Path:     "/test",
		Protocol: "HTTP/1.1",
		Headers: map[string]string{
			"Accept-Encoding": "gzip",
		},
	}

	resp, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should append to existing Vary header
	if resp.Headers["Vary"] != "User-Agent, Accept-Encoding" {
		t.Errorf("Vary header = %v, want 'User-Agent, Accept-Encoding'", resp.Headers["Vary"])
	}
}

func TestLoggingMiddlewareSuccess(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testResponseBody := []byte("test response")
	handler := func(req *Request) (*Response, error) {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers:    make(map[string]string),
			Body:       testResponseBody,
		}, nil
	}

	wrapped := LoggingMiddleware(logger)(handler)

	req := &Request{
		Method:     "GET",
		Path:       "/test/path",
		Protocol:   "HTTP/1.1",
		RemoteAddr: "192.168.1.1:12345",
		Headers: map[string]string{
			"User-Agent":     "TestAgent/1.0",
			"Content-Length": fmt.Sprintf("%d", len(testResponseBody)),
		},
	}

	resp, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", resp.StatusCode)
	}

	// Check log output
	logOutput := buf.String()

	// Should log request
	if !strings.Contains(logOutput, "request") {
		t.Error("Expected 'request' log entry")
	}
	if !strings.Contains(logOutput, "GET") {
		t.Error("Expected method in log")
	}
	if !strings.Contains(logOutput, "/test/path") {
		t.Error("Expected path in log")
	}
	if !strings.Contains(logOutput, "192.168.1.1:12345") {
		t.Error("Expected remote address in log")
	}
	if !strings.Contains(logOutput, "TestAgent/1.0") {
		t.Error("Expected user agent in log")
	}

	// Should log response
	if !strings.Contains(logOutput, "response") {
		t.Error("Expected 'response' log entry")
	}
	if !strings.Contains(logOutput, "status=200") {
		t.Error("Expected status code in log")
	}
	if !strings.Contains(logOutput, "size=13") { // "test response" is 13 bytes
		t.Error("Expected response size in log")
	}
	if !strings.Contains(logOutput, "duration=") {
		t.Error("Expected duration in log")
	}
}

func TestLoggingMiddlewareError(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := func(req *Request) (*Response, error) {
		return nil, io.EOF // Simulate an error
	}

	wrapped := LoggingMiddleware(logger)(handler)

	req := &Request{
		Method:     "POST",
		Path:       "/error/path",
		Protocol:   "HTTP/1.1",
		RemoteAddr: "10.0.0.1:54321",
		Headers:    make(map[string]string),
	}

	_, err := wrapped(req)
	if err == nil {
		t.Error("Expected error to be propagated")
	}

	// Check log output
	logOutput := buf.String()

	// Should log request
	if !strings.Contains(logOutput, "request") {
		t.Error("Expected 'request' log entry")
	}

	// Should log error
	if !strings.Contains(logOutput, "request failed") {
		t.Error("Expected 'request failed' log entry")
	}
	if !strings.Contains(logOutput, "error=") {
		t.Error("Expected error in log")
	}
	if !strings.Contains(logOutput, "EOF") {
		t.Error("Expected specific error message in log")
	}
}

func TestBaseMiddlewareDefaults(t *testing.T) {
	tests := []struct {
		name      string
		handler   HandlerFunc
		request   *Request
		checkFunc func(t *testing.T, resp *Response)
	}{
		{
			name: "adds missing headers",
			handler: func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					Body:       []byte("test"),
				}, nil
			},
			request: &Request{Protocol: "HTTP/1.1"},
			checkFunc: func(t *testing.T, resp *Response) {
				// Should have all default headers
				for key, value := range DefaultResponseHeaders {
					if resp.Headers[key] != value {
						t.Errorf("Header %s = %v, want %v", key, resp.Headers[key], value)
					}
				}
			},
		},
		{
			name: "preserves existing headers",
			handler: func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					Headers: map[string]string{
						"Server":       "custom-server",
						"Content-Type": "application/json",
					},
					Body: []byte("{}"),
				}, nil
			},
			request: &Request{Protocol: "HTTP/1.1"},
			checkFunc: func(t *testing.T, resp *Response) {
				// Should preserve custom headers
				if resp.Headers["Server"] != "custom-server" {
					t.Errorf("Server header = %v, want custom-server", resp.Headers["Server"])
				}
				if resp.Headers["Content-Type"] != "application/json" {
					t.Errorf("Content-Type = %v, want application/json", resp.Headers["Content-Type"])
				}
			},
		},
		{
			name: "sets protocol from request",
			handler: func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					Headers:    make(map[string]string),
					Body:       []byte("test"),
				}, nil
			},
			request: &Request{Protocol: "HTTP/1.0"},
			checkFunc: func(t *testing.T, resp *Response) {
				if resp.Protocol != "HTTP/1.0" {
					t.Errorf("Protocol = %v, want HTTP/1.0", resp.Protocol)
				}
			},
		},
		{
			name: "calculates content length",
			handler: func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					Headers:    make(map[string]string),
					Body:       []byte("Hello, World!"),
				}, nil
			},
			request: &Request{Protocol: "HTTP/1.1"},
			checkFunc: func(t *testing.T, resp *Response) {
				if resp.Headers["Content-Length"] != "13" {
					t.Errorf("Content-Length = %v, want 13", resp.Headers["Content-Length"])
				}
			},
		},
		{
			name: "handles nil headers map",
			handler: func(req *Request) (*Response, error) {
				return &Response{
					StatusCode: 200,
					Body:       []byte("test"),
					// Headers is nil
				}, nil
			},
			request: &Request{Protocol: "HTTP/1.1"},
			checkFunc: func(t *testing.T, resp *Response) {
				if resp.Headers == nil {
					t.Error("Headers map should not be nil")
				}
				// Should have default headers
				if resp.Headers["Server"] != DefaultResponseHeaders["Server"] {
					t.Error("Should have default Server header")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := BaseMiddleware(tt.handler)
			resp, err := wrapped(tt.request)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			tt.checkFunc(t, resp)
		})
	}
}

func TestMiddlewareChaining(t *testing.T) {
	// Test that multiple middlewares work together correctly
	var executionOrder []string

	// Create a test handler
	handler := func(req *Request) (*Response, error) {
		executionOrder = append(executionOrder, "handler")
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type": "text/plain",
			},
			Body: bytes.Repeat([]byte("test"), 500), // Large enough to compress
		}, nil
	}

	// Create middlewares that track execution
	middleware1 := func(next HandlerFunc) HandlerFunc {
		return func(req *Request) (*Response, error) {
			executionOrder = append(executionOrder, "middleware1-before")
			resp, err := next(req)
			executionOrder = append(executionOrder, "middleware1-after")
			return resp, err
		}
	}

	middleware2 := func(next HandlerFunc) HandlerFunc {
		return func(req *Request) (*Response, error) {
			executionOrder = append(executionOrder, "middleware2-before")
			resp, err := next(req)
			executionOrder = append(executionOrder, "middleware2-after")
			return resp, err
		}
	}

	// Chain middlewares
	wrapped := middleware1(middleware2(handler))

	req := &Request{
		Method:   "GET",
		Path:     "/test",
		Protocol: "HTTP/1.1",
		Headers:  make(map[string]string),
	}

	_, err := wrapped(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check execution order
	expectedOrder := []string{
		"middleware1-before",
		"middleware2-before",
		"handler",
		"middleware2-after",
		"middleware1-after",
	}

	if len(executionOrder) != len(expectedOrder) {
		t.Fatalf("Execution order length = %v, want %v", len(executionOrder), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if executionOrder[i] != expected {
			t.Errorf("Execution order[%d] = %v, want %v", i, executionOrder[i], expected)
		}
	}
}

func TestShouldNotCompress(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		// Should not compress
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		{"video/mp4", true},
		{"video/webm", true},
		{"audio/mpeg", true},
		{"audio/ogg", true},
		{"application/zip", true},
		{"application/gzip", true},
		{"application/x-gzip", true},
		{"application/x-compress", true},
		{"application/x-compressed", true},

		// Should compress
		{"text/plain", false},
		{"text/html", false},
		{"text/css", false},
		{"application/javascript", false},
		{"application/json", false},
		{"application/xml", false},

		// Case insensitive
		{"IMAGE/JPEG", true},
		{"Video/MP4", true},
		{"TEXT/PLAIN", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := shouldNotCompress(tt.contentType)
			if result != tt.expected {
				t.Errorf("shouldNotCompress(%s) = %v, want %v", tt.contentType, result, tt.expected)
			}
		})
	}
}
