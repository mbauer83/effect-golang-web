// Command webdemo runs the example scenarios against live capabilities,
// including a real server on a real socket.
//
// It exists so the examples are demonstrably runnable programs and not only
// test fixtures. Each scenario is also composed by an end-to-end test, so the
// two cannot drift apart.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
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
	runBookstore(runtime)
}

// runBookstore starts the HTTP program on a port the operating system chooses,
// talks to it over a real socket, and then lets its scope close -- which is
// what shuts it down.
func runBookstore(runtime *effect.Runtime) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fail(err)
	}
	boundary, err := bookstore.Boundary(runtime)
	if err != nil {
		fail(err)
	}
	store := bookstore.NewStore(
		bookstore.Book{Title: "Zionomicon", Authors: []string{"John A. De Goes"}, Pages: 632},
	)
	surface, err := bookstore.Published(store)
	if err != nil {
		fail(err)
	}

	serving, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(serving, effect.Unit{}, bookstore.Serve(listener, boundary, surface))
	}()

	base := "http://" + listener.Addr().String()
	fmt.Printf("bookstore: listening on %s\n", base)
	report(base+"/books", get(base+"/books"))
	report(base+"/books (post)", post(base+"/books", `{"title":"New","authors":["A"],"pages":10}`))
	report(base+"/books (bad)", post(base+"/books", `{"title":"New","authors":["A"],"pages":0}`))
	report(base+"/books (again)", post(base+"/books", `{"title":"New","authors":["A"],"pages":10}`))
	report(base+"/books/Zionomicon", get(base+"/books/Zionomicon"))
	report(base+"/books/Missing", get(base+"/books/Missing"))
	report(base+"/openapi.json", get(base+"/openapi.json"))

	stop()
	<-stopped

	// The store's operations are effects, so counting what it holds is one
	// too: the runtime interprets it like anything else.
	held, _ := runtime.Run(context.Background(), effect.Unit{}, store.All()).Value()
	fmt.Printf("bookstore: stopped with %d books\n", len(held))
}

func get(url string) *http.Response {
	response, err := http.Get(url)
	if err != nil {
		fail(err)
	}
	return response
}

func post(url string, document string) *http.Response {
	response, err := http.Post(url, "application/json", strings.NewReader(document))
	if err != nil {
		fail(err)
	}
	return response
}

func report(what string, response *http.Response) {
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fail(err)
	}
	fmt.Printf("  %-24s %d %s\n", what, response.StatusCode, strings.TrimSpace(string(body)))
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
