package api

import (
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang-sql/sql"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Fault is what a handler fails with; which status each kind becomes is
// decided once, in StatusFor.
type Fault struct {
	Kind FaultKind
	Err  error
}

// FaultKind is what the catalogue refuses for.
type FaultKind string

const (
	// NotFound is a book asked for that is not kept.
	NotFound FaultKind = "not-found"
	// Refused is a request the catalogue does not offer: a sort, a page or a
	// cursor it does not know, or a book saved under another's ISBN.
	Refused FaultKind = "refused"
	// Unavailable is the database failing.
	Unavailable FaultKind = "unavailable"
)

func (fault Fault) Error() string {
	if fault.Err == nil {
		return string(fault.Kind)
	}
	return string(fault.Kind) + ": " + fault.Err.Error()
}

func (fault Fault) Unwrap() error { return fault.Err }

// StatusFor maps a fault to a response, once.
func StatusFor(fault Fault) web.Response {
	switch fault.Kind {
	case NotFound:
		return web.Empty(http.StatusNotFound)
	case Refused:
		return web.Text(http.StatusBadRequest, fault.Err.Error())
	default:
		return web.Empty(http.StatusInternalServerError)
	}
}

// asAPIEffect is a store's answer with its fault said in the catalogue's words.
func asAPIEffect[A any](answer effect.Effect[effect.Unit, sql.Fault, A]) apiEffect[A] {
	return answer.MapError(func(fault sql.Fault) Fault {
		switch {
		case errors.Is(fault, sql.ErrNoRows):
			return Fault{Kind: NotFound, Err: fault}
		case errors.Is(fault, sql.ErrPageQuery), errors.Is(fault, sql.ErrPageCursor):
			return Fault{Kind: Refused, Err: fault}
		default:
			return Fault{Kind: Unavailable, Err: fault}
		}
	})
}
