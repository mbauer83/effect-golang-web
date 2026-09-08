package architecture

// Nothing in this module holds a value it cannot name -- with four exceptions,
// each on the record and each checked to stay where it says it is.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A description is fully typed. Schema[A] carries its type and structure.Node
// is a sealed interface a projection switches over exhaustively, so nothing has
// to hold a value it cannot name. The easy way to build a JSON Schema is a
// map[string]any, which is why this is the invariant most likely to be lost by
// convenience.
//
// The check is over declarations. A type argument -- Schema[any] -- would slip
// past it, which is why the bans above stay as well.

// untypedBoundaries are the files where a top type is allowed. Each is a place
// where something outside this module is untyped and something has to meet it,
// and each is confined to one file whose whole subject is crossing it. A list
// is a decision on the record rather than a hole someone widened -- and a list
// that grows is a thing a reviewer sees in the diff.
var untypedBoundaries = map[string]string{
	// database/sql scans into a top type and a driver hands one back, because
	// a driver cannot know what a column holds until it reads it.
	"sql/driver_values.go": "a driver's values",
	// An AMQP field table is a set of named values of a dozen kinds, which the
	// protocol defines and the library represents as map[string]any.
	"amqp091/field_values.go": "a message's headers",
	// AMQP 1.0's application properties and annotations are the same shape
	// under a different protocol, with a different set of permitted values.
	"amqp10/property_values.go": "a message's properties",
	// Connect's Codec contract is untyped, because a codec is registered for a
	// content type and marshals whatever message its procedure takes.
	"grpc/connect_codec.go": "a gRPC codec's messages",
}

func TestNoDescriptionEscapesIntoATopType(t *testing.T) {
	typeParameters := regexp.MustCompile(`\[[\w,\s]*any[\w,\s]*\]`)
	topType := regexp.MustCompile(`\binterface\{\}|\bany\b`)

	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || exempt(path) {
			return nil
		}
		for number, line := range strings.Split(readSource(t, path), "\n") {
			code, _, _ := strings.Cut(line, "//")
			code = typeParameters.ReplaceAllString(code, "")
			if topType.MatchString(code) {
				t.Errorf("%s:%d uses a top type: %s",
					display(t, path), number+1, strings.TrimSpace(line))
			}
		}
		return nil
	}
	if err := filepath.WalkDir(moduleRoot(t), walk); err != nil {
		t.Fatal(err)
	}
}

func exempt(path string) bool {
	for boundary := range untypedBoundaries {
		if strings.HasSuffix(filepath.ToSlash(path), boundary) {
			return true
		}
	}
	return false
}

// Each exemption has to stay one file, and it has to stay used. A boundary that
// moved would take its exemption with it silently; one that was no longer
// needed would leave a licence nobody was exercising.
func TestEveryUntypedBoundaryIsWhereItSaysItIs(t *testing.T) {
	for boundary, subject := range untypedBoundaries {
		source, err := os.ReadFile(filepath.Join(moduleRoot(t), boundary))
		if err != nil {
			t.Errorf("%s is exempt from the top-type ban, for %s, and does not exist: %v",
				boundary, subject, err)
			continue
		}
		if !strings.Contains(string(source), " any)") && !strings.Contains(string(source), "]any") {
			t.Errorf("%s is exempt from the top-type ban, for %s, and does not use one",
				boundary, subject)
		}
	}
}
