package web

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/mbauer83/effect-golang/effect"
)

// Request is one inbound request.
//
// It wraps net/http's own request rather than copying it, so nothing is lost
// and an adapter can always reach the original. What it adds is the path
// captures a route matched, which http.Request has no place for.
type Request struct {
	underlying *http.Request
	captures   map[string]string
}

// RequestFrom adapts net/http's request. It is what an adapter calls at the
// boundary; a handler receives the result.
func RequestFrom(underlying *http.Request) Request {
	return Request{underlying: underlying}
}

// WithCaptures returns the request with the segments a route matched.
//
// A route builds this; a handler reads it. It returns a new value rather than
// storing into the http.Request's context, because a capture is part of the
// request as the route understands it and not a value smuggled past the type.
func (request Request) WithCaptures(captures map[string]string) Request {
	request.captures = captures
	return request
}

// Capture returns the value a route matched for a named path segment.
func (request Request) Capture(name string) (string, bool) {
	value, matched := request.captures[name]
	return value, matched
}

// Method is the request method.
func (request Request) Method() string {
	return request.underlying.Method
}

// URL is the requested URL.
func (request Request) URL() *url.URL {
	return request.underlying.URL
}

// Path is the request path, without the query.
func (request Request) Path() string {
	return request.underlying.URL.Path
}

// Query is the parsed query string.
func (request Request) Query() url.Values {
	return request.underlying.URL.Query()
}

// Header is the request's headers.
func (request Request) Header() http.Header {
	return request.underlying.Header
}

// Context is the request's context. Cancellation reaches a handler through it,
// which is why a handler never needs a cancellation mechanism of its own.
func (request Request) Context() context.Context {
	return request.underlying.Context()
}

// Underlying is net/http's own request, for the cases this type does not cover.
func (request Request) Underlying() *http.Request {
	return request.underlying
}

// Body reads the whole entity.
//
// It is an effect because reading a body is I/O that can fail and can be
// cancelled, and it is a package function because the environment it runs in is
// the caller's rather than the request's.
//
// The body is read once, as net/http's is: a second read yields nothing. A
// handler that needs the entity twice should keep what this returns.
func Body[R any](request Request) effect.Effect[R, Fault, []byte] {
	return effect.Try(
		func(context.Context, R) ([]byte, error) {
			if request.underlying.Body == nil {
				return nil, nil
			}
			return io.ReadAll(request.underlying.Body)
		},
		func(err error) Fault { return Fault{Doing: "reading the request body", Err: err} },
	).Named("read-body")
}
