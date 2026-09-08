package inprocess

// The three operations of the port, and one subscription.

import (
	"context"

	"github.com/mbauer83/effect-golang-web/amqp091"
)

// Publish routes a message to every queue the target reaches.
//
// A message that reaches no queue is dropped silently, which is what a real
// broker does unless the publisher asked to be told. That is the behaviour
// worth copying: a test that expected otherwise would be testing a broker
// nobody runs.
func (held *Broker) Publish(
	_ context.Context,
	target amqp091.Target,
	message amqp091.Message,
) error {
	reached, err := held.reached(target, message)
	if err != nil {
		return err
	}
	// Outside the lock, because a full queue must not be a deadlock.
	for _, offered := range reached {
		if err := offered.queue.offer(offered.delivery); err != nil {
			return err
		}
	}
	return nil
}

// offered is one message on its way to one queue.
type offered struct {
	queue    *queue
	delivery amqp091.Delivery
}

// reached is the message, once per queue the target routes it to, with the tag
// the broker gave each copy.
func (held *Broker) reached(
	target amqp091.Target,
	message amqp091.Message,
) ([]offered, error) {
	held.mutex.Lock()
	defer held.mutex.Unlock()

	names, err := held.routed(target)
	if err != nil {
		return nil, err
	}
	going := make([]offered, 0, len(names))
	for _, name := range names {
		waiting, err := held.queueNamed(name)
		if err != nil {
			return nil, err
		}
		held.tag++
		going = append(going, offered{queue: waiting, delivery: amqp091.Delivery{
			Body:        message.Body,
			ContentType: message.ContentType,
			Headers:     message.Headers,
			Exchange:    target.Exchange,
			Key:         target.Key,
			Tag:         held.tag,
		}})
	}
	return going, nil
}

// routed is which queues a target reaches.
//
// The default exchange routes by queue name, which is why a program with no
// topology of its own can publish straight to a queue.
func (held *Broker) routed(target amqp091.Target) ([]string, error) {
	if target.Exchange == "" {
		return []string{target.Key}, nil
	}
	exchange, known := held.exchanges[target.Exchange]
	if !known {
		return nil, errNoSuchExchange
	}
	if exchange.Routing != amqp091.Direct && exchange.Routing != amqp091.Fanout {
		return nil, errNotRoutable
	}

	reached := []string{}
	for _, binding := range held.bindings {
		if binding.Exchange != target.Exchange {
			continue
		}
		if exchange.Routing == amqp091.Fanout || binding.Key == target.Key {
			reached = append(reached, binding.Queue)
		}
	}
	return reached, nil
}

// Consume subscribes to a queue.
func (held *Broker) Consume(_ context.Context, name string) (amqp091.Deliveries, error) {
	held.mutex.Lock()
	defer held.mutex.Unlock()

	waiting, err := held.queueNamed(name)
	if err != nil {
		return nil, err
	}
	return &subscription{broker: held, queue: waiting}, nil
}

// DeclareExchange states one exchange.
func (held *Broker) DeclareExchange(_ context.Context, exchange amqp091.Exchange) error {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.exchanges[exchange.Name] = exchange
	return nil
}

// DeclareQueue states one queue.
//
// Declaring one that is already there leaves it as it is, which is the
// idempotence a real broker gives a program that declares its topology at every
// start-up.
func (held *Broker) DeclareQueue(_ context.Context, declared amqp091.Queue) error {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if _, already := held.queues[declared.Name]; already {
		return nil
	}
	held.queues[declared.Name] = &queue{
		// Bounded, because an unbounded one would let a test that published in
		// a loop grow until the box noticed rather than until the test failed.
		waiting: make(chan amqp091.Delivery, 1024),
		held:    map[uint64]amqp091.Delivery{},
	}
	return nil
}

// Bind sends a queue the messages an exchange routes by a key.
func (held *Broker) Bind(_ context.Context, binding amqp091.Binding) error {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	if _, known := held.exchanges[binding.Exchange]; !known {
		return errNoSuchExchange
	}
	if _, err := held.queueNamed(binding.Queue); err != nil {
		return err
	}
	held.bindings = append(held.bindings, binding)
	return nil
}
