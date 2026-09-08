package catalog

import "github.com/mbauer83/effect-golang-web/schema"

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
	schema.VariantOf("inStock", InStockSchema,
		func(availability Availability) (InStock, bool) {
			stock, is := availability.(InStock)
			return stock, is
		},
		func(stock InStock) Availability { return stock }).
		Documented("on the shelf, with how many copies"),
	schema.VariantOf("awaited", AwaitedSchema,
		func(availability Availability) (Awaited, bool) {
			awaited, is := availability.(Awaited)
			return awaited, is
		},
		func(awaited Awaited) Availability { return awaited }).
		Documented("not yet arrived, with when it is expected"),
	schema.VariantOf("discontinued", DiscontinuedSchema,
		func(availability Availability) (Discontinued, bool) {
			gone, is := availability.(Discontinued)
			return gone, is
		},
		func(gone Discontinued) Availability { return gone }).
		Documented("will not be restocked"),
)
