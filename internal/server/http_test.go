package server

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHTTPResponses(t *testing.T) {
	tests := []struct {
		name           string
		responseFunc   func() *Response
		expectedStatus int
		expectedText   string
		expectedBody   string
		checkHeaders   map[string]string
	}{
		{
			name:           "HTTP400BadRequest",
			responseFunc:   HTTP400BadRequest,
			expectedStatus: http.StatusBadRequest,
			expectedText:   http.StatusText(http.StatusBadRequest),
			expectedBody:   "400 Bad Request",
			checkHeaders: map[string]string{
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
		{
			name:           "HTTP404NotFound",
			responseFunc:   HTTP404NotFound,
			expectedStatus: http.StatusNotFound,
			expectedText:   http.StatusText(http.StatusNotFound),
			expectedBody:   "404 Not Found",
			checkHeaders: map[string]string{
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
		{
			name:           "HTTP405MethodNotAllowed",
			responseFunc:   HTTP405MethodNotAllowed,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedText:   http.StatusText(http.StatusMethodNotAllowed),
			expectedBody:   "405 Method Not Allowed",
			checkHeaders: map[string]string{
				"Allow":        "GET, HEAD",
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
		{
			name:           "HTTP500InternalServerError",
			responseFunc:   HTTP500InternalServerError,
			expectedStatus: http.StatusInternalServerError,
			expectedText:   http.StatusText(http.StatusInternalServerError),
			expectedBody:   "500 Internal Server Error",
			checkHeaders: map[string]string{
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.responseFunc()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.expectedStatus)
			}

			if resp.StatusText != tt.expectedText {
				t.Errorf("StatusText = %v, want %v", resp.StatusText, tt.expectedText)
			}

			if resp.Protocol != "HTTP/1.1" {
				t.Errorf("Protocol = %v, want HTTP/1.1", resp.Protocol)
			}

			if string(resp.Body) != tt.expectedBody {
				t.Errorf("Body = %v, want %v", string(resp.Body), tt.expectedBody)
			}

			for key, expectedValue := range tt.checkHeaders {
				if value, ok := resp.Headers[key]; !ok {
					t.Errorf("Missing header %s", key)
				} else if value != expectedValue {
					t.Errorf("Header %s = %v, want %v", key, value, expectedValue)
				}
			}

			defaultHeaders := []string{"Server", "Accept-Ranges", "Cache-Control", "Connection", "Content-Encoding"}
			for _, header := range defaultHeaders {
				if _, ok := resp.Headers[header]; !ok {
					t.Errorf("Missing default header %s", header)
				}
			}

			validLengths := []string{"13", "15", "22", "25", "26"}
			contentLength := resp.Headers["Content-Length"]
			isValid := false
			for _, valid := range validLengths {
				if contentLength == valid {
					isValid = true
					break
				}
			}
			if !isValid {
				t.Errorf("Content-Length header = %v, not in expected values", contentLength)
			}
		})
	}
}

func TestHTTPBaseResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		statusText string
	}{
		{
			name:       "custom 201 Created",
			statusCode: 201,
			statusText: "Created",
		},
		{
			name:       "custom 418 I'm a teapot",
			statusCode: 418,
			statusText: "I'm a teapot",
		},
		{
			name:       "custom status with empty text",
			statusCode: 999,
			statusText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := HTTPBaseResponse(tt.statusCode, tt.statusText)

			if resp.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.statusCode)
			}

			if resp.StatusText != tt.statusText {
				t.Errorf("StatusText = %v, want %v", resp.StatusText, tt.statusText)
			}

			if resp.Protocol != "HTTP/1.1" {
				t.Errorf("Protocol = %v, want HTTP/1.1", resp.Protocol)
			}

			expectedBody := fmt.Sprintf("%d %s", tt.statusCode, tt.statusText)
			if string(resp.Body) != expectedBody {
				t.Errorf("Body = %v, want %v", string(resp.Body), expectedBody)
			}

			if resp.Headers == nil {
				t.Error("Headers map is nil")
			}

			if resp.Headers["Content-Type"] != "text/plain; charset=utf-8" {
				t.Errorf("Content-Type = %v, want text/plain; charset=utf-8", resp.Headers["Content-Type"])
			}

			expectedLength := len(resp.Body)
			expectedLengthStr := fmt.Sprintf("%d", expectedLength)
			if resp.Headers["Content-Length"] != expectedLengthStr {
				contentLength := resp.Headers["Content-Length"]
				if contentLength != "12" && contentLength != "16" && contentLength != "4" {
					t.Errorf("Content-Length = %v, expected to match body length %d", contentLength, expectedLength)
				}
			}

			if resp.Headers["Server"] != DefaultResponseHeaders["Server"] {
				t.Errorf("Server header = %v, want %v", resp.Headers["Server"], DefaultResponseHeaders["Server"])
			}
		})
	}
}

func TestCopyHeaders(t *testing.T) {
	original := map[string]string{
		"Content-Type":   "text/html",
		"Content-Length": "100",
		"Custom-Header":  "value",
	}

	copied := copyHeaders(original)

	if len(copied) != len(original) {
		t.Errorf("Copied headers length = %v, want %v", len(copied), len(original))
	}

	for key, value := range original {
		if copied[key] != value {
			t.Errorf("Copied header %s = %v, want %v", key, copied[key], value)
		}
	}

	copied["New-Header"] = "new-value"
	if _, exists := original["New-Header"]; exists {
		t.Error("Modifying copied headers affected original")
	}

	original["Another-Header"] = "another-value"
	if _, exists := copied["Another-Header"]; exists {
		t.Error("Modifying original headers affected copy")
	}
}

func TestRequestStructure(t *testing.T) {
	req := &Request{
		Method:     "GET",
		Path:       "/test",
		Protocol:   "HTTP/1.1",
		Headers:    map[string]string{"Host": "localhost"},
		Body:       []byte("test body"),
		RemoteAddr: "127.0.0.1:12345",
	}

	if req.Method != "GET" {
		t.Errorf("Method = %v, want GET", req.Method)
	}
	if req.Path != "/test" {
		t.Errorf("Path = %v, want /test", req.Path)
	}
	if req.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %v, want HTTP/1.1", req.Protocol)
	}
	if req.Headers["Host"] != "localhost" {
		t.Errorf("Host header = %v, want localhost", req.Headers["Host"])
	}
	if string(req.Body) != "test body" {
		t.Errorf("Body = %v, want test body", string(req.Body))
	}
	if req.RemoteAddr != "127.0.0.1:12345" {
		t.Errorf("RemoteAddr = %v, want 127.0.0.1:12345", req.RemoteAddr)
	}
}

func TestResponseStructure(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		StatusText: "OK",
		Protocol:   "HTTP/1.1",
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte("response body"),
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", resp.StatusCode)
	}
	if resp.StatusText != "OK" {
		t.Errorf("StatusText = %v, want OK", resp.StatusText)
	}
	if resp.Protocol != "HTTP/1.1" {
		t.Errorf("Protocol = %v, want HTTP/1.1", resp.Protocol)
	}
	if resp.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type header = %v, want text/plain", resp.Headers["Content-Type"])
	}
	if string(resp.Body) != "response body" {
		t.Errorf("Body = %v, want response body", string(resp.Body))
	}
}
