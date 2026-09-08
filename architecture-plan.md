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
| AMQP 1.0 | `github.com/Azure/go-amqp` | A different protocol under the same name, so a separate package rather than a second adapter: 1.0 has no exchanges, keys or bindings and settles by disposition. This is the maintained Go implementation, and it is at 1.x. |
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

ADDED during implementation: a `Scalar` and a `Sequence` also carry
`Constraints`, from a sealed vocabulary of nine -- inclusive and exclusive
bounds, string lengths, a pattern, item counts. A kind says a value is a number;
a constraint says which numbers, and a description that could not say so would
leave every projection describing a wider type than the codec accepts.

The named string formats sit beside it and are not the same thing. A format is
an annotation a reader acts on and a rule a server enforces, and this makes
both: `uuid`, `email`, `uri`, `url`, `uri-reference`, `hostname`, `ipv4`,
`ipv6`. Where the rule is a regular expression it is recorded as a `Pattern`
alongside the format name, so a consumer whose validator ignores `format` --
which the specification permits, since `format` asserts nothing -- still gets
the check. Where the rule is grammar, as an address is, there is nothing to
record; that asymmetry is real and stays visible rather than being papered over
with a regular expression that is wrong about something.

`Formatted` remains the open case, carrying a name this package has not been
taught and claiming nothing, because the format vocabulary is open by design.

The vocabulary is small on purpose. A constraint earns a place in the
description only if more than one projection can carry it; anything narrower is
a refinement, which every projection describes as the shape underneath it. Each
constraint is its own combinator rather than one generic `Constrained`, because
measuring a value is type-dependent -- a number is compared, a string is
counted, a list is counted differently -- and a generic one would have to take a
measuring function nobody wants to write.

ADDED: a `Scalar` also carries a `Precision`, the Go representation it is
carried in. The wire has two numeric shapes and Go has twelve, and a
description that only said "a whole number" would make a generator guess a
width and make a published contract claim a range wider than the program
accepts. Kind stays the wire vocabulary; Precision is Go-side detail a format
never reads. The wire shape is derived from the width, not declared beside it.

Three dividends, and the first is why the constructors are precisely typed
rather than a width being a field somebody sets: `AtMost(Int8(), 200)` is a
**compile error**, because 200 is not an int8. Go cannot check a bound against
a range at compile time in general, but it checks a constant against a type for
free, and that is the same thing where it matters. The second is that the range
a width implies is recorded, so a contract states it without anyone writing it
down and a description enforces it without the Go type. The third is that a
generator emits the type the author meant.

Only the widths the specification registers a name for become a format
keyword. The rest are stated by minimum and maximum: a range is something a
reader acts on, and "uint16" in a contract is telling a client about Go.

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

## 3.3 A union names its variant as the key, or in one

ADDED during implementation, as a consequence of 3.2 that was not visible until
the sink existed.

Three wire forms for a sum were available:

```text
externally tagged   {"circle": {"radius": 2}}
internally tagged   {"kind": "circle", "radius": 2}
enveloped           {"kind": "circle", "value": {"radius": 2}}
```

The externally tagged form has the selection arrive first by construction, so
it is what `OneOf` writes, and it needs nothing of the format.

CORRECTED once the universal representation existed: the internally tagged form
is available too, as `OneOfBy`, and the earlier "cannot" was about a missing
mechanism rather than about the design. A decoder does have to read the whole
object before it knows what it read -- the name may arrive after the fields
whose meaning it settles -- so that power is asked of the format through an
optional `Buffering` capability rather than assumed. JSON has it; a streaming
source does not and says so. The enveloped form is still absent, because it
buys nothing the other two do not.

The cost is real and stays visible: `OneOf` streams, `OneOfBy` materialises one
object per value. Which to use is usually decided by whoever owns the wire, and
where nobody does, `OneOf` is the cheaper answer.

Two decoding asymmetries follow, and both are deliberate:

- an unknown **field** is skipped, because a decoder that refused one could not
  read a document from a newer producer;
- an unknown **variant** is refused, because there is no value to build without
  it, and tolerating one would yield a zero value the document never named.

The projection must agree with the codec, so an externally tagged union
projects as a `oneOf` of single-member objects, and an internally tagged one as
a `oneOf` of `allOf`s -- the variant's shape and the field pinned to its name
with `const` -- because a variant is a component and a `$ref` has nothing to
add a property to. That agreement is MEASURED, not asserted: the emitted
document is compiled by an external JSON Schema 2020-12 validator and checked to
accept every document the codec writes and refuse every one it refuses. The
validator is a test dependency only.

## 3.4 A schema does not need a Go type

ADDED after the first generator: `Schema[A]` works when there is no `A`.

