package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang/effect"
)

// Route is an endpoint with a handler: everything needed to dispatch a request,
// and everything needed to describe one.
type Route[R, E any] struct {
	declaration Declaration
	segments    []segment
	// build takes the rejection format and the phase names from the assembly
	// rather than fixing them here, because both are properties of the whole
	// surface and not of one route -- and because a build that closed over
	// them would close over the values they had when the route was declared,
	// which is before a surface has said what it wants.
	build func(reject func(error) Response, named phases) Handler[R, E]
	// phases names the parts of the route's own work, or is empty. Empty
	// names mean no spans, which is the default: a surface nobody is watching
	// should not pay for three spans per request instead of none.
	phases phases
	fault  error
}

// phases are the names a route's own parts are spanned under.
//
// Decoding and encoding are the route's work as much as the handler is, and a
// trace that showed one bar for all three could not say which of them a slow
// request spent its time in. A large document to unmarshal is real time, and
// so is a large one to write back.
type phases struct {
	decoding string
	handling string
	encoding string
}

// The names a detailing surface spans its phases under.
//
// Exported because anything keyed by operation needs them: a metric
// vocabulary that does not declare them measures three spans per request as
// "an operation nobody declared", which is how the largest thing in an
// aggregate came to be a bucket with no name on it.
const (
	PhaseDecoding = "decoding"
	PhaseHandling = "handling"
	PhaseEncoding = "encoding"
)

// PhaseNames are the three, for a caller assembling a vocabulary.
//
//	metrics.Naming(append(inspect.Names(surface.Declarations()), web.PhaseNames()...)...)
func PhaseNames() []string {
	return []string{PhaseDecoding, PhaseHandling, PhaseEncoding}
}

// detailed is the naming a surface uses when it details its phases.
func detailed() phases {
	return phases{
		decoding: PhaseDecoding,
		handling: PhaseHandling,
		encoding: PhaseEncoding,
	}
}

// spanned names an effect when a name is given and returns it untouched when
// none is.
//
// One shape for both, so the composition below is written once: a surface that
// is not detailing pays for no spans, and neither path is a second copy of the
// other that could drift from it.
func spanned[R, E, A any](fx effect.Effect[R, E, A], name string) effect.Effect[R, E, A] {
	if name == "" {
		return fx
	}
	return fx.WithSpan(name)
}

// Declaration is what the route says about itself, which is what a published
// document is projected from.
func (route Route[R, E]) Declaration() Declaration {
	return route.declaration
}

// wrapping returns the route with each built handler passed through the
// wrapper, together with the declaration it belongs to.
//
// The wrapper sees the route's whole work: the codecs as well as the handler,
// because it wraps what dispatch calls. That is the useful boundary -- a route
// whose response is expensive to encode is expensive to serve, whatever the
// handler cost.
func (route Route[R, E]) wrapping(each Matched[R, E]) Route[R, E] {
	if route.fault != nil || route.build == nil {
		return route
	}
	inner := route.build
	declaration := route.declaration
	route.build = func(reject func(error) Response, named phases) Handler[R, E] {
		return each(declaration, inner(reject, named))
	}
	return route
}

// Handle gives an endpoint its handler.
//
// The handler takes the decoded input and returns the output value, not a
// response: the endpoint already says how that value is encoded and with what
// status, so a handler that built its own response would be repeating a
// declaration it cannot see.
//
// A request the endpoint's codec refuses never reaches the handler and never
// becomes the application's failure. That is the distinction the failure
// channel is for: a malformed request is the transport's business, and the
// handler's E stays about the application.
func Handle[R, E, In, Out any](
	endpoint Endpoint[In, Out],
	handle func(In) effect.Effect[R, E, Out],
) Route[R, E] {
	route := Route[R, E]{
		declaration: endpoint.Declaration(),
		segments:    endpoint.segments,
		fault:       firstRouteFault(endpoint, handle),
	}
	if route.fault != nil {
		return route
	}
	route.build = func(reject func(error) Response, named phases) Handler[R, E] {
		operations := effect.For[R, E]()
		encode := encoding[R, E](endpoint.output)

		// Decode, handle, encode -- as three steps, so each can be a span of
		// its own.
		phased := func(request Request) effect.Effect[R, E, Response] {
			reading := operations.Suspend(func() effect.Effect[R, E, read[In]] {
				input, err := Decode(endpoint.input, request)
				return operations.Succeed(read[In]{value: input, refusal: err})
			})
			return spanned(reading, named.decoding).
				FlatMap(func(decoded read[In]) effect.Effect[R, E, Response] {
					if decoded.refusal != nil {
						return operations.Succeed(reject(decoded.refusal))
					}
					return spanned(handle(decoded.value), named.handling).
						FlatMap(func(value Out) effect.Effect[R, E, Response] {
							return spanned(encode(value), named.encoding)
						})
				})
		}

		// The same three steps with nothing between them to name, which is
		// one interpretation rather than three and does not carry the
		// decoding's outcome through a value.
		//
		// Two bodies, and the measurement is why: expressing both as the
		// phased one cost 0.26µs and six allocations per request on a surface
		// that had asked for no spans at all. A surface pays for what it asked
		// for, and the choice is made once here rather than per request.
		plain := func(request Request) effect.Effect[R, E, Response] {
			// Suspend, so nothing is read until the runtime interprets the
			// handler. A codec that ran when the route was built would make a
			// route a side effect rather than a description.
			return operations.Suspend(func() effect.Effect[R, E, Response] {
				input, err := Decode(endpoint.input, request)
				if err != nil {
					return operations.Succeed(reject(err))
				}
				return handle(input).FlatMap(encode)
			})
		}

		if named == (phases{}) {
			return plain
		}
		return phased
	}
	return route
}

// read is what decoding produced: the value, or why the request was refused.
//
// Carried rather than short-circuited, because decoding is a phase of its own
// now and a phase reports what it did -- a refusal is an outcome of the
// decoding and not a reason to abandon the composition around it.
type read[In any] struct {
	value   In
	refusal error
}

// detailing returns the route with its own parts named, so a trace shows
// decoding and encoding beside the handler.
func (route Route[R, E]) detailing() Route[R, E] {
	if route.fault != nil {
		return route
	}
	route.phases = detailed()
	return route
}

// encoding turns the handler's output value into the response the endpoint
// declared.
//
// An encoding failure is a defect rather than a typed failure: the value came
// from this program, so a schema that cannot describe it is a mistake here and
// not something a client can be told about or act on.
func encoding[R, E, Out any](output Output[Out]) func(Out) effect.Effect[R, E, Response] {
	return func(value Out) effect.Effect[R, E, Response] {
		response, err := output.encode(value)
		if err != nil {
			return effect.From(func(context.Context, R) effect.Exit[E, Response] {
				return effect.ExitCause[E, Response](
					effect.DieCause[E](effect.Defect{Value: err}),
				)
			})
		}
		return effect.For[R, E]().Succeed(response)
	}
}

func firstRouteFault[R, E, In, Out any](
	endpoint Endpoint[In, Out],
	handle func(In) effect.Effect[R, E, Out],
) error {
	if fault := ValidateEndpoint(endpoint); fault != nil {
		return fault
	}
	if handle == nil {
		return faulted("handling "+endpoint.method+" "+renderPattern(endpoint.segments), errNoHandler)
	}
	return nil
}

// rejected is the default answer to a request a codec refused. It says which
// part was wrong, because a client that is not told cannot fix its request.
func rejected(err error) Response {
	return Text(http.StatusBadRequest, err.Error())
}

var errNoHandler = errors.New("a route has a handler")
