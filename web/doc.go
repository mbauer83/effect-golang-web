// Package web handles HTTP as values and effects over the effect-golang
// runtime.
//
// A handler is a description rather than a call:
//
//	type Handler[R, E any] func(Request) effect.Effect[R, E, Response]
//
// Nothing is handled until the runtime interprets it, cancellation reaches the
// handler through the request's context, and a handler's typed failure is
// mapped to a status by the boundary that owns it rather than by the handler
// itself. A handler that decided its own status could not be reused behind a
// different contract.
//
// net/http is the spine, not something to be replaced. Request and Response
// wrap its types, an existing http.Handler becomes a Handler, and a Handler
// becomes an http.Handler through an Adapter. That boundary stays usable in
// both directions for the same reason the runtime leaves native channels
// native: the ecosystem is worth more than a closed abstraction.
//
// A server's lifetime is a scope. Closing the scope stops the listener and
// awaits the requests already in flight, so graceful shutdown is not a separate
// mechanism to remember -- it is what closing a scope already means.
package web
