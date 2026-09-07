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
	Name  string
	Books []Book
}

// Book is one entry. Subtitle may be absent, which is not the same as being
// empty, so the schema declares it optional and the getter reports presence.
type Book struct {
	Title        string
	Authors      []string
	Pages        int
	Subtitle     string
	Availability Availability
}

// Availability is a sum, modelled the way Go models one: an interface with an
// unexported marker and a concrete type per alternative. The schema describes
// it as a union, so the document names the alternative it carries and a
// decoder never has to guess.
type Availability interface{ availability() }

// InStock is a title on the shelf, with how many copies.
type InStock struct{ Count int }

// Awaited is a title not yet arrived, with when it is expected.
type Awaited struct{ Expected time.Time }

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
