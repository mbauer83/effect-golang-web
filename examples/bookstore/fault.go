package bookstore

import (
	"net/http"

	"github.com/mbauer83/effect-golang-web/web"
)

// Fault is what a bookstore handler can fail with.
//
// A handler produces one of these and never a status. Which status each becomes
// is decided once, at the boundary, which is what leaves the same handler
// usable behind a different contract.
type Fault struct {
	Kind FaultKind
	Err  error
}

// FaultKind is the whole vocabulary of things this application refuses for.
type FaultKind string

const (
	// NotFound is a request for something the store does not hold.
	NotFound FaultKind = "not-found"
	// AlreadyHeld is an attempt to add a title the store already has.
	AlreadyHeld FaultKind = "already-held"
)

func (fault Fault) Error() string {
	if fault.Err == nil {
		return string(fault.Kind)
	}
	return string(fault.Kind) + ": " + fault.Err.Error()
}

func (fault Fault) Unwrap() error {
	return fault.Err
}

// StatusFor maps the application's vocabulary to statuses, once.
//
// This is the only place in the program that mentions a status for a failure.
// A request the schema refused never arrives here at all: that is a rejection,
// answered by the surface, and it is not the application failing.
func StatusFor(fault Fault) web.Response {
	switch fault.Kind {
	case NotFound:
		return web.Empty(http.StatusNotFound)
	case AlreadyHeld:
		return web.Text(http.StatusConflict, fault.Error())
	default:
		return web.Empty(http.StatusInternalServerError)
	}
}
