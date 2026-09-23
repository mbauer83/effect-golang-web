package amqp091

// The channel behind the port.
//
// Every flag amqp091-go takes and this does not is a decision recorded here
// rather than passed on. mandatory and immediate are absent because both report
// an unroutable message by returning it on a channel nobody is reading, which
// is a feature that needs its own shape rather than a boolean. autoAck is
// absent because acknowledgement is explicit -- that is the whole point.
// exclusive, internal, noLocal and noWait are absent because a program that
// needs them has outgrown this and should say so, rather than find them
// silently set.

import (
	"context"
	"fmt"
	"sync/atomic"

	broker "github.com/rabbitmq/amqp091-go"
)

// Publish sends one message.
func (channel *Channel) Publish(ctx context.Context, target Target, message Message) error {
	headers, err := Table(message.Headers)
	if err != nil {
		return err
	}
	return channel.channel.PublishWithContext(ctx, target.Exchange, target.Key, false, false,
		broker.Publishing{
			Headers:      headers,
			ContentType:  message.ContentType,
			DeliveryMode: deliveryMode(message.Durability),
			Body:         message.Body,
		})
}

// Consume subscribes to a queue.
//
// The consumer is named here rather than left to the broker, because the
// subscription has to be cancellable: a stream that stopped early must stop the
// broker sending, and the only way to say which consumer is by its tag -- which
// the library does not hand back when it generates one. The tag has to be
// unique on the channel, so a counter is enough.
func (channel *Channel) Consume(ctx context.Context, queue string) (Deliveries, error) {
	tag := fmt.Sprintf("%s-%d", queue, consumers.Add(1))
	deliveries, err := channel.channel.ConsumeWithContext(ctx, queue, tag,
		false, false, false, false, nil)
	if err != nil {
		return nil, err
	}
	return &subscription{channel: channel.channel, deliveries: deliveries, tag: tag}, nil
}

// consumers names the consumers this process opens. Uniqueness is required per
// channel, so counting past what any one channel needs is harmless.
var consumers atomic.Uint64

// DeclareExchange states one exchange.
func (channel *Channel) DeclareExchange(_ context.Context, exchange Exchange) error {
	return channel.channel.ExchangeDeclare(exchange.Name, kindName(exchange.Kind),
		exchange.Durability == Durable, false, false, false, nil)
}

// DeclareQueue states one queue.
func (channel *Channel) DeclareQueue(_ context.Context, queue Queue) error {
	_, err := channel.channel.QueueDeclare(queue.Name,
		queue.Durability == Durable, false, queue.Access == Exclusive, false,
		deadLetterArguments(queue.DeadLetter))
	return err
}

// deadLetterArguments are the broker's own settings for a queue, which is where a dead
// letter lives: the protocol carries it as a table rather than as a field, and
// nothing else this package declares needs one.
func deadLetterArguments(letter DeadLetter) broker.Table {
	if !letter.IsStated() {
		return nil
	}
	table := broker.Table{"x-dead-letter-exchange": letter.Exchange}
	if letter.Key != "" {
		table["x-dead-letter-routing-key"] = letter.Key
	}
	return table
}

// Bind sends a queue the messages an exchange routes by a key.
func (channel *Channel) Bind(_ context.Context, binding Binding) error {
	return channel.channel.QueueBind(binding.Queue, binding.Key, binding.Exchange, false, nil)
}

func deliveryMode(durability Durability) uint8 {
	if durability == Durable {
		return broker.Persistent
	}
	return broker.Transient
}

func kindName(routing ExchangeKind) string {
	switch routing {
	case Topic:
		return "topic"
	case Fanout:
		return "fanout"
	case ByHeader:
		return "headers"
	default:
		return "direct"
	}
}
