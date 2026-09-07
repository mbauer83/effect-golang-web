package bookstore

import (
	"context"
	"errors"
	"net/http"

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
	return web.NewRoutes(
		listBooks(store),
		addBook(store),
		findBook(store),
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
			web.Describing(web.PathParam("title", schema.Text()), "the title to look for"),
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
