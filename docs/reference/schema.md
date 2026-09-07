# Schema reference

`Schema[A]` describes the shape of `A` once, so that codecs, JSON Schema,
OpenAPI documents, protobuf descriptors and SQL row mapping are projections of
one description rather than several that have to be kept in agreement.

## Writing one

A schema is written, not derived. Go has neither Scala's implicit derivation nor
TypeScript's mapped types, so each field is declared with its wire name, its
shape, a getter and a setter:

```go
var BookSchema = schema.Struct[Book]("Book",
    schema.FieldOf("title", schema.Text(),
        func(book Book) string { return book.Title },
        func(book *Book, title string) { book.Title = title }),
    schema.FieldOf("authors", schema.List(schema.Text()),
        func(book Book) []string { return book.Authors },
        func(book *Book, authors []string) { book.Authors = authors }),
)
```

The setter takes a pointer and the getter a value, so a struct is built from its
zero value one field at a time. That avoids needing an N-argument constructor
for every arity, and it is how a Go program builds a struct anyway.

This is more to write than a struct tag. It is also checked by the compiler,
works when the wire shape differs from the Go shape, and needs no reflection.
Derivation that produces these values is a later addition and not a
replacement.

## Shapes

| Constructor | Describes |
|---|---|
| `Text`, `Formatted(format)` | a string, optionally refined for readers of a projection |
| `Int`, `Int64` | a whole number; `Int` rejects a value that does not fit |
| `Float64` | a finite number; NaN and the infinities are refused on encode |
| `Bool` | a boolean |
| `Bytes` | an opaque byte string, base64 in a text format |
| `Time` | an instant, RFC 3339 in a text format |
| `List(element)` | an ordered sequence; decodes to an empty slice, never nil |
| `Map(value)` | string-keyed values; encoding sorts the keys |
| `Nullable(inner)` | present and null, as a pointer -- not the same as an absent field |
| `Struct(name, fields...)` | a fixed set of named fields |
| `OneOf(name, variants...)` | a choice between named alternatives |
| `Deferred(resolve)` | a schema not built yet, for recursion |

`FieldOf` declares a required field and `OptionalFieldOf` one that may be
absent. An optional field's getter reports presence, because an empty string
that is meant to be sent is not the same as a field that is not there — and an
absent field is omitted rather than written as null.

## Sums

Go models a sum as an interface with an unexported marker and a concrete type
per alternative. `OneOf` describes one, with a `VariantOf` per alternative whose
narrowing function is the type assertion:

```go
var shapeSchema = schema.OneOf[Shape]("Shape",
    schema.VariantOf("circle", circleSchema,
        func(shape Shape) (Circle, bool) { circle, is := shape.(Circle); return circle, is },
        func(circle Circle) Shape { return circle }),
    schema.VariantOf("rectangle", rectangleSchema,
        func(shape Shape) (Rectangle, bool) { r, is := shape.(Rectangle); return r, is },
        func(rectangle Rectangle) Shape { return rectangle }),
)
```

The wire form names the chosen variant as the object's single member:

```json
{"circle": {"radius": 2}}
```

The alternative REST idiom — a discriminator field beside the variant's own
fields — is **not** available, and the reason is structural rather than a
preference. A value passes through a token stream in one pass, so a decoder that
met the fields before the discriminator would have to buffer the object or
rewind to know what it had been reading. Naming the variant as the key means the
selection always arrives first. The same objection applies to an envelope with
separate tag and payload members, whose order a producer is free to choose.

Variants are tried in declared order on encode, so a narrower variant belongs
before a wider one that would also match. A value no variant holds is reported
rather than written as an empty object.

On decode, an unknown variant is **refused** — unlike an unknown field, which is
skipped. There is no value to build without it, so tolerating one would produce
a zero value the document never named. Two variants named at once, or none, are
refused for the same reason.

## Refinement and domain types

`Transform` and `TransformOrFail` derive a schema for one type from a schema for
another, which is how a wire shape and a domain type stay separate:

```go
celsius := schema.TransformOrFail(schema.Float64(),
    func(value float64) (Celsius, error) {
        if value < -273.15 {
            return 0, errors.New("below absolute zero")
        }
        return Celsius(value), nil
    },
    func(value Celsius) (float64, error) { return float64(value), nil },
)
```

A refinement a schema cannot enforce should not look like one it does:
`Formatted("email")` is a hint carried into projections, not a validation.

## Encoding and decoding

```go
func EncodeJSON[A any](schema Schema[A], value A) ([]byte, error)
func EncodeJSONTo[A any](schema Schema[A], value A, to io.Writer) error
func DecodeJSON[A any](schema Schema[A], document []byte) (A, error)
func DecodeJSONFrom[A any](schema Schema[A], from io.Reader) (A, error)
```

A value does not pass through an intermediate tree. The schema drives a `Sink`
and pulls from a `Source`, and a format implements those two contracts once —
so JSON, form encoding, protobuf and SQL rows share one description with one
traversal each.

JSON uses `encoding/json/jsontext` from the standard library, whose token API is
exactly the shape this needs.

Encoding is deterministic: fields appear in declared order and map keys are
sorted, so a document can be compared in a test and cached by an intermediary.

## What a decoder refuses, and what it says

- A **missing required field** is refused, named by its path.
- A **mis-shapen value** is refused, naming both the path and what was wanted:
  `schema: expected a string, found a number at authors.items.1`.
- A **truncated document** says so, because that has a different cause and a
  different fix from a shape mistake.
- An **unknown field** is tolerated and skipped. A decoder that refused one
  could not read a document written by a newer version of its producer.

`PathOf(err)` returns the path, so a handler can tell a client which field it
rejected.

## Declaration mistakes

Building a schema neither panics nor returns an error, for the reason building
an effect does neither: a description is not the place where things go wrong.
Two fields with one name, a field with no name, or a field whose own schema is
unusable are reported by `Validate`, and by the first attempt to use the
schema.

```go
if err := schema.Validate(BookSchema); err != nil {
    log.Fatal(err) // at start-up, rather than on the first request
}
```

`Validate` cannot see through `Deferred`: at the moment an enclosing schema is
being built the deferred one does not exist yet, so asking would report a fault
that is not real. A mistake behind a `Deferred` surfaces on first use. That is
the cost of expressing recursion at all.

## Projections

`Structure()` returns the description a projection walks, and the `structure`
package is public because a projection is an extension point rather than a fixed
set — one to Avro, to a migration or to a form renderer is written the same way
as the built-in ones, with no privileged access.

`jsonschema.Project` produces JSON Schema 2020-12, the dialect OpenAPI 3.1 uses,
and `Render` declares it so a validator need not be told which one to apply.
A named struct or union becomes a component and is referred to wherever it
appears, so a type used by ten endpoints is described once — and a recursive type
terminates, because the second time a name is reached a pointer is emitted rather
than the shape again. `jsonschema.ProjectAll` shares one component set across
several structures, which is what an OpenAPI document needs.

A nullable shape projects as a choice between that shape and `null`, rather than
a widened `type` keyword, because the inner shape may be a reference and a
`$ref` has no `type` to widen. A union projects as a `oneOf` of single-member
objects, because that is what the codec writes — a document describing the unwrapped variant would describe
something this module never produces. That agreement is measured rather than
asserted: the tests compile the emitted document with an external JSON Schema
validator and check that it accepts every document the codec writes and refuses
every one the codec refuses, including the union's three refusals. The validator
is a test dependency; nothing in the module needs it.
