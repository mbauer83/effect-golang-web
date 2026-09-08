# protobuf reference

`schema/protobuf` does two things with one description: it projects a proto3
file, and it encodes a value on the protobuf wire.

```go
document, err := protobuf.Project(OrderSchema.Structure(), "logistics.v1")
proto := document.Render()               // a .proto file

message, err := protobuf.Encode(OrderSchema, order)
order, err := protobuf.Decode(OrderSchema, message)
```

It carries **no third-party dependency**, for the reason the JSON Schema
projection carries none: the wire format and the proto3 language are published
specifications, and depending on an implementation of them would put a
dependency in the schema layer that every user of the HTTP core would acquire.
The canonical implementation appears in the tests, where its job is to check
that what this emits is what protobuf reads.

## Field numbers

Protobuf identifies a field by its **number**, and that number is the contract:
renaming a Go field is safe and renumbering it is not, which is the opposite of
every other projection here. So a number is declared, not derived:

```go
schema.FieldOf("reference", schema.UUID(), get, set).Numbered(1)
schema.DescribedVariant("byRail", railSchema).Numbered(2)
```

`Numbered` is a modifier rather than a parameter of `FieldOf`, because most
schemas never meet such a wire and a number every declaration had to carry
would be noise in all of them. Where one is needed it is **required**: a number
taken from declaration order would change when the declaration was reordered,
which is the exact failure field numbers exist to prevent.

## What the projection refuses

Each refusal is a thing that would have produced a file which compiles today and
means something else next release.

| refused | because |
|---|---|
| a field or variant with no number | order is not a stable substitute |
| an object or union with no name | a name taken from the field holding it would change when that field did |
| a union with a discriminating field | protobuf identifies the chosen member by number, so a discriminator encodes the choice twice and the two could contradict each other |
| a scalar at the root | protobuf transfers messages, and there is no name to invent for a wrapper |
| a list of lists, or a map of lists | proto3 has no repeated repeated; describe the inner list as a member of a named struct |
| a repeated variant | a oneof member's *presence* selects it, and a repeated field has none |

## What it maps

A `Kind` says what a value is on the wire and a `Precision` says what it is in a
program. This is the one projection where the precision is not detail: `int32`
and `int64` are different types on the wire, and widening a schema that said
`int32` would change what every other language generates.

| description | proto3 |
|---|---|
| text | `string` |
| integer, 8/16/32-bit | `int32` (unsigned: `uint32`) |
| integer, 64-bit or unstated | `int64` (unsigned: `uint64`) |
| number, 32-bit | `float` |
| number, otherwise | `double` |
| boolean, bytes | `bool`, `bytes` |
| timestamp | `google.protobuf.Timestamp` |
| named struct | `message` |
| named union | a `message` holding a `oneof` |
| list | `repeated` |
| string-keyed map | `map<string, V>` |
| nullable, or an optional field | `optional` |

Constraints and formats become **comments**. Proto3 has no validation keywords,
and a comment is honest about not being enforced by the wire where an invented
option would look like a rule the wire carried. The server does still enforce
them, through the same schema — which is the point of the schema being one
thing.

## Presence, and why the zero is not invented

This is the subtle part of proto3 and the part a codec gets wrong.

An ordinary proto3 field has **implicit presence**: it writes no bytes when it
holds its zero. So `false`, `0` and `""` are indistinguishable from absent, and
a reader that treated absent as *missing* would find every false boolean
missing. On the way in, a field the wire carried nothing for is therefore the
zero of its kind — not a default invented here, but what the format means.

A field with **explicit presence** — `optional`, a nullable, or any message
field — is different: there absent is a state of its own, which is what the
keyword buys, and it stays absent.

The description's own rules then apply to whatever resulted. A count declared
`AtLeast(1)` refuses the zero the wire implied exactly as it would refuse one
the wire spelled out, so nothing is quietly admitted by going through protobuf
instead of JSON.

## Why this codec is not a Sink and a Source

Every other format here plugs into the streaming
[`Sink` and `Source`](schema.md). Protobuf does not, and that is the format's
requirement rather than a shortcut:

- A nested message's **length precedes it**, so nothing can be written until the
  whole of it is known.
- A repeated field may appear under its number **more than once and in any
  order**, and a packed one may be split, so nothing can be read in place
  either.

Since a protobuf message is always framed — a gRPC request is a length-prefixed
frame — there is no streaming to give up. What crosses in between is the
universal representation, for the reason the [SQL](sql.md) package reads a row
that way: a protobuf message is a set of named values, which is an object, so
`ToDynamic` and `FromDynamic` do the crossing and this package adds only the
numbers.

## What the tests establish

A round trip through this codec alone would say only that its two halves agree
with each other. So:

- the projected `.proto` is **compiled by a real protobuf compiler**
  (`bufbuild/protocompile`), and the assertions are made against the resulting
  descriptor rather than against the text;
- every wire case goes through `google.golang.org/protobuf` in one direction or
  the other, against a message built from *this module's own projected
  descriptor* — so a disagreement about numbers, widths, packing, presence or
  oneof membership shows up as a failure.

An unknown field is skipped, which is the property protobuf is chosen for. The
test for it puts the unknown field **first**, because at the end a skip of the
wrong length would be invisible: there would be nothing after it to corrupt.
