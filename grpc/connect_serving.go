package grpc

// The Connect adapter's serving side.
//
// Connect speaks the gRPC wire protocol over net/http, which is the whole
// reason it is the reference here: what it hands back is an http.Handler, so
// one server, one middleware stack and this module's own routing compose with
// it. A real gRPC client talks to it, because the protocol on the wire is
// gRPC's.
//
// Only the envelope and the status come from Connect. The payload is already
// bytes by the time it arrives, because the port works in bytes and the schema
// layer above it has done the typing.

import (
	"context"
	"net/http"

	rpc "connectrpc.com/connect"
)

// Connected is a Connect transport behind the port.
type Connected struct {
	answered map[string]bool
	mux      *http.ServeMux
}

// NewConnected makes one.
func NewConnected() *Connected {
	return &Connected{answered: map[string]bool{}, mux: http.NewServeMux()}
}

// Answer mounts a procedure.
//
// A path answered twice is a declaration mistake rather than a precedence rule,
// which is the same decision the routing tree makes about ambiguous routes: two
// answers for one name means the author meant one thing and wrote two.
func (transport *Connected) Answer(path string, answer Answering) error {
	if transport.answered[path] {
		return faulted("answering a procedure", path, errAnsweredTwice)
	}
	transport.answered[path] = true

	handler := rpc.NewUnaryHandler(path,
		func(ctx context.Context, request *rpc.Request[payload]) (*rpc.Response[payload], error) {
			written, failure := answer(ctx, request.Msg.bytes)
			if failure != nil {
				return nil, refusal(*failure)
			}
			return rpc.NewResponse(&payload{bytes: written}), nil
		},
		rpc.WithCodec(passingThrough{}),
	)
	transport.mux.Handle(path, handler)
	return nil
}

// Handler is what the HTTP core mounts.
func (transport *Connected) Handler() (http.Handler, error) {
	if len(transport.answered) == 0 {
		return nil, faulted("building a handler", "", errNoProcedures)
	}
	return transport.mux, nil
}

// Paths are the procedures this transport answers, in no particular order. It
// is here because a projection and a test both want to know what was mounted.
func (transport *Connected) Paths() []string {
	paths := make([]string, 0, len(transport.answered))
	for path := range transport.answered {
		paths = append(paths, path)
	}
	return paths
}

// refusal is the Connect error a Failure becomes.
func refusal(failure Failure) error {
	return rpc.NewError(connectCode(failure.Code), errorText(failure.Message))
}

// connectCode maps this package's closed set onto Connect's.
//
// The two sets are the same set -- gRPC's canonical codes -- so this is a
// renaming and not a translation. It exists so that nothing above this file
// mentions Connect, which is what lets a second transport be written.
func connectCode(code Code) rpc.Code {
	switch code {
	case Cancelled:
		return rpc.CodeCanceled
	case InvalidArgument:
		return rpc.CodeInvalidArgument
	case DeadlineExceeded:
		return rpc.CodeDeadlineExceeded
	case NotFound:
		return rpc.CodeNotFound
	case AlreadyExists:
		return rpc.CodeAlreadyExists
	case PermissionDenied:
		return rpc.CodePermissionDenied
	case ResourceExhausted:
		return rpc.CodeResourceExhausted
	case FailedPrecondition:
		return rpc.CodeFailedPrecondition
	case Aborted:
		return rpc.CodeAborted
	case OutOfRange:
		return rpc.CodeOutOfRange
	case Unimplemented:
		return rpc.CodeUnimplemented
	case Internal:
		return rpc.CodeInternal
	case Unavailable:
		return rpc.CodeUnavailable
	case DataLoss:
		return rpc.CodeDataLoss
	case Unauthenticated:
		return rpc.CodeUnauthenticated
	default:
		return rpc.CodeUnknown
	}
}

// ourCode is the reverse, for a client reading what a server said.
func ourCode(code rpc.Code) Code {
	for _, ours := range everyCode {
		if connectCode(ours) == code {
			return ours
		}
	}
	return Unknown
}

var everyCode = []Code{
	Cancelled, Unknown, InvalidArgument, DeadlineExceeded, NotFound, AlreadyExists,
	PermissionDenied, ResourceExhausted, FailedPrecondition, Aborted, OutOfRange,
	Unimplemented, Internal, Unavailable, DataLoss, Unauthenticated,
}
