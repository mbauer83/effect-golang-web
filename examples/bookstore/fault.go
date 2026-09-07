package bookstore

import (
	"errors"
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
	// Unacceptable is a request whose entity the schema refused.
	Unacceptable FaultKind = "unacceptable"
	// Unreadable is a request whose entity could not be read at all.
	Unreadable FaultKind = "unreadable"
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
// A rejected entity says which field was wrong, because the schema knows and a
// client that is not told cannot fix its request.
func StatusFor(fault Fault) web.Response {
	switch fault.Kind {
	case NotFound:
		return web.Empty(http.StatusNotFound)
	case Unacceptable:
		return web.Text(http.StatusBadRequest, fault.Error())
	default:
		return web.Empty(http.StatusBadRequest)
	}
}

var errBlankBook = errors.New("a book has at least one page")
