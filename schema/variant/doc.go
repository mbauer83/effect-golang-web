// Package variant derives the shapes an aggregate has in different roles.
//
// One description is the whole of a thing. What a caller supplies to create one
// is not that shape: the identity the database generates is not theirs to give,
// and neither is a value a trigger will overwrite. What a caller supplies to
// change one is a third shape, without the identity, because a key selects the
// row rather than being part of its new value.
//
// Three shapes, then, and one description. That is what an ORM's mapping
// actually is -- not a transformation of one shape into another, but a
// projection to a variant, where a field the caller cannot supply is simply
// absent. Both Effect's VariantSchema and drizzle-zod arrive at the same trio
// from opposite directions, and the plan's section 7 records why.
//
// Everything here is a function from a structure.Node to a structure.Node, so
// a derived shape feeds every projection the original does: validate it with
// schema.Dynamic, publish it with jsonschema.Project, put it on the wire with
// protobuf, or make a table of it. Deriving in the description rather than in
// the type system is what makes that possible, and what makes it a walk over a
// tree rather than the type-level path extraction the same idea needed in
// TypeScript.
package variant
