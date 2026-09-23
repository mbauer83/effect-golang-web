package web

// Asking, and being told nobody can answer.

import (
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang/effect"
)

// IsSuccessful reports whether the service answered rather than refused, which
// is what decides whether the answer is worth keeping.
func (response ClientResponse) IsSuccessful() bool {
	return response.Status >= http.StatusOK && response.Status < http.StatusMultipleChoices
}

// failUnsuccessful turns a status into a failure while a retry is running.
//
// Only so that a schedule has something to retry. Every status is an answer to
// whoever asked, and recovered below hands it back as one.
func failUnsuccessful[R any](target string) func(ClientResponse) effect.Effect[R, Fault, ClientResponse] {
	return func(response ClientResponse) effect.Effect[R, Fault, ClientResponse] {
		if response.IsSuccessful() {
			return effect.Succeed[R, Fault](response)
		}
		return effect.Fail[R, ClientResponse](Fault{
			Op:  "calling " + target,
			Err: Refusal{Status: response.Status, Entity: response.Entity, header: response.Header},
		})
	}
}

// recoverRefusal hands back the response a refusal was made of, so this answers the
// way Fetch answers: with the status as data.
//
// A fault that is not a refusal is a request that never got a response -- a
// connection that would not open, a context that ended -- and there is nothing
// to hand back.
func recoverRefusal[R any](fault Fault) effect.Effect[R, Fault, ClientResponse] {
	var refusal Refusal
	if !errors.As(fault, &refusal) {
		return effect.Fail[R, ClientResponse](fault)
	}
	return effect.Succeed[R, Fault](ClientResponse{
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
	"a upstream client needs a name, an allowance and a lifetime; see web.Carefully")
