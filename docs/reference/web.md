# Web reference

`net/http` is the spine. `Request` and `Response` wrap its types, an existing
`http.Handler` becomes a `Handler`, and a `Handler` becomes an `http.Handler`.
Nothing here replaces the ecosystem; it makes it composable.

## A handler is a description

```go
type Handler[R, E any] func(Request) effect.Effect[R, E, Response]
```

Nothing is handled until the runtime interprets it, which is what makes a
handler retryable, raceable and timeoutable like any other effect. Cancellation
reaches it through the request's context, so a handler needs no cancellation
mechanism of its own.

A handler's failure has the handler's own type and **never a status**. The
status is decided at the boundary, once — which is what leaves the same handler
usable behind a different contract:

```go
type Fault struct { Kind FaultKind; Err error }

func StatusFor(fault Fault) web.Response {
    switch fault.Kind {
    case NotFound:     return web.Empty(http.StatusNotFound)
    case Unacceptable: return web.Text(http.StatusBadRequest, fault.Error())
    default:           return web.Empty(http.StatusBadRequest)
    }
}
```

## The request

`Request` wraps `*http.Request`, so nothing is lost and `Underlying()` always
reaches the original. What it adds is the path captures a route matched, which
`http.Request` has no place for. `WithCaptures` returns a new request rather
than altering the one it was given.

Reading the entity is an effect, because it is I/O that can fail and can be
cancelled:

```go
func Body[R any](request Request) effect.Effect[R, Fault, []byte]
```

It is a package function because the environment it runs in is the caller's, not
the request's. The body is read once, as `net/http`'s is.

## The response

| Constructor | Entity |
|---|---|
| `Empty(status)` | none |
| `Text(status, body)` | plain text, length declared |
| `Bytes(status, contentType, body)` | bytes in hand, length declared |
| `JSON(status, schema, value)` | encoded through the schema, length declared |
| `Streaming(status, contentType, write)` | produced as it is written, no length |
| `Delegate(handler)` | whatever an existing `http.Handler` writes |

`JSON` encodes before anything is written and returns an error if it cannot,
because a status cannot be taken back once it has gone out — the alternative is
a 200 with a truncated body. `Streaming` accepts that trade deliberately for the
case where materialising the entity first is the wrong one; a failure part-way
through is reported to the boundary, which records it.

`WithStatus` and `WithHeader` return a new response and leave the original
alone, which is what lets middleware wrap a response it does not own.

`Delegate` buffers nothing: the handler runs when the response is written, so a
streaming, flushing or hijacking handler keeps working exactly as it did.
Headers set on the response are applied first, which is how a wrapper adds one
without taking the handler apart.

## The boundary

```go
boundary, err := web.NewAdapter(runtime, environment, StatusFor)
http.Handle("/", boundary.Handler(myHandler))
```

An `Adapter` owns the three things a handler cannot supply for itself: the
runtime that interprets it, the environment it runs in, and how its failure
becomes a status. A missing runtime or failure mapping is refused here, at
composition time, rather than on the first request.

What each outcome answers with:

| Outcome | Answer |
|---|---|
| success | the handler's response |
| typed failure | `OnFailure(failure)` |
| defect | 500, no detail, and reported |
| interruption | 503, and not reported |

A defect outranks an interruption, which is the runtime's own precedence rather
than a second one invented here. A defect's text is never sent to a client: by
definition it is something the application did not account for.

`WithDefectResponse` replaces the last two rows; `WithReport` replaces where a
fault the response cannot express is recorded.

The report is not optional, and this is why. **MEASURED:** the runtime emits its
fiber events for *forked* fibers, so a defect in the effect a boundary
interprets directly — which is what every request is — reaches no observer on
its own. A write failure could not reach one in any case: it happens after the
status has gone, and can only be noted. The default writes a line to standard
error, deliberately not through a `Logger`, because a boundary that could not
send a response is a poor moment to depend on an adapter that may be the thing
that failed.

## The server

A server's lifetime is a scope:

```go
effect.Scoped(func(scope effect.Scope) effect.Effect[R, web.Fault, effect.Unit] {
    return web.Serve[R](scope, ":8080", boundary.Handler(handler)).
        FlatMap(web.Await[R])
})
```

Closing the scope stops the listener and waits for the requests already in
flight. Graceful shutdown is therefore not a separate mechanism to remember: it
is what closing a scope already means, and a caller who forgets cannot leak a
listener because the scope holds it.

`Await` completes when the server stops and fails if it stopped for a reason
other than being shut down — it is what a program that exists to serve waits on.
`Server.Address()` reports where it actually listens, which is what a caller who
asked for port zero needs.

The accept loop is a fiber the scope owns, so it is visible to the runtime's
observer like any other work rather than a goroutine nobody accounts for.
`net/http`'s `Serve` blocks and takes no context, so there is one goroutine
inside that fiber's leaf effect — the ordinary adapter for a blocking library
call, bounded by the effect that owns it.

`ServeWith` takes `Settings`:

| Field | Meaning |
|---|---|
| `Address` | where to listen, as `net.Listen` takes it |
| `Listener` | a socket the caller already has; takes precedence |
| `ReadHeaderTimeout` | defaults to 10s — `net/http` has none, and a client that opens a connection and stops can otherwise hold one forever |
| `ReadTimeout`, `WriteTimeout`, `IdleTimeout` | passed through |
| `Grace` | how long a shutdown waits for in-flight requests; zero waits as long as they take |

A grace period that runs out means requests were abandoned, and that is reported
through the **scope's closing cause** rather than as the serve loop's failure.
The reason is worth knowing: a forked fiber's outcome is observed by joining it,
and a program interrupted while waiting joins nothing — so the one outcome that
must never be lost would be exactly the one that is. A scope composes its
finalizers' faults into the closing cause whatever happened to the body.

## What is not here yet

Routing, path and query codecs, typed endpoints, middleware and OpenAPI
generation are the next step. Until then dispatch is a switch on the method and
the path, as `examples/bookstore` shows — which is the clearest statement of
what routing will replace.
