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

	"github.com/mbauer83/effect-golang-web/examples/quote"
	"github.com/mbauer83/effect-golang-web/grpc"
	"github.com/mbauer83/effect-golang/effect"
)

var rates = map[string]quote.Rate{
	"Kiel-Hamburg": {Carrier: "overland", Currency: "EUR", Cents: 4000},
}

func runQuote(runtime *effect.Runtime) {
	transport := grpc.NewConnectServer()
	boundary, err := grpc.NewBoundary(runtime, effect.Unit{}, transport, quote.FailureFor)
	if err != nil {
		fail(err)
	}
	if err := quote.Answer(boundary, rates); err != nil {
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	base := "http://" + listener.Addr().String()
	fmt.Printf("quote: serving %s on %s\n", quote.Quote.Path(), base)

	client := grpc.DialConnect(&http.Client{Timeout: 5 * time.Second}, base)
	askFor(runtime, client, quote.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 100})
	askFor(runtime, client, quote.Enquiry{Origin: "Kiel", Destination: "Lima", Kilos: 100})
	askFor(runtime, client, quote.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 30000})

	contract, err := grpc.Contract("logistics.v1", quote.Quote)
	if err != nil {
		fail(err)
	}
	fmt.Printf("published contract:\n%s", contract.Render())
}

func askFor(runtime *effect.Runtime, client grpc.ClientTransport, enquiry quote.Enquiry) {
	exit := runtime.Run(context.Background(), effect.Unit{},
		grpc.Ask[effect.Unit](client, quote.Quote, enquiry))

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
