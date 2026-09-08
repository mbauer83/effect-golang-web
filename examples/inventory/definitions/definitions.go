// Package definitions describes the inventory's shapes before any Go type for
// them exists.
//
// These are schemas, not a separate declaration language: the same
// combinators, minus the getters and setters, which are the only part of a
// field declaration that needs a Go type. So a description is usable on its own
// -- it validates, transcodes and publishes -- and the Go types are generated
// from it rather than written twice.
//
// The widths are stated here and not left to the generator. A description that
// only said "a whole number" would make the generator guess, and a published
// contract would claim a range wider than the program accepts.
package definitions

import (
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Item is one stocked line.
var Item = schema.Record("Item",
	schema.DocumentedMember("the stock-keeping unit, three letters and five digits",
		schema.MemberOf("sku", schema.Matching(schema.Text(), `^[A-Z]{3}-[0-9]{5}$`))),
	schema.DocumentedMember("how many are on hand",
		schema.MemberOf("onHand", schema.Uint16())),
	schema.DocumentedMember("what one unit weighs",
		schema.MemberOf("weightGrams", schema.Float32())),
	schema.MemberOf("id", schema.UUID()),
	schema.MemberOf("tags", schema.MinItems(schema.List(schema.Text()), 1)),
	schema.OptionalMemberOf("note", schema.MaxLength(schema.Text(), 200)),
)

// Movement is a change in what is stocked.
var Movement = schema.Choice("Movement",
	schema.DocumentedAlternative("stock arriving",
		schema.AlternativeOf("received", schema.Record("Received",
			schema.MemberOf("count", schema.Uint16())))),
	schema.DocumentedAlternative("stock leaving",
		schema.AlternativeOf("shipped", schema.Record("Shipped",
			schema.MemberOf("count", schema.Uint16()),
			schema.MemberOf("to", schema.Hostname())))),
)

// Described is every shape the generator binds, in the order it should write
// them.
func Described() []structure.Node {
	return []structure.Node{Item.Structure(), Movement.Structure()}
}
