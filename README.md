# effect-golang-web

Typed, composable web and integration capabilities on the
[effect-golang](https://github.com/mbauer83/effect-golang) runtime.

Nothing here is finished yet. What exists, and what is planned, is below.

## Status

| Area | State |
|---|---|
| [HTTP core: request, response, handler, server](docs/reference/web.md) | usable |
| [Routing: matching, dispatch, middleware](docs/reference/web.md) | usable |
| [Typed endpoints](docs/reference/web.md) | usable |
| [HTTP client](docs/reference/web.md) | usable: a request/response client, and a call typed by the endpoint's declared response |
| [OpenAPI generation](docs/reference/openapi.md) | usable |
| [WebSockets](docs/reference/websocket.md) | usable |
| [AMQP 0-9-1](docs/reference/amqp091.md) | usable; the adapter is verified against a broker in CI only |
| [AMQP 1.0](docs/reference/amqp10.md) | usable; the adapter itself unverified against a broker |
| [gRPC](docs/reference/grpc.md) | usable: unary procedures, Connect transport, projected contract |

Descriptions are [effect-golang-schema](https://github.com/mbauer83/effect-golang-schema);
tables and migrations are [effect-golang-sql](https://github.com/mbauer83/effect-golang-sql).
This module is the transports, and depends on both.

The [architecture plan](architecture-plan.md) records the design decisions,
including which underlying library was chosen for each concern and what was
rejected.

## Layout

```text
web/                        Request, Response, Handler, codecs, routes, Server, Client
openapi/                    the OpenAPI 3.1 projection of a surface
websocket/                  a conversation over an upgraded connection
amqp091/                    messages over AMQP 0-9-1, acknowledged explicitly
  amqp091/inprocess/        a broker that runs inside the test that uses it
amqp10/                     messages over AMQP 1.0, settled by disposition
  amqp10/inprocess/         the same, for a protocol with four outcomes
grpc/                       unary procedures over the gRPC wire protocol
examples/bookstore/         a complete HTTP program on the web core
examples/tally/             a websocket conversation, with shared state
examples/dispatch/          a producer and a consumer over the broker port
examples/consign/           the four dispositions AMQP 1.0 settles a message by
examples/quoting/           a gRPC service, and the .proto file it implies
examples/cmd/webdemo/       the examples as a runnable command
test/unit/                  behaviour of the public API
test/acceptance/            the example programs, end to end
test/architecture/          the claims about this module's shape
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
| gRPC transport | pluggable behind a port; `connectrpc.com/connect` is the one here |
| protobuf projection | effect-golang-schema; the reference implementation in tests only |
| OpenAPI | emitted from our own model; `kin-openapi` in tests only |
| h2c, for gRPC over plain TCP | `golang.org/x/net`, in tests only |

No third-party dependency is needed to use the HTTP core, and a transport's
dependency stays inside that transport's package.

## Running the examples

```sh
go run ./examples/cmd/webdemo
```

`examples/bookstore` serves a small collection over JSON on a port the operating
system chooses; `examples/quoting` answers a gRPC procedure and prints the
`.proto` file it implies; `examples/dispatch` and `examples/consign` are a
producer and a consumer over each AMQP protocol. `test/acceptance` composes the
same programs, so an example and its test cannot drift apart.

The description examples are in
[effect-golang-schema](https://github.com/mbauer83/effect-golang-schema)'s own
command, and the database ones in
[effect-golang-sql](https://github.com/mbauer83/effect-golang-sql)'s: each
module demonstrates what it is responsible for.

## Development

`go.mod` requires the runtime and the descriptions by version, so what a
consumer resolves is what this module was built against. Working on several at
once is a workspace's job:

```text
workspace/
  go.work                   where the modules being worked on are
  effect-golang/            the runtime
  effect-golang-schema/     descriptions
  effect-golang-sql/        tables and migrations, on those
  effect-golang-web/        this module, on the first two
```

```sh
cd workspace
go work init ./effect-golang ./effect-golang-schema ./effect-golang-sql ./effect-golang-web
```

The `go.work` file is not checked in to any of them: it belongs to whoever has
several checked out at once, which is why it lives above all four. A `replace`
cannot do this job. It is ignored by anything that depends on the module
carrying it, so it says nothing to a consumer and only ever describes one
person's layout -- and it hides the requirement a consumer will actually
resolve.

The four modules are versioned together and tagged in dependency order:
[RELEASING.md](https://github.com/mbauer83/effect-golang/blob/main/RELEASING.md).
