package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Middleware is a function that wraps a HandlerFunc to add functionality
type Middleware func(next HandlerFunc) HandlerFunc

// BaseMiddleware adds default headers and ensures proper response structure
func BaseMiddleware(next HandlerFunc) HandlerFunc {
	return func(request *Request) (*Response, error) {
		response, err := next(request)
		if err != nil {
			return nil, err
		}

		// Ensure headers map exists
		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		// Add default headers if not already set
		for key, value := range DefaultResponseHeaders {
			if _, exists := response.Headers[key]; !exists {
				response.Headers[key] = value
			}
		}

		// Ensure protocol is set
		if response.Protocol == "" {
			if request.Protocol != "" {
				response.Protocol = request.Protocol
			} else {
				response.Protocol = "HTTP/1.1"
			}
		}

		// Ensure Content-Length is set
		if _, exists := response.Headers["Content-Length"]; !exists {
			response.Headers["Content-Length"] = strconv.Itoa(len(response.Body))
		}

		return response, nil
	}
}

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(request *Request) (*Response, error) {
			start := time.Now()

			// Log request
			logger.Info("request",
				"method", request.Method,
				"path", request.Path,
				"remote", request.RemoteAddr,
				"user-agent", request.Headers["User-Agent"],
			)

			// Process request
			response, err := next(request)

			// Calculate duration
			duration := time.Since(start)

			// Log response
			if err != nil {
				logger.Error("request failed",
					"method", request.Method,
					"path", request.Path,
					"remote", request.RemoteAddr,
					"duration", duration,
					"error", err,
				)
			} else if response != nil {
				logger.Info("response",
					"method", request.Method,
					"path", request.Path,
					"remote", request.RemoteAddr,
					"status", response.StatusCode,
					"duration", duration,
					"size", len(response.Body),
				)
			}

			return response, err
		}
	}
}

// GzipMiddleware compresses responses with gzip when supported by the client
func GzipMiddleware(next HandlerFunc) HandlerFunc {
	return func(request *Request) (*Response, error) {
		// Check if client accepts gzip encoding
		acceptEncoding := request.Headers["Accept-Encoding"]
		if !strings.Contains(acceptEncoding, "gzip") {
			return next(request)
		}

		// Process request
		response, err := next(request)
		if err != nil {
			return response, err
		}

		// Don't compress if already compressed
		if response.Headers["Content-Encoding"] != "" {
			return response, nil
		}

		// Don't compress small responses (less than 1KB)
		if len(response.Body) < 1024 {
			return response, nil
		}

		// Don't compress certain content types
		contentType := response.Headers["Content-Type"]
		if shouldNotCompress(contentType) {
			return response, nil
		}

		// Compress the response body
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)

		// Write compressed data
		if _, err := gz.Write(response.Body); err != nil {
			gz.Close()
			return nil, fmt.Errorf("failed to compress response: %w", err)
		}

		if err := gz.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %w", err)
		}

		// Update response
		response.Body = buf.Bytes()
		response.Headers["Content-Encoding"] = "gzip"
		response.Headers["Content-Length"] = strconv.Itoa(buf.Len())

		// Add Vary header to indicate that response varies based on Accept-Encoding
		if vary := response.Headers["Vary"]; vary != "" {
			response.Headers["Vary"] = vary + ", Accept-Encoding"
		} else {
			response.Headers["Vary"] = "Accept-Encoding"
		}

		return response, nil
	}
}

// shouldNotCompress determines if a content type should not be compressed
func shouldNotCompress(contentType string) bool {
	// Already compressed formats
	noCompress := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"video/",
		"audio/",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-compress",
		"application/x-compressed",
	}

	contentType = strings.ToLower(contentType)
	for _, nc := range noCompress {
		if strings.Contains(contentType, nc) {
			return true
		}
	}

	return false
}

// SecurityMiddleware adds security-related headers
func SecurityMiddleware(next HandlerFunc) HandlerFunc {
	return func(request *Request) (*Response, error) {
		response, err := next(request)
		if err != nil {
			return response, err
		}

		// Add security headers
		response.Headers["X-Content-Type-Options"] = "nosniff"
		response.Headers["X-Frame-Options"] = "DENY"
		response.Headers["X-XSS-Protection"] = "1; mode=block"
		response.Headers["Referrer-Policy"] = "strict-origin-when-cross-origin"

		// Add CSP for HTML responses
		if strings.Contains(response.Headers["Content-Type"], "text/html") {
			response.Headers["Content-Security-Policy"] = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';"
		}

		return response, nil
	}
}

// CORSMiddleware adds CORS headers for cross-origin requests
func CORSMiddleware(allowedOrigins []string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(request *Request) (*Response, error) {
			origin := request.Headers["Origin"]

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			response, err := next(request)
			if err != nil {
				return response, err
			}

			if allowed && origin != "" {
				response.Headers["Access-Control-Allow-Origin"] = origin
				response.Headers["Access-Control-Allow-Methods"] = "GET, HEAD, OPTIONS"
				response.Headers["Access-Control-Allow-Headers"] = "Content-Type, Accept"
				response.Headers["Access-Control-Max-Age"] = "86400"
			}

			return response, nil
		}
	}
}
