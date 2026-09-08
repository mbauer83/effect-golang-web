package unit

// A schema with no Go type. The same vocabulary, used without an A: it
// validates, transcodes, describes itself and composes, which is what a
// description has to do to be worth generating a struct from.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// bookDescription is written before any Go type for it exists. It is Struct
// without the accessors, which is the only part of a field declaration that
// needs the type.
var bookDescription = schema.Struct[dynamic.Value]("Book",
	schema.DescribedField("title", schema.MinLength(schema.Text(), 1)).
		Documented("what the book is called"),
	schema.DescribedField("authors", schema.MinItems(schema.List(schema.Text()), 1)),
	schema.DescribedField("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000)),
	schema.DescribedField("subtitle", schema.Text()).Optional(),
	schema.DescribedField("id", schema.UUID()),
)

const bookDocument = `{"title":"Zionomicon","authors":["John A. De Goes"],` +
	`"pages":632,"id":"123e4567-e89b-12d3-a456-426614174000"}`

func TestADescriptionWithNoGoTypeTranscodes(t *testing.T) {
	if err := schema.Validate(bookDescription); err != nil {
		t.Fatal(err)
	}
	read, err := schema.DecodeJSON(bookDescription, []byte(bookDocument))
	if err != nil {
		t.Fatal(err)
	}
	written, err := schema.EncodeJSON(bookDescription, read)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(written)); got != bookDocument {
		t.Fatalf("expected\n  %s\ngot\n  %s", bookDocument, got)
	}
}

