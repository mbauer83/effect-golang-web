// Package catalog is a complete program showing what one schema is worth.
//
// It reads a catalogue document, decodes it against a hand-written schema,
// writes the document back out in normalised form, and projects the same schema
// into a JSON Schema document that describes what it will accept. Nothing here
// describes the catalogue twice: the reader, the writer and the published
// contract are three projections of one description, which is the property the
// rest of this module is built on.
package catalog

import "time"

// Catalog is what one document holds.
type Catalog struct {
	// Name identifies the catalogue.
	Name string
	// Books are every entry, in the order the catalogue lists them.
	Books []Book
}

// Book is one entry.
//
// Subtitle is a pointer because absent and empty are different things, and an
// optional field's getter has to say which it is rather than leaving the schema
// to guess from a zero value.
type Book struct {
	// Title is what the book is called.
	Title string
	// Authors are credited in the order the book credits them.
	Authors []string
	// Pages is how many pages the book has, and there is at least one.
	Pages int
	// Subtitle is absent for a book that has none.
	Subtitle *string
	// Availability is whether the book can be had.
	Availability Availability
}

// Availability is a sum, modelled the way Go models one: an interface with an
// unexported marker and a concrete type per alternative. The schema describes
// it as a union, so the document names the alternative it carries and a
// decoder never has to guess.
type Availability interface{ availability() }

// InStock is a title on the shelf, with how many copies.
type InStock struct {
	// Count is how many copies are on the shelf.
	Count int
}

// Awaited is a title not yet arrived, with when it is expected.
type Awaited struct {
	// Expected is when the title should arrive.
	Expected time.Time
}

// Discontinued is a title that will not be restocked.
type Discontinued struct{}

func (InStock) availability()      {}
func (Awaited) availability()      {}
func (Discontinued) availability() {}

// Report is what one run of the program found and produced.
type Report struct {
	// Books is how many entries the document held.
	Books int
	// Shelved is how many of them were in stock.
	Shelved int
	// Components names the reusable shapes the published contract declares,
	// which is the evidence that a type used twice is described once.
	Components []string
}
