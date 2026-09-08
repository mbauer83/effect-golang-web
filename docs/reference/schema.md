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

Write a schema this way for a Go type you already have — a domain type with
methods, one from another package, one whose wire shape differs from its Go
shape. Where the *description* is the source of truth instead, the struct and
this schema are both [generated](#generating-the-go-types) from it, and what
comes out is these same calls: one vocabulary, written or generated.

## Describing a shape with no Go type

`Schema[A]` moves a Go value in and out of a format. A description written
before the type it will become exists — or loaded from elsewhere, or read back
out of another schema — has no `A`, and it is still worth validating,
transcoding, inspecting and composing.

It is the same vocabulary — the same `Struct`, the same `OneOf` — minus the
accessors:

```go
var Book = schema.Struct[dynamic.Value]("Book",
    schema.DescribedField("title", schema.MinLength(schema.Text(), 1)).
        Documented("what the book is called"),
    schema.DescribedField("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000)),
    schema.DescribedField("subtitle", schema.Text()).Optional(),
    schema.DescribedField("id", schema.UUID()),
)
```

`DescribedField` is `FieldOf` without the getter and setter — the only part of a
field declaration that needs the Go type, so leaving them out is exactly the
difference between *describing* a shape and *binding* one. `DescribedVariant` is
`VariantOf` without the narrowing, for the same reason: a described value
carries its own tag, so narrowing to a variant is reading a name.
`Dynamic(node)` is the general door: a typed schema's `Structure()` passed
through it is usable without its type.

There is **no second codec**. `Dynamic` rebuilds the description out of the same
combinators a typed schema is written with, so the typed and described paths
cannot disagree about what a shape admits — they are the same path.

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

It enforces **what it records**: a bound, a length, a pattern, an item count, a
width, optionality, and the shape itself. That is what recording constraints in
the description was for — the rules survive without the Go type that stated
them, and they are enforced by the very combinators that stated them.

It cannot enforce a rule that could only be code. `Email()` and `URI()` parse,
and a parse is not a keyword, so a description alone annotates and no more.
`UUID()`, `URL()` and `Hostname()` recorded their expressions, so those *are*
enforced. Saying so plainly beats a described path that silently admits what the
typed path refuses.

## Generating the Go types

Where the description is the source of truth, the Go types come from it:

```go
// examples/inventory/definitions -- unexported, because they are input
var item = schema.Struct[dynamic.Value]("Item",
    schema.DescribedField("sku", schema.Matching(schema.Text(), `^[A-Z]{3}-[0-9]{5}$`)),
    schema.DescribedField("onHand", schema.Uint16()),
    schema.DescribedField("note", schema.MaxLength(schema.Text(), 200)).Optional(),
)

func Descriptions() []structure.Node { return []structure.Node{item.Structure()} }
```

```go
// examples/inventory/gen -- a dozen lines, run by go:generate
written, err := schemagen.WriteBindings("inventory", definitions.Descriptions()...)
```

and out comes the struct, with the widths the description stated, and the typed
schema that binds it:

```go
type Item struct {
    Sku    string  `json:"sku"`
    OnHand uint16  `json:"onHand"`
    Note   *string `json:"note,omitempty"`
}

var ItemSchema = schema.Struct[Item]("Item",
    schema.FieldOf("sku", schema.Matching(schema.Text(), "^[A-Z]{3}-[0-9]{5}$"), get, set),
    ...
)
```

Reading a description needs no parser and no reflection: a description is a Go
value, and what reads a Go value is a Go program. So generation is a program
that imports the descriptions — which is also why the descriptions live in their
own package and are **unexported**. They are input. The application imports the
generated package and uses `ItemSchema`; two usable schemas for one shape would
be one too many.

Generation runs one way. A Go struct cannot be derived from a `Schema[A]`,
because that value names `A` and so `A` must exist for the schema to compile at
all. For a Go type you already have — a domain type with methods, one from
another package — the schema is written with `Struct` and `FieldOf`, which is
what those are for; `examples/catalog` does it that way.

| In the description | In the generated code |
|---|---|
| a named `Struct` | a Go struct, and `NameSchema` binding it |
| a named `OneOf` | an interface with an unexported marker, a struct per variant, and `NameSchema` |
| `.Optional()` | a pointer field with `,omitempty` |
| a stated width | that Go type — `uint16`, `float32` — rather than the widest one the wire could carry |
| a member's prose | the field's doc comment |
| a member name | the exported Go name, with the conventional initialisms: `id` becomes `ID` |

The output is deterministic and checked in, it is compiled as part of the
module, and a test regenerates it in process and compares — so a description
changed without a regeneration fails there rather than at the next request.

## Shapes

| Constructor | Describes |
|---|---|
| `Text` | a string |
| `Formatted(name)` | a string annotated with a format, and nothing more |
| `UUID`, `Email`, `URI`, `URL`, `URIReference`, `Hostname`, `IPv4`, `IPv6` | a string in that standard format, **checked** |
| `Int`, `Int8`, `Int16`, `Int32`, `Int64` | a whole number of that width |
| `Uint`, `Uint8`, `Uint16`, `Uint32`, `Uint64` | an unsigned whole number of that width |
| `Float32`, `Float64` | a finite number; NaN and the infinities are refused on encode |
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

Everything that *modifies* rather than builds is a method, so there is nothing
to remember about which wrap and which are called on what they change:

```go
schema.Struct[Book]("Book", …).Documented("one entry")
schema.FieldOf("note", schema.Text(), get, set).Documented("a note")
schema.VariantOf("circle", circleSchema, narrow, widen).Documented("a circle")
schema.DescribedField("note", schema.Text()).Optional()
```

`Optional()` applies to a field whose absence can be *seen* — a described field,
where the member is either in the object or not. A field bound with `FieldOf`
cannot be made optional this way, because its getter returns a value and not a
value and whether there is one; that is why `OptionalFieldOf` takes a different
getter rather than this taking none.

## Widths

The wire carries two numeric shapes and Go has twelve. A description says which
of the twelve, so the wire shape is **derived** rather than declared: every one
of these is `integer` or `number` on the wire, and a format reads that and needs
to know nothing about the width.

Three things follow, and each of them is the reason to state a width:

- **A bound outside the type's range is a compile error.** `AtMost(Int8(), 200)`
  does not build, because 200 is not an `int8`. The constructor being precisely
  typed is what buys this; no check at run time is involved.
- **The range the width implies is recorded**, so the published contract states
  `minimum: 0, maximum: 255` for a `Uint8` without anyone writing it down — and
  a schema used without its Go type still refuses what the width cannot hold.
- **A generator emits the type the author meant** rather than the widest one
  that would hold it.

Decoding refuses a value the width cannot hold rather than truncating it, and
`Float32` refuses one beyond its range rather than turning it into an infinity.

`Uint` and `Uint64` are bounded by the largest *signed* 64-bit value, not the
largest unsigned one: the wire carries a signed integer, so a value above that
cannot be expressed at all and claiming otherwise would be lying about what the
schema can carry.

Only the widths the specification registers a name for become a `format` —
`int32`, `int64`, `float`, `double`. The rest are stated by `minimum` and
`maximum`, which is what a reader can act on; putting `uint16` in a contract
would be telling a client about Go.

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
