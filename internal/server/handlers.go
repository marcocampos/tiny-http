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
		parsedURL, err := url.Parse(request.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid URL path: %w", err)
		}

		cleanPath := path.Clean(parsedURL.Path)
		cleanPath = strings.TrimPrefix(cleanPath, "/")

		absBase, err := filepath.Abs(h.FileDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute base path: %w", err)
		}

		fullPath := filepath.Join(absBase, cleanPath)

		if !strings.HasPrefix(fullPath, absBase) {
			h.Logger.Warn("attempted directory traversal", "path", request.Path)
			return HTTP404NotFound(), nil
		}

		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
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

		if fileInfo.IsDir() {
			indexPath := filepath.Join(fullPath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				fullPath = indexPath
				fileInfo, _ = os.Stat(fullPath)
			} else {
				return HTTP404NotFound(), nil
			}
		}

		fileSize := fileInfo.Size()

		response := &Response{
			StatusCode: http.StatusOK,
			StatusText: http.StatusText(http.StatusOK),
			Protocol:   request.Protocol,
			Headers:    make(map[string]string),
		}

		contentType := h.detectContentType(fullPath)
		response.Headers["Content-Type"] = contentType
		response.Headers["Content-Length"] = fmt.Sprintf("%d", fileSize)
		response.Headers["Last-Modified"] = fileInfo.ModTime().UTC().Format(http.TimeFormat)

		if h.shouldCache(fullPath) {
			response.Headers["Cache-Control"] = "public, max-age=3600"
		}
		response.Headers["Accept-Ranges"] = "bytes"

		const streamThreshold = 1024 * 1024

		if fileSize > streamThreshold {
			file, err := os.Open(fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file: %w", err)
			}

			response.Reader = file
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
	ext := strings.ToLower(filepath.Ext(filename))

	mimeTypes := map[string]string{
		".html": "text/html; charset=utf-8",
		".htm":  "text/html; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".js":   "application/javascript; charset=utf-8",
		".json": "application/json; charset=utf-8",
		".xml":  "application/xml; charset=utf-8",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",

		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".webp": "image/webp",

		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".otf":   "font/otf",

		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",

		".zip": "application/zip",
		".tar": "application/x-tar",
		".gz":  "application/gzip",

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

	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}

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
