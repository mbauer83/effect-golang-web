package bookstore

import (
	"context"
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang-web/openapi"
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

type storeEffect[A any] = effect.Effect[effect.Unit, Fault, A]

// Surface assembles the bookstore's routes.
//
// Two routes that could match the same request, a path parameter the path does
// not capture, or a schema that cannot encode what it says it will are all
// reported here, at start-up, rather than by a request in production.
func Surface(store *Store) (web.Routes[effect.Unit, Fault], error) {
	return web.NewRoutes(routes(store)...)
}

// Published is the surface together with the route that serves its contract.
//
// The contract describes the API's routes and not the route that serves it,
// which is the ordinary arrangement: a client reads the document to learn what
// it may ask for, and it already knows where the document is.
func Published(store *Store) (web.Routes[effect.Unit, Fault], error) {
	described, err := Surface(store)
	if err != nil {
		return web.Routes[effect.Unit, Fault]{}, err
	}
	contract, err := Contract(described)
	if err != nil {
		return web.Routes[effect.Unit, Fault]{}, err
	}
	return web.NewRoutes(append(routes(store), publishing(contract))...)
}

// Contract projects the surface into an OpenAPI document.
//
// Nothing is written twice: the parameters, the entity and the response shapes
// come from the same declarations that dispatch a request.
func Contract(surface web.Routes[effect.Unit, Fault]) ([]byte, error) {
	return openapi.Describe(
		openapi.Info{
			Title:       "Bookstore",
			Version:     "1.0.0",
			Description: "A small catalogue, served from one description.",
		},
		surface.Declarations(),
	).Render()
}

func routes(store *Store) []web.Route[effect.Unit, Fault] {
	return []web.Route[effect.Unit, Fault]{
		listBooks(store),
		addBook(store),
		findBook(store),
	}
}

// publishing serves the contract as it stands. Its entity is already encoded,
// so it is answered as the bytes it is rather than through a schema.
func publishing(contract []byte) web.Route[effect.Unit, Fault] {
	return web.Handle(
		web.GET("/openapi.json", web.Nothing(),
			web.ReturnsRaw(http.StatusOK, "application/json")).
			Summary("The contract this API is served from"),
		func(effect.Unit) storeEffect[[]byte] {
			return effect.For[effect.Unit, Fault]().Succeed(contract)
		},
	)
}

func listBooks(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(
		web.GET("/books", web.Nothing(), web.Returns(http.StatusOK, CatalogueSchema)).
			Summary("List the catalogue").
			Describe("Every entry, in the order the store holds them."),
		func(effect.Unit) storeEffect[[]Book] {
			return acting("list-books", func() (effect.Exit[Fault, []Book], error) {
				return effect.ExitSuccess[Fault](store.All()), nil
			})
		},
	)
}

func addBook(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(
		web.POST("/books", web.Entity(BookSchema), web.Returns(http.StatusCreated, BookSchema)).
			Summary("Add a book").
			Describe("The entity is the book to add; its title identifies it.").
			Failing(http.StatusConflict, "the store already holds that title"),
		func(book Book) storeEffect[Book] {
			return acting("add-book", func() (effect.Exit[Fault, Book], error) {
				if err := store.Add(book); err != nil {
					return effect.Exit[Fault, Book]{}, err
				}
				return effect.ExitSuccess[Fault](book), nil
			})
		},
	)
}

func findBook(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(
		web.GET("/books/{title}",
			web.PathParam("title", schema.Text()).Documented("the title to look for"),
			web.Returns(http.StatusOK, BookSchema)).
			Summary("Find a book by title").
			Failing(http.StatusNotFound, "no book with that title is held"),
		func(title string) storeEffect[Book] {
			return acting("find-book", func() (effect.Exit[Fault, Book], error) {
				book, held := store.Find(title)
				if !held {
					return effect.Exit[Fault, Book]{}, Fault{Kind: NotFound}
				}
				return effect.ExitSuccess[Fault](book), nil
			})
		},
	)
}

// acting wraps one step against the store.
//
// The store is touched when the runtime interprets the handler and not when the
// route is built, which is what keeps a route a description. A refusal comes
// back as the application's own Fault rather than as a status.
func acting[A any](name string, step func() (effect.Exit[Fault, A], error)) storeEffect[A] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Fault, A] {
		exit, err := step()
		if err == nil {
			return exit
		}
		var refusal Fault
		if errors.As(err, &refusal) {
			return effect.ExitFailure[Fault, A](refusal)
		}
		return effect.ExitCause[Fault, A](effect.DieCause[Fault](effect.Defect{Value: err}))
	}).Named(name)
}
