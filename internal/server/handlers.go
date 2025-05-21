package server

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// HandlerFunc is a function that handles HTTP requests
type HandlerFunc func(*Request) (*Response, error)

// Handler is an interface for HTTP request handlers
type Handler interface {
	Handle() HandlerFunc
}

// FileHandler serves static files from a directory
type FileHandler struct {
	FileDirectory string
	Logger        *slog.Logger
}

// Handle returns the handler function for serving files
func (h *FileHandler) Handle() HandlerFunc {
	return func(request *Request) (*Response, error) {
		// Parse and clean the URL path
		parsedURL, err := url.Parse(request.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid URL path: %w", err)
		}

		// Clean the path to prevent directory traversal
		cleanPath := path.Clean(parsedURL.Path)

		// Remove leading slash for joining with base directory
		cleanPath = strings.TrimPrefix(cleanPath, "/")

		// Get absolute base directory
		absBase, err := filepath.Abs(h.FileDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute base path: %w", err)
		}

		// Construct full file path
		fullPath := filepath.Join(absBase, cleanPath)

		// Ensure the requested file is within the base directory
		if !strings.HasPrefix(fullPath, absBase) {
			h.Logger.Warn("attempted directory traversal", "path", request.Path)
			return HTTP404NotFound(), nil
		}

		// Get file info
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				// If it's a directory or root, try to serve index.html
				if strings.HasSuffix(fullPath, "/") || cleanPath == "" {
					indexPath := filepath.Join(fullPath, "index.html")
					if _, err := os.Stat(indexPath); err == nil {
						fullPath = indexPath
						fileInfo, _ = os.Stat(fullPath)
					} else {
						return HTTP404NotFound(), nil
					}
				} else {
					return HTTP404NotFound(), nil
				}
			} else {
				return nil, fmt.Errorf("failed to stat file: %w", err)
			}
		}

		// Don't serve directories (unless index.html was found above)
		if fileInfo.IsDir() {
			// Try to serve index.html from the directory
			indexPath := filepath.Join(fullPath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				fullPath = indexPath
				fileInfo, _ = os.Stat(fullPath)
			} else {
				return HTTP404NotFound(), nil
			}
		}

		// Get file size
		fileSize := fileInfo.Size()

		// Create response
		response := &Response{
			StatusCode: http.StatusOK,
			StatusText: http.StatusText(http.StatusOK),
			Protocol:   request.Protocol,
			Headers:    make(map[string]string),
		}

		// Set content type based on file extension
		contentType := h.detectContentType(fullPath)
		response.Headers["Content-Type"] = contentType

		// Set content length
		response.Headers["Content-Length"] = fmt.Sprintf("%d", fileSize)

		// Set Last-Modified header
		response.Headers["Last-Modified"] = fileInfo.ModTime().UTC().Format(http.TimeFormat)

		// Set cache headers for static assets
		if h.shouldCache(fullPath) {
			response.Headers["Cache-Control"] = "public, max-age=3600"
		}

		// Add Accept-Ranges header for range request support
		response.Headers["Accept-Ranges"] = "bytes"

		// Determine if we should stream the file
		const streamThreshold = 1024 * 1024 // 1MB threshold

		if fileSize > streamThreshold {
			// Stream large files
			file, err := os.Open(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file: %w", err)
			}

			response.Reader = file // File will be closed by writeResponse
			h.Logger.Debug("streaming large file",
				"path", request.Path,
				"file", fullPath,
				"size", fileSize,
				"content-type", contentType,
			)
		} else {
			// Load small files into memory
			file, err := os.Open(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			data, err := io.ReadAll(file)
			if err != nil {
				return nil, fmt.Errorf("failed to read file: %w", err)
			}

			response.Body = data
			h.Logger.Debug("served small file",
				"path", request.Path,
				"file", fullPath,
				"size", len(data),
				"content-type", contentType,
			)
		}

		return response, nil
	}
}

// detectContentType determines the MIME type of a file based on its extension
func (h *FileHandler) detectContentType(filename string) string {
	// Get file extension
	ext := strings.ToLower(filepath.Ext(filename))

	// Common MIME types
	mimeTypes := map[string]string{
		".html": "text/html; charset=utf-8",
		".htm":  "text/html; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".js":   "application/javascript; charset=utf-8",
		".json": "application/json; charset=utf-8",
		".xml":  "application/xml; charset=utf-8",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",

		// Images
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".webp": "image/webp",

		// Fonts
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".otf":   "font/otf",

		// Documents
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",

		// Archives
		".zip": "application/zip",
		".tar": "application/x-tar",
		".gz":  "application/gzip",

		// Media
		".mp3":  "audio/mpeg",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".ogg":  "audio/ogg",
		".wav":  "audio/wav",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}

	// Try standard library mime type detection
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}

	// Default to plain text for unknown extensions
	return "text/plain; charset=utf-8"
}

// shouldCache determines if a file should be cached based on its extension
func (h *FileHandler) shouldCache(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	cacheableExts := map[string]bool{
		".css":   true,
		".js":    true,
		".png":   true,
		".jpg":   true,
		".jpeg":  true,
		".gif":   true,
		".svg":   true,
		".ico":   true,
		".woff":  true,
		".woff2": true,
		".ttf":   true,
		".otf":   true,
	}
	return cacheableExts[ext]
}
