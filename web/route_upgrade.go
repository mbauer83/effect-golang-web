package web

// A route whose handler takes over the connection.
//
// A websocket endpoint is a GET that answers 101 and then stops being HTTP.
// There is no request body to describe and no response body to encode, so it
// has no codecs -- but it has a method and a path, and it should be dispatched
// and documented by the same tree as everything else rather than mounted
// beside it.

import (
	"net/http"
)

// Upgrading declares a route whose handler takes over the connection.
//
// Its declaration says what is true of the HTTP part: a GET that answers 101
// and carries no entity. What happens after the upgrade is a different protocol
// and is not something an OpenAPI document can describe, which is why the
// declaration stops there rather than pretending.
func Upgrading[R, E any](path string, summary string, handler Handler[R, E]) Route[R, E] {
	segments, err := parsePattern(path)
	if err != nil {
		return Route[R, E]{fault: err}
	}
	if handler == nil {
		return Route[R, E]{fault: faulted("upgrading "+path, errNoHandler)}
	}
	return Route[R, E]{
		declaration: Declaration{
			Method:  http.MethodGet,
			Path:    renderPattern(segments),
			Summary: summary,
			Status:  http.StatusSwitchingProtocols,
		},
		segments: segments,
		// The rejection format is unused: there are no codecs to refuse
		// anything, because the exchange after the upgrade is not described
		// here.
		build: func(func(error) Response) Handler[R, E] { return handler },
	}
}
