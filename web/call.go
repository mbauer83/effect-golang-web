package web

// One endpoint, read from the calling side.
//
// The declaration already says the method, the path pattern, the status it
// answers with and the shape of the entity -- which is what separating it from
// the handler was for. So a client that holds the endpoint needs none of that
// again, and a response is decoded through the same description the server
// encoded it through.
//
// The request side is still explicit. A Codec reads a request into an In, and
// reading is not invertible: Convert takes one function, so a struct a handler
// received cannot be turned back into the parts it came from. Filling the
// captures, the query and the entity is therefore the caller's, and what the
// endpoint contributes is everything that has one answer.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mbauer83/effect-golang/effect"
)

// Refusal is a response whose status was not the one the endpoint declared.
//
// It carries the entity, because a server that refuses usually says why and a
// caller that could not read it would be guessing. It is the Err of a Fault, so
// errors.As reaches it.
type Refusal struct {
	Status int
	Entity []byte
	// header is what came with it, kept unexported because a refusal is about
	// the status and the body. FetchCarefully needs it to hand back the whole
	// response it made a refusal of, and nothing else has asked for it.
	header http.Header
}

func (refusal Refusal) Error() string {
	said := ""
	if len(refusal.Entity) > 0 {
		said = ": " + string(refusal.Entity)
	}
	return "the server answered " + strconv.Itoa(refusal.Status) + said
}

// Call sends a request to an endpoint and reads what the endpoint declared it
// answers with.
//
// A status the endpoint did not declare is a Fault carrying a Refusal, so a
// caller can act on it: a declared failure status is documentation on the
// server side, and on this side it is the answer that arrived instead of the
// one promised.
func Call[R, In, Out any](
	client *Client,
	endpoint Endpoint[In, Out],
	requesting Requesting,
) effect.Effect[R, Fault, Out] {
	if fault := ValidateEndpoint(endpoint); fault != nil {
		return effect.Fail[R, Out](asFault("calling an endpoint", fault))
	}
	path, err := fillPattern(endpoint.segments, requesting.Path)
	if err != nil {
		return effect.Fail[R, Out](asFault("building the path", err))
	}
	return Fetch[R](client, endpoint.method, path, requesting).
		FlatMap(func(received Received) effect.Effect[R, Fault, Out] {
			return answered[R](endpoint.output, received, endpoint.method+" "+path)
		})
}

// answered reads the response the output declared, or says what arrived
// instead.
func answered[R, Out any](
	output Output[Out],
	received Received,
	called string,
) effect.Effect[R, Fault, Out] {
	operations := effect.For[R, Fault]()
	if received.Status != output.status {
		return operations.Fail[Out](Fault{
			Doing: "calling " + called,
			Err:   Refusal{Status: received.Status, Entity: received.Entity},
		})
	}
	value, err := output.decode(received.Entity)
	if err != nil {
		return operations.Fail[Out](Fault{Doing: "reading the response body", Err: err})
	}
	return operations.Succeed(value)
}

// fillPattern writes the captured segments a caller supplied into the path.
//
// A capture with nothing to fill it is refused rather than sent as the literal
// "{title}", which a server would answer 404 for and leave the caller looking
// at the wrong end of the problem. A wildcard takes the rest of the path, so
// its value may contain slashes and is not escaped.
func fillPattern(segments []segment, captures map[string]string) (string, error) {
	if len(segments) == 0 {
		return "", errUnpatternedEndpoint
	}
	written := make([]string, 0, len(segments))
	for _, part := range segments {
		if part.kind == literalSegment {
			written = append(written, part.text)
			continue
		}
		value, given := captures[part.text]
		if !given {
			return "", errors.New("the path captures " + part.text +
				" and nothing was given for it")
		}
		written = append(written, value)
	}
	return "/" + strings.Join(written, "/"), nil
}

var errUnpatternedEndpoint = errors.New("the endpoint has no path")
