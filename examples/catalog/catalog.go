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
//
//schema:generate
type Catalog struct {
	// Name identifies the catalogue.
	Name string `json:"name"`
	// Books are every entry, in the order the catalogue lists them.
	Books []Book `json:"books"`
}

// Book is one entry.
//
// Subtitle is a pointer because absent and empty are different things, and a
// generated optional field reads presence from the pointer rather than guessing
// it from a zero value.
//
//schema:generate
type Book struct {
	// Title is what the book is called.
	Title string `json:"title" schema:"minLength=1"`
	// Authors are credited in the order the book credits them.
	Authors []string `json:"authors"`
	// Pages is how many pages the book has, and there is at least one.
	Pages int `json:"pages" schema:"min=1,max=20000"`
	// Subtitle is absent for a book that has none.
	Subtitle *string `json:"subtitle,omitempty"`
	// Availability is whether the book can be had.
	Availability Availability `json:"availability"`
}

// Availability is a sum, modelled the way Go models one: an interface with an
// unexported marker and a concrete type per alternative. The schema describes
// it as a union, so the document names the alternative it carries and a
// decoder never has to guess.
type Availability interface{ availability() }

// InStock is a title on the shelf, with how many copies.
//
//schema:generate
type InStock struct {
	// Count is how many copies are on the shelf.
	Count int `json:"count"`
}

// Awaited is a title not yet arrived, with when it is expected.
//
//schema:generate
type Awaited struct {
	// Expected is when the title should arrive.
	Expected time.Time `json:"expected"`
}

// Discontinued is a title that will not be restocked.
//
//schema:generate
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
