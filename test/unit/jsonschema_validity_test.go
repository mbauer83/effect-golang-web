package unit

// The projection and the codec are tested as a pair: a real JSON Schema
// validator compiles the emitted document and is asked about the documents the
// codec writes and the documents it refuses. Asserting the projection's shape
// only says it is the shape I expected; this says it means what the codec does.
//
// The validator is a test dependency. Nothing in the module needs it, which is
// why the projection is written rather than delegated.

import (
	"strings"
	"testing"
	"time"

	validator "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// compiled renders the projection of one structure and compiles it with a
// validator that has never seen this module.
func compiled(t *testing.T, node structure.Node) *validator.Schema {
	t.Helper()
	rendered, err := jsonschema.Project(node).Render()
	if err != nil {
		t.Fatal(err)
	}
	document, err := validator.UnmarshalJSON(strings.NewReader(string(rendered)))
	if err != nil {
		t.Fatalf("the emitted document is not JSON: %v\n%s", err, rendered)
	}

	compiler := validator.NewCompiler()
	if err := compiler.AddResource("emitted.json", document); err != nil {
		t.Fatalf("the emitted document is not a schema: %v\n%s", err, rendered)
	}
	compiledSchema, err := compiler.Compile("emitted.json")
	if err != nil {
		t.Fatalf("the emitted document does not compile: %v\n%s", err, rendered)
	}
	return compiledSchema
}

func instance(t *testing.T, document string) any {
	t.Helper()
	value, err := validator.UnmarshalJSON(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTheEmittedDocumentAcceptsWhatTheCodecWrites(t *testing.T) {
	written, err := schema.EncodeJSON(bookSchema, Book{
		Title:    "Zionomicon",
		Authors:  []string{"John A. De Goes"},
		Pages:    632,
		Subtitle: "A field guide",
		HasIndex: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled(t, bookSchema.Structure()).Validate(instance(t, string(written))); err != nil {
		t.Fatalf("the projection rejects what the codec wrote: %v\n%s", err, written)
	}
}

func TestTheEmittedDocumentAcceptsAnOmittedOptionalField(t *testing.T) {
	written, err := schema.EncodeJSON(bookSchema, Book{Title: "T", Authors: []string{}, Pages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled(t, bookSchema.Structure()).Validate(instance(t, string(written))); err != nil {
		t.Fatalf("the projection requires a field the codec omits: %v\n%s", err, written)
	}
}

func TestTheEmittedDocumentRefusesWhatTheCodecRefuses(t *testing.T) {
	emitted := compiled(t, bookSchema.Structure())
	cases := map[string]string{
		"a missing required field": `{"title":"T","authors":[]}`,
		"a mis-typed field":        `{"title":"T","authors":[],"pages":"many","hasIndex":false}`,
		"a mis-typed element":      `{"title":"T","authors":[7],"pages":1,"hasIndex":false}`,
	}
	for mistake, document := range cases {
		if err := emitted.Validate(instance(t, document)); err == nil {
			t.Errorf("the projection accepts %s, which the codec refuses", mistake)
		}
	}
}

func TestTheEmittedDocumentToleratesAnUnknownFieldAsTheCodecDoes(t *testing.T) {
	// A decoder that refused an unknown field could not read a document written
	// by a newer producer, so the published contract must not refuse one either.
	document := `{"title":"T","authors":[],"pages":1,"hasIndex":false,"isbn":"x"}`
	if err := compiled(t, bookSchema.Structure()).Validate(instance(t, document)); err != nil {
		t.Fatalf("the projection refuses a field the codec tolerates: %v", err)
	}
	if _, err := schema.DecodeJSON(bookSchema, []byte(document)); err != nil {
		t.Fatalf("the codec refuses a field the projection tolerates: %v", err)
	}
}

func TestTheEmittedUnionAgreesWithTheCodecOnEveryCase(t *testing.T) {
	emitted := compiled(t, shapeSchema.Structure())
	accepted := map[string]string{
		"a named variant":   `{"circle":{"radius":2}}`,
		"the other variant": `{"rectangle":{"width":1,"height":2}}`,
	}
	for description, document := range accepted {
		if err := emitted.Validate(instance(t, document)); err != nil {
			t.Errorf("the projection refuses %s: %v", description, err)
		}
		if _, err := schema.DecodeJSON(shapeSchema, []byte(document)); err != nil {
			t.Errorf("the codec refuses %s: %v", description, err)
		}
	}

	refused := map[string]string{
		"no variant":         `{}`,
		"an unknown variant": `{"triangle":{"base":1}}`,
		"two variants":       `{"circle":{"radius":1},"rectangle":{"width":1,"height":2}}`,
	}
	for description, document := range refused {
		if err := emitted.Validate(instance(t, document)); err == nil {
			t.Errorf("the projection accepts %s, which the codec refuses", description)
		}
		if _, err := schema.DecodeJSON(shapeSchema, []byte(document)); err == nil {
			t.Errorf("the codec accepts %s, which the projection refuses", description)
		}
	}
}

// everyShapeDocument is what the codec writes for the whole-contract fixture,
// which is the document the emitted schema has to accept.
func everyShapeDocument(t *testing.T, missing *string) string {
	t.Helper()
	written, err := schema.EncodeJSON(everyShapeSchema, everyShape{
		Text:     "text",
		Whole:    7,
		Wide:     1 << 40,
		Fraction: 1.5,
		Flag:     true,
		Opaque:   []byte{1, 2, 3},
		Moment:   time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC),
		Labels:   map[string]string{"a": "one"},
		Missing:  missing,
		Nested:   []int{1, 2},
		Detail:   Detail{Note: "kept"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(written)
}

func TestTheEmittedDocumentAdmitsNullWhereTheCodecWritesIt(t *testing.T) {
	// A nullable value is present and null. The projection has to say so, or it
	// describes a document the codec is free to produce and it would refuse.
	emitted := compiled(t, everyShapeSchema.Structure())
	held := "held"
	for _, missing := range []*string{nil, &held} {
		document := everyShapeDocument(t, missing)
		if err := emitted.Validate(instance(t, document)); err != nil {
			t.Errorf("the projection refuses what the codec wrote: %v\n%s", err, document)
		}
	}
}

func TestTheEmittedDocumentRefusesNullWhereTheCodecWould(t *testing.T) {
	emitted := compiled(t, everyShapeSchema.Structure())
	broken := strings.Replace(everyShapeDocument(t, nil), `"text":"text"`, `"text":null`, 1)

	if err := emitted.Validate(instance(t, broken)); err == nil {
		t.Error("the projection admits null in a field that is not nullable")
	}
	if _, err := schema.DecodeJSON(everyShapeSchema, []byte(broken)); err == nil {
		t.Error("the codec admits null in a field that is not nullable")
	}
}
