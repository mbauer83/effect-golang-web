# gRPC reference

`grpc` serves and calls procedures over the gRPC wire protocol. A procedure is a
name, a request schema, a response schema and a handler — the same four things
an [endpoint](web.md) is, and for the same reason: one description encodes the
request, decodes the response, and becomes the `.proto` file another language
generates its client from.

```go
var Quote = grpc.Unary("logistics.v1.Rates", "Quote", EnquirySchema, RateSchema).
    Documented("Quote prices one shipment, or says why it cannot be priced.")

transport := grpc.NewConnected()
boundary, err := grpc.NewBoundary(runtime, environment, transport, Coded)
err = grpc.Answer(boundary, Quote, Priced(rates))
handler, err := boundary.Handler()   // mount it like any http.Handler
```

## Unary only, and why that is said out loud

A streaming procedure is a `Stream` on one side or both, which is a different
shape from a handler and needs what a [websocket](websocket.md) needed — the
boundary *interpreting* rather than answering. It is a thing to add when there
is a caller for it, on that seam. What is here answers one request with one
response.

## Why Connect

`connectrpc.com/connect` speaks the gRPC wire protocol **over `net/http`**, so
one server, one middleware stack and this module's own routing compose with it.
`google.golang.org/grpc` runs its own server with its own interceptors — a
parallel universe to `net/http` — and stays implementable behind the same port
for callers who need xDS or a service mesh.

The port works in **bytes**: a transport's job is the envelope, the protocol
negotiation and the status, not the meaning of the payload. That is what keeps
it small enough for a second implementation to be plausible — Connect and
grpc-go disagree about almost everything except that a unary call is bytes in,
and bytes or a code out.

```go
type Serving interface {
    Answer(path string, answer Answering) error
    Handler() (http.Handler, error)
}
type Calling interface {
    Call(ctx context.Context, path string, request []byte) ([]byte, *Failure)
}
type Answering func(ctx context.Context, request []byte) ([]byte, *Failure)
```

`Answering` returns a `Failure` rather than an `error` so a transport has the
code without having to guess one. A transport inventing a code from an error
string would be inventing the one thing the caller branches on.

## Codes, and why they are not statuses

`Code` is gRPC's own closed set of canonical codes, named rather than numbered.
It is deliberately **not** the HTTP status vocabulary: an RPC says `NotFound`
where a resource says 404, and the two are different alphabets for overlapping
ideas — so `Boundary` is parallel to [`web.Adapter`](web.md) rather than built
on it. Routing an application's refusals through the HTTP vocabulary on the way
would mean translating twice and losing in both directions.

An application maps its own refusals in one place:

```go
func Coded(refusal Refusal) grpc.Failure {
    switch refusal.Reason {
    case TooHeavy:   return grpc.Failure{Code: grpc.FailedPrecondition, Message: refusal.Details}
    case NoCapacity: return grpc.Failure{Code: grpc.Unavailable, Message: refusal.Details}
    default:         return grpc.Failure{Code: grpc.NotFound, Message: refusal.Details}
    }
}
```

A handler never mentions a code, which is what makes the mapping readable on its
own. `Failure.Message` is prose for an operator; a caller that has to branch
branches on the `Code`, which is why that is a closed set and this is not.

A defect answers `Unknown` and an interruption `Cancelled`, with no detail: a
defect is by definition something the application did not account for, so its
text is not fit to send to a caller. It is reported instead —
`WithReport` — for the same MEASURED reason the HTTP boundary reports one: the
runtime emits its fiber events for *forked* fibers, so a defect in the effect a
boundary interprets directly reaches no observer on its own.

## Calling

```go
grpc.Ask[R](client, Quote, enquiry)   // Effect[R, grpc.Failure, Rate]
```

The failure channel is `grpc.Failure`, because that is what the peer said: the
code and the message are the only things it told us, and mapping them into the
application's own failures is the application's business — the same separation
the HTTP boundary makes between a status and a refusal.

A request the caller's own description refuses is `InvalidArgument` **before
anything is sent**, since the caller holds the same description and a round trip
would learn nothing. A peer answering with something *its* contract refuses is
`Internal`, because that is a fault of the peer.

`Dial` speaks gRPC proper, which needs HTTP/2 — over plain TCP that means an
`http.Client` configured for h2c, a decision about the deployment rather than
about the RPC. `DialConnect` speaks Connect's own protocol over HTTP/1.1, for a
caller whose peers are all Connect servers.

## The contract

```go
document, err := grpc.Contract("logistics.v1", Quote, Book, Track)
proto := document.Render()
```

Every shape is declared once and shared, so a type used by ten procedures
appears once, and the order is the order the procedures were given — so the same
set always produces the same file and the file can be checked in.

This is the same idea as the [OpenAPI](openapi.md) document a surface implies,
and the difference is worth saying: an OpenAPI document is *read*, where a
`.proto` file is *compiled*. Another language generates its client from this, so
a field number changing here changes that client's wire format. Which is why
[the numbers are declared and not derived](protobuf.md#field-numbers).

A procedure names its service in full — `logistics.v1.Rates` — because the full
name is what forms the path a client calls. A proto file names it bare, because
the package declaration supplies the rest. `Contract` derives one from the other
and refuses a service that is not inside the package being projected: the file
would compile and declare a service at an address nobody calls.

## The one untyped file

`grpc/connect_codec.go` is where this package holds a value it cannot name.
Connect's `Codec` contract is untyped, because a codec is registered for a
content type and marshals whatever message the procedure it serves happens to
take. Since the port works in bytes, what crosses is always one type — and the
assertion is checked rather than assumed. The architecture test names the file,
as it names the other three.

The codec is called `proto`, which puts `application/grpc+proto` on the wire, so
a client generated from the projected file talks to this without knowing
anything about it. Naming it otherwise would produce a content type no generated
client asks for.

## What the tests establish

The protocol is the point, so a test that called a handler directly would test
the schema layer and skip everything the transport does. Instead:

- an `http.Server` on a port the operating system chose, a client over it, and
  the assertions made on what came back;
- one suite over **gRPC proper**, behind h2c, which reads the content type and
  the HTTP version off the request the server received — `application/grpc` over
  `HTTP/2.0` is what a generated client sends, and nothing else would do;
- the contract **compiled by a real protobuf compiler**, with the assertions
  made against the descriptor, including that the path it implies is the path
  the transport answers at;
- the server's own contract check exercised by calling the transport with the
  bytes a generated client sends for a zero — which is the half that matters for
  a caller that does not hold the description, and which the typed client's own
  refusal was hiding.

[`examples/quoting`](../../examples/quoting/quoting.go) is the service. Nothing
in it imports Connect.
