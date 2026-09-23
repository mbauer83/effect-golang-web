package main

// The bookstore called through its own declarations.
//
// The raw net/http calls beside this one are deliberate: they prove there is a
// real server on a real socket, answering something that has never seen this
// module. What they cannot show is the other half of the claim -- that one
// Endpoint value serves a request, projects into the contract, and is what a
// caller calls. This shows that.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// callEffect is the client's channel: the transport's own faults, which is what a
// caller of somebody else's server can be told.
type callEffect[A any] = effect.Effect[effect.Unit, web.Fault, A]

// runClient adds a book and reads it back, both through the endpoints the
// server is serving, and then asks for one that is not there.
func runClient(runtime *effect.Runtime, base string) {
	client := web.Dial(http.DefaultClient, base)
	sample := bookstore.Book{Title: "Called", Authors: []string{"A Client"}, Pages: 12}

	program := effect.Gen(func(do *effect.Do[effect.Unit, web.Fault]) bookstore.Book {
		request, err := web.WithEntity(web.ClientRequest{}, bookstore.BookSchema, sample)
		if err != nil {
			do.Await(effect.Fail[effect.Unit, bookstore.Book](
				web.Fault{Op: "encoding the book", Err: err}))
		}
		do.Await(web.Call[effect.Unit](client, bookstore.AddBook, request))
		// The title fills the endpoint's captured segment by name, so the path
		// is built from the declaration rather than pasted together here.
		return do.Await(web.Call[effect.Unit](client, bookstore.FindBook,
			web.ClientRequest{Path: map[string]string{"title": sample.Title}}))
	})

	book, ok := runtime.Run(context.Background(), effect.Unit{}, program).Value()
	if !ok {
		fail(fmt.Errorf("calling: the round trip failed"))
	}
	fmt.Printf("calling: added and read back %q by %v, %d pages\n",
		book.Title, book.Authors, book.Pages)

	// A status the endpoint did not declare arrives as a Refusal, which
	// carries what the server said about it.
	lookup := web.Call[effect.Unit](client, bookstore.FindBook,
		web.ClientRequest{Path: map[string]string{"title": "Missing"}})
	if fault, refused := runtime.Run(context.Background(), effect.Unit{}, lookup).Cause(); refused {
		if failure, is := fault.Failure(); is {
			fmt.Printf("calling: asking for one that is not there -- %v\n", failure.Err)
		}
	}
}