A description written before the type it will become exists, or loaded from
elsewhere, has no Go type and is still worth validating, transcoding,
inspecting and composing -- and generating a struct from a description is
worthless if the description cannot first be trusted. So the adaptation is the
one ZIO Schema makes with DynamicValue and Effect's Schema makes by being
AST-first: a universal value representation, and the same vocabulary over it.

`Record` is `Struct` without the getters and setters, which are the only part
of a field declaration that needs the type; `Choice` is the same for a union;
`Dynamic(node)` is the general door, so a typed schema's own description is
usable without its type. Nothing else changes, because a Schema never cared
what A was: the same Validate, the same codecs, the same projections, the same
combinators.

`dynamic.Value` is a sealed sum of nine cases -- the Sink's twelve calls seen
from the other side, so a format learns nothing new to carry one. A struct, a
mapping and a union all appear as an object, because that is what all three are
on the wire; which one a value is meant to be is the description's business.
Giving them three near-identical cases would record the same fact twice, and it
would break the sink-and-source bridge that `ToDynamic` and `FromDynamic` are
built from.

What a description enforces is what it records, which is the second dividend of
recording constraints and of recording a format's expression beside its name.
A rule that could only be code -- an address, a URI -- annotates and no more.
The typed and described paths are tested to reach the same verdict on the same
value, because a generated struct that accepted what its description refused
would be the one bug this whole arrangement exists to prevent.

## 3.5 Derivation is generation, and runs from the description

CORRECTED twice. The claim that a struct could not be derived from a schema was
wrong about the mechanism; the answer of doing both directions was wrong about
the cost.

A Go struct cannot be derived from a `Schema[A]` value, because that value
names `A` and so `A` must exist for it to compile. But a description that names
no Go type -- `Record`, `Choice`, a `structure.Node` -- compiles on its own, and
reading it needs no parser and no reflection: it is a Go value, so a Go program
reads it by calling `Structure()`. Generation from a description is therefore a
program that imports the descriptions and writes the types, which is fifteen
lines and no toolchain gymnastics.

Generation therefore runs from the description, and only from it. The other
direction was built first and then removed: reading a struct meant a second
place to write a constraint -- `schema:"min=1,maxLength=200"` in a backtick
string -- and a second vocabulary for the same ideas. Two ways to say one thing
is the cost, and it is not worth a generator for something a person can write
in six lines.

A Go type you already have still gets a schema: `Struct` and `FieldOf` are
exactly that, and `examples/catalog` writes one. `examples/inventory` has its
types generated from descriptions. The generated file is compiled as part of the
module, so the compiler checks it, and a drift test regenerates it in process
and compares.

The descriptions are unexported, and that is the point of the arrangement
rather than a detail. The generated `NameSchema` is not a second copy of the
description: it is the description bound to a Go type, and it is the only thing
that can turn a document into a `Name` -- the description cannot, having no
`Name` to produce. That is the one thing it does not afford, and the whole
reason to generate anything.

The binding also *contains* the description, so `Dynamic(NameSchema.Structure())`
recovers it: a caller who wants the shape without the type has it from the one
exported thing, and exporting the description as well would be the duplicate.
That the two agree is MEASURED rather than argued -- the binding's structure is
deep-equal to the description it came from, and both refuse the same documents
-- because a binding that admitted a different shape from the one it was
generated from is the single bug this arrangement exists to prevent.

## 3.6 Why the binding cannot be written away

ASKED, and worth recording so it is not re-litigated: Python lets an annotation
name a type that does not exist yet, as a string. Could a schema do the same and
be its own binding, so there is one declaration rather than a description and a
generated `NameSchema`?

The forward reference itself is already here and is not the obstacle. A
description names a shape it has not built -- itself, or another description --
through `Deferred`, which contributes a `structure.Reference`: a name and a way
to reach what it names later. Two descriptions may name each other. And the type
name in `Struct[dynamic.Value]("Item")` is exactly the string Python writes: a
name for a Go type that need not exist, resolved by generation rather than by
`get_type_hints`.

What Python's version rides on is the part Go does not have. `pydantic` reads
annotations and *constructs* objects reflectively, setting fields it discovered
at run time. Skipping the binding needs that: something has to fill an `Item`,
and without accessors the only filler is reflection.

Three arrangements were considered and two are impossible:

```text
one hand-written schema naming "Item"      the accessors cannot be written,
                                           because Item does not exist yet
a generated binding referring to the       the accessor knows B, the description
hand-written description                   knows a node; converting between them
                                           needs a Schema[B] that only the
                                           accessor could have supplied
a generated struct and a generated         what is built
binding, from one description
```

The middle one is the interesting failure. A binding that referred to the
description rather than restating it would put the shape in exactly one place,
which is what the question is really after -- and it cannot be typed: giving
each accessor its member's schema restates the base types anyway, and not giving
it one leaves no way from a `dynamic.Value` to a `B`.

So the restatement in the generated file is forced, and what makes it safe is
that it is checked rather than trusted: the binding's structure is deep-equal to
the description it came from, and both refuse the same documents.

