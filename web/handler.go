package web

import (
	"net/http"

	"github.com/mbauer83/effect-golang/effect"
)

// Handler describes what to do with a request. It is a description: nothing is
// handled until the runtime interprets it.
//
// The failure channel is the handler's own. A handler does not choose a status
// for its failure, because the same handler is reusable behind a different
// contract only if it does not; the boundary that owns the route maps E to a
// status.
type Handler[R, E any] func(Request) effect.Effect[R, E, Response]

// Respond is the handler that always answers the same way, which is what a
// health check, a redirect or a fixed error page is.
func Respond[R, E any](response Response) Handler[R, E] {
	return func(Request) effect.Effect[R, E, Response] {
		return effect.For[R, E]().Succeed(response)
	}
}

// FromHTTP adapts an existing http.Handler.
//
// The handler runs when the response is written rather than when the effect is
// interpreted, so a streaming, flushing or hijacking handler keeps working
// exactly as it did and nothing is buffered on its behalf. That is the same
// rule the runtime follows with native channels: the ecosystem boundary stays
// usable, rather than being replaced by something that only resembles it.
func FromHTTP[R, E any](handler http.Handler) Handler[R, E] {
	return Respond[R, E](Delegate(handler))
}

// Transform derives a handler from another by rewriting its response, which is
// the shape of every wrapper that adds a header, a cache directive or a
// content encoding.
//
// It is a package function rather than a method because a method on a named
// function type cannot grow the type parameters a transform needs.
func Transform[R, E any](handler Handler[R, E], rewrite func(Response) Response) Handler[R, E] {
	return func(request Request) effect.Effect[R, E, Response] {
		return handler(request).Map(rewrite)
	}
}
