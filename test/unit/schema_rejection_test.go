package unit

// What a schema refuses, and what it says about why. A decoder that rejects a
// document without naming the part responsible is not usable, so every case
// here checks the message as well as the refusal.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
)

func TestAMissingRequiredFieldIsRejectedAndNamed(t *testing.T) {
	_, err := schema.DecodeJSON(bookSchema, []byte(`{"title":"T","authors":[]}`))
	if err == nil {
		t.Fatal("expected a missing required field to be rejected")
	}
	path, isSchemaError := schema.PathOf(err)
	if !isSchemaError || !reflect.DeepEqual(path, []string{"pages"}) {
		t.Fatalf("expected the path to name the field, got %v from %v", path, err)
	}
}

func TestAMisshapenFieldIsRejectedWithItsPath(t *testing.T) {
	_, err := schema.DecodeJSON(bookSchema,
		[]byte(`{"title":"T","authors":["ok",7],"pages":1,"hasIndex":false}`))
	if err == nil {
		t.Fatal("expected a mis-shapen element to be rejected")
	}
	path, _ := schema.PathOf(err)
	if !reflect.DeepEqual(path, []string{"authors", "items.1"}) {
		t.Fatalf("expected the path to name the element, got %v from %v", path, err)
	}
	if !strings.Contains(err.Error(), "expected a string") {
		t.Fatalf("expected the reason to say what was wanted, got %v", err)
	}
}

func TestANonFiniteNumberIsRejectedRatherThanWrittenAsInvalidJSON(t *testing.T) {
	for _, value := range []float64{
		1 / func() float64 { return 0 }(),
		-1 / func() float64 { return 0 }(),
	} {
		if _, err := schema.EncodeJSON(schema.Float64(), value); err == nil {
			t.Fatalf("expected %v to be rejected", value)
		}
	}
}

func TestADuplicateFieldNameIsADeclarationMistakeReportedNotPanicked(t *testing.T) {
	broken := schema.Struct[Book]("Book",
		schema.FieldOf("title", schema.Text(),
			func(book Book) string { return book.Title },
			func(book *Book, title string) { book.Title = title }),
		schema.FieldOf("title", schema.Text(),
			func(book Book) string { return book.Subtitle },
			func(book *Book, title string) { book.Subtitle = title }),
	)

	// Building it neither panicked nor returned an error, exactly as building a
	// zero Effect does not.
	err := schema.Validate(broken)
	if err == nil {
		t.Fatal("expected Validate to report the duplicate")
	}
	if !strings.Contains(err.Error(), "two fields are named title") {
		t.Fatalf("expected the message to name the mistake, got %v", err)
	}
	if _, err := schema.EncodeJSON(broken, Book{}); err == nil {
		t.Fatal("expected using the schema to report the mistake too")
	}
}

func TestAFaultInAFieldsSchemaPropagatesWithItsPath(t *testing.T) {
	var unusable schema.Schema[string]
	outer := schema.Struct[Book]("Book",
		schema.FieldOf("title", unusable,
			func(book Book) string { return book.Title },
			func(book *Book, title string) { book.Title = title }),
	)

	err := schema.Validate(outer)
	if err == nil {
		t.Fatal("expected the inner fault to reach the outer schema")
	}
	if path, _ := schema.PathOf(err); !reflect.DeepEqual(path, []string{"title"}) {
		t.Fatalf("expected the path to name the field, got %v from %v", path, err)
	}
}

func TestATruncatedDocumentSaysSoRatherThanReportingAShapeMistake(t *testing.T) {
	_, err := schema.DecodeJSON(bookSchema, []byte(`{"title":"T","authors":[`))
	if err == nil {
		t.Fatal("expected a truncated document to be rejected")
	}
	if !strings.Contains(err.Error(), "document ended") {
		t.Fatalf("expected the reason to name truncation, got %v", err)
	}
}

func TestARefinementSaysWhichLayerRefusedAndKeepsItsOwnError(t *testing.T) {
	// "could not be processed at pages" tells a client nothing it can act on.
	// The refinement's own message is what says why, and errors.Is has to keep
	// reaching the sentinel it used so a caller can still branch on it.
	type entry struct{ Pages int }
	entrySchema := schema.Struct[entry]("Entry",
		schema.FieldOf("pages",
			schema.TransformOrFail(schema.Int(),
				func(pages int) (int, error) {
					if pages < 1 {
						return 0, errTooFew
					}
					return pages, nil
				},
				func(pages int) (int, error) { return pages, nil }),
			func(value entry) int { return value.Pages },
			func(value *entry, pages int) { value.Pages = pages }),
	)

	_, err := schema.DecodeJSON(entrySchema, []byte(`{"pages":0}`))
	if err == nil {
		t.Fatal("expected the refinement to refuse the value")
	}
	path, _ := schema.PathOf(err)
	if !reflect.DeepEqual(path, []string{"pages"}) {
		t.Fatalf("expected the path to name the field, got %v", path)
	}
	if !strings.Contains(err.Error(), "did not pass its refinement") ||
		!strings.Contains(err.Error(), "at least one page") {
		t.Fatalf("expected both the layer and the reason, got %v", err)
	}
	if !errors.Is(err, errTooFew) {
		t.Fatalf("expected the refinement's own error to stay reachable, got %v", err)
	}
}

var errTooFew = errors.New("an entry has at least one page")
