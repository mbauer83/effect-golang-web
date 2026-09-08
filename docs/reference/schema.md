# Schema reference

`Schema[A]` describes the shape of `A` once, so that codecs, JSON Schema,
OpenAPI documents, protobuf descriptors and SQL row mapping are projections of
one description rather than several that have to be kept in agreement.

## Writing one

Go has neither Scala's implicit derivation nor TypeScript's mapped types, so a
schema declares each field with its wire name, its shape, a getter and a setter:

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

It is also what the generator writes for you — see [Deriving one](#deriving-one)
below. A hand-written schema stays the honest path: everything the generator
emits is one of these calls, so the two are interchangeable and there is one
thing to learn rather than two.

## Describing a shape with no Go type

`Schema[A]` moves a Go value in and out of a format. A description written
before the type it will become exists — or loaded from elsewhere, or read back
out of another schema — has no `A`, and it is still worth validating,
transcoding, inspecting and composing.

It is the same vocabulary, minus the accessors:

```go
var Book = schema.Record("Book",
    schema.DocumentedMember("what the book is called",
        schema.MemberOf("title", schema.MinLength(schema.Text(), 1))),
    schema.MemberOf("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000)),
    schema.OptionalMemberOf("subtitle", schema.Text()),
    schema.MemberOf("id", schema.UUID()),
)   // Schema[dynamic.Value]
```

`Record` is `Struct` without the getters and setters — which are the only part
of a field declaration that needs the Go type, so leaving them out is exactly
the difference between *describing* a shape and *binding* one. `Choice` and
`AlternativeOf` do the same for a union. `Dynamic(node)` is the general door: a
typed schema's `Structure()` passed through it is usable without its type.

Everything else is unchanged, because a `Schema` never cared what `A` was:

```go
schema.Validate(Book)              // reports a mistake in it
schema.DecodeJSON(Book, document)  // validates and yields a dynamic.Value
schema.EncodeJSON(Book, value)     // writes it back
schema.List(Book)                  // composes with typed schemas
jsonschema.Project(Book.Structure())  // publishes exactly as a typed one does
```

`dynamic.Value` is a sealed sum of nine cases — the twelve calls of a `Sink`
seen from the other side, which is why a format needs to learn nothing new to
carry one. A struct, a string-keyed mapping and a union all appear as
`dynamic.Object`, because that is what all three are on the wire; which one a
value is meant to be is the description's business, not the value's.

`ToDynamic` and `FromDynamic` cross between a typed value and a described one,
so the two ways of using this package meet.

### What a description can and cannot enforce

It enforces **what it records**: a bound, a length, a pattern, an item count,
optionality, and the shape itself. That is what recording constraints in the
description was for — the rules survive without the Go type that stated them,
and the typed and described paths are tested to reach the same verdict.

It cannot enforce a rule that could only be code. `Email()` and `URI()` parse,
and a parse is not a keyword, so a description alone annotates and no more.
`UUID()`, `URL()` and `Hostname()` recorded their expressions, so those *are*
enforced. Saying so plainly beats a described path that silently admits what the
typed path refuses.

## Deriving one

A struct's fields and a schema for it say the same thing twice, and the second
copy is the one that rots. `cmd/schemagen` writes it:

```go
//schema:generate
type Book struct {
    Title   string   `json:"title"`
    Authors []string `json:"authors"`
    // Pages is how many pages the book has, and there is at least one.
    Pages int `json:"pages" schema:"use=pagesSchema"`
}
```

```go
//go:generate go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .
```

Derivation runs **one way only**. A Go struct cannot be derived from a schema,
because Go cannot compute a type from a value; the struct is the source of truth
and the schema follows it.

It is a generator rather than reflection because the module's whole claim is to
be fully typed: reflection would discover the structure at run time and hand
back the erasure the rest of this design refuses. Generated code is
deterministic, formatted, checked in, and regenerated by a test that compares it
with the structs it came from, so it cannot drift.

| In the struct | In the schema |
|---|---|
| `json:"name"` | the wire name; without a tag, the field name lower-camelled |
| `json:",omitempty"` on a `*T` | `OptionalFieldOf`, present when the pointer is not nil |
| `*T` without `omitempty` | `Nullable(T)` — present and null, which is a different thing |
| `schema:"use=expr"` | that expression, for a refinement or a union |
| `schema:"-"` | nothing; the field is not on the wire |
| an unexported field | nothing |
| a named type `N` in the package | `NSchema` |
| the first paragraph of a doc comment | the prose a projection publishes |

A struct field carries a type and a name. Everything else a schema says — a
bound, a length, a pattern, a format — has nowhere to live but the tag, so the
tag carries the same vocabulary the [constraint](#constraints) combinators do:

| Tag item | Becomes | Applies to |
|---|---|---|
| `min=N`, `max=N` | `AtLeast`, `AtMost` | a number |
| `above=N`, `below=N` | `Above`, `Below` | a number |
| `minLength=N`, `maxLength=N` | `MinLength`, `MaxLength` | a string |
| `pattern=RE` | `Matching` | a string |
| `minItems=N`, `maxItems=N` | `MinItems`, `MaxItems` | a list |
| `format=X` | the checked constructor for a standard `X`, or `Formatted` for one this package has not been taught | a string |

```go
//schema:generate
type Reading struct {
    Code  string   `json:"code" schema:"pattern=^[A-Z]{2}-[0-9]{4}$"`
    Pages int      `json:"pages" schema:"min=1,max=20000"`
    Tags  []string `json:"tags" schema:"minItems=1,maxItems=8"`
}
```

Items are separated by commas, **except inside braces, brackets or
parentheses** — `[a-z]{2,8}` carries a comma of its own, and splitting on it
would cut the pattern in half. Constraints are applied in the order written, so
the tag and the generated code read the same way.

A constraint written on a type it cannot apply to is refused rather than
emitted: `minLength` on an `int` would compile into nothing sensible, and an
explanation beats handing the reader a compile error. A constraint on a list
applies to the list; to constrain its elements, describe the element with `use=`.

Three things it refuses rather than guesses:

- **`omitempty` on a non-pointer.** A zero value is not absence; the schema
  refuses to guess that it is, so a generator that guessed for it would be worse
  than an error.
- **A type the table cannot name.** Guessing would produce a schema that
  compiles and describes the wrong thing. Write one by hand and point at it with
  `use=`.
- **A marked type that is not a struct.** A union's alternatives, and how to
  narrow to each, are not in an interface's declaration — see
  [Sums](#sums).

## Shapes

| Constructor | Describes |
|---|---|
| `Text` | a string |
| `Formatted(name)` | a string annotated with a format, and nothing more |
| `UUID`, `Email`, `URI`, `URL`, `URIReference`, `Hostname`, `IPv4`, `IPv6` | a string in that standard format, **checked** |
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

## Constraints

A kind says a value is a number; a constraint says which numbers.

```go
pages := schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000)
code  := schema.Matching(schema.Text(), `^[A-Z]{2}-[0-9]{4}$`)
tags  := schema.MinItems(schema.List(schema.Text()), 1)
```

| Combinator | Narrows |
|---|---|
| `AtLeast`, `AtMost` | a number, inclusively |
| `Above`, `Below` | a number, exclusively |
| `MinLength`, `MaxLength` | a string, in **characters** rather than bytes |
| `Matching(pattern)` | a string, by a Go (RE2) regular expression |
| `MinItems`, `MaxItems` | how many elements a list carries |

One declaration does two jobs: it refuses a value and it appears in the
published contract. A constraint that only did the first would leave a client to
discover the rule by being rejected.

Checking happens on **encode as well as decode**. A value this program built
that breaks its own constraint is a mistake here, and finding it at the boundary
beats sending it.

A refusal names the bound — `schema: is less than 1 at pages` — because a client
that is only told "invalid" cannot fix its request. A pattern that does not
compile, and a constraint on a shape with nowhere to put it (an object, a
union), are declaration mistakes reported by `Validate`.

The vocabulary is deliberately small: a constraint earns a place in the
description only if more than one projection can carry it. Anything narrower
belongs in a refinement below, which every projection describes as the shape
underneath it.

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

## Formats

A format is two different claims, and this package makes both. It is an
annotation a reader of the contract acts on — JSON Schema's own `format` keyword
asserts nothing, by design — and it is a rule a server has to enforce, because a
request is refused here or it is not refused at all.

```go
schema.UUID()      // 123e4567-e89b-12d3-a456-426614174000
schema.Email()     // parsed, not matched: the grammar is not a regular language
schema.URI()       // absolute: it says what scheme it is
schema.URL()       // absolute and located: a scheme, "://", and a host
schema.Hostname()  // RFC 1123 labels, at most 253 characters
schema.IPv4()      // told from the parsed form, so ::1 is not one
```

Where the rule **is** a regular expression — `uuid`, `url`, `hostname` — the
expression is recorded as well as the format name, so a consumer whose validator
ignores `format` still gets the check from `pattern`. Where the rule is grammar
or arithmetic — an address, a URI — there is nothing to record and the document
can only annotate. That asymmetry is real and is not hidden.

`Email` refuses `Ada <ada@example.test>`: that is a mailbox, and a field asking
for an address means the address. `URL` refuses `mailto:ada@example.test`, which
is the whole difference between naming a thing and saying where it is; it
annotates as `uri`, because that is the registered name and there is none for a
locator.

`Formatted(name)` is the open case: the format vocabulary is open, so a name
this package has not been taught is carried into the projection and **claims
nothing**. `Matching(pattern)` is the escape hatch for a rule of your own.

A refinement a schema cannot enforce should not look like one it does, which is
why those two are named differently from the rest.

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

A constraint becomes the keyword that says the same thing — `minimum`,
`maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `minLength`, `maxLength`,
`pattern`, `minItems`, `maxItems` — and that agreement is measured with the
external validator, on every case, in both directions.

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
