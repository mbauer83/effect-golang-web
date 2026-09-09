package web

// Asking, and being told nobody can answer.

import (
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang/effect"
)

// IsSuccessful reports whether the service answered rather than refused, which
// is what decides whether the answer is worth keeping.
func (received Received) IsSuccessful() bool {
	return received.Status >= http.StatusOK && received.Status < http.StatusMultipleChoices
}

// refusing turns a status into a failure while a retry is running.
//
// Only so that a schedule has something to retry. Every status is an answer to
// whoever asked, and recovered below hands it back as one.
func refusing[R any](called string) func(Received) effect.Effect[R, Fault, Received] {
	return func(received Received) effect.Effect[R, Fault, Received] {
		if received.IsSuccessful() {
			return effect.Succeed[R, Fault](received)
		}
		return effect.Fail[R, Received](Fault{
			Doing: "calling " + called,
			Err:   Refusal{Status: received.Status, Entity: received.Entity, header: received.Header},
		})
	}
}

// recovered hands back the response a refusal was made of, so this answers the
// way Fetch answers: with the status as data.
//
// A fault that is not a refusal is a request that never got a response -- a
// connection that would not open, a context that ended -- and there is nothing
// to hand back.
func recovered[R any](failed Fault) effect.Effect[R, Fault, Received] {
	var refusal Refusal
	if !errors.As(failed, &refusal) {
		return effect.Fail[R, Received](failed)
	}
	return effect.Succeed[R, Fault](Received{
		Status: refusal.Status,
		Header: refusal.header,
		Entity: refusal.Entity,
	})
}

// ErrUnstatedTerms is terms that do not say enough to read a service by.
//
// Exported because a caller wrapping this in its own adapter has the same
// refusal to make before it can build one, and a second sentence saying the
// same thing would be a second sentence to keep in step.
var ErrUnstatedTerms = errors.New(
	"a careful client needs a name, an allowance and a lifetime; see web.Carefully")
