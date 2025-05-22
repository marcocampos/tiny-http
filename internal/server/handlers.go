package server

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

type HandlerFunc func(request *Request) (*Response, error)

type Handler interface {
	Handle() HandlerFunc
}

type RootHandler struct{}

func (h *RootHandler) Handle() HandlerFunc {
	return func(request *Request) (*Response, error) {
		body := []byte("Hello, World!")
		return &Response{
			StatusCode: 200,
			StatusText: "OK",
			Protocol:   "HTTP/1.1",
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       body,
		}, nil
	}
}

type FileHandler struct {
	FileDirectory string
}

func (h *FileHandler) Handle() HandlerFunc {
	return func(request *Request) (*Response, error) {
		parsedURL, err := url.Parse(request.Path)
		if err != nil {
			return nil, err
		}
		cleanPath := filepath.Clean("/" + parsedURL.Path)
		absBase, err := filepath.Abs(h.FileDirectory)
		if err != nil {
			return nil, err
		}

		absPath, err := filepath.Abs(filepath.Join(absBase, cleanPath))
		if err != nil {
			return nil, err
		}

		file, err := os.Open(absPath)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			return &Response{
				StatusCode: 404,
				StatusText: "Not Found",
				Protocol:   "HTTP/1.1",
				Body:       []byte("404 Not Found"),
			}, nil
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}

		response := &Response{
			StatusCode: 200,
			StatusText: "OK",
			Headers:    make(map[string]string),
			Body:       data,
		}

		response.Headers["Content-Type"] = "application/octet-stream"
		return response, nil
	}
}
