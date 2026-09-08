// Package definitions describes the inventory's shapes, and is the source of
// truth for them.
//
// These are schemas, not a separate declaration language: the same Struct, the
// same OneOf, the same constraints. What they leave out is the getters and
// setters, which are the only part of a field declaration that needs a Go type
// -- so a description compiles before the type it will become exists, and the
// Go types are generated from it rather than written a second time.
//
// The descriptions are unexported. They are input to generation, and the
// application uses the generated package: two usable schemas for one shape
// would be one too many, and the typed one is the one a program wants.
//
// The widths are stated here and not left to the generator. A description that
// only said "a whole number" would make the generator guess, and a published
// contract would claim a range wider than the program accepts.
package definitions

import (
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// item is one stocked line.
var item = schema.Struct[dynamic.Value]("Item",
	schema.Describing("sku", schema.Matching(schema.Text(), `^[A-Z]{3}-[0-9]{5}$`)).
		Documented("the stock-keeping unit, three letters and five digits"),
	schema.Describing("onHand", schema.Uint16()).
		Documented("how many are on hand"),
	schema.Describing("weightGrams", schema.Float32()).
		Documented("what one unit weighs"),
	schema.Describing("id", schema.UUID()),
	schema.Describing("tags", schema.MinItems(schema.List(schema.Text()), 1)),
	schema.Describing("note", schema.MaxLength(schema.Text(), 200)).Optional(),
).Documented("one stocked line")

// movement is a change in what is stocked.
var movement = schema.OneOf[dynamic.Value]("Movement",
	schema.Choosing("received", schema.Struct[dynamic.Value]("Received",
		schema.Describing("count", schema.Uint16()),
	)).Documented("stock arriving"),
	schema.Choosing("shipped", schema.Struct[dynamic.Value]("Shipped",
		schema.Describing("count", schema.Uint16()),
		schema.Describing("to", schema.Hostname()),
	)).Documented("stock leaving"),
).Documented("a change in what is stocked")

// Descriptions are every shape to generate, in the order to write them.
func Descriptions() []structure.Node {
	return []structure.Node{item.Structure(), movement.Structure()}
}

// Faults reports a description that could not be used, so a mistake in one is a
// start-up error here rather than a puzzling generator failure.
func Faults() []error {
	faults := []error{}
	for _, described := range []schema.Schema[dynamic.Value]{item, movement} {
		if err := schema.Validate(described); err != nil {
			faults = append(faults, err)
		}
	}
	return faults
}
