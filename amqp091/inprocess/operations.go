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
func (broker *Broker) Publish(
	_ context.Context,
	target amqp091.Target,
	message amqp091.Message,
) error {
	placements, err := broker.queuesFor(target, message)
	if err != nil {
		return err
	}
	// Outside the lock, because a full queue must not be a deadlock.
	for _, placement := range placements {
		if err := placement.queue.offer(placement.delivery); err != nil {
			return err
		}
	}
	return nil
}

// placement is one message on its way to one queue.
type placement struct {
	queue    *queue
	delivery amqp091.Delivery
}

// queuesFor is the message, once per queue the target routes it to, with the tag
// the broker gave each copy.
func (broker *Broker) queuesFor(
	target amqp091.Target,
	message amqp091.Message,
) ([]placement, error) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()

	names, err := broker.routeTargets(target)
	if err != nil {
		return nil, err
	}
	placements := make([]placement, 0, len(names))
	for _, name := range names {
		queue, err := broker.findQueue(name)
		if err != nil {
			return nil, err
		}
		broker.tag++
		placements = append(placements, placement{queue: queue, delivery: amqp091.Delivery{
			Body:        message.Body,
			ContentType: message.ContentType,
			Headers:     message.Headers,
			Exchange:    target.Exchange,
			Key:         target.Key,
			Tag:         broker.tag,
		}})
	}
	return placements, nil
}

// routeTargets is which queues a target reaches.
//
// The default exchange routes by queue name, which is why a program with no
// topology of its own can publish straight to a queue.
func (broker *Broker) routeTargets(target amqp091.Target) ([]string, error) {
	if target.Exchange == "" {
		return []string{target.Key}, nil
	}
	exchange, known := broker.exchanges[target.Exchange]
	if !known {
		return nil, errNoSuchExchange
	}
	if exchange.Kind != amqp091.Direct && exchange.Kind != amqp091.Fanout {
		return nil, errNotRoutable
	}

	queues := []string{}
	for _, binding := range broker.bindings {
		if binding.Exchange != target.Exchange {
			continue
		}
		if exchange.Kind == amqp091.Fanout || binding.Key == target.Key {
			queues = append(queues, binding.Queue)
		}
	}
	return queues, nil
}

// Consume subscribes to a queue.
func (broker *Broker) Consume(_ context.Context, name string) (amqp091.Deliveries, error) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()

	queue, err := broker.findQueue(name)
	if err != nil {
		return nil, err
	}
	return &subscription{broker: broker, queue: queue}, nil
}

// DeclareExchange states one exchange.
func (broker *Broker) DeclareExchange(_ context.Context, exchange amqp091.Exchange) error {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.exchanges[exchange.Name] = exchange
	return nil
}

// DeclareQueue states one queue.
//
// Declaring one that is already there leaves it as it is, which is the
// idempotence a real broker gives a program that declares its topology at every
// start-up.
func (broker *Broker) DeclareQueue(_ context.Context, declaration amqp091.Queue) error {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	if _, already := broker.queues[declaration.Name]; already {
		return nil
	}
	broker.queues[declaration.Name] = &queue{
		// Bounded, because an unbounded one would let a test that published in
		// a loop grow until the box noticed rather than until the test failed.
		backlog:   make(chan amqp091.Delivery, 1024),
		unsettled: map[uint64]amqp091.Delivery{},
	}
	return nil
}

// Bind sends a queue the messages an exchange routes by a key.
func (broker *Broker) Bind(_ context.Context, binding amqp091.Binding) error {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	if _, known := broker.exchanges[binding.Exchange]; !known {
		return errNoSuchExchange
	}
	if _, err := broker.findQueue(binding.Queue); err != nil {
		return err
	}
	broker.bindings = append(broker.bindings, binding)
	return nil
}
