package web

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/mbauer83/effect-golang/effect"
)

// Routes is an assembled surface: the routes it serves, compiled for dispatch.
//
// Assembly is where two routes that could match the same request are refused,
// so a precedence surprise is a start-up error rather than a request that went
// to the wrong handler in production.
type Routes[R, E any] struct {
	declarations []Declaration
	tree         *treeNode[R, E]
}

// NewRoutes assembles routes, answering a malformed request with 400 and the
// reason the codec gave.
func NewRoutes[R, E any](routes ...Route[R, E]) (Routes[R, E], error) {
	return NewRoutesRejecting(rejected, routes...)
}

// NewRoutesRejecting assembles routes with its own answer to a request the
// codecs refused.
//
// The format is a property of the whole surface rather than of one route,
// because a client meets one API and not a collection of separately-worded
// ones.
func NewRoutesRejecting[R, E any](
	reject func(error) Response,
	routes ...Route[R, E],
) (Routes[R, E], error) {
	if reject == nil {
		return Routes[R, E]{}, faulted("assembling routes", errNoRejection)
	}
	if len(routes) == 0 {
		return Routes[R, E]{}, faulted("assembling routes", errNoRoutes)
	}

	assembled := Routes[R, E]{tree: newTreeNode[R, E]()}
	for _, route := range routes {
		if route.fault != nil {
			return Routes[R, E]{}, route.fault
		}
		pattern := renderPattern(route.segments)
		err := assembled.tree.insert(route.segments, route.declaration.Method, route.build(reject), pattern)
		if err != nil {
			return Routes[R, E]{}, faulted("assembling routes", err)
		}
		assembled.declarations = append(assembled.declarations, route.declaration)
	}
	return assembled, nil
}

// Declarations are what the routes say about themselves, in declared order.
// A published document is projected from these.
func (routes Routes[R, E]) Declarations() []Declaration {
	return routes.declarations
}

// Handler dispatches a request to the route that matches it.
//
// It is a Handler rather than an http.Handler, so middleware composes around it
// and the boundary interprets it like any other.
func (routes Routes[R, E]) Handler() Handler[R, E] {
	operations := effect.For[R, E]()
	return func(request Request) effect.Effect[R, E, Response] {
		found := routes.tree.resolve(pathSegments(request.Path()), request.Method(), nil)
		switch {
		case found.found:
			return found.handler(request.WithCaptures(captured(found.captures)))
		case len(found.allowed) > 0:
			// The path matched and the method did not, which is a different
			// thing from nothing being there -- and the client is told which
			// methods it could have used, as the specification requires.
			return operations.Succeed(methodNotAllowed(found.allowed))
		default:
			return operations.Succeed(Empty(http.StatusNotFound))
		}
	}
}

// methodNotAllowed answers 405 with the Allow header, sorted so the same
// mismatch always produces the same answer.
func methodNotAllowed(allowed []string) Response {
	return Empty(http.StatusMethodNotAllowed).
		WithHeader("Allow", strings.Join(slices.Sorted(slices.Values(allowed)), ", "))
}

var (
	errNoRoutes    = errors.New("a surface serves at least one route")
	errNoRejection = errors.New("a rejection format is required; use NewRoutes for the default")
)
