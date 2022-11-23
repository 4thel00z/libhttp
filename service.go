package libhttp

import (
	"net/http"
)

// A Service is a function that takes a request and produces a response. Services are used symmetrically in
// both clients and servers.
type Service func(req Request) Response

// Filter vends a new service wrapped in the provided filter.
func (svc Service) Filter(f Filter) Service {
	return func(req Request) Response {
		return f(req, svc)
	}
}

// ServeHTTP is the only method that needs to be present on
// Service to implement the http.Handler
// This makes it more convenient to interface with serverless applications
func (svc Service) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	HttpHandler(svc).ServeHTTP(rw, r)
}

// FromHTTPHandler turns your legacy http handlers into a libhttp Service
func FromHTTPHandler(handler http.Handler) Service {
	return func(req Request) Response {
		response := req.Response(nil)
		handler.ServeHTTP(responseWriterWrapper{r: &response}, &req.Request)
		return response
	}
}
