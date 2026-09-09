package amqp091

// What this package needs of a broker, and no more.
//
// Three interfaces rather than one, because most programs use one of them: a
// producer publishes, a consumer consumes, and whoever owns the topology
// declares it -- usually at start-up and usually once. A consumer that depended
// on Declaring would be claiming a right it does not exercise.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
)

// Publishing sends messages.
type Publishing interface {
	Publish(ctx context.Context, target Target, message Message) error
}

// Consuming reads them.
type Consuming interface {
	// Consume subscribes to a queue. The broker pushes; the returned handle is
	// pulled, so a consumer reads at the rate it can work.
	Consume(ctx context.Context, queue string) (Deliveries, error)
}

// Declaring states what the broker should hold. It is separate because a
// program declares its topology once, at start-up, and the rest of it has no
// business reshaping the broker.
type Declaring interface {
	DeclareExchange(ctx context.Context, exchange Exchange) error
	DeclareQueue(ctx context.Context, queue Queue) error
	Bind(ctx context.Context, binding Binding) error
}

// Target says where a message goes: an exchange, and the key it is routed by.
//
// An empty exchange is the broker's default one, which routes by queue name --
// so Target{Key: "orders"} publishes straight to the queue called orders,
// which is what a program with no topology of its own wants.
type Target struct {
	Exchange string
	Key      string
}

// Message is one message to publish.
type Message struct {
	Body        []byte
	ContentType string
	// Headers are the message's own named values. A header table is a set of
	// named values, which is an object, so the universal representation carries
	// one and no second vocabulary was needed.
	Headers dynamic.Object
	// Durability is whether the broker should keep this message across a
	// restart. It is a choice with two named outcomes rather than a flag,
	// because "true" does not say which way round the question was asked.
	Durability Durability
}

// Delivery is one message received.
type Delivery struct {
	Body        []byte
	ContentType string
	Headers     dynamic.Object
	// Exchange and Key are where it came from, which a consumer bound to
	// several routing keys needs in order to tell which one this was.
	Exchange string
	Key      string
	// Tag is the broker's name for this delivery, which acknowledgement uses.
	Tag uint64
	// Redelivered is the broker saying it has offered this message before, so a
	// consumer that keeps a record can notice a message going round.
	Redelivered bool
}

// Deliveries is one consumer's subscription.
//
// Acknowledgement is here rather than on a Delivery because it is the
// subscription that owes the broker an answer, and a delivery outliving its
// subscription can no longer be acknowledged at all.
type Deliveries interface {
	// Next waits for the next delivery, reporting false when the subscription
	// has ended.
	Next(ctx context.Context) (Delivery, bool, error)
	// Ack accepts a delivery, so the broker may forget it.
	Ack(tag uint64) error
	// Discard rejects it without return. The broker drops it, or routes it
	// wherever the queue's dead-letter configuration says.
	Discard(tag uint64) error
	// Requeue rejects it and asks for it back, for a consumer that cannot
	// handle it now and expects to later.
	Requeue(tag uint64) error
	// Close ends the subscription. It is called by the stream that owns it.
	Close() error
}

// Durability is whether the broker keeps something across a restart.
type Durability uint8

const (
	// Transient is lost when the broker restarts.
	Transient Durability = iota
	// Lasting survives it.
	Lasting
)

// Routing is how an exchange decides where a message goes.
type Routing uint8

const (
	// Direct routes to the queues bound by exactly this key.
	Direct Routing = iota
	// Topic routes by pattern, where * is one word and # is any number.
	Topic
	// Fanout routes to every bound queue and ignores the key.
	Fanout
	// ByHeader routes on the message's headers rather than its key.
	ByHeader
)

// Exchange is a place messages are published to.
type Exchange struct {
	Name       string
	Routing    Routing
	Durability Durability
}

// Access is who may read a queue.
type Access uint8

const (
	// Shared lets any connection consume from the queue, which is what a queue
	// named in a topology is for.
	Shared Access = iota
	// Owned restricts it to the connection that declared it, and the broker
	// deletes it when that connection closes -- a reply queue, or one
	// instance's own subscription.
	//
	// It is also how a queue says it needs no keeping: a broker may refuse one
	// that is neither Lasting nor Owned, because a queue nothing persists and
	// nobody owns is one it cannot account for. RabbitMQ 4 does.
	Owned
)

// Queue is a place messages wait.
type Queue struct {
	Name       string
	Durability Durability
	Access     Access
	// DeadLetter is where the broker sends what this queue's consumer
	// discarded. Its zero value sends it nowhere, which is what Discard means
	// without one.
	DeadLetter DeadLetter
}

// DeadLetter is where a discarded message goes instead of nowhere.
//
// Part of the queue rather than of a rejection, because that is where the
// broker keeps it: a consumer discards a message, and what happens to it next
// was decided when the queue was declared. A consumer cannot choose per
// message, and a queue whose dead letter is added later does not acquire it --
// the broker refuses a declaration that contradicts what is there, which is
// how that mistake is heard about at start-up.
//
// Worth stating rather than leaving to a caller's arguments, because a queue
// with no dead letter is a queue where a message no consumer can act on is
// simply gone: the consumer's only alternatives are to requeue it, which with
// one consumer is a loop, or to acknowledge something it did not do.
type DeadLetter struct {
	Exchange string
	// Key is what the dead-lettered message is routed by. Empty keeps the
	// key it arrived with, which is the broker's own default and is usually
	// what a single dead-letter exchange wants.
	Key string
}

// IsStated reports whether a queue says where its discards go.
func (letter DeadLetter) IsStated() bool { return letter.Exchange != "" }

// Binding is an exchange sending a queue the messages that match a key.
type Binding struct {
	Exchange string
	Queue    string
	Key      string
}
