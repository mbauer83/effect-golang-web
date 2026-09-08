package catalog

import "github.com/mbauer83/effect-golang-web/schema"

// Schema, BookSchema and the variants' schemas are generated from the structs;
// see schema_generated.go. What is written by hand here is what a generator
// cannot infer: a refinement, and a union.
//
//go:generate go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .

// Schema describes a whole catalogue document.
//
// It is a package-level value because a schema is immutable and reusable:
// building it once and validating it at start-up is the whole point of
// separating description from use.
var Schema = CatalogSchema

// pagesSchema refines the wire shape into the domain's rule. A page count of
// zero is not a book, and refusing it here means no handler has to.
//
// The struct points at this with a use= tag, because a refinement is not
// something a generator can infer from a type: an int is not the same thing as
// a page count, and only the domain knows the difference.
var pagesSchema = schema.TransformOrFail(schema.Int(),
	func(pages int) (int, error) {
		if pages < 1 {
			return 0, errBlankBook
		}
		return pages, nil
	},
	func(pages int) (int, error) { return pages, nil },
)

// AvailabilitySchema names the alternative it carries, so a decoder reading
// {"inStock": {...}} knows which shape follows before it reads it.
//
// A union is written by hand: which Go types are the alternatives, and how to
// narrow to each, is not in the interface's declaration.
var AvailabilitySchema = schema.OneOf[Availability]("Availability",
	schema.DocumentedVariant("on the shelf, with how many copies",
		schema.VariantOf("inStock", InStockSchema,
			func(availability Availability) (InStock, bool) {
				stock, is := availability.(InStock)
				return stock, is
			},
			func(stock InStock) Availability { return stock })),
	schema.DocumentedVariant("not yet arrived, with when it is expected",
		schema.VariantOf("awaited", AwaitedSchema,
			func(availability Availability) (Awaited, bool) {
				awaited, is := availability.(Awaited)
				return awaited, is
			},
			func(awaited Awaited) Availability { return awaited })),
	schema.DocumentedVariant("will not be restocked",
		schema.VariantOf("discontinued", DiscontinuedSchema,
			func(availability Availability) (Discontinued, bool) {
				gone, is := availability.(Discontinued)
				return gone, is
			},
			func(gone Discontinued) Availability { return gone })),
)
