package grpc

// What went wrong, and the code an RPC says it with.

import "errors"

// Fault is a fault of this package: a procedure that could not be declared, a
// message that could not be encoded, a call that did not get through.
type Fault struct {
	// Doing names the stage.
	Doing string
	// Procedure is the one it was about, where the stage had one.
	Procedure string
	Err       error
}

func (fault Fault) Error() string {
	rendered := "grpc: " + fault.Doing
	if fault.Procedure != "" {
		rendered += " [" + fault.Procedure + "]"
	}
	if fault.Err != nil {
		rendered += ": " + fault.Err.Error()
	}
	return rendered
}

// Unwrap keeps errors.Is and errors.As working through the boundary.
func (fault Fault) Unwrap() error {
	return fault.Err
}

func faulted(doing string, procedure string, err error) Fault {
	return Fault{Doing: doing, Procedure: procedure, Err: err}
}

// Code is what an RPC answers with when it does not answer with a message.
//
// These are gRPC's own canonical codes, named rather than numbered, and the set
// is closed: a transport maps them onto its protocol and an application chooses
// among them. It is deliberately not the HTTP status vocabulary -- an RPC says
// NotFound where a resource says 404, and the two are different alphabets for
// overlapping ideas, so translating between them twice would lose in both
// directions.
type Code uint8

const (
	// Cancelled is the caller having gone away.
	Cancelled Code = iota + 1
	// Unknown is a failure with no better description, which is what a defect
	// becomes.
	Unknown
	// InvalidArgument is a request the service will never accept, whatever the
	// state of the system.
	InvalidArgument
	// DeadlineExceeded is the call having taken too long.
	DeadlineExceeded
	// NotFound is the thing asked for not being there.
	NotFound
	// AlreadyExists is the thing being created being there already.
	AlreadyExists
	// PermissionDenied is the caller being known and not allowed.
	PermissionDenied
	// ResourceExhausted is a quota or a rate limit.
	ResourceExhausted
	// FailedPrecondition is the system not being in a state where this can be
	// done, where retrying without a change would fail the same way.
	FailedPrecondition
	// Aborted is a conflict the caller should retry at a higher level, such as
	// a lost race for a lock.
	Aborted
	// OutOfRange is an argument past the end of what exists, which a caller
	// iterating can act on where InvalidArgument would stop it.
	OutOfRange
	// Unimplemented is a procedure this service does not offer.
	Unimplemented
	// Internal is an invariant of the service having been broken.
	Internal
	// Unavailable is the service being temporarily unable, which a caller may
	// retry with a backoff.
	Unavailable
	// DataLoss is unrecoverable loss or corruption.
	DataLoss
	// Unauthenticated is the caller not being known.
	Unauthenticated
)

// Failure is what a procedure answers with instead of a message: a code, and a
// message for a person reading a log.
type Failure struct {
	Code Code
	// Message is for an operator rather than for a program. A caller that has
	// to branch on what went wrong branches on the Code, which is why that is
	// a closed set and this is prose.
	Message string
}

var (
	errNoRuntime        = errors.New("a boundary needs a runtime to interpret handlers")
	errNoFailureMapping = errors.New("a boundary needs to know what a failure answers with")
	errNoService        = errors.New("a procedure belongs to a named service")
	errNoMethod         = errors.New("a procedure has a name")
	errNoTransport      = errors.New("a boundary needs a transport to serve procedures")
)
