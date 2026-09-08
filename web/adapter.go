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
		return Adapter[R, E]{}, faulted("building an adapter", errNoRuntime)
	}
	if onFailure == nil {
		return Adapter[R, E]{}, faulted("building an adapter", errNoFailureMapping)
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
	if !failed || cause.IsInterruptedOnly() {
		return
	}
	adapter.report(ctx, errors.New("web: the exchange ended badly: "+cause.String()))
}

// Handler interprets one handler as an http.Handler.
func (adapter Adapter[R, E]) Handler(handler Handler[R, E]) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// The request's context carries the cancellation, so a client that goes
		// away interrupts the handler without any mechanism of its own.
		exit := adapter.runtime.Run(
			request.Context(),
			adapter.environment,
			handler(RequestFrom(request)),
		)
		response := exit.Fold(adapter.responseForCause, sameResponse)

		if err := response.WriteTo(writer, request); err != nil {
			// The status has already gone out, so this can only be recorded.
			adapter.report(request.Context(), faulted("writing the response", err))
		}
	})
}

// responseForCause decides what a failed outcome answers with. A defect
// outranks an interruption, which outranks nothing: the precedence is the
// runtime's own, so a boundary does not invent a second one.
func (adapter Adapter[R, E]) responseForCause(cause effect.Cause[E]) Response {
	if cause.ContainsDefect() {
		adapter.report(context.Background(), causeError[E](cause))
		return adapter.onDefect(cause)
	}
	if failures := cause.Failures(); len(failures) > 0 {
		return adapter.onFailure(failures[0])
	}
	return adapter.onDefect(cause)
}

// defectResponse is the default answer to something the application did not
// account for.
func defectResponse[E any](cause effect.Cause[E]) Response {
	if cause.IsInterruptedOnly() {
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
