// Package serve runs the catalogue: its tables made if they are not there,
// and its routes served over one database.
package serve

import (
	"context"
	"errors"
	"net"

	"github.com/mbauer83/effect-golang-sql/ddl"
	"github.com/mbauer83/effect-golang-sql/sql"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"

	"github.com/mbauer83/effect-golang-web/examples/shelf/api"
	"github.com/mbauer83/effect-golang-web/examples/shelf/store"
)

// Database is which database the catalogue is kept in, and in which dialect:
// a deployment's Postgres from its address, a test's SQLite in a file of its
// own. The program is the same for either; this is what it is given.
type Database struct {
	Dialect ddl.Dialect
	Driver  string
	Source  string
}

// Sessions is the database as the layer the program requires: connected
// when it starts, closed when it ends.
func (database Database) Sessions() effect.Layer[effect.Unit, sql.Fault, sql.Session] {
	return sql.SessionLayer(database.Dialect, database.Driver, database.Source)
}

// Serve makes the catalogue's tables if they are not there and serves it
// until the context ends.
func Serve(within context.Context, listener net.Listener, database Database) error {
	runtime, err := effect.NewRuntime()
	if err != nil {
		return err
	}
	statements, err := store.CreateStatements(database.Dialect)
	if err != nil {
		return err
	}
	if err := store.Books.Check(database.Dialect); err != nil {
		return err
	}
	surface, err := api.Surface()
	if err != nil {
		return err
	}
	// The program requires a session and nothing else: the tables made in
	// it, then the routes served with it as their environment.
	program := effect.Environment[sql.Session, error]().
		FlatMap(func(session sql.Session) effect.Effect[sql.Session, error, effect.Unit] {
			boundary, err := web.NewAdapter(runtime, session, api.StatusFor)
			if err != nil {
				return effect.For[sql.Session, error]().Fail[effect.Unit](err)
			}
			creation := effect.ForEach(statements, func(statement string) effect.Effect[sql.Session, sql.Fault, sql.Outcome] {
				return sql.Execute[sql.Session](session.Database, statement)
			}).MapError(func(fault sql.Fault) error { return fault })
			serving := effect.Scoped(func(scope effect.Scope) effect.Effect[sql.Session, error, effect.Unit] {
				return web.ServeWith[sql.Session](scope, web.Settings{Listener: listener}, boundary.Handler(surface.Handler())).
					FlatMap(web.Await[sql.Session]).
					MapError(func(fault web.Fault) error { return fault })
			})
			return creation.AndThen(serving)
		})
	sessions := database.Sessions().MapError(func(fault sql.Fault) error { return fault })
	exit := runtime.Run(within, effect.Unit{}, program.ProvideLayerSame(sessions))
	if cause, failed := exit.Cause(); failed {
		if fault, isFailure := cause.Failure(); isFailure {
			return fault
		}
		return errors.New(cause.String())
	}
	return nil
}
