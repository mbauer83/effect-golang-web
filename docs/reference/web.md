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

## Reading a request

A request's parts decode independently, each described by the same `Schema` that
describes a field of a body — so a path segment, a query parameter and a header
need no description of their own:

| Codec | Reads |
|---|---|
| `PathParam(name, schema)` | a captured path segment; always required |
| `QueryParam(name, schema)` | a required query parameter |
| `OptionalQueryParam(name, schema)` | one that may be absent, as a pointer |
| `HeaderParam(name, schema)` | a required header |
| `OptionalHeaderParam(name, schema)` | one that may be absent |
| `Entity(schema)` | the JSON body, decoded straight from its reader |
| `Nothing()` | nothing |

`codec.Documented(doc)` attaches prose for the published document, to a codec
that reads exactly one parameter.

Absent and empty are different: `?shelf=` carries an empty value and no `shelf`
at all carries none, and an optional codec tells them apart.

Two codecs combine into one that keeps both parts:

```go
codec := web.Convert(
    web.Both(web.QueryParam("shelf", schema.Text()), web.QueryParam("page", schema.Int())),
    func(parts effect.Product[string, int]) (Query, error) {
        return Query{Shelf: parts.First, Page: parts.Second}, nil
    },
)
```

The result is a `Product` because no information may be discarded and Go has no
type-level record to widen — the same structural composition the runtime uses
for environments, for the same reason. `Both` is a package function because a
method cannot grow the type parameters its own result needs.

Two codecs that both read the entity, or that read the same parameter twice, are
a declaration mistake reported by `ValidateCodec`.

## Endpoints

An endpoint declares what a route accepts and returns; its handler is separate.
That is what makes three things possible from one value — dispatch, a published
document, and a client — where a route carrying only a function could give none
of them.

```go
web.Handle(
    web.GET("/books/{title}",
        web.PathParam("title", schema.Text()).Documented("the title to look for"),
        web.Returns(http.StatusOK, BookSchema)).
        Summary("Find a book by title").
        Failing(http.StatusNotFound, "no book with that title is held"),
    func(title string) storeEffect[Book] { ... },
)
```

The handler takes the decoded input and returns the **output value**, not a
response: the endpoint already says how that value is encoded and with what
status. `Returns(status, schema)` encodes through a schema; `ReturnsNothing(status)`
sends none. `Failing` documents a status the boundary's mapping will produce —
it does not perform the mapping.

A request the codecs refuse never reaches the handler and never becomes the
application's failure. That is what the failure channel is for: a malformed
request is the transport's business, and the handler's `E` stays about the
application.

`ValidateEndpoint` reports a declaration mistake, including a path parameter the
path does not capture — a mistyped name would otherwise be a rejection on every
request, found in production. The other direction is allowed: a pattern often
needs a variable segment the handler has no use for.

## Calling a server

```go
client := web.Dial(http.DefaultClient, "http://host:8080")

web.Fetch[R](client, "GET", "/books", web.Requesting{})        // Effect[R, Fault, Received]
web.Call[R](client, FindBook, web.Requesting{                  // Effect[R, Fault, Book]
    Path: map[string]string{"title": "Zionomicon"},
})
sending, err := web.Carrying(web.Requesting{}, BookSchema, book)
```

`Dial` takes the `http.Client` rather than replacing it. TLS, proxies, pooling,
redirects, timeouts and HTTP/2 stay where they already work, which is the same
division [`grpc.Dial`](grpc.md) makes: how a deployment reaches a peer is not a
property of a call.

**`Fetch` reads the entity and closes the body inside the effect**, so there is
no open response for a caller to forget. Cancellation, retry and observation
come from the interpretation and not from here — the context is the runtime's,
so `Retry` and a `Schedule` compose over a call as they do over anything.

**`Call` takes the endpoint and needs nothing the declaration already says.**
The method, the path pattern, the status and the response shape all come from
the same value the server dispatches with, and the response is decoded through
the very schema it was encoded through. `Requesting.Path` fills the captured
segments by name; a capture with nothing to fill it is refused *before anything
is sent*, because the alternative is sending the literal `{title}`, getting a
404, and looking at the wrong end of your own mistake.

A status the endpoint did not declare becomes a `Fault` carrying a `Refusal`
with the status and the entity, reachable with `errors.As`. `Failing` documents
a status on the serving side; on this side it is the answer that arrived instead
of the one promised, and the body usually says why.

