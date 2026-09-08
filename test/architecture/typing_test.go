package architecture

// Nothing in this module holds a value it cannot name -- with one exception, on
// the record and checked to stay where it says it is.

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
// One file is exempt, and the exemption is a decision on the record rather than
// a hole someone widened: database/sql scans into a top type and a driver hands
// one back, because a driver cannot know what a column holds until it reads it.
// That boundary is real, so it is confined to one file whose whole subject is
// crossing it.
//
// The check is over declarations. A type argument -- Schema[any] -- would slip
// past it, which is why the bans above stay as well.
// driverBoundary is the one file where a top type is allowed, because the
// standard library's row scanning is untyped and something has to meet it.
const driverBoundary = "sql/driver_values.go"

func TestNoDescriptionEscapesIntoATopType(t *testing.T) {
	typeParameters := regexp.MustCompile(`\[[\w,\s]*any[\w,\s]*\]`)
	topType := regexp.MustCompile(`\binterface\{\}|\bany\b`)

	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, driverBoundary) {
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

// The exemption has to stay one file, and it has to stay used. A boundary that
// moved would take the exemption with it silently; one that was no longer
// needed would leave a licence nobody was exercising.
func TestTheOneUntypedBoundaryIsWhereItSaysItIs(t *testing.T) {
	boundary := filepath.Join(moduleRoot(t), driverBoundary)
	source, err := os.ReadFile(boundary)
	if err != nil {
		t.Fatalf("%s is exempt from the top-type ban and does not exist: %v", driverBoundary, err)
	}
	if !strings.Contains(string(source), " any)") && !strings.Contains(string(source), "]any") {
		t.Errorf("%s is exempt from the top-type ban and does not use one", driverBoundary)
	}
}