There is one real alternative, not built: a reflection-based `Bind[A](description)`
in a package of its own, giving one line instead of a generated binding. It
would discover the struct at run time, so it would lose the compile-time check
that the type matches the description, and it would put erasure back into a
module whose architecture test forbids a top type. It stays an option rather
than a default for that reason.

## 3.7 What a generator will not guess

Go has neither Scala's implicit derivation nor TypeScript's mapped types, so a
schema for a struct is written rather than summoned. Three ways existed to
change that, and they are not equivalent:

```text
combinators     statically typed, no reflection, verbose for a wide struct
reflection      zero boilerplate, but the structure is discovered at run time
generation      statically typed and terse, but adds a build step
```

The combinator core is required by all three, because reflection and generation
both *produce* combinator schemas, so it was built first and the choice
deferred until there were real applications to measure the verbosity against.

DECIDED after two of them existed: **generation**. Reflection would discover
the structure at run time and hand back the erasure the rest of this design
refuses; the module's own architecture test forbids a top type in production
code for exactly that reason, and reflection is that ban by another name.
Generated code is deterministic, formatted, fully typed, checked in, and
regenerated in process by a test that compares it with the structs it came
from.

Derivation runs one way only. A Go struct cannot be derived from a schema,
because Go cannot compute a type from a value; the struct is the source of
truth and the schema follows it. The other direction would need a source of
truth outside Go, which is a different project.

Everything the generator emits is a call a person could have written, so a
generated schema and a written one remain interchangeable and there is one
vocabulary to learn. What it will not do is guess: an anonymous shape has no Go
type, a union variant that is not an object cannot become one, and a member
name that cannot be exported is refused rather than mangled.

ADDED: every modifier is a method -- `Documented`, `Named`, `Optional` -- and
not a function that wraps what it changes. A reader should not have to remember
which of them do which, and the wrapping form read inside-out at exactly the
places a schema is longest.

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

ADDED during implementation, from two things that only became visible once it
ran:

- A **shutdown that gives up** on requests still in flight is reported through
  the scope's closing cause, not as the serve loop's failure. A forked fiber's
  outcome is observed by joining it, and a program interrupted while waiting
  joins nothing -- so the outcome that must never be lost would be exactly the
  one that is. A scope composes its finalizers' faults into the closing cause
  whatever happened to the body.
- A **defect at the boundary** reaches no observer on its own. MEASURED: the
  runtime emits its fiber events for forked fibers, and every request is an
  effect the boundary interprets directly. The boundary therefore has a report
  hook with a default rather than an optional one. A write failure could not
  reach an observer in any case, because it happens after the status has gone.

The read-header deadline has a non-zero default for the same class of reason:
net/http has none, so a client that opens a connection and never finishes its
headers can hold one indefinitely.

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

**WebSocket.** A conversation is a `Stream` of inbound messages and a sink for
outbound ones. The upgrade happens inside a scope, so a closed scope closes the
socket.

DONE, with three things settled by building it:

- `Accept` takes the **boundary**, not a handler's return. A conversation
  answers with no response, so there is nothing for a `Handler` to return, and
  the runtime, environment and reporting it needs are the boundary's. That is
  what `Adapter.Interpret` exists for, and a transport that takes over the
  connection is the only thing that needs it.
- A websocket endpoint is mounted as a route (`web.Upgrading`), so it is
  dispatched and documented by the same tree. Its declaration says what is true
  of the HTTP part -- a GET answering 101 with no entity -- and stops, because
  the protocol after the upgrade is not something an OpenAPI document can
  describe.
- The scope's release **says goodbye** rather than closing abruptly. A peer told
  the conversation is over ends its stream and reports nothing; a peer whose
  connection vanishes must treat that as the failure it usually is. The first
  version closed abruptly and reported every normal departure as a fault, which
  the reporting test caught.

A message the schema refuses fails the stream, deliberately: a conversation is
stateful, so a peer that said something unreadable has said something about the
whole exchange. `Inbound` is there for a conversation that would rather skip
one.

**AMQP 0-9-1.** A channel is a scoped resource; a consumer is a `Stream`;
publishing is an effect. Acknowledgement is explicit, because at-least-once
delivery is a property the caller must decide about, not one a library should
hide.

DONE, with four things settled by building it.

- The version is in the package name. `amqp091`, not `amqp`, because AMQP 1.0
  shares the name and almost nothing else and `amqp` beside `amqp10` would read
  as the general one. See below.
- There is a port, and not for the reason `sql` has one: amqp091-go is the only
  serious implementation. It is there because a program that publishes and
  consumes must be testable without a broker, and because `Publishing`,
  `Consuming` and `Declaring` are what an application actually depends on --
  three interfaces rather than one, since most programs use one of them.
  `amqp091/inprocess` is the substitute, and it is **shipped** rather than kept
  in a test file: if testability is the argument for the port, the thing that
  makes it testable is part of the capability. `effecttest` in the runtime is
  the precedent.
