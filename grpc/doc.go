// Package grpc serves and calls procedures over the gRPC wire protocol.
//
// A procedure is a name, a request schema, a response schema and a handler --
// the same four things an endpoint is, and for the same reason: one description
// encodes the request, decodes the response, and produces the .proto file
// another language generates its client from.
//
// The transport is behind a port so that neither implementation is baked in.
// connectrpc.com/connect is the one here, and it was chosen because it speaks
// the gRPC wire protocol *over* net/http: one server, one middleware stack, and
// this module's own routing compose with it. google.golang.org/grpc runs its own
// server with its own interceptors -- a parallel universe to net/http -- and
// stays implementable behind the same port for callers who need xDS or a
// service mesh.
package grpc
