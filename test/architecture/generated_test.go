package architecture

// Generated code is checked in, so it can drift from the structs it came from.
// This regenerates it in process and compares: a struct changed without a
// regeneration fails here rather than at the next request that met the stale
// shape.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/examples/inventory/definitions"
	"github.com/mbauer83/effect-golang-web/schemagen"
)

func firstDifference(checkedIn string, regenerated string) string {
	was := strings.Split(checkedIn, "\n")
	is := strings.Split(regenerated, "\n")
	for index := range max(len(was), len(is)) {
		if line(was, index) != line(is, index) {
			return "  line " + strconv.Itoa(index+1) + "\n    checked in:  " + line(was, index) +
				"\n    regenerated: " + line(is, index)
		}
	}
	return "  the files differ only in length"
}

func line(lines []string, index int) string {
	if index >= len(lines) {
		return "(end of file)"
	}
	return lines[index]
}

func TestGeneratedBindingsMatchTheDescriptionsTheyCameFrom(t *testing.T) {
	// The other direction: a description is the source of truth and the Go
	// types are generated from it. The generated file is compiled as part of
	// this module, so the compiler checks it and this checks it is current.
	checkedIn, err := os.ReadFile(filepath.Join(moduleRoot(t),
		"examples", "inventory", schemagen.BindingsFileName))
	if err != nil {
		t.Fatal(err)
	}
	regenerated, err := schemagen.WriteBindings("inventory", definitions.Descriptions()...)
	if err != nil {
		t.Fatal(err)
	}
	if string(regenerated) != string(checkedIn) {
		t.Fatalf("the bindings are not what the descriptions imply; run go generate ./...\n%s",
			firstDifference(string(checkedIn), string(regenerated)))
	}
}

func TestBindingGenerationIsDeterministic(t *testing.T) {
	first, err := schemagen.WriteBindings("inventory", definitions.Descriptions()...)
	if err != nil {
		t.Fatal(err)
	}
	second, err := schemagen.WriteBindings("inventory", definitions.Descriptions()...)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("the same descriptions generated differently")
	}
}
