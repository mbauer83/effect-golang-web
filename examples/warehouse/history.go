package warehouse

// What the pallet has been.
//
// Version one is the description above; every later one is derived from the
// steps here, so there is no second declaration to keep in step with them. The
// same steps produce the statements that move the tables and the function that
// moves a value, which is the reason to declare a change rather than diff for
// it.

import (
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/evolve"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Pallets is the pallet's history.
//
// Version two renames the warehouse to a site and adds how the pallet is to be
// handled. The rename is the change a diff cannot see: it would have found a
// column gone and a column arrived, and the statements it wrote would have
// thrown away every pallet's location.
var Pallets = evolve.From("logistics.v1.Pallet", PalletSchema.Structure()).
	Then(
		evolve.Renamed{From: "warehouse", To: "site"},
		evolve.Added{Field: structure.Field{
			Name: "handling",
			// Bounded, because MySQL takes no default on an unbounded text
			// column and would reject the statement -- so a description that
			// wants both has to say how long the text is.
			Node: schema.MaxLength(schema.MinLength(schema.Text(), 1), 32).Structure(),
			Doc:  "Handling is how the pallet is to be moved.",
			// A default, because the pallets that already exist have no value
			// for it -- and without one a database will not add a not-null
			// column to a table that has rows in it.
			Default: structure.DefaultTo{Value: dynamic.OfText("standard")},
		}},
	)

// Sited is what a pallet looks like from version two on.
type Sited struct {
	Site     string
	Handling string
}

// SitedSchema reads the two columns version two introduced.
var SitedSchema = schema.Struct[Sited]("Sited",
	schema.FieldOf("site", schema.Text(),
		func(held Sited) string { return held.Site },
		func(held *Sited, value string) { held.Site = value }),
	schema.FieldOf("handling", schema.Text(),
		func(held Sited) string { return held.Handling },
		func(held *Sited, value string) { held.Handling = value }),
)
