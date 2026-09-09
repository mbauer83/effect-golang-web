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
	// phases is how the parts of the route's own work are named and
	// measured, or is the zero value. Its shape and its default are in
	// phase.go.
	phases phases
	fault  error
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
			return within(reading, named.decoding, named.sample).
				FlatMap(func(decoded read[In]) effect.Effect[R, E, Response] {
					if decoded.refusal != nil {
						return operations.Succeed(reject(decoded.refusal))
					}
					return within(handle(decoded.value), named.handling, named.sample).
						FlatMap(func(value Out) effect.Effect[R, E, Response] {
							return within(encode(value), named.encoding, named.sample)
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

		if named.quiet() {
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
// decoding and encoding beside the handler, and measured by the sampler if one
// was given.
func (route Route[R, E]) detailing(sample Sampling) Route[R, E] {
	if route.fault != nil {
		return route
	}
	route.phases = detailed()
	route.phases.sample = sample
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
