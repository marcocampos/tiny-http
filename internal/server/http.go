package server

type Request struct {
	Method   string
	Path     string
	Protocol string
	Headers  map[string]string
	Body     []byte
}

type Response struct {
	StatusCode int
	StatusText string
	Protocol   string
	Headers    map[string]string
	Body       []byte
}