- A delivery the schema refuses does **not** fail the stream, which is the
  opposite of the websocket decision and for the reason that one was made: a
  conversation is stateful, so an unreadable message says something about the
  whole exchange; a queue is a sequence of separate messages, so it says
  something about one. That needed `CollectStreamEffect` in the runtime --
  `MapStreamEffect` is one-for-one, so a consumer deciding *effectfully* whether
  a value survives had nowhere to put the decision.
- A message's headers are an AMQP field table, which the protocol defines as a
  set of named values and the library represents as `map[string]any`. That is
  the module's **second** erasure boundary, named by the same architecture test
  as the first. It maps better than the database one: a field table permits
  every kind the representation has, nesting included, so nothing is narrowed.
  The two conversions are public, with `map[string]any` in their signatures
  rather than the library's named type, so the boundary can be tested from
  outside without anything above the package acquiring the dependency.

One correction, found by writing the reference rather than by a test: the first
version of the subscription's release cancelled nothing, on the reasoning that
the subscription's lifetime is the channel's. That is wrong. A stream that
stopped early leaves a broker that has no idea and goes on sending, into a Go
channel nobody reads; with a prefetch set those deliveries are unacknowledged
and the queue stalls behind them. Cancelling needs a consumer tag, and the
library does not hand back the one it generates -- so the consumer is named
here. It is checked only in the integration suite, because the in-process broker
does not push and so cannot witness it.

**AMQP 1.0** is a separate package, `amqp10`, over `github.com/Azure/go-amqp`.
Not a variant of the above and not behind the same port: 1.0 has no exchanges,
no routing keys and no bindings -- it addresses nodes directly -- and it settles
a delivery by disposition (accept, reject, release, modify) rather than by
acknowledgement. A port covering both would cover neither. What the two share is
the shape of the answer: scoped connection and session, a receiver as a
`Stream`, sending as an effect, settlement explicit, and the same `Schema`
describing the body.

DONE, with four things settled by building it.

- The fourth disposition is why this is a package and not an adapter. `Release`
  and `Modify` both put a message back, and the difference is what the broker
  learns: a release says the message is back and nothing else, where a modify
  says a receiver tried and failed, or that this receiver is the wrong one, and
  records what it found out. A 0-9-1 requeue is a release, so a broker there
  deciding whether a message has failed too often has only its own redelivery
  count to go on. `examples/consign` is three finalities -- *not now*, *not me*,
  *never* -- each mapping to exactly one disposition, and telling them apart is
  the application's job because nothing else can.
- There is no topology. 1.0 has nothing in the protocol to declare a node with:
  an address is the broker's own configuration. So the in-process broker's
  `Declare` stands in for that configuration rather than for a protocol
  operation, and the integration suite needs two environment variables where
  0-9-1's needed one.
- Credit is not a prefetch. A 0-9-1 consumer with no prefetch gets everything;
  a 1.0 link with no credit gets **nothing**, because the protocol is
  credit-based. So `Receiver` takes it as an argument rather than offering it as
  a tuning step.
- The application properties are the module's **third** erasure boundary, and it
  is deliberately not shared with the second. The two do the same-looking thing
  and permit different value sets -- 1.0 has unsigned integers, UUIDs, symbols
  and described types 0-9-1 cannot encode -- with a differently named nested map
  in each library. Merging them would invent "a broker's untyped map", a concept
  neither specification has, and the first value one protocol accepted and the
  other did not would split it again. Recorded here because a reviewer seeing
  two near-identical files should know it was considered.

Both AMQP packages were built on a machine with no broker reachable and no
usable Docker daemon, so what the adapters themselves do -- the sections and
maps they write, the flags they pass, the cancellation and the detached-link
ending -- is covered only by tests that skip locally. A skipped test is not
evidence.

CI now runs the 0-9-1 ones against a RabbitMQ service container, in a job of
their own so a broker that will not start does not block the suite that needs
none. That job has never run: it was written where it could not be exercised,
so until a pushed run is green it establishes nothing either. The 1.0 job is
not written, because a 1.0 node has no protocol operation to declare it -- the
address is broker configuration, so the container would need provisioning
before the test rather than a variable set for it.

**gRPC.** The protobuf codec is a schema projection. The transport sits behind a
port so Connect and `grpc-go` are both implementable, and neither is baked in.

The projection and the codec are DONE, with four things settled by building
them.

- Protobuf is the one projection where the **contract is the number and the
  name**, not the shape: renaming a Go field is safe and renumbering it is not,
  which is the opposite of every other projection here. So `Numbered` is a
  modifier on a field and on a variant, and it is required rather than derived
  from declaration order -- deriving it would reproduce exactly the failure
  field numbers exist to prevent. An unnamed object is refused for the same
  reason: a name taken from the field holding it would move when that field
  was renamed.
