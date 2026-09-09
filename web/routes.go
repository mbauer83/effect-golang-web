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
	// assembled and reject are what this was made from, kept so the surface
	// can be rebuilt with a wrapper around every route. A Routes that could
	// not be re-derived from its own parts would force a caller to keep the
	// parts itself, which is a worse place for them.
	assembled []Route[R, E]
	reject    func(error) Response
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

	assembled := Routes[R, E]{
		tree:      newTreeNode[R, E](),
		assembled: slices.Clone(routes),
		reject:    reject,
	}
	for _, route := range routes {
		if route.fault != nil {
			return Routes[R, E]{}, route.fault
		}
		pattern := renderPattern(route.segments)
		err := assembled.tree.insert(route.segments, route.declaration.Method,
			route.build(reject, route.phases), pattern)
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

// DeclarationsOf are what a set of unassembled routes say about themselves.
//
// For the routes a caller has but has not assembled yet -- another module's,
// mounted alongside its own. Anything keyed by route needs them before the
// surface exists: a metric vocabulary declared from the surface it will
// measure is a vocabulary that cannot fall behind it, and a route left out of
// it is a route whose traffic is lumped in with everything undeclared.
func DeclarationsOf[R, E any](routes ...Route[R, E]) []Declaration {
	described := make([]Declaration, 0, len(routes))
	for _, route := range routes {
		described = append(described, route.Declaration())
	}
	return described
}

// Matched derives a handler from a handler, and is told which route it is
// deriving it for.
//
// The shape every cross-cutting concern that has to name the route needs:
// observation, a per-route rate limit, an audit line. Ordinary Middleware
// cannot do it, because it wraps the surface's handler and by then the only
// thing left of the route is the path the client asked for -- and a path is an
// unbounded value, so naming anything after it is how a metric label or a span
// name becomes one series per request.
type Matched[R, E any] func(Declaration, Handler[R, E]) Handler[R, E]

// Wrapping applies one wrapper to every route, giving each the declaration it
// belongs to.
//
// A setting on the surface, applied in one place, rather than something a
// caller has to remember at every call site: a route that was not written
// through the right constructor would be a route that quietly is not observed,
// and nothing would say so.
//
//	surface, err := web.NewRoutes(routes...)
//	surface = surface.Wrapping(inspect.Observing(costs))
//
// It cannot fail. The patterns are the ones that already assembled, and a
// wrapper does not change them, so there is nothing left to refuse. A zero
// Routes wraps nothing and stays itself.
func (routes Routes[R, E]) Wrapping(each Matched[R, E]) Routes[R, E] {
	if each == nil || len(routes.assembled) == 0 {
		return routes
	}
	wrapped := make([]Route[R, E], 0, len(routes.assembled))
	for _, route := range routes.assembled {
		wrapped = append(wrapped, route.wrapping(each))
	}
	// The same routes with the same patterns, so this cannot refuse what it
	// already accepted; an error here would be a bug in the tree rather than
	// a caller's mistake, and reporting it as the caller's would be a lie.
	rebuilt, err := NewRoutesRejecting(routes.reject, wrapped...)
	if err != nil {
		return routes
	}
	return rebuilt
}

// Detailing makes every route name the parts of its own work -- decoding,
// handling, encoding -- so a trace shows them separately.
//
// A second setting rather than part of Wrapping, because they answer different
// questions and cost differently: a route span says which request was slow, and
// these say which part of it was.
//
// The cost is worth stating, because it is larger than it sounds and it was
// measured rather than guessed: a route answering from memory took 2.04µs and
// 39 allocations plain, and 5.51µs and 79 detailed. That cost falls inside the
// route and outside its phases, so a detailed trace of very fast work shows
// small bars separated by gaps -- and the gaps are the instrumentation.
//
// None of it is paid by a surface that leaves this off: the plain figures are
// what the route cost before the setting existed, to the allocation. Turn it
// on for work that takes milliseconds; leave it off for work that takes
// microseconds, which will otherwise tell you about WithSpan rather than about
// itself.
//
//	surface = surface.Detailing().Wrapping(inspect.Observing(costs))
//
// Decoding and encoding are the route's work as much as the handler is. A
// large document to unmarshal is real time, and a trace that showed one bar
// for all three could not say which of them a slow request spent it in.
func (routes Routes[R, E]) Detailing() Routes[R, E] {
	return routes.detailing(nil)
}

// Measuring is Detailing that also hands each phase to a sampler, so a caller
// who can measure the process -- which this module cannot; it reads no
// counters and depends on nothing that does -- accounts for the phases as well
// as naming them.
//
// It names them too, because a phase that was measured and not named is one
// nobody can find: the account is keyed by the phase's name, and the timeline
// is what somebody looking for it is reading.
//
// The seam costs 1.5µs and forty allocations per request on top of detailing --
// three suspensions and three finalizers -- before the sampler does anything at
// all. What the sampler itself costs is the sampler's business. The measurement
// is in test/unit/web_cost_test.go.
//
//	surface = surface.Measuring(inspect.Sampling(watched.Costs)).
//	    Wrapping(inspect.Observing[Env, Refusal](watched.Costs))
func (routes Routes[R, E]) Measuring(each Sampling) Routes[R, E] {
	return routes.detailing(each)
}

func (routes Routes[R, E]) detailing(sample Sampling) Routes[R, E] {
	if len(routes.assembled) == 0 {
		return routes
	}
	detailing := make([]Route[R, E], 0, len(routes.assembled))
	for _, route := range routes.assembled {
		detailing = append(detailing, route.detailing(sample))
	}
	rebuilt, err := NewRoutesRejecting(routes.reject, detailing...)
	if err != nil {
		return routes
	}
	return rebuilt
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
