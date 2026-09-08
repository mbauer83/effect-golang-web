package catalog

import (
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

// CatalogSchema describes Catalog, binding the Go type to the wire.
var CatalogSchema = schema.Struct[Catalog]("Catalog",
	schema.FieldOf("name", schema.Text(),
		func(value Catalog) string { return value.Name },
		func(value *Catalog, field string) { value.Name = field }).Documented("Name identifies the catalogue."),
	schema.FieldOf("books", schema.List(BookSchema),
		func(value Catalog) []Book { return value.Books },
		func(value *Catalog, field []Book) { value.Books = field }).Documented("Books are every entry, in the order the catalogue lists them."),
).Documented("Catalog is what one document holds.")

// BookSchema describes Book, binding the Go type to the wire.
var BookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.MinLength(schema.Text(), 1),
		func(value Book) string { return value.Title },
		func(value *Book, field string) { value.Title = field }).Documented("Title is what the book is called."),
	schema.FieldOf("authors", schema.List(schema.Text()),
		func(value Book) []string { return value.Authors },
		func(value *Book, field []string) { value.Authors = field }).Documented("Authors are credited in the order the book credits them."),
	schema.FieldOf("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000),
		func(value Book) int { return value.Pages },
		func(value *Book, field int) { value.Pages = field }).Documented("Pages is how many pages the book has, and there is at least one."),
	schema.OptionalFieldOf("subtitle", schema.Text(),
		func(value Book) (string, bool) {
			var absent string
			if value.Subtitle == nil {
				return absent, false
			}
			return *value.Subtitle, true
		},
		func(value *Book, field string) { value.Subtitle = &field }).Documented("Subtitle is absent for a book that has none."),
	schema.FieldOf("availability", AvailabilitySchema,
		func(value Book) Availability { return value.Availability },
		func(value *Book, field Availability) { value.Availability = field }).Documented("Availability is whether the book can be had."),
).Documented("Book is one entry.")

// InStockSchema describes InStock, binding the Go type to the wire.
var InStockSchema = schema.Struct[InStock]("InStock",
	schema.FieldOf("count", schema.Int(),
		func(value InStock) int { return value.Count },
		func(value *InStock, field int) { value.Count = field }).Documented("Count is how many copies are on the shelf."),
).Documented("InStock is a title on the shelf, with how many copies.")

// AwaitedSchema describes Awaited, binding the Go type to the wire.
var AwaitedSchema = schema.Struct[Awaited]("Awaited",
	schema.FieldOf("expected", schema.Time(),
		func(value Awaited) time.Time { return value.Expected },
		func(value *Awaited, field time.Time) { value.Expected = field }).Documented("Expected is when the title should arrive."),
).Documented("Awaited is a title not yet arrived, with when it is expected.")

// DiscontinuedSchema describes Discontinued, binding the Go type to the wire.
var DiscontinuedSchema = schema.Struct[Discontinued]("Discontinued").Documented("Discontinued is a title that will not be restocked.")
