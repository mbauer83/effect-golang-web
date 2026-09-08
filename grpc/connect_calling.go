package grpc

// The Connect adapter's calling side, and the typed call above it.

import (
	"context"
	"errors"
	"net/http"

	rpc "connectrpc.com/connect"

	"github.com/mbauer83/effect-golang-schema/schema/protobuf"
	"github.com/mbauer83/effect-golang/effect"
)

// Dialled is a Connect client behind the port.
//
// It holds the base address and the HTTP client, which is what a gRPC
// connection is when the protocol runs over net/http: there is no separate
// connection object to keep, and cancellation, timeouts and transport
// configuration are the http.Client's.
type Dialled struct {
	client  rpc.HTTPClient
	address string
	// grpcProtocol says whether to speak gRPC's own protocol rather than
	// Connect's. It is set once, because which protocol a peer speaks is a
	// property of the peer and not of a call.
	grpcProtocol bool
}

// Dial makes a client that speaks the gRPC wire protocol.
//
// The address is the server's base URL -- "http://host:8080" -- and the
// procedure's path is appended per call. h2c is the caller's business: gRPC
// proper needs HTTP/2, and over plain TCP that means an http.Client configured
// for it, which is a decision about the deployment rather than about the RPC.
func Dial(client *http.Client, address string) *Dialled {
	return &Dialled{client: client, address: address, grpcProtocol: true}
}

// DialConnect makes a client that speaks Connect's own protocol, which runs
// over HTTP/1.1 and needs no h2c.
//
// It exists because that is what makes a test of this package a test rather
// than a test of h2c, and because a caller whose peers are all Connect servers
// has no reason to pay for HTTP/2.
func DialConnect(client *http.Client, address string) *Dialled {
	return &Dialled{client: client, address: address}
}

// Call invokes a procedure.
func (transport *Dialled) Call(
	ctx context.Context,
	path string,
	request []byte,
) ([]byte, *Failure) {
	options := []rpc.ClientOption{rpc.WithCodec(passingThrough{})}
	if transport.grpcProtocol {
		options = append(options, rpc.WithGRPC())
	}
	client := rpc.NewClient[payload, payload](transport.client, transport.address+path, options...)

	answer, err := client.CallUnary(ctx, rpc.NewRequest(&payload{bytes: request}))
	if err != nil {
		return nil, refused(err)
	}
	return answer.Msg.bytes, nil
}

// refused is the Failure a Connect error is.
func refused(err error) *Failure {
	var refusal *rpc.Error
	if !errors.As(err, &refusal) {
		// Not a refusal from the peer: the call did not get through at all,
		// which is Unavailable rather than a code the service chose.
		return &Failure{Code: Unavailable, Message: err.Error()}
	}
	return &Failure{Code: ourCode(refusal.Code()), Message: refusal.Message()}
}

// Ask calls a procedure and decodes its answer, as an effect.
//
// The failure channel is this package's Failure, because that is what the peer
// said: the code and the message are the only things it told us, and mapping
// them into the application's own failures is the application's business --
// the same separation the HTTP boundary makes between a status and a refusal.
func Ask[R, In, Out any](
	transport Calling,
	procedure Procedure[In, Out],
	request In,
) effect.Effect[R, Failure, Out] {
	return effect.For[R, Failure]().
		Suspend(func() effect.Effect[R, Failure, Out] {
			if err := procedure.Fault(); err != nil {
				return failing[R, Out](InvalidArgument, err.Error())
			}
			written, err := protobuf.Encode(procedure.request, request)
			if err != nil {
				// The caller's own request does not satisfy the contract, so
				// nothing is sent: InvalidArgument is what the peer would have
				// said, and saying it here saves a round trip.
				return failing[R, Out](InvalidArgument, err.Error())
			}
			return asking[R](transport, procedure, written)
		}).
		Named("ask")
}

func asking[R, In, Out any](
	transport Calling,
	procedure Procedure[In, Out],
	request []byte,
) effect.Effect[R, Failure, Out] {
	return effect.From(func(ctx context.Context, _ R) effect.Exit[Failure, Out] {
		answer, failure := transport.Call(ctx, procedure.Path(), request)
		if failure != nil {
			return effect.ExitFailure[Failure, Out](*failure)
		}
		read, err := protobuf.Decode(procedure.response, answer)
		if err != nil {
			// The peer answered with something its own contract refuses, which
			// is a fault of the peer rather than of this caller.
			return effect.ExitFailure[Failure, Out](Failure{
				Code:    Internal,
				Message: "the response does not satisfy " + procedure.Path() + ": " + err.Error(),
			})
		}
		return effect.ExitSuccess[Failure](read)
	}).Named("call")
}

func failing[R, Out any](code Code, message string) effect.Effect[R, Failure, Out] {
	return effect.For[R, Failure]().Fail[Out](Failure{Code: code, Message: message})
}
