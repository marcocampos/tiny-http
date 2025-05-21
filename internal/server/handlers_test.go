package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileHandlerDetectContentType(t *testing.T) {
	handler := &FileHandler{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		filename    string
		data        []byte
		expected    string
		description string
	}{
		// Text files
		{"test.html", []byte("<html>"), "text/html; charset=utf-8", "HTML file"},
		{"test.htm", []byte("<html>"), "text/html; charset=utf-8", "HTM file"},
		{"test.css", []byte("body {}"), "text/css; charset=utf-8", "CSS file"},
		{"test.js", []byte("console.log()"), "application/javascript; charset=utf-8", "JavaScript file"},
		{"test.json", []byte("{}"), "application/json; charset=utf-8", "JSON file"},
		{"test.xml", []byte("<xml>"), "application/xml; charset=utf-8", "XML file"},
		{"test.txt", []byte("text"), "text/plain; charset=utf-8", "Text file"},
		{"test.md", []byte("# Header"), "text/markdown; charset=utf-8", "Markdown file"},

		// Images
		{"test.jpg", []byte{}, "image/jpeg", "JPEG file"},
		{"test.jpeg", []byte{}, "image/jpeg", "JPEG file (jpeg extension)"},
		{"test.png", []byte{}, "image/png", "PNG file"},
		{"test.gif", []byte{}, "image/gif", "GIF file"},
		{"test.svg", []byte{}, "image/svg+xml", "SVG file"},
		{"test.ico", []byte{}, "image/x-icon", "ICO file"},
		{"test.webp", []byte{}, "image/webp", "WebP file"},

		// Fonts
		{"test.woff", []byte{}, "font/woff", "WOFF font"},
		{"test.woff2", []byte{}, "font/woff2", "WOFF2 font"},
		{"test.ttf", []byte{}, "font/ttf", "TTF font"},
		{"test.otf", []byte{}, "font/otf", "OTF font"},

		// Documents
		{"test.pdf", []byte{}, "application/pdf", "PDF file"},
		{"test.doc", []byte{}, "application/msword", "DOC file"},
		{"test.docx", []byte{}, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "DOCX file"},

		// Archives
		{"test.zip", []byte{}, "application/zip", "ZIP file"},
		{"test.tar", []byte{}, "application/x-tar", "TAR file"},
		{"test.gz", []byte{}, "application/gzip", "GZIP file"},

		// Media
		{"test.mp3", []byte{}, "audio/mpeg", "MP3 file"},
		{"test.mp4", []byte{}, "video/mp4", "MP4 file"},
		{"test.webm", []byte{}, "video/webm", "WebM file"},
		{"test.ogg", []byte{}, "audio/ogg", "OGG file"},
		{"test.wav", []byte{}, "audio/wav", "WAV file"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := handler.detectContentType(tt.filename)
			if result != tt.expected {
				t.Errorf("detectContentType(%s) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestFileHandlerShouldCache(t *testing.T) {
	handler := &FileHandler{
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		filename string
		expected bool
	}{
		// Cacheable files
		{"style.css", true},
		{"script.js", true},
		{"image.jpg", true},
		{"image.jpeg", true},
		{"image.png", true},
		{"image.gif", true},
		{"logo.svg", true},
		{"favicon.ico", true},
		{"font.woff", true},
		{"font.woff2", true},
		{"font.ttf", true},
		{"font.otf", true},

		// Non-cacheable files
		{"index.html", false},
		{"data.json", false},
		{"document.pdf", false},
		{"text.txt", false},
		{"README.md", false},
		{"archive.zip", false},

		// Mixed case (should still work)
		{"STYLE.CSS", true},
		{"Script.JS", true},
		{"IMAGE.JPG", true},

		// No extension
		{"noextension", false},

		// Unknown extension
		{"file.xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := handler.shouldCache(tt.filename)
			if result != tt.expected {
				t.Errorf("shouldCache(%s) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestFileHandlerEdgeCases(t *testing.T) {
	// Create a temporary directory with test files
	tempDir := t.TempDir()

	// Create test files and directories
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a directory
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create a file with special characters in name
	specialFile := filepath.Join(tempDir, "file with spaces.txt")
	if err := os.WriteFile(specialFile, []byte("special content"), 0644); err != nil {
		t.Fatalf("Failed to create special file: %v", err)
	}

	// Create a symlink (if supported by the OS)
	symlinkSupported := true
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	if err := os.Symlink(testFile, symlinkPath); err != nil {
		symlinkSupported = false
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
		description    string
	}{
		{
			name:           "URL with query parameters",
			path:           "/test.txt?param=value",
			expectedStatus: 200,
			description:    "Should ignore query parameters",
		},
		{
			name:           "URL with fragment",
			path:           "/test.txt#section",
			expectedStatus: 200,
			description:    "Should ignore URL fragments",
		},
		{
			name:           "URL with encoded spaces",
			path:           "/file%20with%20spaces.txt",
			expectedStatus: 200,
			description:    "Should handle URL-encoded characters",
		},
		{
			name:           "Double slashes in path",
			path:           "//test.txt",
			expectedStatus: 404,
			description:    "Double slashes result in empty path component",
		},
		{
			name:           "Trailing slash on file",
			path:           "/test.txt/",
			expectedStatus: 200,
			description:    "path.Clean removes trailing slash",
		},
		{
			name:           "Directory without trailing slash",
			path:           "/subdir",
			expectedStatus: 404,
			description:    "Directory without index.html should 404",
		},
		{
			name:           "Empty path",
			path:           "",
			expectedStatus: 404,
			description:    "Empty path without index.html should 404",
		},
		{
			name:           "Dot file",
			path:           "/.hidden",
			expectedStatus: 404,
			description:    "Hidden files should 404 (doesn't exist)",
		},
		{
			name:           "Path with null byte",
			path:           "/test.txt\x00.jpg",
			expectedStatus: 500,
			description:    "Path with null byte should cause parsing error",
		},
	}

	// Add symlink test if supported
	if symlinkSupported {
		tests = append(tests, struct {
			name           string
			path           string
			expectedStatus int
			description    string
		}{
			name:           "Symlink file",
			path:           "/symlink.txt",
			expectedStatus: 200,
			description:    "Should follow symlinks",
		})
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
			if tt.expectedStatus == 500 {
				// For expected errors, we should get an error and nil response
				if err == nil {
					t.Errorf("%s: Expected error but got none", tt.description)
				}
				if resp != nil {
					t.Errorf("%s: Expected nil response for error case", tt.description)
				}
			} else {
				// For non-error cases
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if resp == nil {
					t.Fatalf("%s: Got nil response", tt.description)
				}
				if resp.StatusCode != tt.expectedStatus {
					t.Errorf("%s: StatusCode = %v, want %v", tt.description, resp.StatusCode, tt.expectedStatus)
				}
			}
		})
	}
}

func TestFileHandlerDirectoryTraversal(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tempDir, "safe.txt")
	if err := os.WriteFile(testFile, []byte("safe content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	// Various directory traversal attempts
	traversalPaths := []string{
		"/../etc/passwd",
		"/../../etc/passwd",
		"/../../../../../../../etc/passwd",
		"/./../../etc/passwd",
		"/subdir/../../../etc/passwd",
		"/%2e%2e/etc/passwd",
		"/%2e%2e%2f%2e%2e%2fetc/passwd",
		"/..\\etc\\passwd",                                // Windows-style
		"/../" + strings.Repeat("../", 50) + "etc/passwd", // Deep traversal
	}

	for _, path := range traversalPaths {
		t.Run("traversal attempt: "+path, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp, err := handler.Handle()(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// All traversal attempts should result in 404
			if resp.StatusCode != 404 {
				t.Errorf("Directory traversal not blocked for path %s: got status %d, want 404", path, resp.StatusCode)
			}
		})
	}
}

func TestFileHandlerMethods(t *testing.T) {
	// Create a temporary directory with a test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("test content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	tests := []struct {
		method         string
		expectedStatus int
		expectBody     bool
	}{
		{"GET", 200, true},
		{"HEAD", 200, false}, // HEAD should be handled by server, not handler
	}

	for _, tt := range tests {
		t.Run(tt.method+" request", func(t *testing.T) {
			req := &Request{
				Method:   tt.method,
				Path:     "/test.txt",
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp, err := handler.Handle()(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.expectedStatus)
			}

			if tt.expectBody && len(resp.Body) == 0 {
				t.Error("Expected response body, but got empty")
			}

			if tt.expectBody && string(resp.Body) != string(testContent) {
				t.Errorf("Body = %v, want %v", string(resp.Body), string(testContent))
			}
		})
	}
}

func TestFileHandlerHeaders(t *testing.T) {
	// Create a temporary directory with test files
	tempDir := t.TempDir()

	// Create different types of files
	htmlFile := filepath.Join(tempDir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<html><body>Hello</body></html>"), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	cssFile := filepath.Join(tempDir, "style.css")
	if err := os.WriteFile(cssFile, []byte("body { color: red; }"), 0644); err != nil {
		t.Fatalf("Failed to create CSS file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	tests := []struct {
		path          string
		expectedCache string
	}{
		{"/index.html", ""},                    // No cache header for HTML
		{"/style.css", "public, max-age=3600"}, // Cache header for CSS
	}

	for _, tt := range tests {
		t.Run("headers for "+tt.path, func(t *testing.T) {
			req := &Request{
				Method:   "GET",
				Path:     tt.path,
				Protocol: "HTTP/1.1",
				Headers:  make(map[string]string),
			}

			resp, err := handler.Handle()(req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Check Content-Length header
			if resp.Headers["Content-Length"] == "" {
				t.Error("Missing Content-Length header")
			}

			// Check Last-Modified header
			if resp.Headers["Last-Modified"] == "" {
				t.Error("Missing Last-Modified header")
			}

			// Check Cache-Control header
			if tt.expectedCache != "" {
				if resp.Headers["Cache-Control"] != tt.expectedCache {
					t.Errorf("Cache-Control = %v, want %v", resp.Headers["Cache-Control"], tt.expectedCache)
				}
			}
		})
	}
}

func TestFileHandlerLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create a temporary directory with a large file
	tempDir := t.TempDir()
	largeFile := filepath.Join(tempDir, "large.txt")

	// Create a 10MB file
	largeContent := make([]byte, 10*1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	req := &Request{
		Method:   "GET",
		Path:     "/large.txt",
		Protocol: "HTTP/1.1",
		Headers:  make(map[string]string),
	}

	resp, err := handler.Handle()(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", resp.StatusCode)
	}
}

func TestFileHandlerPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a file with no read permissions
	noReadFile := filepath.Join(tempDir, "noread.txt")
	if err := os.WriteFile(noReadFile, []byte("secret"), 0000); err != nil {
		t.Fatalf("Failed to create no-read file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := &FileHandler{
		FileDirectory: tempDir,
		Logger:        logger,
	}

	req := &Request{
		Method:   "GET",
		Path:     "/noread.txt",
		Protocol: "HTTP/1.1",
		Headers:  make(map[string]string),
	}

	resp, err := handler.Handle()(req)
	if err == nil {
		t.Error("Expected error for file with no read permissions")
	}

	// The handler should return an error, not a response
	if resp != nil {
		t.Error("Expected nil response for permission error")
	}
}