**The request side is explicit, and that is a real limit.** A `Codec` reads a
request into an `In`, and reading is not invertible — `Convert` takes one
function, so a struct a handler received cannot be turned back into the parts it
came from. So a caller fills the captures, the query and the entity itself;
what the endpoint contributes is everything that has exactly one answer. Making
codecs invertible would let `Call` take an `In` directly, and it would mean an
inverse on every `Convert` — worth doing when a caller wants it, not before.

## Matching

```text
/books                a literal
/books/{title}        one captured segment
/files/{path...}      a wildcard capturing the rest, only at the end
```

Routes compile into a tree over segments. A literal is tried before a capture
and a capture before a wildcard, so the most specific pattern that can match
does — and the walk backtracks, so a literal that matches one segment but leads
nowhere does not shadow a capture that would have matched the whole path. That
is the classic router bug this tree exists to avoid, and it is tested.

A path that matched with the wrong method answers **405 with an `Allow` header**,
sorted, rather than 404. Backtracking applies here too: a literal branch that
matches the path but not the method does not stop a capture branch that serves
it.

A trailing slash is part of the pattern rather than something to normalise away:
`/books` and `/books/` are different paths, and a router that quietly redirected
between them would be guessing.

**Ambiguity is a construction error.** Two routes that could match the same
request, or two routes capturing one segment under different names, are refused
when the surface is assembled — the second because they share one node in the
tree, so one of the two names would silently never be bound.

```go
surface, err := web.NewRoutes(listBooks, addBook, findBook)
```

`NewRoutesRejecting` supplies the surface's own answer to a request the codecs
refused, because a client meets one API and not a collection of separately
worded ones. `Declarations()` returns what the routes say about themselves, in
declared order, which is what a published document is projected from.

## Wrapping every route

```go
type Matched[R, E any] func(Declaration, Handler[R, E]) Handler[R, E]

surface, err := web.NewRoutes(routes...)
surface = surface.Wrapping(observing)     // one setting, whole surface
```

`Middleware` wraps the surface's handler, and by then the only thing left of
the route is the path the client asked for. That is enough for authentication
or a rate limit and **not** enough for anything that has to *name* the route: a
concrete path is an unbounded value, so a span name or a metric label made from
one becomes a series per request.

`Matched` is the wrapper that is told which route it is wrapping. `Wrapping`
applies one to every route, giving each its own `Declaration`, and returns the
surface rebuilt — the same declarations, the same precedence, the same
rejections.

**It is a setting and not a convention.** A concern applied at every call site
is one that can be forgotten at one call site, and nothing would say so; a
concern applied to the assembled surface covers the route somebody added this
morning. Turning it off is not applying it, which a caller can decide from a
flag at start-up. `Wrapping(nil)` and a zero `Routes` are both the surface
itself.

A wrapper sees the route's whole work — the codecs as well as the handler,
because it wraps what dispatch calls. That is the useful boundary: a route
whose response is expensive to encode is expensive to serve whatever its
handler cost, and a request the codecs refuse never reaches the handler at all.

It cannot fail. The patterns are the ones that already assembled and a wrapper
does not change them, so there is nothing left to refuse.

## Middleware

```go
type Middleware[R, E any] func(Handler[R, E]) Handler[R, E]

handler := web.Wrap(surface.Handler(), authenticating, logging)
```

Applied outermost first: the first given sees the request first and the response
last, which is the order the list reads in. A nil entry is skipped. Because a
handler is a description, middleware composes with retries, races and timeouts
rather than sitting outside them. `Transform` is the narrower form for a wrapper
that only needs to see the response.

## Deliberately absent

**A response read as a stream.** `Fetch` reads the entity whole. Streaming one
needs a scope to own the body and a `Stream` to pull it, which is a shape worth
settling against a caller rather than guessing at.

**Invertible request codecs**, for the reason the calling section gives.

**Publisher-side content negotiation.** One media type per entity, declared.
A route that answered three depending on `Accept` would need three schemas and
three descriptions, and nothing has asked for it.

[OpenAPI generation](openapi.md), [WebSockets](websocket.md),
[AMQP 0-9-1](amqp091.md), [AMQP 1.0](amqp10.md) and [gRPC](grpc.md) are their
own references.
