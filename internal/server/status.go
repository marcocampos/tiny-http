package server

import "strconv"

func HttpBaseResponse(statusCode int, statusText string) *Response {
	body := []byte(statusText)
	headers := copyHeaders(DefaulResponsetHeaders)
	headers["Content-Length"] = strconv.Itoa(len(body))
	return &Response{
		StatusCode: statusCode,
		StatusText: statusText,
		Protocol:   "HTTP/1.1",
		Headers:    headers,
		Body:       body,
	}
}

func Http400BadRequest() *Response {
	return HttpBaseResponse(400, "Bad Request")
}

func Http404NotFound() *Response {
	return HttpBaseResponse(404, "Not Found")
}

func Http405MethodNotAllowed() *Response {
	return HttpBaseResponse(405, "Method Not Allowed")
}

func Http500InternalServerError() *Response {
	return HttpBaseResponse(500, "Internal Server Error")
}
