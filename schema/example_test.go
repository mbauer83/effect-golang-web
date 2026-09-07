package schema_test

// Runnable examples for the package documentation. Each one is the smallest
// program that shows what a reader would otherwise have to infer from the
// signatures.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema"
)

type Book struct {
	Title   string
	Authors []string
	Pages   int
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
)

// A schema is written once and used in both directions. Encoding is
// deterministic: fields appear in the order they were declared.
func Example() {
	document, err := schema.EncodeJSON(bookSchema, Book{
		Title:   "Zionomicon",
		Authors: []string{"John A. De Goes", "Adam Fraser"},
		Pages:   632,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.TrimSpace(string(document)))

	loaded, err := schema.DecodeJSON(bookSchema, document)
	if err != nil {
		panic(err)
	}
	fmt.Println(loaded.Title, "has", loaded.Pages, "pages")
	// Output:
	// {"title":"Zionomicon","authors":["John A. De Goes","Adam Fraser"],"pages":632}
	// Zionomicon has 632 pages
}

// A rejection names the part responsible, so a handler can tell a client which
// field it refused rather than that something was wrong somewhere.
func ExamplePathOf() {
	_, err := schema.DecodeJSON(bookSchema, []byte(`{"title":"T","authors":["ok",7],"pages":1}`))
	path, isSchemaFailure := schema.PathOf(err)
	fmt.Println(isSchemaFailure, strings.Join(path, "."))
	fmt.Println(err)
	// Output:
	// true authors.items.1
	// schema: expected a string, found a number at authors.items.1
}

// TransformOrFail is how a refinement is expressed: the schema describes what
// is on the wire, and the conversion decides whether the domain accepts it.
func ExampleTransformOrFail() {
	type Celsius float64

	celsius := schema.TransformOrFail(schema.Float64(),
		func(degrees float64) (Celsius, error) {
			if degrees < -273.15 {
				return 0, errors.New("below absolute zero")
			}
			return Celsius(degrees), nil
		},
		func(degrees Celsius) (float64, error) { return float64(degrees), nil },
	)

	warm, err := schema.DecodeJSON(celsius, []byte(`21.5`))
	fmt.Println(warm, err)

	_, err = schema.DecodeJSON(celsius, []byte(`-300`))
	fmt.Println(err)
	// Output:
	// 21.5 <nil>
	// schema: did not pass its refinement: below absolute zero
}

// A declaration mistake is reported rather than panicking, and Validate is
// where a program asks about it: at start-up, rather than on the first document
// that happens to arrive.
func ExampleValidate() {
	repeated := schema.Struct[Book]("Book",
		schema.FieldOf("title", schema.Text(),
			func(book Book) string { return book.Title },
			func(book *Book, title string) { book.Title = title }),
		schema.FieldOf("title", schema.Text(),
			func(book Book) string { return book.Title },
			func(book *Book, title string) { book.Title = title }),
	)

	fmt.Println(schema.Validate(bookSchema))
	fmt.Println(schema.Validate(repeated))
	// Output:
	// <nil>
	// schema: two fields are named title
}
