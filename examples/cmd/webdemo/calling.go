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
	"github.com/mbauer83/effect-golang/experimental/direct"
)

// calling is the client's channel: the transport's own faults, which is what a
// caller of somebody else's server can be told.
type calling[A any] = effect.Effect[effect.Unit, web.Fault, A]

// runCalling adds a book and reads it back, both through the endpoints the
// server is serving, and then asks for one that is not there.
func runCalling(runtime *effect.Runtime, base string) {
	client := web.Dial(http.DefaultClient, base)
	added := bookstore.Book{Title: "Called", Authors: []string{"A Client"}, Pages: 12}

	program := direct.Run(func(bind *direct.Binder[effect.Unit, web.Fault]) bookstore.Book {
		sending, err := web.Carrying(web.Requesting{}, bookstore.BookSchema, added)
		if err != nil {
			direct.Bind(bind, effect.Fail[effect.Unit, bookstore.Book](
				web.Fault{Doing: "encoding the book", Err: err}))
		}
		direct.Bind(bind, web.Call[effect.Unit](client, bookstore.AddBook, sending))
		// The title fills the endpoint's captured segment by name, so the path
		// is built from the declaration rather than pasted together here.
		return direct.Bind(bind, web.Call[effect.Unit](client, bookstore.FindBook,
			web.Requesting{Path: map[string]string{"title": added.Title}}))
	})

	read, ok := runtime.Run(context.Background(), effect.Unit{}, program).Value()
	if !ok {
		fail(fmt.Errorf("calling: the round trip failed"))
	}
	fmt.Printf("calling: added and read back %q by %v, %d pages\n",
		read.Title, read.Authors, read.Pages)

	// A status the endpoint did not declare arrives as a Refusal, which
	// carries what the server said about it.
	missing := web.Call[effect.Unit](client, bookstore.FindBook,
		web.Requesting{Path: map[string]string{"title": "Missing"}})
	if fault, refused := runtime.Run(context.Background(), effect.Unit{}, missing).Cause(); refused {
		if failure, is := fault.Failure(); is {
			fmt.Printf("calling: asking for one that is not there -- %v\n", failure.Err)
		}
	}
}
