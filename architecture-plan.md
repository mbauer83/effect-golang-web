# effect-golang-web: architecture and implementation plan

## Objective

Typed, composable web and integration capabilities on top of the
`effect-golang` runtime: a schema abstraction, HTTP routing with matching and
dispatch, OpenAPI generation, WebSockets, AMQP, gRPC/protobuf and database
access.

The runtime's own principles carry over unchanged, and they decide most of the
questions below:

- goroutines are the execution primitive and the Go scheduler is the scheduler;
- `context.Context` is the cancellation boundary;
- native Go types stay native, and an abstraction is introduced only where it
  adds semantics the native thing genuinely lacks;
- typed failures, defects and interruption stay distinct;
- resources and concurrent work share one `Scope` mechanism;
- type-growing composition uses `Product` and `Either` and is expressed as
  package functions, because Go rejects a generic method whose result argument
  is built from its receiver (`golang/go#80172`).

## Research baseline

- [zio-http at `75b5983`](https://github.com/zio/zio-http), especially
  `Endpoint`, the per-location `HttpCodec` family, `RoutePattern`, `Routes`,
  `Middleware` and OpenAPI generation.
- `zio-schema` and Effect's `Schema`, for what a schema abstraction has to
  expose so that codecs, JSON Schema, protobuf and row mapping can all be
  derived from one description.
- Effect-TS, for the endpoint-declaration-separate-from-implementation shape.

Translate invariants, not syntax. Scala's implicit derivation, TypeScript's
mapped types and both runtimes' schedulers are not portable.

---

# 1. Library decisions

These were chosen on maintenance, protocol coverage and whether they compose
with `net/http`, which is the module's spine.

| Concern | Choice | Why, and what was rejected |
|---|---|---|
| HTTP server and client | `net/http` | It *is* the ecosystem boundary: `http.Handler`, middleware, TLS and HTTP/2 come with it, and every Go library already speaks it. `fasthttp` wins synthetic benchmarks but has an incompatible API, no HTTP/2, and would cut this module off from that ecosystem. |
| Routing | our own typed matcher | `chi`, `echo`, `gin` and `httprouter` are handler-centric and untyped; they cannot carry the type information OpenAPI generation and a typed client need. `net/http`'s own `ServeMux` patterns are untyped for the same reason. |
| JSON syntax | `encoding/json/jsontext` | Standard library in Go 1.27, token-based streaming, which is exactly the shape a schema-driven codec needs. No third-party JSON library is faster *and* token-based, and none is better maintained than the standard library. |
| WebSocket | `github.com/coder/websocket` | Context-aware on every operation, which is what this runtime's cancellation model needs; minimal surface, no cgo, runs over `net/http`, released within months. `gorilla/websocket` is two years stale and is not context-native. |
| AMQP 0-9-1 | `github.com/rabbitmq/amqp091-go` | Maintained by the RabbitMQ team; the successor to `streadway/amqp`. There is no serious alternative. |
| protobuf | `google.golang.org/protobuf` | The only maintained implementation. |
| gRPC transport | pluggable, `connectrpc.com/connect` as the reference | Connect speaks the gRPC wire protocol *over* `net/http`, so one server, one middleware stack and this module's routing compose with it. `google.golang.org/grpc` runs its own server with its own interceptors, a parallel universe to `net/http`; it remains supportable behind the same port for callers who need xDS or a service mesh. |
| SQL | `database/sql` port, `github.com/jackc/pgx/v5` reference adapter | Defining the port over `database/sql` keeps every driver usable; pgx is the best-maintained and fastest Postgres driver and works both ways. |
| OpenAPI | emitted from our own model; `github.com/getkin/kin-openapi` in tests only | The document is a projection of types we already hold, so adopting another document model would mean mapping into it and back. Validating what we emit against a real parser is exactly what a test dependency is for. |

No third-party dependency is required to use the HTTP core. Each transport's
dependency is confined to that transport's package.

---

# 2. Layering

```text
schema/                  Schema[A]: structure, codecs, projections
web/                     Request, Response, Handler, Route, Endpoint, Server
web/openapi/             the document, projected from endpoints
websocket/               framed conversations over web
sql/                     query and row decoding over schema
amqp/                    messages over schema
grpc/                    protobuf codec over schema, transport behind a port
```

Dependencies point left. `schema` knows nothing about HTTP. `web` knows nothing
about AMQP. Each transport depends on `schema` and on `effect-golang`, and on
its own client library and nothing else.

The module root holds no source; every package is a directory, and tests live
under `test/` as they do in `effect-golang`.

---

# 3. Schema

Schema is the keystone: request and response codecs, OpenAPI, protobuf
descriptors and SQL row mapping are all projections of one description. It is
built first for that reason.

## 3.1 It must expose structure, not only codecs

A pair of encode/decode functions is enough for JSON and useless for OpenAPI. A
projection has to *walk* the description, so `Schema[A]` is a typed view over an
erased structural node, exactly as `Cause[E]` is a typed view over an erased
cause tree.

```text
Node = Scalar(kind) | Object(fields) | Sequence(element) | Mapping(key, value)
     | Union(variants) | Nullable(inner) | Reference(name)
```

`Nullable` is a node rather than a flag on the others because it applies to any
shape, and because it is not the same thing as an optional field: a nullable
value is present and null, an optional field is not there at all.

`Reference` exists for two reasons at once: recursive types terminate, and
OpenAPI wants `$ref` rather than an inlined copy at every use.

## 3.2 Values pass through a sink, not a tree

CORRECTED during implementation. The first draft had `Schema[A]` convert between
`A` and a format-neutral `Value`, with each format converting between `Value`
and its own syntax -- which is what zio-schema and Effect's Schema do.

It converts between `A` and a `Sink` instead: the schema drives, calling
`Text`, `BeginObject`, `FieldName` and so on, and a format implements those
calls once. Decoding is the mirror, with a `Source` the schema pulls from.

Two reasons, and the second is why this could not have been deferred:

- an intermediate tree is a second allocation and a second traversal per value,
  on the hottest path a web framework has;
- the choice appears in the type of every codec, every format and every
  projection, so changing it later would change every signature -- the same
  class of decision as a stream's chunking.

One description still serves every format, which was the point of the tree. A
format that genuinely wants a materialised tree can implement `Sink` to build
one.

Object decoding is the one place this costs more than a tree would. JSON
delivers fields in document order and a struct schema declares them in its own,
so the object decoder dispatches on field name rather than reading positionally,
skips unknown fields, and reports missing required ones.

## 3.3 A union names its variant as the key

ADDED during implementation, as a consequence of 3.2 that was not visible until
the sink existed.

Three wire forms for a sum were available:

```text
externally tagged   {"circle": {"radius": 2}}
internally tagged   {"kind": "circle", "radius": 2}
enveloped           {"kind": "circle", "value": {"radius": 2}}
```

The internally tagged form is the common REST idiom and it is the one this
design cannot have. A value passes through a token stream in one pass, so a
decoder that met `radius` before `kind` would have to buffer the object or
rewind to learn what it had been reading. The enveloped form fails for the same
reason at one remove: a producer is free to write `value` before `kind`.

The externally tagged form has the selection arrive first by construction, so it
is what `OneOf` writes. This is a real cost of 3.2 and belongs recorded next to
it rather than presented as a preference.

Two decoding asymmetries follow, and both are deliberate:

- an unknown **field** is skipped, because a decoder that refused one could not
  read a document from a newer producer;
- an unknown **variant** is refused, because there is no value to build without
  it, and tolerating one would yield a zero value the document never named.

The projection must agree with the codec, so a union projects as a `oneOf` of
single-member objects. That agreement is MEASURED, not asserted: the emitted
document is compiled by an external JSON Schema 2020-12 validator and checked to
accept every document the codec writes and refuse every one it refuses. The
validator is a test dependency only.

## 3.4 Derivation is a separate decision

Go has neither Scala's implicit derivation nor TypeScript's mapped types, so a
schema for a struct is written rather than summoned. Three ways exist to change
that, and they are not equivalent:

```text
combinators     statically typed, no reflection, verbose for a wide struct
reflection      zero boilerplate, but the structure is discovered at run time
generation      statically typed and terse, but adds a build step
```

The combinator core is required by all three, because reflection and generation
would both *produce* combinator schemas. It is therefore built first, and which
derivation to add is deferred until there is a real application to measure the
verbosity against. Nothing built now is wasted by that decision.

Whatever is added must not become the only usable path: a schema a caller can
write by hand is what keeps the description honest when a struct's wire shape
differs from its Go shape.

---

# 4. HTTP

## 4.1 Request and response are values

`Request` and `Response` wrap `net/http`'s own types rather than replacing
them, and expose the body as an effect. A handler is:

```text
Handler[R, E] = Request -> Effect[R, E, Response]
```

so a handler is a description, cancellation reaches it through the request's
context, and a failing handler's typed `E` is mapped to a status by the route
that owns it rather than by the handler itself.

`http.Handler` interoperability goes both ways: an existing `http.Handler`
becomes a `Handler`, and a `Routes` value becomes an `http.Handler`. That is the
same rule native channels follow -- the ecosystem boundary stays usable.

## 4.2 The server is a capability

Serving is an effect over a scoped resource: the listener is acquired in a
scope, and closing that scope shuts the server down gracefully and awaits
in-flight requests. Graceful shutdown is therefore not a separate mechanism; it
is what scope closure already means.

## 4.3 Codecs are per location and compose structurally

A request's parts decode independently:

```text
PathCodec     segments and captures
QueryCodec    query parameters
HeaderCodec   headers
BodyCodec     the entity, via a Schema
```

Combining two codecs yields a codec of `Product[A, B]`, which is the same
structural composition the runtime already uses, and for the same reason: Go
cannot compute a type-level record. Combination is a package function, per the
instantiation-cycle rule.

## 4.4 An endpoint is a declaration, its handler is separate

```text
Endpoint[In, E, Out]     what the route accepts, returns and can fail with
  .Handle(handler)  ->   Route
```

Separating them is what makes three things possible from one value: dispatch, an
OpenAPI document, and eventually a typed client. A route that carried only a
function could give none of them.

## 4.5 Matching is a tree, and ambiguity is an error

Routes compile into a segment tree: literal segments beat captures, captures
beat wildcards, and a method mismatch on a matched path is `405` rather than
`404`. Two routes that could match the same request are a construction error
reported when the routes are assembled, not a silent precedence surprise at run
time.

---

# 5. OpenAPI

The document is projected from the endpoints and their schemas. Component
schemas come from `Reference` nodes, so a type used by ten endpoints appears
once. Emitting is total: an endpoint with no documentation still produces a
valid document.

Tests validate the emitted document against `kin-openapi`, so "valid OpenAPI"
is checked by a parser rather than asserted by us.

---

# 6. Transports

Each follows the same shape, so learning one teaches the others:

```text
a connection is a scoped resource
a conversation is a Stream in and a sink out
messages are encoded by a Schema
cancellation closes the connection through scope closure
```

**WebSocket.** A conversation is a `Stream` of inbound frames and a sink for
outbound ones. The upgrade happens inside a scope, so a closed scope closes the
socket.

**AMQP.** A channel is a scoped resource; a consumer is a `Stream`; publishing
is an effect. Acknowledgement is explicit, because at-least-once delivery is a
property the caller must decide about, not one a library should hide.

**gRPC.** The protobuf codec is a schema projection. The transport sits behind a
port so Connect and `grpc-go` are both implementable, and neither is baked in.

---

# 7. Database

A `Database` capability with `Query`, `QueryRow` and `Exec`, rows decoded
through a `Schema`, and streaming results as a `Stream` so a large result set
does not have to be held. Transactions are scoped resources: commit on success,
roll back on failure or interruption, which is `AcquireRelease` with an
exit-aware release and nothing new.

Prepared statements are the default, per the project's standards.

---

# 8. Implementation sequence

```text
1. schema core: nodes, values, combinators, JSON codec, JSON Schema projection  DONE
2. web core: Request, Response, Handler, net/http interoperability, Server
3. codecs, Endpoint, route matching and dispatch, middleware
4. OpenAPI generation
5. websocket
6. sql
7. amqp
8. grpc
```

Each step gets tests, an example and documentation before the next begins, on
the same expansion-and-consolidation cycle the runtime was built with.

Step 1 delivered: `Schema[A]` with scalars, lists, maps, nullables, structs with
optional fields, unions, refinements and recursion; a public `structure` package
a projection walks; a JSON codec over `encoding/json/jsontext`; the JSON Schema
2020-12 projection with shared components; `examples/catalog` as a complete
program; and the pair test above. Derivation and the remaining projections stay
deferred as 3.4 records.

---

# 9. Invariants to freeze

1. **No request is handled before interpretation.** A handler is a description.
2. **`net/http` stays reachable.** Handlers convert both ways.
3. **A server's lifetime is a scope.** Shutdown awaits in-flight requests.
4. **A connection is a scoped resource.** Nothing outlives its scope.
5. **Typed failures map to statuses at the route, not in the handler.**
6. **Defects and interruption never become a status by accident**; an
   unhandled defect is a 500 *and* a reported diagnostic.
7. **One schema serves every format.** A format never gets its own description.
8. **A schema can always be written by hand.**
9. **Ambiguous routes are a construction error.**
10. **The emitted OpenAPI document is valid**, checked by a parser.
11. **No third-party dependency is needed to use the HTTP core.**
12. **A transport's dependency stays in that transport's package.**
