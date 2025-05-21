package server

import (
	"bytes"
	"compress/gzip"
	"strconv"
	"strings"
)

type Middleware func(next HandlerFunc) HandlerFunc

func BaseMiddleware(next HandlerFunc) HandlerFunc {
	return func(request *Request) (*Response, error) {
		response, err := next(request)
		if err != nil {
			return nil, err
		}

		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		for key, value := range DefaulResponsetHeaders {
			if _, exists := response.Headers[key]; !exists {
				response.Headers[key] = value
			}
		}

		if request.Protocol != "" {
			response.Protocol = request.Protocol
		} else {
			response.Protocol = "HTTP/1.1"
		}

		return response, nil
	}
}

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

		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, err = gz.Write(response.Body)
		if err != nil {
			return nil, err
		}
		gz.Close()

		response.Body = buf.Bytes()

		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		response.Headers["Content-Encoding"] = "gzip"
		response.Headers["Content-Length"] = strconv.Itoa(buf.Len())
		return response, nil
	}
}
