// Package schemagen writes the Go types a description implies.
//
// A schema describes a shape. Where that description is the source of truth --
// written before the type exists, or shared by more than one program -- the Go
// types are generated from it rather than written a second time and kept in
// agreement by hand.
//
// Generation reads the description by calling Structure on it. That needs no
// parser and no reflection, because a description is a Go value and the thing
// that reads a Go value is a Go program: put the descriptions in their own
// package and give it a generator of a dozen lines.
//
// \tfunc main() {
// \t\twritten, err := schemagen.WriteBindings("inventory", definitions.Descriptions()...)
// \t\tif err != nil { ... }
// \t\tos.WriteFile("../"+schemagen.BindingsFileName, written, 0o644)
// \t}
//
// The output is deterministic, formatted, fully typed and checked in, and it
// emits the same combinator calls a hand-written schema uses -- so a generated
// schema and a written one stay interchangeable, and there is one vocabulary to
// learn rather than two.
//
// It runs one way. A Go struct cannot be derived from a Schema[A], because that
// value names A and so A must exist for the schema to compile at all; a
// description that names no Go type compiles on its own, which is what makes
// this the direction that works. A schema for a Go type you already have is
// written with Struct and FieldOf, which is what those are for.
package schemagen
