package catalog

import "github.com/mbauer83/effect-golang-web/schema"

// Schema, BookSchema and the variants' schemas are generated from the structs;
// see schema_generated.go. What is written by hand here is the one thing a
// generator cannot infer: a union.
//
//go:generate go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .

// Schema describes a whole catalogue document.
//
// It is a package-level value because a schema is immutable and reusable:
// building it once and validating it at start-up is the whole point of
// separating description from use.
var Schema = CatalogSchema

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