- The precision stops being Go-side detail. Everywhere else a `Kind` is all a
  format reads; here `int32` and `int64` are different wire types, and widening
  a schema that said `int32` would change what every other language generates.
- **This codec is not a Sink and a Source**, and that is the format's
  requirement. A nested message's length precedes it, so nothing can be written
  until the whole of it is known; a repeated field may appear under its number
  more than once and in any order, so nothing can be read in place. A protobuf
  message is always framed, so there is no streaming to give up -- and what
  crosses in between is the universal representation, for the reason the SQL
  package reads a row that way.
- One correction, and it reversed a decision. The first version refused to
  substitute proto3's defaults, on the reasoning that an absent field is absent
  and the description should decide what that means. That is wrong: an ordinary
  proto3 field writes **no bytes** for its zero, so absent and zero are the
  same message and there is no reading in which the field was not sent.
  Refusing to substitute did not defer to the description -- it made proto3
  undecodable, and the cross-check against the reference implementation failed
  on a boolean that was merely false. The constraints still apply to the implied
  zero, which is where a zero gets refused, so nothing was lost by fixing it.

The transport is DONE too, with four things settled by building it.

- The port works in **bytes**. A transport's job is the envelope, the protocol
  negotiation and the status; the meaning of the payload is the schema's, above
  it. That is what keeps the port small enough for a second implementation to be
  plausible -- Connect and grpc-go disagree about almost everything except that
  a unary call is bytes in and bytes or a code out. `Answering` returns a
  `Failure` rather than an `error`, because a transport inventing a code from an
  error string would be inventing the one thing the caller branches on.
- `Boundary` is **parallel to `web.Adapter`, not built on it**, and the
  duplication is worth its cost: an RPC answers with a code and a message where
  a resource answers with a status and an entity, and an application's refusals
  map onto one or the other directly. "No such customer" is `NotFound` to an
  RPC and 404 to a resource, and neither is the other's translation -- routing
  through the HTTP vocabulary on the way would translate twice and lose in both
  directions.
- **Unary only**, said out loud rather than left to be discovered. A streaming
  procedure is a `Stream` on one side or both, which is a different shape from a
  handler and needs what the websocket needed: the boundary interpreting rather
  than answering. `Adapter.Interpret` is that seam and it already exists, so
  this is a thing to add when there is a caller.
- Connect's `Codec` contract is untyped, so this is the module's **fourth**
  erasure boundary -- one file, named by the architecture test, in which the
  assertion is checked rather than assumed. The codec is called `proto` so that
  `application/grpc+proto` goes on the wire and a generated client talks to it
  without knowing anything about it.

One test was hiding a gap, found by neutering. The server's own contract check
looked covered, but the typed client refuses a bad request before sending it --
so removing the server's check changed nothing. The half that matters is a
caller that does *not* hold the description, which is every client generated
from the projected file in another language, and it needed a test that calls
the transport with the bytes such a client sends for a zero.

`google.golang.org/protobuf` and `bufbuild/protocompile` are **test**
dependencies only, as are `golang.org/x/net` (h2c, so gRPC proper can be
exercised over plain TCP). The schema layer carries no third-party dependency, and the
format is a published specification; what the canonical implementation is for
here is checking that what this emits is what protobuf reads, in both
directions, against a descriptor produced by this module's own projection. A
round trip through one codec says only that its two halves agree.

---

# 7. Database

A `Database` capability with `Query`, `QueryRow` and `Exec`, rows decoded
through a `Schema`, and streaming results as a `Stream` so a large result set
does not have to be held. Transactions are scoped resources: commit on success,
roll back on failure or interruption, which is `AcquireRelease` with an
exit-aware release and nothing new.

Prepared statements are the default, per the project's standards.

DONE, and one thing turned out better than planned and one worse.

Better: a row needed no description of its own. A row is a set of named values,
which is an object, so the universal representation carries one and
`FromDynamic` decodes it with the schema that would decode a request body.
There is no Source implementation for rows and no row-mapping vocabulary --
the dynamic bridge, built for a different reason, did the whole job.

Worse: `database/sql` scans into a top type and a driver hands one back,
because a driver cannot know what a column holds until it reads it. That is the
first genuine erasure boundary in this module, so it is confined to one file
that the architecture test names, with a second test that the exemption stays
where it says it is and stays used. Above that line everything is the universal
representation.

"Prepared statements by default" is read as: the port has no way to pass a value
except as an argument, so a statement built by concatenation cannot be expressed
through it. Statement caching is the driver's business, and a caching adapter is
something to add when measurement asks rather than before.

`Columns` and `Arguments` come from one schema so the two halves that must agree
cannot drift. An absent optional member binds as null **in its own position**,
because leaving it out would shift the arguments after it.

