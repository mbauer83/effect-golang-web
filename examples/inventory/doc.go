// Package inventory is generated from a description rather than written.
//
// Its types and their schemas come from examples/inventory/definitions, which
// describes the shapes with the same combinators a typed schema uses, minus the
// getters and setters. Nothing here is edited: the description is the source of
// truth, and a test regenerates this and compares.
//
// ItemSchema is not a second copy of that description. It is the description
// bound to a Go type, and the binding is the only thing that can turn a
// document into an Item: the description cannot, because it has no Item to
// produce. That is the one thing it does not afford, and the whole reason to
// generate anything.
//
// The relationship runs one way and is checked both ways. A binding's
// Structure is the description it came from, so a caller who wants the shape
// without the type has it from the one exported thing -- which is why the
// descriptions are unexported, and why exporting them as well would be the
// duplicate. Nothing an application links carries the description at all: only
// the generator and a test import that package.
//
// examples/catalog is the other way round: the Go type came first and its
// schema is written by hand with Struct and FieldOf, which is what those are
// for.
package inventory

//go:generate go run ./gen
