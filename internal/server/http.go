package server

import (
	"fmt"
	"io"
	"net/http"
)

// Request represents an HTTP request
type Request struct {
	Method     string
	Path       string
	Protocol   string
	Headers    map[string]string
	Body       []byte
	RemoteAddr string // Client's remote address
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	StatusText string
	Protocol   string
	Headers    map[string]string
	Body       []byte
	Reader     io.ReadCloser // Add this field for streaming large files
}

// Common HTTP status responses

// HTTP400BadRequest returns a 400 Bad Request response
func HTTP400BadRequest() *Response {
	return HTTPBaseResponse(http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
}

// HTTP404NotFound returns a 404 Not Found response
func HTTP404NotFound() *Response {
	return HTTPBaseResponse(http.StatusNotFound, http.StatusText(http.StatusNotFound))
}

// HTTP405MethodNotAllowed returns a 405 Method Not Allowed response
func HTTP405MethodNotAllowed() *Response {
	response := HTTPBaseResponse(http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed))
	response.Headers["Allow"] = "GET, HEAD"
	return response
}

// HTTP500InternalServerError returns a 500 Internal Server Error response
func HTTP500InternalServerError() *Response {
	return HTTPBaseResponse(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}

// HTTPBaseResponse creates a basic HTTP response with default headers
func HTTPBaseResponse(statusCode int, statusText string) *Response {
	body := []byte(fmt.Sprintf("%d %s", statusCode, statusText))
	headers := copyHeaders(DefaultResponseHeaders)
	headers["Content-Length"] = fmt.Sprintf("%d", len(body))
	headers["Content-Type"] = "text/plain; charset=utf-8"

	return &Response{
		StatusCode: statusCode,
		StatusText: statusText,
		Protocol:   "HTTP/1.1",
		Headers:    headers,
		Body:       body,
	}
}
