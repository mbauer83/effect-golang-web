// Command shelf serves the catalogue from an SQLite file, on :8080 unless
// SHELF_ADDRESS says otherwise.
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"

	_ "modernc.org/sqlite"

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
	within, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := serve.Serve(within, listener, "file:shelf.db?_pragma=foreign_keys(1)"); err != nil {
		log.Fatal(err)
	}
}
