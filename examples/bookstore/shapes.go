package bookstore

import (
	"github.com/mbauer83/effect-golang-schema/schema"
)

// BookSchema describes Book, binding the Go type to the wire.
var BookSchema = schema.Struct("Book",
	schema.FieldAt("title", schema.Text().Check(schema.MinLength(1), schema.MaxLength(200)),
		func(value *Book) *string { return &value.Title }).WithDescription("Title is what the book is called."),
	schema.FieldAt("authors", schema.List(schema.Text()).Check(schema.MaxItems[string](64)),
		func(value *Book) *[]string { return &value.Authors }).WithDescription("Authors are credited in the order the book credits them."),
	schema.FieldAt("pages", schema.Int().Check(schema.AtLeast(1), schema.AtMost(20000)),
		func(value *Book) *int { return &value.Pages }).WithDescription("Pages is how many pages the book has, and there is at least one."),
).WithDescription("Book is one entry.")
