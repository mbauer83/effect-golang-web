package grpc

// The boundary: where a procedure's handler is interpreted, and where its typed
// failure becomes a code.
//
// It is parallel to web.Adapter and not built on it, which is a duplication
// worth its cost: an RPC answers with a code and a message where a resource
// answers with a status and an entity, and an application's refusals map onto
// one or the other directly. Routing them through the HTTP vocabulary on the
// way would mean translating twice and losing in both directions -- "no such
// customer" is NotFound to an RPC and 404 to a resource, and neither is the
// other's translation.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/mbauer83/effect-golang-schema/schema/protobuf"
	"github.com/mbauer83/effect-golang/effect"
)

// Boundary interprets procedure handlers.
//
// It owns the three things a handler cannot supply for itself: the runtime that
// interprets it, the environment it runs in, and how its typed failure becomes
// a code.
type Boundary[R, E any] struct {
	runtime     *effect.Runtime
	environment R
	transport   Serving
	onFailure   func(E) Failure
	onDefect    func(effect.Cause[E]) Failure
	report      func(context.Context, error)
}

// NewBoundary builds one. A missing runtime, transport or failure mapping is a
// declaration mistake and is reported here rather than on the first call.
func NewBoundary[R, E any](
	runtime *effect.Runtime,
	environment R,
	transport Serving,
	onFailure func(E) Failure,
) (*Boundary[R, E], error) {
	switch {
	case runtime == nil:
		return nil, faulted("building a boundary", "", errNoRuntime)
	case transport == nil:
		return nil, faulted("building a boundary", "", errNoTransport)
	case onFailure == nil:
		return nil, faulted("building a boundary", "", errNoFailureMapping)
	}
	return &Boundary[R, E]{
		runtime:     runtime,
		environment: environment,
		transport:   transport,
		onFailure:   onFailure,
		onDefect:    defectFailure[E],
		report:      reportToStandardError,
	}, nil
}

// WithDefectFailure replaces what a defect or an interruption answers with.
//
// The default is Unknown for a defect and Cancelled for an interruption, with
// no detail: a defect is by definition something the application did not
// account for, so its text is not fit to send to a caller.
func (boundary *Boundary[R, E]) WithDefectFailure(
	answer func(effect.Cause[E]) Failure,
) *Boundary[R, E] {
	boundary.onDefect = answer
	return boundary
}

// WithReport replaces where a fault the answer cannot express is recorded: a
// defect, principally.
//
// MEASURED, as at the HTTP boundary: the runtime emits its fiber events for
// forked fibers, so a defect in the effect a boundary interprets directly --
// which is what every call is -- reaches no observer on its own.
func (boundary *Boundary[R, E]) WithReport(
	report func(context.Context, error),
) *Boundary[R, E] {
	boundary.report = report
	return boundary
}

// Handler is what the HTTP core mounts, once every procedure has been answered.
func (boundary *Boundary[R, E]) Handler() (http.Handler, error) {
	return boundary.transport.Handler()
}

// Answer serves a procedure with a handler.
//
// The schemas do the typing: the request's bytes are decoded through the
// request schema, the handler works in Go values, and the response is encoded
// through the response schema. A request the schema refuses is InvalidArgument
// and the handler is never called, which is the point of the description being
// the contract.
func Answer[R, E, In, Out any](
	boundary *Boundary[R, E],
	procedure Procedure[In, Out],
	handle func(In) effect.Effect[R, E, Out],
) error {
	if err := procedure.Fault(); err != nil {
		return err
	}
	if handle == nil {
		return faulted("answering a procedure", procedure.Path(),
			errors.New("a procedure needs a handler"))
	}
	return boundary.transport.Answer(procedure.Path(),
		func(ctx context.Context, request []byte) ([]byte, *Failure) {
			return answering(ctx, boundary, procedure, handle, request)
		})
}

func answering[R, E, In, Out any](
	ctx context.Context,
	boundary *Boundary[R, E],
	procedure Procedure[In, Out],
	handle func(In) effect.Effect[R, E, Out],
	request []byte,
) ([]byte, *Failure) {
	asked, err := protobuf.Decode(procedure.request, request)
	if err != nil {
		// The description refused it, so the handler is never called: a
		// request that does not satisfy the contract is InvalidArgument
		// whatever the state of the service.
		return nil, &Failure{
			Code:    InvalidArgument,
			Message: fmt.Sprintf("the request does not satisfy %s: %v", procedure.Path(), err),
		}
	}

	// The call's context carries the cancellation, so a caller that goes away
	// interrupts the handler without any mechanism of its own.
	exit := boundary.runtime.Run(ctx, boundary.environment, handle(asked))
	answer, succeeded := exit.Value()
	if !succeeded {
		cause, _ := exit.Cause()
		return nil, boundary.failureForCause(ctx, cause)
	}

	written, err := protobuf.Encode(procedure.response, answer)
	if err != nil {
		// The handler produced something its own description refuses, which is
		// a fault of the service rather than of the caller.
		boundary.report(ctx, faulted("encoding a response", procedure.Path(), err))
		return nil, &Failure{Code: Internal, Message: "the response could not be encoded"}
	}
	return written, nil
}

// failureForCause decides what a failed outcome answers with. A defect
// outranks an interruption, which outranks nothing: the precedence is the
// runtime's own, so a boundary does not invent a second one.
func (boundary *Boundary[R, E]) failureForCause(
	ctx context.Context,
	cause effect.Cause[E],
) *Failure {
	if cause.ContainsDefect() {
		boundary.report(ctx, errors.New("grpc: a handler failed: "+cause.String()))
		answer := boundary.onDefect(cause)
		return &answer
	}
	if failure, typed := cause.Failure(); typed {
		answer := boundary.onFailure(failure)
		return &answer
	}
	answer := boundary.onDefect(cause)
	return &answer
}

// defectFailure is what a defect or an interruption answers with by default.
func defectFailure[E any](cause effect.Cause[E]) Failure {
	if cause.IsInterruptedOnly() {
		return Failure{Code: Cancelled, Message: "the call was cancelled"}
	}
	return Failure{Code: Unknown, Message: "the service failed"}
}

func reportToStandardError(_ context.Context, err error) {
	fmt.Fprintln(os.Stderr, err)
}
