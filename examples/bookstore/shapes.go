package bookstore

import (
	"github.com/mbauer83/effect-golang-schema/schema"
)

// BookSchema describes Book, binding the Go type to the wire.
var BookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.MaxLength(schema.MinLength(schema.Text(), 1), 200),
		func(value Book) string { return value.Title },
		func(value *Book, field string) { value.Title = field }).Documented("Title is what the book is called."),
	schema.FieldOf("authors", schema.MaxItems(schema.List(schema.Text()), 64),
		func(value Book) []string { return value.Authors },
		func(value *Book, field []string) { value.Authors = field }).Documented("Authors are credited in the order the book credits them."),
	schema.FieldOf("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000),
		func(value Book) int { return value.Pages },
		func(value *Book, field int) { value.Pages = field }).Documented("Pages is how many pages the book has, and there is at least one."),
).Documented("Book is one entry.")
