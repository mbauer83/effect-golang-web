package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/mbauer83/effect-golang/effect"
)

// Adapter interprets handlers as net/http handlers.
//
// It owns the three things a handler cannot supply for itself: the runtime that
// interprets it, the environment it runs in, and how its typed failure becomes
// a status. Keeping those here is what leaves a handler reusable behind a
// different contract.
type Adapter[R, E any] struct {
	runtime     *effect.Runtime
	environment R
	onFailure   func(E) Response
	onDefect    func(effect.Cause[E]) Response
	report      func(context.Context, error)
	crossOrigin CrossOrigin
	quiet       bool
}

// NewAdapter builds an adapter. A nil runtime or failure mapping is a
// declaration mistake and is reported here, at composition time, rather than on
// the first request.
func NewAdapter[R, E any](
	runtime *effect.Runtime,
	environment R,
	onFailure func(E) Response,
) (Adapter[R, E], error) {
	if runtime == nil {
		return Adapter[R, E]{}, faultOf("build an adapter", errNoRuntime)
	}
	if onFailure == nil {
		return Adapter[R, E]{}, faultOf("build an adapter", errNoFailureMapping)
	}
	return Adapter[R, E]{
		runtime:     runtime,
		environment: environment,
		onFailure:   onFailure,
		onDefect:    defectResponse[E],
		report:      reportToStandardError,
	}, nil
}

// WithDefectResponse replaces what a defect or an interruption answers with.
//
// The default is a bare 500 for a defect and a 503 for an interruption, with no
// detail: a defect is by definition something the application did not account
// for, so its text is not fit to send to a client.
func (adapter Adapter[R, E]) WithDefectResponse(respond func(effect.Cause[E]) Response) Adapter[R, E] {
	adapter.onDefect = respond
	return adapter
}

// WithReport replaces where a fault the response cannot express is recorded: a
// defect, or a body that failed part-way through writing.
//
// This is the only place either is recorded, which is why it has a default
// rather than being optional. MEASURED: the runtime emits its fiber events for
// forked fibers, so a defect in the effect a boundary interprets directly --
// which is what every request is -- reaches no observer on its own. A write
// failure could not reach one in any case, because it happens after the status
// has gone and can only be noted.
func (adapter Adapter[R, E]) WithReport(report func(context.Context, error)) Adapter[R, E] {
	adapter.report = report
	return adapter
}

// WithCrossOrigin declares which other origins a browser may let read these answers.
//
// At the boundary because this is the only place that sees every answer: a
// page has to be able to read a 401 to know to sign in again, and a 401 is
// produced after a handler has failed -- past where anything wrapping the
// handler can still add a header. It also answers the permission a browser
// asks for before it will send a token at all, which no route declares
// because no route is being asked for.
//
//	boundary = boundary.WithCrossOrigin(web.CrossOrigin{
//	    AllowedOrigins: []string{"https://films.example"},
//	    AllowedHeaders: []string{"Authorization", "Content-Type"},
//	    MaxAge:         10 * time.Minute,
//	})
//
// Declaring nothing shares nothing, which is what a surface only its own
// origin reads wants.
func (adapter Adapter[R, E]) WithCrossOrigin(crossOrigin CrossOrigin) Adapter[R, E] {
	adapter.crossOrigin = crossOrigin
	return adapter
}

// Interpret runs an effect the way this boundary runs a handler, and records
// what it could not answer with.
//
// It is what a transport that takes over the connection needs. A websocket or
// an event stream answers with no response at all -- the exchange continues
// after the upgrade -- so such a transport cannot go through Handler, and it
// still wants the runtime, the environment and the reporting that the boundary
// owns.
//
// Anything but a plain interruption is reported, because a conversation that
// ended in a failure or a defect is something the operator wants to know about
// and there is no client left to tell.
func (adapter Adapter[R, E]) Interpret(ctx context.Context, fx effect.Effect[R, E, effect.Unit]) {
	exit := adapter.runtime.Run(ctx, adapter.environment, fx)
	cause, failed := exit.Cause()
	if !failed || cause.HasInterruptsOnly() {
		return
	}
	adapter.report(ctx, errors.New("web: the exchange ended badly: "+cause.String()))
}

