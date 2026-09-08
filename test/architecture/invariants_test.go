package architecture

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Dependencies point one way. Schema knows nothing about HTTP, a projection
// knows nothing about a transport, and no transport knows about another. That
// is what keeps a caller who wants only the HTTP core from acquiring an AMQP
// dependency.
// A transport knows the core and no other transport; a projection knows the
// declaration and never the other way round.
//
// The description, the tables and the migrations are other modules now, so the
// edges to them are enforced by the compiler rather than by this: nothing here
// can import effect-golang-sql without go.mod saying so, and effect-golang-sql
// cannot import this at all.
var forbiddenImports = map[string][]string{
	"web": {
		// A projection depends on the declaration, never the other way round.
		"effect-golang-web/openapi",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp091",
		"effect-golang-web/amqp10",
		"effect-golang-web/grpc",
		// A database is not this module's business at all. The edge is here as
		// well as in go.mod because go.mod would happily let a transport reach
		// for one, and a transport that did would make this module unusable
		// without a driver.
		"effect-golang-sql",
	},
	"openapi": {
		"effect-golang-web/websocket",
		"effect-golang-web/amqp091",
		"effect-golang-web/amqp10",
		"effect-golang-web/grpc",
	},
	"websocket": {
		"effect-golang-web/openapi",
		"effect-golang-web/amqp091",
		"effect-golang-web/amqp10",
		"effect-golang-web/grpc",
	},
	"amqp091": {
		"effect-golang-web/web",
		"effect-golang-web/openapi",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp10",
		"effect-golang-web/grpc",
	},
	"amqp10": {
		"effect-golang-web/web",
		"effect-golang-web/openapi",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp091",
		"effect-golang-web/grpc",
	},
	"grpc": {
		"effect-golang-web/openapi",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp091",
		"effect-golang-web/amqp10",
	},
}

func TestPackagesDependOnlyInward(t *testing.T) {
	for directory, banned := range forbiddenImports {
		for _, path := range sourcesIn(t, directory) {
			for _, line := range strings.Split(readSource(t, path), "\n") {
				held, isImport := imported(line)
				if !isImport {
					continue
				}
				for _, forbidden := range banned {
					if strings.Contains(held, forbidden) {
						t.Errorf("%s imports %q, which points outward",
							display(t, path), held)
					}
				}
			}
		}
	}
}

// imported is the path an import line names.
//
// Only an import line, because a doc comment may perfectly well *mention*
// another package -- a link to the sibling protocol, say -- and the earlier
// version of this test read the whole file and would have called that a
// dependency.
func imported(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") {
		return "", false
	}
	start := strings.Index(trimmed, `"`)
	if start < 0 || !strings.HasSuffix(trimmed, `"`) {
		return "", false
	}
	held := strings.Trim(trimmed[start:], `"`)
	if !strings.Contains(held, "/") && !strings.Contains(held, ".") {
		return "", false
	}
	return held, true
}

// The HTTP core must be usable without acquiring anyone else's dependency, so
// no third-party import may appear outside the transport that needs it.
var thirdPartyAllowedIn = map[string]bool{
	"websocket": true,
	"amqp091":   true,
	"amqp10":    true,
	"grpc":      true,
}

func TestOnlyATransportCarriesItsOwnDependency(t *testing.T) {
	root := moduleRoot(t)
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		top, _, _ := strings.Cut(filepath.ToSlash(relative), "/")
		if thirdPartyAllowedIn[top] || top == "test" || top == "examples" {
			return nil
		}
		for _, line := range strings.Split(readSource(t, path), "\n") {
			if third, found := thirdPartyImport(line); found {
				t.Errorf("%s imports %q; only a transport may carry a dependency",
					relative, third)
			}
		}
		return nil
	}
	if err := filepath.WalkDir(root, walk); err != nil {
		t.Fatal(err)
	}
}

// thirdPartyImport reports an import that is neither the standard library nor
// this project. A standard-library path has no dot before its first slash.
func thirdPartyImport(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	start := strings.Index(trimmed, `"`)
	if start < 0 || !strings.HasSuffix(trimmed, `"`) {
		return "", false
	}
	path := strings.Trim(trimmed[start:], `"`)
	host, _, hasSlash := strings.Cut(path, "/")
	if !hasSlash || !strings.Contains(host, ".") {
		return "", false
	}
	if strings.HasPrefix(path, "github.com/mbauer83/") {
		return "", false
	}
	return path, true
}

// The module root is for project metadata and documentation. Every package is a
// directory, for the reason the runtime's is: a root full of source files is
// not a layout.
func TestModuleRootHoldsNoSource(t *testing.T) {
	for _, path := range sourcesIn(t, ".") {
		t.Errorf("%s sits in the module root; source belongs in a package directory", display(t, path))
	}
}

// Go source files here have a 250-line soft limit and a 350-line hard limit,
// as they do in the runtime. Raising either bound is a deliberate edit to this
// test rather than something a growing file can do by drifting past a review.
const (
	softLineLimit = 250
	hardLineLimit = 350
)

func TestSourceFilesStayWithinTheirLineLimits(t *testing.T) {
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		lines := strings.Count(readSource(t, path), "\n") + 1
		if lines <= softLineLimit {
			return nil
		}
		limit := "soft"
		if lines > hardLineLimit {
			limit = "hard"
		}
		t.Errorf("%s has %d lines, past the %s limit; split it by domain role",
			display(t, path), lines, limit)
		return nil
	}
	if err := filepath.WalkDir(moduleRoot(t), walk); err != nil {
		t.Fatal(err)
	}
}

// Documentation the index does not reach is documentation nobody reads.
func TestEveryPublicDocumentIsLinkedFromTheReadme(t *testing.T) {
	root := moduleRoot(t)
	readme := readSource(t, filepath.Join(root, "README.md"))

	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if !strings.Contains(readme, filepath.ToSlash(relative)) {
			t.Errorf("%s is not linked from README.md", relative)
		}
		return nil
	}
	if err := filepath.WalkDir(filepath.Join(root, "docs"), walk); err != nil {
		t.Fatal(err)
	}
}
