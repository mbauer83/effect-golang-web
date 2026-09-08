# WebSocket reference

A conversation over an upgraded connection. It follows the shape every
transport here follows, so learning one teaches the others:

```text
a connection is a scoped resource
a conversation is a Stream in and a sink out
messages are encoded by a Schema
cancellation closes the connection through scope closure
```

The library underneath is [`github.com/coder/websocket`](https://github.com/coder/websocket):
context-aware throughout, no goroutine per connection of its own, and
maintained. It stays inside this package, so nothing that imports the HTTP core
acquires it.

## Accepting

```go
web.Upgrading("/tally", "Keep a running tally",
    websocket.Accept(boundary, converse, websocket.Settings{}))
```

`Accept` takes the **boundary** rather than returning a value, because a
conversation answers with no response: the exchange continues after the
upgrade, so there is nothing for a `Handler` to return, and the runtime, the
environment and the reporting a handler needs are the boundary's. That is what
`Adapter.Interpret` is for, and a transport that takes over the connection is
the only thing that needs it.

`web.Upgrading` mounts it as a route, so a websocket endpoint is dispatched and
documented by the same tree as everything else. Its declaration says what is
true of the HTTP part — a `GET` that answers 101 and carries no entity — and
stops there, because what happens after the upgrade is a different protocol and
not something an OpenAPI document can describe.

`Settings{}` accepts a conversation from the serving host only, which is what a
browser enforces. `Origins: []string{"*"}` accepts any, and is a decision worth
writing down rather than defaulting into.

## Dialling

```go
effect.Scoped(func(scope effect.Scope) effect.Effect[R, websocket.Fault, A] {
    return websocket.Dial[R](scope, "ws://host/tally").
        FlatMap(func(socket websocket.Socket) ... )
})
```

Both sides are here and both get the same `Socket`: a conversation is
symmetrical once it has begun, and the only asymmetry — who accepted and who
dialled — is over by the time there is a socket at all. A library that only did
the server half would leave every test of it reaching for something else.

## Saying things

| | |
|---|---|
| `Send(socket, message)` | one message, whole |
| `SendText(socket, text)` | the common case |
| `SendValue(socket, schema, value)` | encoded through its schema, as one text message |
| `Receive(socket)` | the next message |
| `ReceiveValue(socket, schema)` | the next message, decoded |
| `Inbound(socket)` | every message, as a `Stream` |
| `Values(socket, schema)` | every message decoded, as a `Stream` |
| `Close(socket, reason)` | goodbye, with a reason |

The same `Schema` that describes a request body describes a message, so a
conversation and an endpoint agree about a shape without anyone writing it
twice. `SendValue` encodes before it writes, so a value the schema refuses is
reported rather than sent as half a message the peer would have to make sense
of.

`Receive`/`ReceiveValue` are for an exchange that is a question and an answer;
`Inbound`/`Values` are for one that is a conversation. A stream is what an
inbound side *is*: pulled one message at a time, ended by the peer rather than
by the reader, and finished when the scope closes. Nothing buffers and no
goroutine is needed.

A message `Values` cannot decode **fails the stream**. A conversation is
stateful, so a peer that said something this side cannot read has said
something about the whole exchange rather than about one message. A conversation
that would rather skip it reads `Inbound` and decodes each message itself.

## Lifetime

The socket is acquired in a scope, so **closing the scope closes the socket** —
a conversation cannot outlive the request that started it, and nothing has to
remember to hang up. Using a socket after its scope has closed fails, which is
what a scoped resource promises.

The release **says goodbye first**. A peer that is told the conversation is over
ends its own inbound stream and reports nothing; a peer whose connection simply
vanishes has to treat that as the failure it usually is. Closing abruptly would
make every normal departure look like a dropped connection and fill an
operator's log with them. If the polite close cannot be sent — the peer said
goodbye first, or the write would have blocked — the connection goes anyway:
once it has gone the release owes nothing, so `CloseNow`'s complaint that it was
already closed is the outcome that was wanted rather than a fault.

Correspondingly, `Inbound` **ends** on a normal closure or a cancelled read and
**fails** on anything else. A peer saying it is finished is not a failure; a
peer that vanished is.

## The example

`examples/tally` is the whole shape in one screen: a client sends changes, the
server answers with the running total, the total is a `Ref` because several
conversations share it, and the messages are described by schemas. Its answers
depend on everything said before them, which is what makes it a conversation
rather than a request and a response.
