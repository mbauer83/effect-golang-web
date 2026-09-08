package unit

// A description for these tests to carry.
//
// Its own, rather than reached out of the schema module's fixtures: what the
// tests here are about is what a transport does with a description, so the
// description is a detail of the test and belongs beside it.

import (
	"errors"

	"github.com/mbauer83/effect-golang-schema/schema"
)

// Book is what an endpoint in these tests accepts and answers with.
type Book struct {
	Title    string
	Authors  []string
	Pages    int
	Subtitle string
	HasIndex bool
}

var bookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.Text(),
		func(book Book) string { return book.Title },
		func(book *Book, title string) { book.Title = title }),
	schema.FieldOf("authors", schema.List(schema.Text()),
		func(book Book) []string { return book.Authors },
		func(book *Book, authors []string) { book.Authors = authors }),
	schema.FieldOf("pages", schema.Int(),
		func(book Book) int { return book.Pages },
		func(book *Book, pages int) { book.Pages = pages }),
	schema.OptionalFieldOf("subtitle", schema.Text(),
		func(book Book) (string, bool) { return book.Subtitle, book.Subtitle != "" },
		func(book *Book, subtitle string) { book.Subtitle = subtitle }),
	schema.FieldOf("hasIndex", schema.Bool(),
		func(book Book) bool { return book.HasIndex },
		func(book *Book, has bool) { book.HasIndex = has }),
)

// errRefused is a writer that would not take the bytes, for the tests that ask
// what a boundary does when the body cannot be written.
var errRefused = errors.New("the writer refused")
