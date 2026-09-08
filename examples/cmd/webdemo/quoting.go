package main

// The gRPC service, on a real socket, called by a real client.
//
// It also prints the .proto file the service implies, which is the deliverable
// that makes it usable from anywhere else: another language generates its
// client from that file, and it is a projection of the same descriptions the
// server enforces, so the two cannot drift.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mbauer83/effect-golang-web/examples/quoting"
	"github.com/mbauer83/effect-golang-web/grpc"
	"github.com/mbauer83/effect-golang/effect"
)

var carried = map[string]quoting.Rate{
	"Kiel-Hamburg": {Carrier: "overland", Currency: "EUR", Cents: 4000},
}

func runQuoting(runtime *effect.Runtime) {
	transport := grpc.NewConnected()
	boundary, err := grpc.NewBoundary(runtime, effect.Unit{}, transport, quoting.Coded)
	if err != nil {
		fail(err)
	}
	if err := quoting.Answer(boundary, carried); err != nil {
		fail(err)
	}
	handler, err := boundary.Handler()
	if err != nil {
		fail(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fail(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		stopping, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(stopping)
	}()

	base := "http://" + listener.Addr().String()
	fmt.Printf("quoting: serving %s on %s\n", quoting.Quote.Path(), base)

	client := grpc.DialConnect(&http.Client{Timeout: 5 * time.Second}, base)
	askFor(runtime, client, quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 100})
	askFor(runtime, client, quoting.Enquiry{Origin: "Kiel", Destination: "Lima", Kilos: 100})
	askFor(runtime, client, quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 30000})

	contract, err := grpc.Contract("logistics.v1", quoting.Quote)
	if err != nil {
		fail(err)
	}
	fmt.Printf("published contract:\n%s", contract.Render())
}

func askFor(runtime *effect.Runtime, client grpc.Calling, enquiry quoting.Enquiry) {
	exit := runtime.Run(context.Background(), effect.Unit{},
		grpc.Ask[effect.Unit](client, quoting.Quote, enquiry))

	rate, priced := exit.Value()
	if priced {
		fmt.Printf("  %-22s %s %d %s\n", enquiry.Destination,
			rate.Currency, rate.Cents, rate.Carrier)
		return
	}
	cause, _ := exit.Cause()
	failure, typed := cause.Failure()
	if !typed {
		fmt.Printf("  %-22s %v\n", enquiry.Destination, cause)
		return
	}
	fmt.Printf("  %-22s code %d: %s\n", enquiry.Destination, failure.Code, failure.Message)
}