The rollback is verified against a transaction that records what was asked of
it, not against a database: whether an un-rolled-back transaction blocks the
next statement depends on the driver, the journal mode and the connection pool,
and the integration test passed with the rollback removed. That is the kind of
test that looks like evidence and is not.

The same recording transaction shows that the read deciding a write goes through
the transaction the write does -- the statements it kept are its own -- and
`examples/library`'s `Take` is that against sqlite. It is the reason `Querying`
and `Beginning` are two interfaces: a repository is written against the
operations and does not know which it is running on.

The erasure boundary is checked in both directions against a driver written for
the purpose, because sqlite produces four of the seven kinds and never exceeds
the contract. That driver produces all seven, and in one statement something no
driver is allowed to produce -- so the branch that makes the exemption
defensible, naming what arrived instead of guessing, is the one the test lands
on. Before it existed, `Scan` and the binding direction were at 40% and 33%.

---

## 7.1 DDL and migrations: researched, not built

Asked for: generate DDL for Postgres and MySQL/MariaDB from a description, with
the mapping an ORM needs. RESEARCHED. Nothing is built, and the findings are
here so the decision is on the record.

### What the reference implementations do

**Effect-TS generates no DDL.** In the pinned tree, `CREATE TABLE` appears only
in hand-written test fixtures; `@effect/sql-pg`'s migrator runs `.sql` files and
shells out to `pg_dump` for schema dumps. There is no projection from `Schema`
to DDL anywhere in it.

What it does have is the thing an ORM mapping actually needs, and it is not a
transformation: `unstable/schema/VariantSchema` turns one field set into six
related schemas -- `select`, `insert`, `update`, `json`, `jsonCreate`,
`jsonUpdate` -- with each field declaring which variants it appears in.
`Model.GeneratedByDb` appears in `select` and `json` only, so the insert shape
genuinely lacks the database-generated column. `SqlModel.makeRepository` then
takes `tableName`, `idColumn` and `softDeleteColumn` as plain strings, not
derived from anything.

**Drizzle runs the other direction.** `drizzle-zod` derives Zod schemas *from*
table definitions -- `createSelectSchema`, `createInsertSchema`,
`createUpdateSchema`. The table is the source of truth. The same trio of
variants, arrived at independently.

**The Zod-to-DDL packages that exist hit the predictable wall.** They emit
`CREATE TABLE`; the one that attempts diffing skips destructive operations by
default and emits primary-key changes as comments for a human.

### Why a direct Schema-to-DDL projection is wrong

DDL is **under-determined** by `Schema[A]`. The description cannot supply column
identity across renames, keys, indexes, foreign keys, defaults, generated
columns, collation, or the nullable-versus-optional distinction as a database
means it.

And rename detection is not a maturity problem. `drizzle-kit`, the most
developed tool in that ecosystem, cannot tell a rename from a drop-plus-add: it
asks interactively, the prompt has open bugs where no keypress advances it, its
programmatic API throws when a diff contains both a create and a delete of the
same kind, and choosing "renamed" emits the rename while dropping the
accompanying type change.

This module already learned that lesson elsewhere. Protobuf needed `Numbered`
because renaming is safe and renumbering is not. A table needs the same, for
the same reason.

Dialect differences are structural rather than cosmetic, so one rendering will
not do: Postgres has no `UNSIGNED`, MySQL's `BOOLEAN` is `TINYINT(1)`,
`SERIAL`/`IDENTITY` against `AUTO_INCREMENT`, `ENUM` is a separate `CREATE TYPE`
in Postgres, and only Postgres has transactional DDL -- which changes migration
*strategy* and not only syntax.

### The migration approach to take, and it is not diffing

The industry splits into two camps, and Atlas names them: **versioned**, a
script per change, against **declarative**, a desired state plus a diff against
the live database. Atlas is candid that declarative plans are
**non-deterministic**, because they depend on the state they find.

The approach recorded here is neither, and it is better than both: a migration
is a **declared function from the table declaration at one pinned version to the
declaration at the next**, monotonically versioned, carrying the mapping --
which column became which, where a new column's values come from -- as data
rather than as inference. Nothing is guessed, so a rename is exact rather than
interactive, and a plan is deterministic because it never consults the database
to decide *what* to do.

The prior art is good. Django's migrations are already close: a checked-in list
of `Operation` objects with a dependency DAG executed in topological order, and
`RenameField` an explicit operation rather than an inference -- untyped and
imperative, but the right shape. And Cambria (Litt et al., 2021) goes further
with composable bidirectional **lenses**, generating the TypeScript types and
the JSON Schema *from the lens definitions themselves*.

That last part is the refinement worth taking: write **version 1 and the lenses**
and derive versions 2..n, rather than writing each version's declaration and a
function between them and then having to check that the function really takes
one to the other. Derive, but **materialise** -- emit each derived version as a
checked-in artifact with a drift test, which is exactly what `schemagen`
already does for bindings.

