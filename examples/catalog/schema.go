package catalog

import (
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

// Schema describes a whole catalogue document. It is a package-level value
// because a schema is immutable and reusable: building it once and validating
// it at start-up is the whole point of separating description from use.
var Schema = schema.Struct[Catalog]("Catalog",
	schema.FieldOf("name", schema.Text(),
		func(catalog Catalog) string { return catalog.Name },
		func(catalog *Catalog, name string) { catalog.Name = name }),
	schema.DocumentedField("every entry, in the order the catalogue lists them",
		schema.FieldOf("books", schema.List(bookSchema),
			func(catalog Catalog) []Book { return catalog.Books },
			func(catalog *Catalog, books []Book) { catalog.Books = books })),
)

var bookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.Text(),
		func(book Book) string { return book.Title },
		func(book *Book, title string) { book.Title = title }),
	schema.FieldOf("authors", schema.List(schema.Text()),
		func(book Book) []string { return book.Authors },
		func(book *Book, authors []string) { book.Authors = authors }),
	schema.FieldOf("pages", pagesSchema,
		func(book Book) int { return book.Pages },
		func(book *Book, pages int) { book.Pages = pages }),
	// Absent and empty are different things, so presence is the getter's answer
	// rather than something the schema infers from a zero value.
	schema.OptionalFieldOf("subtitle", schema.Text(),
		func(book Book) (string, bool) { return book.Subtitle, book.Subtitle != "" },
		func(book *Book, subtitle string) { book.Subtitle = subtitle }),
	schema.FieldOf("availability", availabilitySchema,
		func(book Book) Availability { return book.Availability },
		func(book *Book, availability Availability) { book.Availability = availability }),
)

// pagesSchema refines the wire shape into the domain's rule. A page count of
// zero is not a book, and refusing it here means no handler has to.
var pagesSchema = schema.TransformOrFail(schema.Int(),
	func(pages int) (int, error) {
		if pages < 1 {
			return 0, errBlankBook
		}
		return pages, nil
	},
	func(pages int) (int, error) { return pages, nil },
)

// availabilitySchema names the alternative it carries, so a decoder reading
// {"inStock": {...}} knows which shape follows before it reads it.
var availabilitySchema = schema.OneOf[Availability]("Availability",
	schema.DocumentedVariant("on the shelf, with how many copies",
		schema.VariantOf("inStock",
			schema.Struct[InStock]("InStock",
				schema.FieldOf("count", schema.Int(),
					func(stock InStock) int { return stock.Count },
					func(stock *InStock, count int) { stock.Count = count })),
			func(availability Availability) (InStock, bool) {
				stock, is := availability.(InStock)
				return stock, is
			},
			func(stock InStock) Availability { return stock })),
	schema.DocumentedVariant("not yet arrived, with when it is expected",
		schema.VariantOf("awaited",
			schema.Struct[Awaited]("Awaited",
				schema.FieldOf("expected", schema.Time(),
					func(awaited Awaited) time.Time { return awaited.Expected },
					func(awaited *Awaited, expected time.Time) { awaited.Expected = expected })),
			func(availability Availability) (Awaited, bool) {
				awaited, is := availability.(Awaited)
				return awaited, is
			},
			func(awaited Awaited) Availability { return awaited })),
	schema.DocumentedVariant("will not be restocked",
		schema.VariantOf("discontinued",
			schema.Struct[Discontinued]("Discontinued"),
			func(availability Availability) (Discontinued, bool) {
				gone, is := availability.(Discontinued)
				return gone, is
			},
			func(gone Discontinued) Availability { return gone })),
)
