// Command webdemo runs the example scenarios against live capabilities.
//
// It exists so the examples are demonstrably runnable programs and not only
// test fixtures. Each scenario is also composed by an end-to-end test, so the
// two cannot drift apart.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mbauer83/effect-golang-web/examples/catalog"
	"github.com/mbauer83/effect-golang/effect"
)

const document = `{
  "name": "shelf one",
  "books": [
    {"title": "Zionomicon", "authors": ["John A. De Goes", "Adam Fraser"],
     "pages": 632, "subtitle": "A field guide",
     "availability": {"inStock": {"count": 3}}},
    {"title": "Out of Print", "authors": [], "pages": 120,
     "availability": {"discontinued": {}}}
  ]
}`

func main() {
	workspace, err := os.MkdirTemp("", "webdemo")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(workspace)

	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		fail(err)
	}
	defer reportShutdown(runtime)

	runCatalog(runtime, workspace)
}

func runCatalog(runtime *effect.Runtime, workspace string) {
	inputPath := filepath.Join(workspace, "catalogue.json")
	if err := os.WriteFile(inputPath, []byte(document), 0o600); err != nil {
		fail(err)
	}
	contractPath := filepath.Join(workspace, "contract.json")

	program := catalog.Program(inputPath, filepath.Join(workspace, "normalised.json"), contractPath)
	exit := runtime.Run(context.Background(), effect.Unit{}, program)

	report, succeeded := exit.Value()
	if !succeeded {
		fail(fmt.Errorf("catalogue: %v", exit))
	}
	fmt.Printf("catalogue: %d books, %d shelved, components %v\n",
		report.Books, report.Shelved, report.Components)

	contract, err := os.ReadFile(contractPath)
	if err != nil {
		fail(err)
	}
	fmt.Printf("published contract:\n%s\n", contract)
}

func reportShutdown(runtime *effect.Runtime) {
	remaining := runtime.LiveWork()
	cleanup := runtime.Close(context.Background())
	fmt.Printf("shutdown: %d fibers and %d resources still owned at Close\n",
		remaining.Fibers, remaining.Resources)
	if !cleanup.IsEmpty() {
		fmt.Printf("shutdown cleanup: %s\n", cleanup)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "webdemo: %v\n", err)
	os.Exit(1)
}
