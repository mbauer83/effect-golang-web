# OpenAPI reference

A published document is a projection of the same declarations that dispatch a
request. Nothing is written twice, so nothing can drift.

```go
surface, err := web.NewRoutes(listBooks, addBook, findBook)

document := openapi.Describe(
    openapi.Info{Title: "Bookstore", Version: "1.0.0"},
    surface.Declarations(),
    openapi.Server{URL: "https://books.example"},
)
rendered, err := document.Render()
```

## The version is 3.1

OpenAPI 3.1's schemas *are* JSON Schema 2020-12, which is what the schema layer
already projects. One projection serves both, and nothing has to be translated
into an older dialect that says almost the same thing with different keywords.

## What it produces

- **Components.** Every shape goes through one projection, so a type used by ten
  operations becomes one component under `#/components/schemas` that all ten
  refer to. That is the reason the schema layer exposes its structure at all.
- **Parameters**, with the location the codec declared and the schema that reads
  them.
- **A request body** when a codec reads the entity; it is required, because a
  codec that read an optional body would be describing two shapes and would say
  so itself.
- **Responses**, ordered by status: the success the endpoint declared, and every
  status `Failing` documented. A description is required of each, so one is
  supplied from the status text when the endpoint said nothing.
- **An operation id**, derived from the method and the path — `GET /books/{title}`
  is `getBooksByTitle` — so that one exists at all and is stable whoever
  generates the document.

Paths appear in declared order and components in sorted order, so the same
routes always produce the same bytes: an author's order is more use to a reader
than an alphabetical one, and a stable order is what makes a document diffable.

## Two places the projection has to say more than the route did

A **captured segment nothing reads** is still declared. The specification
requires an operation to declare all of its path template's parameters, and a
route may legitimately capture a segment its handler has no use for. Leaving it
out makes the document invalid — which the parser says plainly, and which is how
this was found.

A **wildcard** has no counterpart in the specification. `/files/{path...}`
becomes `/files/{path}`: describing it as an ordinary parameter is the closest
honest thing, and it is why a wildcard route's document says less than the route
knows.

## An entity with no shape

`ReturnsRaw(status, mediaType)` answers with bytes this program did not build
from a value — a published contract, a file, an image. The document names the
media type and says nothing about the shape, which is exactly what a content
entry with no schema means.

## Validity is measured

The emitted document is loaded and validated by
[kin-openapi](https://github.com/getkin/kin-openapi), a parser that has never
seen this module, in both the unit and the end-to-end suites — the second
against the document actually served over a socket. It is a test dependency;
nothing in the module needs it.

That check earns its place. It found the missing path template parameter above,
and removing any of the projection's guarantees — an undescribed response, a
component pointed at `$defs` instead of `#/components/schemas`, two operations
on one path emitted as two paths — makes it fail.
