package inprocess

// The two links, attached to a node the test declared.

import (
	"context"
	"fmt"
	"sync"

	"github.com/mbauer83/effect-golang-web/amqp10"
)

// Sender attaches a link to a node.
//
// It refuses an address no node is at, because that is what a real broker
// refuses -- at attach, before a single message -- and a fake that invented the
// node would hide a wrong address until deployment.
func (held *Broker) Sender(address string) (amqp10.Sending, error) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if _, known := held.nodes[address]; !known {
		return nil, errNoSuchNode
	}
	return &sender{broker: held, address: address}, nil
}

// Receiver attaches a link from a node.
func (held *Broker) Receiver(address string) (amqp10.Receiving, error) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if _, known := held.nodes[address]; !known {
		return nil, errNoSuchNode
	}
	return &receiver{
		broker:    held,
		address:   address,
		unsettled: map[string]amqp10.Delivery{},
	}, nil
}

type sender struct {
	broker   *Broker
	address  string
	mutex    sync.Mutex
	detached bool
}

func (link *sender) Address() string { return link.address }

func (link *sender) Send(_ context.Context, message amqp10.Message) error {
	link.mutex.Lock()
	if link.detached {
		link.mutex.Unlock()
		return errDetached
	}
	link.mutex.Unlock()

	link.broker.mutex.Lock()
	defer link.broker.mutex.Unlock()
	node, known := link.broker.nodes[link.address]
	if !known {
		return errNoSuchNode
	}
	link.broker.tag++
	node.waiting = append(node.waiting, amqp10.Delivery{
		Body:        message.Body,
		ContentType: message.ContentType,
		Subject:     message.Subject,
		Properties:  message.Properties,
		Tag:         fmt.Sprintf("%s-%d", link.address, link.broker.tag),
	})
	return nil
}

func (link *sender) Close(context.Context) error {
	link.mutex.Lock()
	defer link.mutex.Unlock()
	link.detached = true
	return nil
}

type receiver struct {
	broker  *Broker
	address string

	mutex     sync.Mutex
	unsettled map[string]amqp10.Delivery
	detached  bool
}

func (link *receiver) Address() string { return link.address }

// Receive takes the next message, or reports the link detached.
//
// It does not wait. A real link waits for the broker to send, and waiting here
// would mean a test that read one message too many hung instead of failing --
// so an empty node is the end of the messages, which is the same answer a
// detached link gives and the one a stream can act on.
func (link *receiver) Receive(ctx context.Context) (amqp10.Delivery, bool, error) {
	if err := ctx.Err(); err != nil {
		return amqp10.Delivery{}, false, err
	}
	link.mutex.Lock()
	if link.detached {
		link.mutex.Unlock()
		return amqp10.Delivery{}, false, nil
	}
	link.mutex.Unlock()

	link.broker.mutex.Lock()
	node, known := link.broker.nodes[link.address]
	if !known {
		link.broker.mutex.Unlock()
		return amqp10.Delivery{}, false, errNoSuchNode
	}
	if len(node.waiting) == 0 {
		link.broker.mutex.Unlock()
		return amqp10.Delivery{}, false, nil
	}
	delivery := node.waiting[0]
	node.waiting = node.waiting[1:]
	link.broker.mutex.Unlock()

	link.mutex.Lock()
	defer link.mutex.Unlock()
	link.unsettled[delivery.Tag] = delivery
	return delivery, true, nil
}

func (link *receiver) Close(context.Context) error {
	link.mutex.Lock()
	defer link.mutex.Unlock()
	link.detached = true
	return nil
}
