package unit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

var errTraceExhausted = errors.New("the trace ended early")

func errTraceWanted(wanted string, found string) error {
	return errors.New("wanted " + wanted + ", found " + found)
}

func TestOneDescriptionServesASecondFormat(t *testing.T) {
	// Nothing in the schema knows about this format, and nothing in the format
	// knows about Book. The description is the only thing they share.
	original := Book{
		Title:    "Zionomicon",
		Authors:  []string{"John A. De Goes", "Adam Fraser"},
		Pages:    632,
		Subtitle: "A field guide",
		HasIndex: true,
	}
	_, decoded, err := traced(bookSchema, original)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", original, decoded)
	}
}

func TestTheTraversalASchemaDrivesIsTheOneAFormatMustImplement(t *testing.T) {
	tokens, _, err := traced(bookSchema, Book{Title: "T", Authors: []string{"a"}, Pages: 1})
	if err != nil {
		t.Fatal(err)
	}
	traversal := strings.Join(tokens, " ")
	// An absent optional field contributes nothing at all, which is what makes
	// omission rather than null the schema's answer in every format.
	want := "{ k:title s:T k:authors [ s:a ] k:pages i:1 k:hasIndex b:false }"
	if traversal != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, traversal)
	}
}

func TestEveryShapeReachesTheSecondFormat(t *testing.T) {
	// One value per Sink call, so a format author who passes this has
	// implemented the whole contract rather than the parts JSON exercises.
	moment := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	sample := everyShape{
		Text:     "text",
		Whole:    7,
		Wide:     1 << 40,
		Fraction: 1.5,
		Flag:     true,
		Opaque:   []byte{1, 2, 3},
		Moment:   moment,
		Labels:   map[string]string{"b": "two", "a": "one"},
		Missing:  nil,
		Nested:   []int{1, 2},
		Detail:   Detail{Note: "kept"},
	}
	tokens, decoded, err := traced(everyShapeSchema, sample)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, sample) {
		t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", sample, decoded)
	}
	for _, call := range []string{"s:", "i:", "n:", "b:", "y:", "t:", "null", "{", "k:", "}", "[", "]"} {
		if !strings.Contains(strings.Join(tokens, " "), call) {
			t.Errorf("no value exercised %q", call)
		}
	}
}

func TestTheSecondFormatSkipsAnUnknownFieldToo(t *testing.T) {
	// Tolerance is the schema's rule, not JSON's, so it must hold wherever the
	// tokens came from -- including over a nested value.
	source := &traceSource{tokens: []string{
		"{", "k:title", "s:T", "k:notes", "{", "k:any", "[", "s:x", "]", "}",
		"k:authors", "[", "]", "k:pages", "i:1", "k:hasIndex", "b:false", "}",
	}}
	decoded, err := schema.Decode(bookSchema, source)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Title != "T" || decoded.Pages != 1 {
		t.Fatalf("unexpected value: %#v", decoded)
	}
}

func TestASecondFormatsFailureCarriesTheSchemaPath(t *testing.T) {
	source := &traceSource{tokens: []string{"{", "k:title", "i:7", "}"}}
	_, err := schema.Decode(bookSchema, source)
	if err == nil {
		t.Fatal("expected the format's own refusal to be reported")
	}
	path, isSchemaFailure := schema.PathOf(err)
	if !isSchemaFailure || !reflect.DeepEqual(path, []string{"title"}) {
		t.Fatalf("expected the path to name the field, got %v from %v", path, err)
	}
}
