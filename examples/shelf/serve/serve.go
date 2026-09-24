// Package serve runs the catalogue: its table creation if it is not there, and
// its routes served over one database.
package serve

import (
	"context"
	"net"

	"github.com/mbauer83/effect-golang-sql/ddl"
	"github.com/mbauer83/effect-golang-sql/sql"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"

	"github.com/mbauer83/effect-golang-web/examples/shelf/api"
	"github.com/mbauer83/effect-golang-web/examples/shelf/store"
)

// Serve makes the catalogue's table if it is not there and serves it until
// the context ends.
func Serve(within context.Context, listener net.Listener, file string) error {
	runtime, err := effect.NewRuntime()
	if err != nil {
		return err
	}
	statements, err := store.CreateStatements(ddl.SQLite)
	if err != nil {
		return err
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, api.StatusFor)
	if err != nil {
		return err
	}
	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, error, effect.Unit] {
		return sql.Open[effect.Unit](scope, "sqlite", file).MapError(func(fault sql.Fault) error { return fault }).
			FlatMap(func(database *sql.Database) effect.Effect[effect.Unit, error, effect.Unit] {
				surface, err := api.Surface(store.NewStore(database, ddl.SQLite))
				if err != nil {
					return effect.For[effect.Unit, error]().Fail[effect.Unit](err)
				}
				creation := effect.ForEach(statements, func(statement string) effect.Effect[effect.Unit, sql.Fault, sql.Outcome] {
					return sql.Execute[effect.Unit](database, statement)
				}).MapError(func(fault sql.Fault) error { return fault })
				serve := web.ServeWith[effect.Unit](scope, web.Settings{Listener: listener}, boundary.Handler(surface.Handler())).
					FlatMap(web.Await[effect.Unit]).MapError(func(fault web.Fault) error { return fault })
				return creation.AndThen(serve)
			})
	})
	exit := runtime.Run(within, effect.Unit{}, program)
	if cause, failed := exit.Cause(); failed {
		if fault, isFailure := cause.Failure(); isFailure {
			return fault
		}
	}
	return nil
}
