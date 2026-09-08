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
	// build takes the rejection format from the assembly rather than fixing it
	// here, because a consistent answer to a malformed request is a property of
	// the whole surface and not of one route.
	build func(reject func(error) Response) Handler[R, E]
	fault error
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
	route.build = func(reject func(error) Response) Handler[R, E] {
		return each(declaration, inner(reject))
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
	route.build = func(reject func(error) Response) Handler[R, E] {
		operations := effect.For[R, E]()
		encode := encoding[R, E](endpoint.output)
		return func(request Request) effect.Effect[R, E, Response] {
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
	}
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
