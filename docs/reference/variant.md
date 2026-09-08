# Variant reference

One description is the whole of a thing. What a caller supplies to *create* one
is not that shape — the identity the database generates is not theirs to give,
and neither is a value a trigger will overwrite. What a caller supplies to
*change* one is a third shape, without the identity, because a key selects the
row rather than being part of its new value.

```go
variant.Select(node)              // the whole thing, unchanged
variant.Create(node)              // what a caller supplies to make one
variant.Update(node)              // what a caller supplies to change one
variant.CreateWithEntities(node)  // the aggregate, created in one act
variant.UpdateWithEntities(node)
```

Three shapes, one description. **That is what an ORM's mapping actually is** —
not a transformation of one shape into another, but a projection to a variant
where a field the caller cannot supply is simply absent. Effect's
`VariantSchema` and `drizzle-zod` arrive at the same trio from opposite
directions; [the plan's §7](../../architecture-plan.md#71-ddl-and-migrations-researched-not-built)
records the evidence.

## The two marks

```go
schema.FieldOf("id", schema.Int64(), get, set).Identity().Computed()
schema.FieldOf("reference", schema.UUID(), get, set).Identity()
schema.FieldOf("placedAt", schema.Time(), get, set).Computed()
```

**`Identity`** — the field that distinguishes one of these from another. A
projection to storage makes it the key; an update shape leaves it out.

**`Computed`** — the value comes from somewhere other than the caller: a
default, a trigger, a derivation. Left out of every shape a caller supplies,
because asking for a value that will be overwritten is asking a question with no
answer.

They **compose**, and the composition is the distinction other libraries spell
with two separate concepts: an identity the application generates is `Identity`
alone and appears in a create shape; one the database generates is both and does
not. Effect needs `GeneratedByApp` and `GeneratedByDb` for the same two cases.

## An entity is derived, not declared

An object is an **entity** — a thing in its own right — exactly when it has an
identity. An object without one is a **value** belonging to whatever holds it.

```go
object.IsEntity()      // has an identity
object.Identity()      // (Field, bool)
```

This is a deliberate simplification of the prior art, which had a third marker
for it. A separate marker could only ever agree with the identity or contradict
it, and there is nothing useful to say in the contradicting case. An address
inside a customer is a value and lives in the customer's row; an order line has
an identity and lives in its own table.

## What each shape does

| | identity | computed | nested entities | requiredness |
|---|---|---|---|---|
| `Select` | kept | kept | kept | as declared |
| `Create` | kept if not computed | dropped | dropped | as declared |
| `CreateWithEntities` | kept if not computed | dropped | kept, derived the same way | as declared |
| `Update` | dropped | dropped | dropped | **everything optional** |
| `UpdateWithEntities` | dropped | dropped | kept, derived the same way | **everything optional** |

An update shape makes every remaining field optional, because a change says what
is *changing* and a field nobody mentioned is a field nobody is changing. That
is the whole difference between an update shape and a create shape with the
identity removed — and it is why they are two functions.

Nested entities are dropped by default: they have identities of their own, so
creating one is its own act. `…WithEntities` is a separate function rather than a
boolean, because the two are different requests and a flag at the call site
would not say which was meant. When they are kept, they are derived **the same
way at every depth**, so a computed column on a child is left out of the child.

A wrapper survives the derivation. A list of order lines becomes a list of
derived order lines; a nullable one stays nullable; a map's values are derived
and its keys are not. How many there are, and whether there is one, is nothing a
derivation has business changing.

**The marks do not survive.** A shape a caller supplies has no identity to
declare and nothing computed left in it, so carrying them through would say
something untrue — and a projection reading them would make a key out of a field
that is no longer one. A derived shape is therefore not an entity.

## What is refused

- a **scalar**, or anything with no fields: it has one shape in every role;
- a shape where **every field was left out**, which is almost always a mistake
  in the marks rather than a shape worth publishing;
- a **reference with nothing behind it**: a derived shape has to be built, so a
  name the walker cannot follow is not something to pass through. A reference to
  a *value object* passes through untouched, because this walk does not reach
  into one.

## Why it derives descriptions and not types

Everything here is `structure.Node` → `structure.Node`, so a derived shape feeds
**every projection the original does**: validate it with `schema.Dynamic`,
publish it with `jsonschema.Project`, put it on the wire with `protobuf`, or
make a table of it.

That is not a convenience. The same idea in TypeScript needs type-level path
extraction —  `CreateType<Type, ComputedFieldPathArrays<…>, IdFieldPathArrays<…>>` —
and the prior art's version of it carries a `@ts-expect-error`, in a language
*with* mapped and conditional types. Here a description is already data, so
deriving is a walk over a tree. Where a Go type per variant is wanted,
[`schemagen`](../../schemagen) writes it from the derived description with a
drift test.

One thing to know about the result: a member the derived shape does not declare
is **skipped, not refused** — this layer's ordinary tolerance. So a client that
reads an order and posts the whole of it back to create another is not refused
over the identity it could not have known to omit, and the identity it sent is
dropped rather than carried through. MEASURED, not assumed.
