# effect-golang-web

Typed, composable web and integration capabilities on the
[effect-golang](https://github.com/mbauer83/effect-golang) runtime.

Nothing here is finished yet. What exists, and what is planned, is below.

## Status

| Area | State |
|---|---|
| [Schema](docs/reference/schema.md) | usable: shapes, sums (both taggings), constraints, formats, JSON, JSON Schema, descriptions with no Go type, generation from a description |
| [HTTP core: request, response, handler, server](docs/reference/web.md) | usable |
| [Routing: matching, dispatch, middleware](docs/reference/web.md) | usable |
| [Typed endpoints](docs/reference/web.md) | usable |
| [OpenAPI generation](docs/reference/openapi.md) | usable |
| [WebSockets](docs/reference/websocket.md) | usable |
| [SQL](docs/reference/sql.md) | usable |
| [AMQP 0-9-1](docs/reference/amqp091.md) | usable; the adapter itself unverified against a broker |
| [AMQP 1.0](docs/reference/amqp10.md) | usable; the adapter itself unverified against a broker |
| gRPC and protobuf | planned |

The [architecture plan](architecture-plan.md) records the design decisions,
including which underlying library was chosen for each concern and what was
rejected.

## Layout

```text
schema/                     Schema[A]: shapes, codecs, refinement
  schema/structure/         the description a projection walks
  schema/dynamic/           the value a description carries when there is no Go type
  schema/jsonschema/        the JSON Schema 2020-12 projection
schemagen/                  writes the Go types a description implies
web/                        Request, Response, Handler, codecs, routes, Server
openapi/                    the OpenAPI 3.1 projection of a surface
websocket/                  a conversation over an upgraded connection
sql/                        statements, rows decoded by a Schema, transactions
amqp091/                    messages over AMQP 0-9-1, acknowledged explicitly
  amqp091/inprocess/        a broker that runs inside the test that uses it
amqp10/                     messages over AMQP 1.0, settled by disposition
  amqp10/inprocess/         the same, for a protocol with four outcomes
examples/bookstore/         a complete HTTP program on the web core
examples/tally/             a websocket conversation, with shared state
examples/library/           a repository over the database port, driver-free
examples/dispatch/          a producer and a consumer over the broker port
examples/consign/           the four dispositions AMQP 1.0 settles a message by
examples/catalog/           one schema three ways, sequenced in direct style
examples/inventory/         Go types generated from a description
examples/cmd/webdemo/       the examples as a runnable command
test/unit/                  behaviour of the public API
test/acceptance/            the example programs, end to end
docs/                       reference, how-to, explanation
architecture-plan.md        design decisions and the implementation sequence
```

The module root holds no source. Dependencies point one way: `schema` knows
nothing about HTTP, and no transport knows about another — so a caller who wants
only the HTTP core does not acquire an AMQP dependency.

## Underlying libraries

Chosen on maintenance, protocol coverage and whether they compose with
`net/http`, which is this module's spine. The reasoning and the rejected
alternatives are in the [architecture plan](architecture-plan.md).

| Concern | Choice |
|---|---|
| HTTP server and client | `net/http` |
| Routing | our own typed matcher |
| JSON syntax | `encoding/json/jsontext` (standard library) |
| WebSocket | `github.com/coder/websocket` |
| AMQP 0-9-1 | `github.com/rabbitmq/amqp091-go` |
| AMQP 1.0 | `github.com/Azure/go-amqp` (a different protocol, a separate package) |
| protobuf | `google.golang.org/protobuf` |
| gRPC transport | pluggable; `connectrpc.com/connect` as the reference |
| SQL | `database/sql` port and adapter; `modernc.org/sqlite` in tests |
| OpenAPI | emitted from our own model; `kin-openapi` in tests only |
| JSON Schema validation | `santhosh-tekuri/jsonschema/v6`, in tests only |

No third-party dependency is needed to use the HTTP core, and a transport's
dependency stays inside that transport's package.

## Running the examples

```sh
go run ./examples/cmd/webdemo
```

`examples/bookstore` serves a small collection over JSON on a port the operating
system chooses, and `examples/catalog` loads a catalogue document, writes it back normalised, and
publishes the JSON Schema contract that says what it accepts — three uses of one
description. `test/acceptance` composes the same program, so the example and its
test cannot drift apart.

## Building

`effect-golang` is not published yet, so `go.mod` resolves it from the working
copy beside this one. Replace that directive with a version requirement once it
is tagged.

Go 1.27 is required, by the runtime and by `encoding/json/v2`.
