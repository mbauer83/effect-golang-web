package inprocess

// The broker's own state: the nodes a test declared, and what was settled.

import (
	"errors"
	"sync"

	"github.com/mbauer83/effect-golang-web/amqp10"
)

// Broker holds nodes in memory. One per test: a shared one shares its nodes,
// and then two tests fail in an order-dependent way.
type Broker struct {
	mutex   sync.Mutex
	nodes   map[string]*node
	tag     uint64
	settled settlements
}

// node is one address messages wait at.
type node struct {
	waiting []amqp10.Delivery
}

// Rejection is one message refused, with the reason the receiver gave. The
// reason is the point: a broker records it, and whoever reads the dead-letter
// node afterwards has the only explanation there is going to be.
type Rejection struct {
	Tag    string
	Reason string
}

// Modification is one message given back with something said about it, which is
// the disposition AMQP 0-9-1 cannot express.
type Modification struct {
	Tag    string
	Change amqp10.Change
}

type settlements struct {
	accepted []string
	rejected []Rejection
	released []string
	modified []Modification
}

// NewBroker makes a broker with no nodes.
func NewBroker() *Broker {
	return &Broker{nodes: map[string]*node{}}
}

// Declare states a node at an address. A real broker's addresses come from its
// own configuration, so this is the test standing in for that configuration.
func (held *Broker) Declare(address string) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if _, already := held.nodes[address]; !already {
		held.nodes[address] = &node{}
	}
}

// Accepted is the tags of the messages a receiver said it was done with.
func (held *Broker) Accepted() []string {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return append([]string(nil), held.settled.accepted...)
}

// Rejected is the messages a receiver said would never be processed, and why.
func (held *Broker) Rejected() []Rejection {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return append([]Rejection(nil), held.settled.rejected...)
}

// Released is the tags of the messages a receiver gave back unchanged.
func (held *Broker) Released() []string {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return append([]string(nil), held.settled.released...)
}

// Modified is the messages a receiver gave back with something said about them.
func (held *Broker) Modified() []Modification {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return append([]Modification(nil), held.settled.modified...)
}

// Waiting is how many messages a node holds that nobody has taken.
func (held *Broker) Waiting(address string) int {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	waiting, known := held.nodes[address]
	if !known {
		return 0
	}
	return len(waiting.waiting)
}

var (
	errNoSuchNode = errors.New("no node has been declared at that address")
	errDetached   = errors.New("the link has been detached")
	errUnknownTag = errors.New("no unsettled delivery has that tag")
)