### What Go can and cannot give this

Three limits to state plainly rather than discover later.

- **Not compile-time.** A table declaration is a *value* in Go, not a type, so
  the compiler cannot check that a migration accounts for every column of its
  target. Encoding columns in types would need a type parameter per column and
  would be unusable. What is available is **assembly-time totality**: a
  `Migration` value validates that it accounts for every column of the target
  and carries a fault if it does not, checked in a test and at start-up. That is
  the pattern already used for ambiguous routes, endpoint declarations and
  schema faults. Scala or Haskell could do better here; Go cannot, and the
  design should not pretend otherwise.
- **Inverses are not total.** Dropping a column loses data, so a backward lens
  can only fabricate a default. Down-migrations are therefore best-effort by
  nature, and the design should say so rather than promise reversibility.
- **A declared chain cannot reconcile drift.** Someone who alters production by
  hand leaves a database the chain does not describe -- the one thing
  declarative diffing does better. The answer is a cheap verification step:
  before applying N to N+1, assert the live database matches the declaration at
  N, and refuse rather than proceed. That keeps determinism and still notices.

### The shape, if it is built

A `Table` declaration referencing schemas for its column shapes and adding what
a table needs: declared column identity, keys, indexes, foreign keys, defaults,
generated columns. `select`/`insert`/`update` variants projected from it, which
is the ORM mapping and makes today's implicit one explicit -- `sql.Columns` and
`sql.Arguments` currently take the field name as the column name and the
declared order as the argument order, which is a mapping nobody wrote down.
Per-dialect rendering that **refuses** what a dialect cannot express rather than
approximating it, as every other projection here refuses rather than guesses.

## 7.1.1 The derived shapes, built

`schema/variant` is the first piece, and it is the piece everything else needs:
`Select`, `Create`, `Update`, and the two `...WithEntities` forms, each a
function from a `structure.Node` to a `structure.Node`.

Three decisions came out of building it.

**Two marks, not three.** `Identity` and `Computed` on a field, and an entity is
**derived** from having an identity rather than declared. A separate entity
marker could only ever agree with the identity or contradict it, and there is
nothing useful to say in the contradicting case -- so the prior art's third
marker is gone. The two compose to cover what Effect needs `GeneratedByApp` and
`GeneratedByDb` for: an identity the application supplies is `Identity` alone
and appears in a create shape, one the database generates is both and does not.

**An update shape is not a create shape with the identity removed.** Every field
it keeps becomes optional, because a change says what is changing and a field
nobody mentioned is a field nobody is changing. That is the whole difference,
and it is why they are two functions rather than one with a flag. The nested
forms are separate functions for the same reason: creating a root and creating a
whole aggregate are different requests, and a boolean at the call site would not
say which was meant.

**The marks do not survive the derivation.** A shape a caller supplies has no
identity to declare and nothing computed left in it, so carrying them through
would say something untrue -- and a projection reading them would make a key out
of a field that is no longer one.

Two tests were vacuous and neutering found both. The aggregate's own identity is
generated, so `Computed` dropped it first and the identity rule was never
exercised: it needed an entity whose identity is the application's. And after
derivation the root has no marked field left, so checking the root for surviving
marks proved nothing -- the kept identity on a nested entity is where one would
show.

One behaviour worth recording because it was measured rather than assumed: a
member a derived shape does not declare is **skipped, not refused**. So a client
that reads an order and posts the whole of it back to create another is not
refused over the identity it could not have known to omit, and the identity it
sent is dropped rather than carried through. That is this layer's ordinary
tolerance and the right answer here.

## 7.2 Prior art: a previous attempt at exactly this

`up2parts-aggregate-schema` (TypeScript, over Zod) is a working attempt at the
versioned-declaration idea, and it converges on the design above from a
different direction. It also stops short of DDL: it emits JSON Schema and
OpenAPI component artifacts to files, and there is no `CREATE TABLE` anywhere in
it. Three of its decisions are additions to what is recorded above, and one of
its difficulties is a warning.

### The aggregate, not the table, is the unit

`defineEntity` marks a nested schema as an entity with its own identity. So one
declaration describes an aggregate root *and* the entities under it, and
`getCreateSchema(version, withNestedEntities)` decides whether a derived shape
reaches into them.

That changes the DDL problem materially: an aggregate is **several tables with
foreign keys**, not one table, and the derived create shape for an aggregate is
not the same thing as the insert shape for its root table. Nothing recorded in
7.1 accounted for that, and a `Table`-per-declaration design would have got it
wrong.

### Identity is a fully-qualified name plus a version

A schema is identified by an FQDN and a version, not by a Go package path or a
struct name. That is what a registry needs, and it is what makes a declaration a
contract between services rather than a detail of one program -- the same reason
a protobuf service carries its fully-qualified name here.