// Handler interprets one handler as an http.Handler.
func (adapter Adapter[R, E]) Handler(handler Handler[R, E]) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		response := adapter.answer(handler, request)

		if err := response.WriteTo(writer, request); err != nil {
			// The status has already gone out, so this can only be recorded.
			adapter.report(request.Context(), faultOf("write the response", err))
		}
	})
}

// answer is what this boundary replies to one request, shared with the page
// that asked when the surface shares with its origin.
//
// A browser asking permission is answered here and not routed: it is asking
// about a method the path may well not have, and routing it would answer 405
// -- which a browser reads as "no" and then never sends the request it was
// asking about.
func (adapter Adapter[R, E]) answer(handler Handler[R, E], request *http.Request) Response {
	origin, shared := adapter.crossOrigin.allowedOrigin(request)
	if shared && isPreflight(request) {
		return adapter.crossOrigin.preflightResponse(request, origin)
	}
	// The request's context carries the cancellation, so a client that goes
	// away interrupts the handler without any mechanism of its own.
	exit := adapter.runtime.Run(
		request.Context(),
		adapter.environment,
		handler(RequestFrom(request)),
	)
	response := exit.Fold(adapter.responseForCause, sameResponse)
	if !shared {
		return response
	}
	return adapter.crossOrigin.share(response, origin)
}

// responseForCause decides what a failed outcome answers with. A defect
// outranks an interruption, which outranks nothing: the precedence is the
// runtime's own, so a boundary does not invent a second one.
//
// Every one of them is recorded, which it was not before: a typed failure
// became a status and left nothing behind, so the only account of why a client
// got a 404 was the client's. Anybody asking why had to reproduce it. What a
// cause carries -- the line that raised it and the span it was raised inside
// -- is exactly what makes recording it worth doing rather than noise.
func (adapter Adapter[R, E]) responseForCause(cause effect.Cause[E]) Response {
	if cause.ContainsDefect() {
		adapter.report(context.Background(), causeError[E](cause))
		return adapter.onDefect(cause)
	}
	if failures := cause.Failures(); len(failures) > 0 {
		adapter.recordRefusal(cause)
		return adapter.onFailure(failures[0])
	}
	return adapter.onDefect(cause)
}

// recordRefusal notes a typed failure that became a status.
//
// Through the same sink a defect goes to, because the question they answer is
// the same one -- why did this request end that way -- and an operator reading
// one wants the other in the same place. A refusal is not a fault of this
// program, so the text says refused rather than unhandled: a log full of
// "error" for a client sending a bad body is a log nobody reads.
func (adapter Adapter[R, E]) recordRefusal(cause effect.Cause[E]) {
	if adapter.quiet {
		return
	}
	adapter.report(context.Background(),
		errors.New("web: refused: "+cause.String()))
}

// Quiet stops this boundary recording the refusals it answers with.
//
// For a surface where a refusal is the ordinary case and the volume would bury
// everything else -- a validating endpoint behind a form, a health check
// somebody polls. Off by default, because a refusal nobody recorded is a
// question nobody can answer afterwards, and that was the state this started
// in.
func (adapter Adapter[R, E]) Quiet() Adapter[R, E] {
	adapter.quiet = true
	return adapter
}

// defectResponse is the default answer to something the application did not
// account for.
func defectResponse[E any](cause effect.Cause[E]) Response {
	if cause.HasInterruptsOnly() {
		return Empty(http.StatusServiceUnavailable)
	}
	return Empty(http.StatusInternalServerError)
}

func causeError[E any](cause effect.Cause[E]) error {
	return errors.New("web: unhandled defect: " + cause.String())
}

// reportToStandardError is the last-resort sink, deliberately not a Logger: a
// boundary that could not send a response is not a good moment to depend on an
// adapter that might also be the thing that failed.
func reportToStandardError(_ context.Context, err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
}

func sameResponse(response Response) Response { return response }

var (
	errNoRuntime        = errors.New("a Runtime is required to interpret a handler")
	errNoFailureMapping = errors.New("a failure mapping is required; a handler does not choose its own status")
)