func TestADescriptionWithNoGoTypeYieldsAValueThatCanBeRead(t *testing.T) {
	read, err := schema.DecodeJSON(bookDescription, []byte(bookDocument))
	if err != nil {
		t.Fatal(err)
	}
	object, isObject := read.(dynamic.Object)
	if !isObject {
		t.Fatalf("expected an object, got %#v", read)
	}
	title, present := object.Member("title")
	if !present || title != (dynamic.Text{Value: "Zionomicon"}) {
		t.Fatalf("expected the title, got %#v", title)
	}
	// An absent optional member is not there, rather than being there as null.
	if _, present := object.Member("subtitle"); present {
		t.Fatal("expected the absent member to be absent")
	}
	// Members come back in the order the description declares, not the order
	// the document carried them.
	names := []string{}
	for _, field := range object.Fields {
		names = append(names, field.Name)
	}
	if !reflect.DeepEqual(names, []string{"title", "authors", "pages", "id"}) {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestADescriptionEnforcesEveryRuleItRecords(t *testing.T) {
	// This is what recording a constraint in the description was for: the rules
	// survive without the Go type that stated them.
	for reason, document := range map[string]string{
		"a title below its minimum length": `{"title":"","authors":["A"],"pages":1,"id":"123e4567-e89b-12d3-a456-426614174000"}`,
		"no authors at all":                `{"title":"T","authors":[],"pages":1,"id":"123e4567-e89b-12d3-a456-426614174000"}`,
		"a page count below its bound":     `{"title":"T","authors":["A"],"pages":0,"id":"123e4567-e89b-12d3-a456-426614174000"}`,
		"a page count above its bound":     `{"title":"T","authors":["A"],"pages":20001,"id":"123e4567-e89b-12d3-a456-426614174000"}`,
		"an identifier that is not one":    `{"title":"T","authors":["A"],"pages":1,"id":"not-a-uuid"}`,
		"a missing required member":        `{"title":"T","authors":["A"],"pages":1}`,
	} {
		if _, err := schema.DecodeJSON(bookDescription, []byte(document)); err == nil {
			t.Errorf("expected %s to be refused", reason)
		}
	}
}

func TestADescriptionAndItsTypedTwinAgree(t *testing.T) {
	// The typed path enforces a bound because each combinator knows the Go type
	// it measures; the dynamic path enforces it from what was recorded. They
	// have to reach the same verdict, or a generated struct would accept what
	// its description refuses.
	typed := schema.AtMost(schema.AtLeast(schema.Int(), 1), 10)
	described := schema.Dynamic(typed.Structure())

	for _, document := range []string{"0", "1", "5", "10", "11"} {
		_, typedErr := schema.DecodeJSON(typed, []byte(document))
		_, dynamicErr := schema.DecodeJSON(described, []byte(document))
		if (typedErr == nil) != (dynamicErr == nil) {
			t.Errorf("%s: typed says %v, described says %v", document, typedErr, dynamicErr)
		}
	}
}

func TestADescriptionSaysWhatItCannotEnforce(t *testing.T) {
	// A rule that could only be code has nothing recorded, so a description
	// alone cannot enforce it. Saying so beats a dynamic path that silently
	// admits what the typed path refuses.
	typed := schema.Email()
	described := schema.Dynamic(typed.Structure())

	if _, err := schema.DecodeJSON(typed, []byte(`"ada"`)); err == nil {
		t.Fatal("expected the typed schema to refuse the address")
	}
	if _, err := schema.DecodeJSON(described, []byte(`"ada"`)); err != nil {
		t.Fatalf("expected the description to admit what it cannot check, got %v", err)
	}

	// Where the rule is an expression, it was recorded, and the description
	// does enforce it.
	if _, err := schema.DecodeJSON(schema.Dynamic(schema.UUID().Structure()),
		[]byte(`"not-a-uuid"`)); err == nil {
		t.Fatal("expected a recorded expression to be enforced without the type")
	}
}

func TestADescriptionComposesWithTypedSchemas(t *testing.T) {
	// One vocabulary: a description is a Schema, so every combinator takes it.
	catalogue := schema.List(bookDescription)

	read, err := schema.DecodeJSON(catalogue, []byte("["+bookDocument+"]"))
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 {
		t.Fatalf("expected one entry, got %d", len(read))
	}
}

func TestATypedValueCrossesToADescriptionAndBack(t *testing.T) {
	// The optional field is present, so the crossing has to ask whether the
	// value is null and still find it there afterwards.
	original := Book{
		Title:    "T",
		Authors:  []string{"A"},
		Pages:    1,
		Subtitle: "A field guide",
		HasIndex: true,
	}

	crossed, err := schema.ToDynamic(bookSchema, original)
	if err != nil {
		t.Fatal(err)
	}
	object, isObject := crossed.(dynamic.Object)
	if !isObject {
		t.Fatalf("expected an object, got %#v", crossed)
	}
	if pages, _ := object.Member("pages"); pages != (dynamic.Integer{Value: 1}) {
		t.Fatalf("expected the page count, got %#v", pages)
	}
	if subtitle, present := object.Member("subtitle"); !present ||
		subtitle != (dynamic.Text{Value: "A field guide"}) {
		t.Fatalf("expected the present optional member, got %#v", subtitle)
	}

	back, err := schema.FromDynamic(bookSchema, crossed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, original) {
		t.Fatalf("the crossing changed the value:\n  before %#v\n  after  %#v", original, back)
	}
}

func TestADescriptionsMistakesAreReportedRatherThanPanicking(t *testing.T) {
	cases := map[string]error{
		"a nameless member":  schema.Validate(schema.Struct[dynamic.Value]("Book", schema.DescribedField("", schema.Text()))),
		"two members alike":  schema.Validate(schema.Struct[dynamic.Value]("Book", schema.DescribedField("title", schema.Text()), schema.DescribedField("title", schema.Text()))),
		"an unusable member": schema.Validate(schema.Struct[dynamic.Value]("Book", schema.DescribedField("title", schema.Schema[string]{}))),
		// A faulted schema has a shape as well as a fault, so a description
		// that took the shape and dropped the fault would look complete.
		"a member whose schema is faulted": schema.Validate(
			schema.Struct[dynamic.Value]("Book", schema.DescribedField("title", schema.Matching(schema.Text(), `[`)))),
		"no description":  schema.Validate(schema.Dynamic(nil)),
		"no alternatives": schema.Validate(schema.OneOf[dynamic.Value]("Shape")),
		// A bound field's getter returns a value and not whether there is one,
		// so it cannot be made optional after the fact.
		"a bound field made optional": schema.Validate(schema.Struct[Book]("Book",
			schema.FieldOf("title", schema.Text(),
				func(book Book) string { return book.Title },
				func(book *Book, title string) { book.Title = title }).Optional())),
	}
	for mistake, err := range cases {
		if err == nil {
			t.Errorf("expected %s to be reported", mistake)
		}
	}
}

func TestPassingADescriptionThroughDynamicLeavesItUnchanged(t *testing.T) {
	// Dynamic rebuilds the description as combinators to get a codec. What it
	// must not do is hand back the rebuilt description: the rebuild works in
	// the wire's two numeric shapes, so a recorded width would be lost and a
	// generator reading it would emit int64 where the author wrote uint16.
	original := schema.Uint16().Structure()
	round := schema.Dynamic(original).Structure()

	before := original.(structure.Scalar)
	after, isScalar := round.(structure.Scalar)
	if !isScalar {
		t.Fatalf("expected a scalar, got %#v", round)
	}
	if after.Precision != before.Precision {
		t.Fatalf("expected the width kept, got %v from %v", after.Precision, before.Precision)
	}
	if len(after.Constraints) != len(before.Constraints) {
		t.Fatalf("expected the constraints kept, got %d of %d",
			len(after.Constraints), len(before.Constraints))
	}
}
