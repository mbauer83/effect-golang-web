// Package inventory is generated from a description rather than written.
//
// Its types and their schemas come from examples/inventory/definitions, which
// describes the shapes with the same combinators a typed schema uses, minus the
// getters and setters. Nothing here is edited: the description is the source of
// truth, and a test regenerates this and compares.
//
// It is the other direction from examples/catalog, where the Go struct is the
// source of truth and the schema is generated from it. Both exist because both
// are right sometimes: a wire shape that differs from the domain type wants the
// struct first, and a contract several programs share wants the description
// first.
package inventory

//go:generate go run ./gen
