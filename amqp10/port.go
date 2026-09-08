package amqp10

// What this package needs of a broker, and no more.
//
// Two interfaces, because a link goes one way: 1.0 attaches a sender or a
// receiver to a node, and a program that only sends should not depend on the
// four dispositions it never makes.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
)

// Sending is a link to a node that accepts messages.
type Sending interface {
	Send(ctx context.Context, message Message) error
	// Address is the node this link is attached to. The link knows it, so a
	// caller reporting a fault should not have to repeat it and cannot get it
	// wrong.
	Address() string
	// Close detaches the link. It is called by the scope that attached it.
	Close(ctx context.Context) error
}

// Receiving is a link from a node that produces them.
//
// The dispositions are here rather than on a Delivery because it is the link
// that owes the broker an answer, and a delivery outliving its link can no
// longer be settled at all.
type Receiving interface {
	// Receive waits for the next message, reporting false when the link has
	// been detached and there will be no more.
	Receive(ctx context.Context) (Delivery, bool, error)
	// Address is the node this link is attached to.
	Address() string
	// Accept says the message is done and the broker may forget it.
	Accept(ctx context.Context, tag string) error
	// Reject says it will never be processed. The broker dead-letters it, or
	// drops it, according to the node's configuration.
	Reject(ctx context.Context, tag string, reason string) error
	// Release gives it back unchanged, as though this receiver had never had
	// it, so another may take it.
	Release(ctx context.Context, tag string) error
	// Modify gives it back with something said about it, which is the outcome
	// AMQP 0-9-1 has no way to express.
	Modify(ctx context.Context, tag string, change Change) error
	// Close detaches the link. It is called by the scope that attached it.
	Close(ctx context.Context) error
}

// Message is one message to send.
type Message struct {
	Body        []byte
	ContentType string
	// Subject is a summary of what the message is for, which is the field a
	// broker's own filters and rules are usually written against.
	Subject string
	// Properties are the message's application properties: its own named
	// values, as distinct from the annotations aimed at the infrastructure. A
	// set of named values is an object, so the universal representation carries
	// one and no second vocabulary was needed.
	Properties dynamic.Object
	// Durability is whether the broker should keep this message across a
	// restart. Two named outcomes rather than a flag, because "true" does not
	// say which way round the question was asked.
	Durability Durability
}

// Delivery is one message received.
type Delivery struct {
	Body        []byte
	ContentType string
	Subject     string
	Properties  dynamic.Object
	// Tag is the broker's name for this delivery, which a disposition names.
	Tag string
	// Attempts is how many times the message has been delivered before,
	// according to the broker. A consumer that keeps no record of its own can
	// still tell a message going round from a message arriving.
	Attempts uint32
}

// Change is what Modify says about a message it is giving back.
//
// Two independent facts rather than one choice of four, because they are
// independent: a delivery can have been attempted and failed while still being
// deliverable here, and can be undeliverable here without anything having been
// attempted. Naming them as fields is what keeps them from being two positional
// booleans at a call site.
type Change struct {
	// Tried says the delivery was attempted and failed, which is what a broker
	// counts when it decides a message has failed enough times.
	Tried bool
	// Elsewhere says this receiver cannot handle the message but another may,
	// so the broker should not offer it here again.
	Elsewhere bool
	// Annotations are what to record on the message, for whoever gets it next.
	Annotations dynamic.Object
}

// Durability is whether the broker keeps a message across a restart.
type Durability uint8

const (
	// Transient is lost when the broker restarts.
	Transient Durability = iota
	// Lasting survives it.
	Lasting
)
