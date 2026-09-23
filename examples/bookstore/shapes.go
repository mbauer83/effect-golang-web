package bookstore

import (
	"github.com/mbauer83/effect-golang-schema/schema"
)

// BookSchema describes Book, binding the Go type to the wire.
var BookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.Text().Check(schema.MinLength(1), schema.MaxLength(200)),
		func(value Book) string { return value.Title },
		func(value *Book, field string) { value.Title = field }).WithDescription("Title is what the book is called."),
	schema.FieldOf("authors", schema.List(schema.Text()).Check(schema.MaxItems[string](64)),
		func(value Book) []string { return value.Authors },
		func(value *Book, field []string) { value.Authors = field }).WithDescription("Authors are credited in the order the book credits them."),
	schema.FieldOf("pages", schema.Int().Check(schema.AtLeast[int](1), schema.AtMost[int](20000)),
		func(value Book) int { return value.Pages },
		func(value *Book, field int) { value.Pages = field }).WithDescription("Pages is how many pages the book has, and there is at least one."),
).WithDescription("Book is one entry.")
