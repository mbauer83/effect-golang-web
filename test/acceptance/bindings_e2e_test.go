package acceptance

// What a generated binding is for, and what it is not.
//
// A description says what shape a document has. It cannot produce an Item,
// because it has no Item to produce -- that is the one thing it does not
// afford, and the whole reason the binding exists. These check that the two
// agree about everything else, so the binding adds a Go type and nothing else.

import (
	"reflect"
	"testing"

	"github.com/mbauer83/effect-golang-web/examples/inventory"
	"github.com/mbauer83/effect-golang-web/examples/inventory/definitions"
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
)

const itemDocument = `{"sku":"ABC-12345","onHand":4,"weightGrams":250.5,` +
	`"id":"123e4567-e89b-12d3-a456-426614174000","tags":["metal"]}`

// itemDescription is the shape the generator read, reached the way the
// generator reaches it.
func itemDescription() schema.Schema[dynamic.Value] {
	return schema.Dynamic(definitions.Descriptions()[0])
}

func TestABindingDescribesExactlyWhatItWasGeneratedFrom(t *testing.T) {
	// Not "the same source text", which the drift test checks, but the same
	// description: a binding that admitted a different shape from the one it
	// came from would be the one bug this arrangement exists to prevent.
	if !reflect.DeepEqual(inventory.ItemSchema.Structure(), definitions.Descriptions()[0]) {
		t.Fatalf("the binding describes something else:\n  %#v\n  %#v",
			inventory.ItemSchema.Structure(), definitions.Descriptions()[0])
	}
}

func TestTheBindingAndTheDescriptionReadTheSameDocument(t *testing.T) {
	typed, err := schema.DecodeJSON(inventory.ItemSchema, []byte(itemDocument))
	if err != nil {
		t.Fatal(err)
	}
	described, err := schema.DecodeJSON(itemDescription(), []byte(itemDocument))
	if err != nil {
		t.Fatal(err)
	}

	// The binding hands back a Go value with typed fields. That is what it
	// affords and the description does not.
	if typed.OnHand != 4 || typed.WeightGrams != 250.5 {
		t.Fatalf("unexpected value: %#v", typed)
	}
	// The description hands back the shape, and nothing more can be said about
	// it without asking member by member.
	onHand, present := described.(dynamic.Object).Member("onHand")
	if !present || onHand != (dynamic.Integer{Value: 4}) {
		t.Fatalf("unexpected member: %#v", onHand)
	}

	// And the two are the same value seen two ways.
	crossed, err := schema.ToDynamic(inventory.ItemSchema, typed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(crossed, described) {
		t.Fatalf("the binding and the description disagree:\n  %#v\n  %#v", crossed, described)
	}
}

func TestTheBindingAndTheDescriptionRefuseTheSameDocuments(t *testing.T) {
	// The rules are in the description, so both enforce them. A binding that
	// was stricter or looser than what it was generated from would make the
	// published contract a lie.
	for reason, document := range map[string]string{
		"a sku that does not match": `{"sku":"abc","onHand":1,"weightGrams":1,"id":"123e4567-e89b-12d3-a456-426614174000","tags":["x"]}`,
		"no tags at all":            `{"sku":"ABC-12345","onHand":1,"weightGrams":1,"id":"123e4567-e89b-12d3-a456-426614174000","tags":[]}`,
		"a count beyond its width":  `{"sku":"ABC-12345","onHand":65536,"weightGrams":1,"id":"123e4567-e89b-12d3-a456-426614174000","tags":["x"]}`,
		"an identifier that is not": `{"sku":"ABC-12345","onHand":1,"weightGrams":1,"id":"nope","tags":["x"]}`,
		"a missing required member": `{"sku":"ABC-12345","onHand":1,"weightGrams":1,"tags":["x"]}`,
	} {
		_, typedErr := schema.DecodeJSON(inventory.ItemSchema, []byte(document))
		_, describedErr := schema.DecodeJSON(itemDescription(), []byte(document))
		if typedErr == nil {
			t.Errorf("the binding admits %s", reason)
		}
		if describedErr == nil {
			t.Errorf("the description admits %s", reason)
		}
	}
}

func TestTheDescriptionIsRecoverableFromTheBinding(t *testing.T) {
	// Which is why the descriptions are unexported: a caller who wants the
	// shape without the type already has it from the one exported thing, and
	// exporting the description as well would be the duplicate.
	recovered := schema.Dynamic(inventory.ItemSchema.Structure())

	value, err := schema.DecodeJSON(recovered, []byte(itemDocument))
	if err != nil {
		t.Fatal(err)
	}
	if sku, _ := value.(dynamic.Object).Member("sku"); sku != (dynamic.Text{Value: "ABC-12345"}) {
		t.Fatalf("unexpected member: %#v", sku)
	}
}
