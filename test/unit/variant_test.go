package unit

// The three shapes one description has.
//
// The aggregate here is deliberately awkward: an identity the database
// generates, a value the database computes, a value object with no identity of
// its own, and a collection of entities that do have one. Every rule the
// derivation has shows up in one of those.

import (
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
	"github.com/mbauer83/effect-golang-web/schema/variant"
)

// address is a value object: it belongs to whatever holds it and has no
// identity, so it lives in the order's own row and appears in every shape.
var address = schema.Struct[dynamic.Value]("Address",
	schema.DescribedField("street", schema.Text()),
	schema.DescribedField("city", schema.Text()),
)

// orderLine is an entity: it has an identity, so it is a thing rather than a
// part -- and its own computed field has to be left out of its own shape.
var orderLine = schema.Struct[dynamic.Value]("OrderLine",
	schema.DescribedField("id", schema.UUID()).Identity(),
	schema.DescribedField("sku", schema.Text()),
	schema.DescribedField("quantity", schema.AtLeast(schema.Int32(), 1)),
	schema.DescribedField("lineTotal", schema.Int64()).Computed(),
)

var order = schema.Struct[dynamic.Value]("Order",
	// The database generates it, so it is both: an identity, and not the
	// caller's to give.
	schema.DescribedField("id", schema.Int64()).Identity().Computed(),
	schema.DescribedField("reference", schema.UUID()),
	schema.DescribedField("shipTo", address),
	schema.DescribedField("lines", schema.List(orderLine)),
	schema.DescribedField("placedAt", schema.Time()).Computed(),
)

// named is the field names of a derived object, in order.
func named(t *testing.T, node structure.Node) []string {
	t.Helper()
	object, isObject := node.(structure.Object)
	if !isObject {
		t.Fatalf("expected an object, got %T", node)
	}
	names := make([]string, 0, len(object.Fields))
	for _, field := range object.Fields {
		names = append(names, field.Name)
	}
	return names
}

func TestTheCreateShapeLeavesOutWhatTheCallerCannotSupply(t *testing.T) {
	created, err := variant.Create(order.Structure())
	if err != nil {
		t.Fatal(err)
	}

	// No id: the database generates it. No placedAt: computed. No lines: they
	// have identities of their own, so creating one is its own act.
	if got := named(t, created); len(got) != 2 || got[0] != "reference" || got[1] != "shipTo" {
		t.Fatalf("unexpected fields: %v", got)
	}
	// The value object stays and stays whole, because it has no identity and
	// so is part of the order rather than a thing beside it.
	shipTo := fieldNamed(t, created, "shipTo")
	if got := named(t, shipTo.Node); len(got) != 2 {
		t.Fatalf("expected the value object whole, got %v", got)
	}
	// And a create shape is not partial: every field it kept is required,
	// because creating something means saying what it is.
	if shipTo.Optional {
		t.Error("expected the create shape's fields to be required")
	}
}

func TestTheUpdateShapeLeavesOutTheIdentityAndAsksForNothing(t *testing.T) {
	updated, err := variant.Update(order.Structure())
	if err != nil {
		t.Fatal(err)
	}

	if got := named(t, updated); len(got) != 2 || got[0] != "reference" {
		t.Fatalf("unexpected fields: %v", got)
	}
	// Every field optional: a change says what is changing, and a field nobody
	// mentioned is a field nobody is changing. That is the whole difference
	// between this and a create shape with the identity removed.
	object := updated.(structure.Object)
	for _, field := range object.Fields {
		if !field.Optional {
			t.Errorf("expected %q optional in an update shape", field.Name)
		}
	}

	// The order's own identity is generated, so Computed alone would have
	// dropped it and this says nothing about the identity rule. A line's
	// identity is the application's -- Identity and not Computed -- so it is
	// present in a create shape and absent from an update shape, and only the
	// identity rule can do that.
	line, err := variant.Update(orderLine.Structure())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range named(t, line) {
		if name == "id" {
			t.Error("expected an application-generated identity left out of an update shape")
		}
	}
	created, err := variant.Create(orderLine.Structure())
	if err != nil {
		t.Fatal(err)
	}
	if got := named(t, created); len(got) == 0 || got[0] != "id" {
		t.Errorf("expected the same identity kept in a create shape, got %v", got)
	}
}

