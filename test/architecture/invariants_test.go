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
var forbiddenImports = map[string][]string{
	"schema": {
		// A generator reads a description; a description knows nothing about
		// the generator that will read it.
		"effect-golang-web/schemagen",
		"effect-golang-web/web",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp",
		"effect-golang-web/grpc",
		"effect-golang-web/sql",
		"effect-golang-web/schema/jsonschema",
	},
	"schema/structure": {
		"effect-golang-web/schema\"",
		"effect-golang-web/web",
	},
	"websocket": {
		// A transport knows the core and no other transport.
		"effect-golang-web/amqp",
		"effect-golang-web/grpc",
		"effect-golang-web/sql",
		"effect-golang-web/openapi",
	},
	"web": {
		// A projection depends on the declaration, never the other way round.
		"effect-golang-web/openapi",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp",
		"effect-golang-web/grpc",
		"effect-golang-web/sql",
	},
	"schemagen": {
		"effect-golang-web/web",
		"effect-golang-web/openapi",
	},
	"openapi": {
		"effect-golang-web/websocket",
		"effect-golang-web/amqp",
		"effect-golang-web/grpc",
		"effect-golang-web/sql",
	},
	"schema/jsonschema": {
		// A projection walks the description. Reaching for the codec package
		// would let one projection depend on how another format encodes.
		"effect-golang-web/schema\"",
		"effect-golang-web/web",
		"effect-golang-web/websocket",
		"effect-golang-web/amqp",
		"effect-golang-web/grpc",
		"effect-golang-web/sql",
	},
}

func TestPackagesDependOnlyInward(t *testing.T) {
	for directory, banned := range forbiddenImports {
		for _, path := range sourcesIn(t, directory) {
			source := readSource(t, path)
			for _, importPath := range banned {
				if strings.Contains(source, importPath) {
					t.Errorf("%s imports %q, which points outward", display(t, path), importPath)
				}
			}
		}
	}
}

// The HTTP core must be usable without acquiring anyone else's dependency, so
// no third-party import may appear outside the transport that needs it.
var thirdPartyAllowedIn = map[string]bool{
	"websocket": true,
	"amqp":      true,
	"grpc":      true,
	"sql":       true,
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
