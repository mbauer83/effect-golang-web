// Command webdemo runs the transport examples against live capabilities,
// including a real server on a real socket.
//
// The description examples are in effect-golang-schema's own command, and the
// database ones in effect-golang-sql's: each module demonstrates what it is
// responsible for.
//
// It exists so the examples are demonstrably runnable programs and not only
// test fixtures. Each scenario is also composed by an end-to-end test, so the
// two cannot drift apart.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
	"github.com/mbauer83/effect-golang/effect"
)

func main() {
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		fail(err)
	}
	defer reportShutdown(runtime)

	runBookstore(runtime)
	runDispatch(runtime)
	runConsign(runtime)
	runQuoting(runtime)
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
	// The store holds a Ref, so building one is an effect: its state cannot
	// exist before something interprets the description that makes it.
	store, made := runtime.Run(context.Background(), effect.Unit{},
		bookstore.NewStore(
			bookstore.Book{Title: "Zionomicon", Authors: []string{"John A. De Goes"}, Pages: 632},
		)).Value()
	if !made {
		fail(errors.New("the store could not be built"))
	}
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
