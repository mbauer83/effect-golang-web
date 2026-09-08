// Package schemagen writes the schema a struct already implies.
//
// A struct's fields and a hand-written Schema for it say the same thing twice,
// and the second copy is the one that rots. This derives it.
//
// Derivation runs one way only. A Go struct cannot be derived from a schema,
// because Go cannot compute a type from a value; the struct is the source of
// truth and the schema follows it.
//
// Mark a struct and run the generator over its package:
//
//	//schema:generate
//	type Book struct {
//	    Title   string   `json:"title"`
//	    Authors []string `json:"authors"`
//	    Pages   int      `json:"pages" schema:"use=pagesSchema"`
//	}
//
// The output is deterministic, formatted, fully typed and checked in, and a
// test regenerates it in process to prove the file matches the structs it came
// from. Nothing here hides control flow or invents a second language: it emits
// the same combinator calls a hand-written schema uses, which is what keeps a
// generated schema and a written one interchangeable.
//
// It is internal because it is a tool, not a capability: a caller writes
// schemas or runs the generator, and never imports it.
package schemagen

import "fmt"

// FileName is the file the generator owns in each package it is run over.
const FileName = "schema_generated.go"

// Generate returns the source for one package's marked structs.
//
// It reads the directory and returns bytes rather than writing them, so the
// drift test can compare what the structs imply with what is checked in
// without running a toolchain.
func Generate(directory string) ([]byte, error) {
	collected, err := collect(directory)
	if err != nil {
		return nil, err
	}
	if len(collected.types) == 0 {
		return nil, fmt.Errorf("%s declares no type marked %s", directory, directive)
	}
	return render(collected)
}