### Three markers, arrived at independently

`markAsId`, `markAsComputed` and `defineEntity` are the whole extra vocabulary,
added for the create/update derivation rather than for DDL. They are almost
exactly the annotations DDL needs, and almost exactly Effect's
`Model.GeneratedByDb` and `Sensitive`. Three sources converging on the same
short list is the strongest evidence available that the list is right.

### Migrations: adjacent steps, composed

Each version carries a schema, an `upMigration` from the previous version and a
`downMigration` to it. `buildMigration` finds the direction by index and folds
the intervening functions; `buildAllMigrations` materialises the whole
version-by-version matrix.

So **N-1 functions are written and every one of the N-squared pairs is derived**
-- which is Cambria's lens composition, arrived at without it. It is also the
answer to the objection raised in 7.1 about writing the target twice: only the
steps are authored. Each version's declaration is still written out beside its
step there, so the Cambria refinement -- derive the declaration from the chain
and materialise it -- remains available and is still worth taking.

### The warning: where the type-level work ran out

`CreateType<Type, ComputedFieldPathArrays<...>, NestedEntityPathArrays<...>,
IncludeNested, IdFieldPathArrays<...>>` computes the derived shape by extracting
field *paths* at the type level. It carries a `@ts-expect-error`. The
machinery was at the edge of what TypeScript could check, in a language with
mapped types, conditional types and template literal types.

Go has none of those, so this approach is not merely harder here -- it is
unavailable. That is not a problem, because it is unnecessary: in this module a
description is **already data** (`structure.Node`), so deriving a create or
update shape is a pure function over a tree rather than a type-level
computation. Where a Go type per variant is wanted, `schemagen` already writes
the Go types a description implies, with a drift test. The escape hatch becomes
a generator.

The same applies to the migration matrix. `buildAllMigrations` builds it at
runtime behind erased signatures; in Go the N-squared compositions would be
**generated**: deterministic, checked-in, fully typed, gofmt-formatted -- which
is what this project's rule on generation permits, and N-squared mechanical
compositions of hand-written steps is what "demonstrated repetition" means. Each
step stays typed as `func(FromVersion) ToVersion`; only the registry that holds
steps of differing types needs a seam, and that is one named boundary of the
kind this module already has four of.

# 8. Implementation sequence

```text
1. schema core: nodes, values, combinators, JSON codec, JSON Schema projection  DONE
2. web core: Request, Response, Handler, net/http interoperability, Server  DONE
3. codecs, Endpoint, route matching and dispatch, middleware  DONE
4. OpenAPI generation  DONE
5. websocket  DONE
6. sql  DONE
7. amqp 0-9-1  DONE
8. amqp 1.0  DONE
9. grpc  DONE
```

Each step gets tests, an example and documentation before the next begins, on
the same expansion-and-consolidation cycle the runtime was built with.

Step 1 delivered: `Schema[A]` with scalars, lists, maps, nullables, structs with
optional fields, unions, refinements and recursion; a public `structure` package
a projection walks; a JSON codec over `encoding/json/jsontext`; the JSON Schema
2020-12 projection with shared components; `examples/catalog` as a complete
program; and the pair test above. Derivation and the remaining projections stay
deferred as 3.5 records.

Step 2 delivered: Request and Response over net/http's own types, Handler as a
description, an Adapter that interprets one as an http.Handler, and a server
whose lifetime is a scope. `examples/bookstore` is the program.

Two findings from building it are recorded where they matter rather than only
here. A defect in the effect a boundary interprets directly reaches no observer,
because the runtime emits its fiber events for forked fibers -- so the
boundary's report hook is not optional (4.2). And a shutdown that abandons
in-flight requests must report through the scope's closing cause rather than the
serve fiber's failure, because an interrupted program joins nothing (4.2).

Step 3 delivered: codecs per location over the same Schema that describes a
body, structural combination through Product, Endpoint separated from its
handler, a backtracking segment tree with ambiguity refused at assembly, and
middleware. `examples/bookstore` was rewritten onto it and its end-to-end tests
did not change, which is the evidence that the layer replaced the hand-written
switch rather than displacing the behaviour.

One correction: 4.4's check that a path's captures and its path parameters agree
holds in one direction only. A parameter the path does not capture is a mistake;
a captured segment nothing reads is ordinary, because a pattern often needs a
variable segment whose value the handler has no use for.

Step 4 delivered: an OpenAPI 3.1 document projected from Declarations, with
shared components, and validated by kin-openapi in both suites -- the
end-to-end one against the document the bookstore actually serves at
/openapi.json.

The validator earned its place immediately. It found that an operation must
declare every parameter of its path template, including a captured segment the
codecs do not read, so the projection supplies one; without that the emitted
document was invalid and no amount of reading it would have said so.

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