func TestReachingIntoTheEntitiesDerivesThemTheSameWay(t *testing.T) {
	// An aggregate created in one act. The lines come along, and each line's
	// own computed field is left out of the line -- the rule applies at every
	// depth rather than only at the root.
	created, err := variant.CreateWithEntities(order.Structure())
	if err != nil {
		t.Fatal(err)
	}

	if got := named(t, created); len(got) != 3 || got[2] != "lines" {
		t.Fatalf("unexpected fields: %v", got)
	}
	lines := fieldNamed(t, created, "lines")
	// The list survives, because how many there are is not something a
	// derivation has any business changing.
	sequence, isList := lines.Node.(structure.Sequence)
	if !isList {
		t.Fatalf("expected the list kept, got %T", lines.Node)
	}
	// The line keeps its own identity, because a line created with its parent
	// is still the caller's to name -- and loses its computed total.
	if got := named(t, sequence.Element); len(got) != 3 || got[0] != "id" {
		t.Fatalf("unexpected line fields: %v", got)
	}
	for _, name := range named(t, sequence.Element) {
		if name == "lineTotal" {
			t.Error("expected the line's computed field left out")
		}
	}
}

func TestADerivedShapeStillValidatesAndStillProjects(t *testing.T) {
	// The reason to derive in the description rather than in the type system:
	// what comes out is a description, so everything that reads one reads this.
	created, err := variant.Create(order.Structure())
	if err != nil {
		t.Fatal(err)
	}

	shape := schema.Dynamic(created)
	// The reference is required in the create shape, so a document without it
	// is refused -- by the same enforcement path every other schema uses.
	if _, err := schema.DecodeJSON(shape, []byte(`{"shipTo":{"street":"a","city":"b"}}`)); err == nil {
		t.Error("expected the missing reference to be refused")
	}
	// And a document that satisfies it is read.
	document := `{"reference":"8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1","shipTo":{"street":"a","city":"b"}}`
	read, err := schema.DecodeJSON(shape, []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if _, present := read.(dynamic.Object).Member("reference"); !present {
		t.Fatalf("unexpected value: %#v", read)
	}
	// An id the caller cannot supply is *ignored* rather than refused, which is
	// this layer's ordinary tolerance for a member nobody declared -- and the
	// right answer here: a client that reads an order and sends the whole of it
	// back to create another should not be refused over the identity it could
	// not have known to omit. MEASURED, not assumed: the decoder skips it.
	withIdentity := `{"reference":"8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",` +
		`"shipTo":{"street":"a","city":"b"},"id":1}`
	tolerated, err := schema.DecodeJSON(shape, []byte(withIdentity))
	if err != nil {
		t.Fatalf("expected the unsupplied identity to be skipped, got %v", err)
	}
	// Skipped rather than carried: what comes out holds only what the shape
	// declares, so nothing downstream can act on an identity the caller sent.
	if _, present := tolerated.(dynamic.Object).Member("id"); present {
		t.Error("expected the identity dropped rather than carried through")
	}
}

func TestTheMarksDoNotSurviveIntoTheDerivedShape(t *testing.T) {
	// A shape a caller supplies has no identity to declare and nothing
	// computed left in it, so carrying the marks through would say something
	// untrue about it -- and a projection reading them would make a key out of
	// a field that is no longer one.
	created, err := variant.CreateWithEntities(order.Structure())
	if err != nil {
		t.Fatal(err)
	}
	object := created.(structure.Object)
	for _, field := range object.Fields {
		if field.Identity || field.Computed {
			t.Errorf("%q still carries a mark", field.Name)
		}
	}
	if object.IsEntity() {
		t.Error("a create shape is not an entity: it has no identity")
	}

	// The root has no marked field left to check -- id and placedAt were both
	// dropped -- so checking only the root proves nothing. The line's kept
	// identity is where a surviving mark would show, and a projection reading
	// it would make a key out of a field that is no longer one.
	lines := fieldNamed(t, created, "lines")
	element := lines.Node.(structure.Sequence).Element.(structure.Object)
	for _, field := range element.Fields {
		if field.Identity || field.Computed {
			t.Errorf("the line's %q still carries a mark", field.Name)
		}
	}
	if element.IsEntity() {
		t.Error("a derived line is not an entity either")
	}
}

func fieldNamed(t *testing.T, node structure.Node, name string) structure.Field {
	t.Helper()
	object, isObject := node.(structure.Object)
	if !isObject {
		t.Fatalf("expected an object, got %T", node)
	}
	for _, field := range object.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field named %q", name)
	return structure.Field{}
}
