package grpc

// What this package needs of a gRPC transport, and no more.
//
// The port works in bytes. A transport's job is the envelope, the protocol
// negotiation and the status -- not the meaning of the payload, which is the
// schema's above it. That is what keeps the port small enough for a second
// implementation to be plausible: Connect and grpc-go disagree about almost
// everything except that a unary call is bytes in and bytes or a code out.

import (
	"context"
	"net/http"
)

// Answering is what a transport calls when a request arrives: the request's
// bytes, and either the response's bytes or the failure to report.
//
// It returns a Failure rather than an error so a transport has the code without
// having to guess one, and a nil Failure pointer is the successful case. A
// transport that invented a code from an error string would be inventing the
// one thing the caller branches on.
type Answering func(ctx context.Context, request []byte) ([]byte, *Failure)

// Serving is a transport that answers procedures.
type Serving interface {
	// Answer adds a procedure at its path. A path already answered is a
	// declaration mistake, and the transport says so here rather than
	// resolving it silently.
	Answer(path string, answer Answering) error
	// Handler is what the HTTP core mounts. It is built after every procedure
	// has been added, because a transport may need to know the whole set.
	Handler() (http.Handler, error)
}

// Calling is a transport that invokes a procedure.
//
// The address and the transport are one thing here rather than two: a client
// holds a connection, and which procedure it calls is per-call.
type Calling interface {
	Call(ctx context.Context, path string, request []byte) ([]byte, *Failure)
}
