package web

// Naming every request for the route that served it.

import (
	"log/slog"

	"github.com/mbauer83/effect-golang/effect"
)

// RouteName is the name a route's requests are observed under: its method and
// its pattern -- "GET /books/{title}" -- never the path that was asked for,
// because a series per path is a series per request.
func RouteName(declaration Declaration) string {
	return declaration.Method + " " + declaration.Path
}

// RouteSpans is middleware that makes every request a span named for its
// route, annotated with the method and the pattern.
//
// What a tracer exporting to somebody's collector needs from a surface, and
// all it needs: applied with WithMiddleware, it covers every route including
// the one added this morning, and a surface that does not apply it opens no
// span per request at all.
func RouteSpans[R, E any]() RouteMiddleware[R, E] {
	return func(declaration Declaration, handler Handler[R, E]) Handler[R, E] {
		name := RouteName(declaration)
		method := slog.String("method", declaration.Method)
		route := slog.String("route", declaration.Path)
		return func(request Request) effect.Effect[R, E, Response] {
			// Annotated outside the span, where the annotation reaches the
			// span's own start and end rather than only the work inside it.
			return handler(request).WithName(name).WithSpan(name).Annotate(method, route)
		}
	}
}
