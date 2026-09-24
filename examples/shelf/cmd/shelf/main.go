// Command shelf serves the catalogue on :8080 unless SHELF_ADDRESS says
// otherwise: from Postgres at SHELF_POSTGRES_URL when it is set, and from an
// SQLite file beside it when it is not.
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/mbauer83/effect-golang-sql/ddl"

	"github.com/mbauer83/effect-golang-web/examples/shelf/serve"
)

func main() {
	address := os.Getenv("SHELF_ADDRESS")
	if address == "" {
		address = ":8080"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	database := serve.Database{Dialect: ddl.SQLite, Driver: "sqlite", Source: "file:shelf.db?_pragma=foreign_keys(1)"}
	if url := os.Getenv("SHELF_POSTGRES_URL"); url != "" {
		database = serve.Database{Dialect: ddl.Postgres, Driver: "pgx", Source: url}
	}
	within, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := serve.Serve(within, listener, database); err != nil {
		log.Fatal(err)
	}
}
