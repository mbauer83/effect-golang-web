package web

import (
	"errors"

	"github.com/mbauer83/effect-golang/effect/fault"
)

// Fault is what the web layer itself can fail with: a request whose parts could
// not be read, or a server that could not start.
//
// It is deliberately not an application failure. A handler's own failures have
// the handler's own type, and the boundary maps those to statuses; this type is
// for the transport's own troubles, which every application handles the same
// way.
type Fault struct {
	// Op names what was being attempted, which is what a caller acts on.
	Op  string
	Err error
}

func (fault Fault) Error() string {
	if fault.Err == nil {
		return "web: " + fault.Op
	}
	return "web: " + fault.Op + ": " + fault.Err.Error()
}

// Unwrap keeps errors.Is and errors.As working through the boundary, so a
// caller can still ask whether a listener failed because the port was taken.
func (fault Fault) Unwrap() error {
	return fault.Err
}

// Kind is what the status of a refusal says, or else what the cause states: a
// declaration that cannot be served and a body that would not decode are
// Unreadable, a request missing what it must carry Unacceptable, and a
// connection that failed Unavailable.
func (failure Fault) Kind() fault.Kind {
	var refusal Refusal
	if errors.As(failure.Err, &refusal) {
		return refusal.Kind()
	}
	return fault.KindOf(failure.Err)
}

// sentinel is one of this package's own reasons, stating its kind.
type sentinel struct {
	reason string
	kind   fault.Kind
}

func (reason *sentinel) Error() string    { return reason.reason }
func (reason *sentinel) Kind() fault.Kind { return reason.kind }

// declarationMistake is a route, codec or server declared so it cannot be
// served; no retry mends it.
func declarationMistake(reason string) error {
	return &sentinel{reason: reason, kind: fault.Unreadable}
}

// unreadableValue is a value a request carried that is not what its
// parameter declares.
func unreadableValue(reason string) error { return &sentinel{reason: reason, kind: fault.Unreadable} }

// requestMistake is a request that lacks what it must carry.
func requestMistake(reason string) error { return &sentinel{reason: reason, kind: fault.Unacceptable} }

// faultOf names what failed, and reports nothing when nothing did, so a caller
// composing stages does not have to check twice.
func faultOf(op string, err error) error {
	if err == nil {
		return nil
	}
	return Fault{Op: op, Err: err}
}

// errNoListener reports a server asked to serve without anything to serve on.
var errNoListener = declarationMistake("neither an address nor a listener was given")
