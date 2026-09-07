package acceptance

// The catalogue program end to end: a document on disk becomes a value, the
// value becomes a document again, and the same schema becomes the contract that
// says what will be accepted. What is checked is that all three agree.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/examples/catalog"
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang/effect"
)

const catalogDocument = `{
  "name": "shelf one",
  "books": [
    {"title": "Zionomicon", "authors": ["John A. De Goes", "Adam Fraser"],
     "pages": 632, "subtitle": "A field guide",
     "availability": {"inStock": {"count": 3}}},
    {"title": "Effect in Practice", "authors": ["Michael Arnaldi"],
     "pages": 410, "availability": {"awaited": {"expected": "2026-11-01T00:00:00Z"}}},
    {"title": "Out of Print", "authors": [], "pages": 120,
     "availability": {"discontinued": {}}}
  ]
}`

// run interprets the program over live capabilities and returns its exit.
func run(t *testing.T, document string) (string, effect.Exit[catalog.Fault, catalog.Report]) {
	t.Helper()
	workspace := t.TempDir()
	inputPath := filepath.Join(workspace, "catalogue.json")
	if err := os.WriteFile(inputPath, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	program := catalog.Program(inputPath,
		filepath.Join(workspace, "normalised.json"),
		filepath.Join(workspace, "contract.json"))
	exit := runtime.Run(context.Background(), effect.Unit{}, program)

	if live := runtime.LiveWork(); !live.IsEmpty() {
		t.Fatalf("the program left work behind: %#v", live)
	}
	return workspace, exit
}

func TestTheCatalogueProgramLoadsNormalisesAndPublishes(t *testing.T) {
	workspace, exit := run(t, catalogDocument)
	report, succeeded := exit.Value()
	if !succeeded {
		t.Fatalf("unexpected exit: %+v", exit)
	}

	if report.Books != 3 || report.Shelved != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	// Every named shape is described once and referred to thereafter, which is
	// what makes one description serve a document used by many endpoints.
	if names := strings.Join(report.Components, ","); names !=
		"Availability,Awaited,Book,Catalog,Discontinued,InStock" {
		t.Fatalf("unexpected components: %s", names)
	}
	for _, name := range []string{"normalised.json", "contract.json"} {
		written, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(written) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

func TestTheNormalisedDocumentDecodesToTheSameValue(t *testing.T) {
	// The round trip is checked on the bytes the program actually wrote, not on
	// an in-memory value, because that is what a downstream consumer reads.
	workspace, exit := run(t, catalogDocument)
	if _, succeeded := exit.Value(); !succeeded {
		t.Fatalf("unexpected exit: %+v", exit)
	}

	written, err := os.ReadFile(filepath.Join(workspace, "normalised.json"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := schema.DecodeJSON(catalog.Schema, written)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "shelf one" || len(loaded.Books) != 3 {
		t.Fatalf("unexpected value: %#v", loaded)
	}
	// The optional field was absent for the second entry and must stay absent,
	// rather than becoming an empty string that the document then asserts.
	if strings.Contains(string(written), `"subtitle":""`) {
		t.Fatalf("an absent field was written as empty:\n%s", written)
	}
}

func TestARejectedDocumentNamesTheStageAndTheField(t *testing.T) {
	broken := strings.Replace(catalogDocument, `"pages": 410`, `"pages": 0`, 1)
	_, exit := run(t, broken)

	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the document to be rejected, got %+v", exit)
	}
	failure, isTypedFailure := cause.Failure()
	if !isTypedFailure {
		t.Fatalf("expected a typed failure rather than a defect, got %+v", cause)
	}
	if failure.Stage != "decoding the catalogue" {
		t.Fatalf("expected the stage named, got %#v", failure)
	}
	// The typed failure is one type at the boundary and still carries the
	// schema failure underneath it, so a handler can report which field.
	path, isSchemaFailure := schema.PathOf(failure)
	if !isSchemaFailure ||
		strings.Join(path, ".") != "books.items.1.pages" {
		t.Fatalf("expected the path to reach the field, got %v from %v", path, failure)
	}
	if !errors.Is(failure, failure.Err) {
		t.Fatalf("expected the underlying error to stay reachable, got %#v", failure)
	}
}
