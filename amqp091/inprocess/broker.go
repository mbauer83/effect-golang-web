package inprocess

// The broker's own state: what it has been told to hold, and what is waiting.

import (
	"errors"
	"sync"

	"github.com/mbauer83/effect-golang-web/amqp091"
)

// Broker holds queues in memory and routes to them.
//
// One Broker per test. A shared one shares its queues, and then two tests fail
// in an order-dependent way.
type Broker struct {
	mutex     sync.Mutex
	exchanges map[string]amqp091.Exchange
	queues    map[string]*queue
	bindings  []amqp091.Binding
	tag       uint64
	settled   settlements
}

// queue is one place messages wait.
//
// waiting is buffered rather than synchronous so a publisher does not block on
// a consumer that has not subscribed yet, which is the ordinary shape of a test:
// publish three, then read them.
type queue struct {
	waiting chan amqp091.Delivery
	held    map[uint64]amqp091.Delivery
}

// offer puts a message on the queue without waiting for room.
//
// Never while holding the broker's lock, and never blocking: a send that waited
// would hold the lock every reader needs, and the deadlock would look like a
// hung test rather than a full queue. A full queue says so, which is a thing a
// test can act on.
func (waiting *queue) offer(delivery amqp091.Delivery) error {
	select {
	case waiting.waiting <- delivery:
		return nil
	default:
		return errQueueFull
	}
}

// settlements is what the consumers decided, in the order they decided it.
type settlements struct {
	accepted  []uint64
	discarded []uint64
	requeued  []uint64
}

// NewBroker makes an empty broker.
func NewBroker() *Broker {
	return &Broker{
		exchanges: map[string]amqp091.Exchange{},
		queues:    map[string]*queue{},
	}
}

// Accepted, Discarded and Requeued are the delivery tags the consumers settled
// each way. They are what a test asks about, because acknowledgement is the
// decision the caller makes and a library should not.
func (held *Broker) Accepted() []uint64 {
	return held.settledAs(func(settled settlements) []uint64 { return settled.accepted })
}

// Discarded is the tags rejected without return.
func (held *Broker) Discarded() []uint64 {
	return held.settledAs(func(settled settlements) []uint64 { return settled.discarded })
}

// Requeued is the tags asked for again.
func (held *Broker) Requeued() []uint64 {
	return held.settledAs(func(settled settlements) []uint64 { return settled.requeued })
}

// Waiting is how many messages a queue is holding that nobody has taken.
func (held *Broker) Waiting(name string) int {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	waiting, known := held.queues[name]
	if !known {
		return 0
	}
	return len(waiting.waiting)
}

func (held *Broker) settledAs(which func(settlements) []uint64) []uint64 {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return append([]uint64(nil), which(held.settled)...)
}

// queueNamed is the queue of that name, or the refusal that there is none.
// Declaring first is the broker's rule, not this package's: a real one refuses
// a binding or a consumer on a queue it does not hold.
func (held *Broker) queueNamed(name string) (*queue, error) {
	waiting, known := held.queues[name]
	if !known {
		return nil, errNoSuchQueue
	}
	return waiting, nil
}

var (
	errNoSuchQueue    = errors.New("no queue of that name has been declared")
	errNoSuchExchange = errors.New("no exchange of that name has been declared")
	errNotRoutable    = errors.New("this broker routes directly and by fanout, and nothing else")
	errUnknownTag     = errors.New("no unsettled delivery has that tag")
	errQueueFull      = errors.New("this broker holds a thousand messages a queue, and that one is full")
)
