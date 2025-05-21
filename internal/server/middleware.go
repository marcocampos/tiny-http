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

		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		for key, value := range DefaultResponseHeaders {
			if _, exists := response.Headers[key]; !exists {
				response.Headers[key] = value
			}
		}

		if response.Protocol == "" {
			if request.Protocol != "" {
				response.Protocol = request.Protocol
			} else {
				response.Protocol = "HTTP/1.1"
			}
		}

		if _, exists := response.Headers["Content-Length"]; !exists {
			response.Headers["Content-Length"] = strconv.Itoa(len(response.Body))
		}

		return response, nil
	}
}

func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(request *Request) (*Response, error) {
			start := time.Now()

			logger.Info("request",
				"method", request.Method,
				"path", request.Path,
				"remote", request.RemoteAddr,
				"user-agent", request.Headers["User-Agent"],
			)

			response, err := next(request)
			duration := time.Since(start)

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
		acceptEncoding := request.Headers["Accept-Encoding"]
		if !strings.Contains(acceptEncoding, "gzip") {
			return next(request)
		}

		response, err := next(request)
		if err != nil {
			return response, err
		}

		if response.Headers["Content-Encoding"] != "" {
			return response, nil
		}

		if len(response.Body) < 1024 {
			return response, nil
		}

		contentType := response.Headers["Content-Type"]
		if shouldNotCompress(contentType) {
			return response, nil
		}

		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		defer gz.Close()

		if _, err := gz.Write(response.Body); err != nil {
			return nil, fmt.Errorf("failed to compress response: %w", err)
		}

		if err := gz.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %w", err)
		}

		response.Body = buf.Bytes()
		response.Headers["Content-Encoding"] = "gzip"
		response.Headers["Content-Length"] = strconv.Itoa(buf.Len())

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

		response.Headers["X-Content-Type-Options"] = "nosniff"
		response.Headers["X-Frame-Options"] = "DENY"
		response.Headers["X-XSS-Protection"] = "1; mode=block"
		response.Headers["Referrer-Policy"] = "strict-origin-when-cross-origin"

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
