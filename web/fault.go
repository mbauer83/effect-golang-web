package web

import "errors"

// Fault is what the web layer itself can fail with: a request whose parts could
// not be read, or a server that could not start.
//
// It is deliberately not an application failure. A handler's own failures have
// the handler's own type, and the boundary maps those to statuses; this type is
// for the transport's own troubles, which every application handles the same
// way.
type Fault struct {
	// Doing names what was being attempted, which is what a caller acts on.
	Doing string
	Err   error
}

func (fault Fault) Error() string {
	if fault.Err == nil {
		return "web: " + fault.Doing
	}
	return "web: " + fault.Doing + ": " + fault.Err.Error()
}

// Unwrap keeps errors.Is and errors.As working through the boundary, so a
// caller can still ask whether a listener failed because the port was taken.
func (fault Fault) Unwrap() error {
	return fault.Err
}

// faulted names what failed, and reports nothing when nothing did, so a caller
// composing stages does not have to check twice.
func faulted(doing string, err error) error {
	if err == nil {
		return nil
	}
	return Fault{Doing: doing, Err: err}
}

// errNoListener reports a server asked to serve without anything to serve on.
var errNoListener = errors.New("neither an address nor a listener was given")
