# Migration reference

`schema/evolve` carries a description from one version to the next, and the
values with it. `ddl.Alter` turns the same steps into statements.

```go
var Pallets = evolve.From("logistics.v1.Pallet", PalletSchema.Structure()).
    Then(
        evolve.Renamed{From: "warehouse", To: "site"},
        evolve.Added{Field: structure.Field{
            Name:    "handling",
            Node:    schema.MinLength(schema.Text(), 1).Structure(),
            Default: structure.DefaultTo{Value: dynamic.OfText("standard")},
        }},
    )

Pallets.At(2)                                   // the derived description
Pallets.Migrate(1, 2, value)                    // the value, carried
ddl.Alter(ddl.Postgres, Pallets, 1, 2)          // the statements
ddl.Alter(ddl.Postgres, Pallets, 2, 1)          // and back
```

## Declared, not diffed

The industry splits two ways, and [Atlas names them](https://atlasgo.io/concepts/declarative-vs-versioned):
**versioned**, a script per change, and **declarative**, a desired state
compared against whatever the database happens to hold — which Atlas is candid
is *non-deterministic*, because the plan depends on the state it finds.

This is neither. A step is a **declared** list of changes between two pinned
versions. Nothing is inferred, so a plan is the same every time, and a rename is
exact.

That last point is the whole argument. `drizzle-kit`, the most developed tool in
the TypeScript ecosystem, [cannot tell a rename from a drop-and-add](https://github.com/drizzle-team/drizzle-orm/issues/6053):
it asks interactively, the prompt has open bugs, its programmatic API throws
when a diff contains both a create and a delete, and choosing "renamed" emits
the rename while [dropping the accompanying type change](https://github.com/drizzle-team/drizzle-orm/issues/5499).
Declared, it is simply known — and the acceptance suite writes a row, migrates,
and reads the value back under its new name, which a drop-and-add would have
lost.

## Only version one is written

Every later version is **derived** by applying the steps. There is no second
declaration to disagree with them, which is the objection that would otherwise
apply: writing V1, V2 and a function between them means something has to check
that the function really takes one to the other.

That refinement comes from [Cambria](https://www.inkandswitch.com/cambria/),
which generates its type definitions from the lens definitions themselves.
Materialise a derived version with [`schemagen`](../../schemagen) if a Go type
for it is wanted, the way any other generated artifact is materialised.

## Four changes, and one of them is the point

```go
evolve.Added{Field: structure.Field{…}}
evolve.Removed{Name: "legacyCode"}
evolve.Renamed{From: "depot", To: "warehouse"}
evolve.Retyped{Name: "count", Node: schema.Int64().Structure()}
```

A closed set, so a projection switches over it and knows it has covered
everything — and so a step cannot contain something nobody taught the DDL to
write. `Renamed` is the member a diff cannot see.

`Added` refuses a field that is neither optional nor defaulted: the rows that
already exist have no value for it, and a database will not add such a column to
a table that is not empty. `Retyped` says what the new shape is and lets the
projection say what it costs, because whether a change of type is safe is the
database's business and differs by dialect.

## Both directions, from N−1 steps

Each change knows its inverse, so every one of the N² version pairs is answered
by composition — forward by concatenating the steps, backward by inverting each
change and **reversing twice over**: the steps in reverse, and the changes within
each step in reverse too. A step that renamed one field and added another has to
drop the addition before undoing the rename, or the inverse would be looking for
a field under a name it no longer has.

Nothing is precomputed. The plan imagined generating the N² typed compositions,
as the TypeScript prior art builds them at runtime — but that was needed there
because it had a Go-type equivalent *per version*. Here values cross as the
universal representation, typed at the edges and untyped in the middle, exactly
as the [protobuf](protobuf.md) codec is. So there is nothing to generate and
nothing to erase, and folding a handful of changes costs less than remembering
the answer.

## What a down migration cannot do

**Invent data.** Dropping a column loses what was in it, so putting the column
back restores the shape and not the values — and the restored field is made
*optional*, because a required column with no values is one no row satisfies.

A rename's inverse is a rename, which is the one inverse in the set that loses
nothing at all.

## One declaration, two projections

The same changes produce the statements that move the table *and* the function
that moves a value. So a value carried forward in memory is a value the migrated
table accepts — a property that would otherwise be two things kept in step by
hand. The acceptance suite checks exactly that: migrate a value, migrate the
table, write the one into the other.

`Migrate` leaves a `Retyped` field's value alone. Converting it would mean
guessing how — a number to a string is a format nobody stated, a string to a
number is a parse that may fail — so what comes out is checked against the
target version's schema, which is where a value that no longer fits is reported.

## What Go cannot give this

**Compile-time totality.** A version is a *value* in Go, not a type, so the
compiler cannot check that a step accounts for every column of its target;
encoding a version's columns in types would need a type parameter per column and
be unusable. What is available is **assembly-time** totality: a `History`
carries a `Fault` and answers nothing until it is clear, the way an ambiguous
route and an endpoint declaration are checked. A test that asks `Fault` is a test
that has checked it. Scala or Haskell could do better here; Go cannot, and this
does not pretend otherwise.

**Drift reconciliation.** A declared chain cannot know that someone altered
production by hand — the one thing declarative diffing does better. The intended
answer keeps determinism: before applying N→N+1, assert the live database
matches the declaration at N and refuse rather than proceed. **That check is not
built.**

## Dialects

`Renamed`, `Added` and `Removed` are spelled the same by Postgres and MySQL.
`Retyped` is not, and the difference is not cosmetic: Postgres's
`alter column x type y` changes the type and leaves the rest of the definition
alone, where MySQL's `modify column x y …` restates the whole definition — so
anything left out of the restatement is lost. SQLite **refuses** it outright,
because its `ALTER TABLE` cannot do it: the way is a new table, a copy, a drop
and a rename, which is four statements and a decision about values that no
longer fit.

`Alter` covers the aggregate's **root table** only. A change to a nested entity
is a change to that entity's description, so it has a history of its own — a step
that silently reached into a child would be one whose effect depended on where
the change happened to be written.

## Scope

- The **verification step** described above is not built.
- **Splits and merges** — one column becoming two, two becoming one — are not in
  the closed set. They need a value function in both directions, which is the
  escape hatch this deliberately does not yet have.
- Nothing runs migrations. These are statements and a function; applying them in
  order, recording which have been applied, and doing it once across several
  processes is a migrator, and is not here.

[`examples/warehouse`](../../examples/warehouse/history.go) is the history.
