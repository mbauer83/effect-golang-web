package unit

// The generator. What matters is that it writes what the struct implies, that
// it writes the same thing every time, and that it refuses a struct it cannot
// describe instead of guessing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/internal/schemagen"
)

const fixtures = "testdata"

func generated(t *testing.T, fixture string) []byte {
	t.Helper()
	rendered, err := schemagen.Generate(filepath.Join(fixtures, fixture))
	if err != nil {
		t.Fatalf("%s: %v", fixture, err)
	}
	return rendered
}

func TestGenerationWritesWhatTheStructImplies(t *testing.T) {
	// The golden file is the whole of what generation produces for a struct
	// holding one of every shape, so a change to any part of the output has to
	// be looked at rather than noticed later.
	golden, err := os.ReadFile(filepath.Join(fixtures, "derived", "schema.go.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(generated(t, "derived")); got != string(golden) {
		t.Fatalf("the generated source is not the golden file:\n%s", got)
	}
}

func TestTheGoldenFileShowsEachDecisionTheGeneratorMakes(t *testing.T) {
	// Read as a checklist: each of these is a rule stated once in the output.
	rendered := string(generated(t, "derived"))
	rules := map[string]string{
		"a pointer is present-and-null":      `schema.Nullable(schema.Text())`,
		"a pointer with omitempty is absent": `schema.OptionalFieldOf("absent"`,
		"a named type uses its own schema":   `schema.FieldOf("nested", InnerSchema,`,
		"a use= tag wins over the table":     `schema.FieldOf("address", addressSchema,`,
		"an untagged field is lower-camel":   `schema.FieldOf("firstName"`,
		"a doc comment becomes prose":        `schema.DocumentedField("Text is a string."`,
		"only the first paragraph is prose":  `schema.Documented("Record is one of everything.",`,
	}
	for rule, evidence := range rules {
		if !strings.Contains(rendered, evidence) {
			t.Errorf("%s: expected %s in the output", rule, evidence)
		}
	}
	for rule, absent := range map[string]string{
		"a schema:\"-\" field is skipped":        `"ignored"`,
		"an unexported field is skipped":         `hidden`,
		"the rest of a doc comment stays behind": `whoever maintains the type`,
	} {
		if strings.Contains(rendered, absent) {
			t.Errorf("%s: did not expect %s in the output", rule, absent)
		}
	}
}

func TestGenerationRefusesWhatItCannotDescribe(t *testing.T) {
	cases := map[string]string{
		// A zero value is not absence. The schema refuses to guess that it is,
		// so a generator that guessed for it would be worse than an error.
		"optionalnotpointer": "is optional and not a pointer",
		// Guessing a shape would produce a schema that compiles and describes
		// the wrong thing, which is the one outcome worth refusing.
		"unsupported": "no schema for url.URL",
		// A union's alternatives are not in the interface's declaration.
		"notastruct": "is not a struct",
		"nomarks":    "declares no type marked",
	}
	for fixture, reason := range cases {
		_, err := schemagen.Generate(filepath.Join(fixtures, fixture))
		if err == nil {
			t.Errorf("%s: expected a refusal", fixture)
			continue
		}
		if !strings.Contains(err.Error(), reason) {
			t.Errorf("%s: expected %q, got %v", fixture, reason, err)
		}
	}
}

func TestARefusalNamesWhereToLook(t *testing.T) {
	// A generator that says only "unsupported type" leaves the reader to find
	// which field it meant.
	_, err := schemagen.Generate(filepath.Join(fixtures, "unsupported"))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "types.go:") {
		t.Fatalf("expected the position of the field, got %v", err)
	}
}

func TestGenerationIsDeterministicAcrossRuns(t *testing.T) {
	first := generated(t, "derived")
	second := generated(t, "derived")
	if string(first) != string(second) {
		t.Fatal("the same structs generated differently")
	}
}
