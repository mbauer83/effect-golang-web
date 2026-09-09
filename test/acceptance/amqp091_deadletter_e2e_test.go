package acceptance

// What a broker does with a message a consumer discarded.
//
// Only a real broker can answer it: the field table a queue is declared with
// is the whole of the arrangement, and the in-process broker says plainly that
// it has no dead-lettering. So this is gated on an address like its siblings,
// and it is the test that says the argument this adapter writes is the one
// RabbitMQ reads.
//
// The alternative to having this arrangement at all is worth stating, because
// it is what the queue's own doc says: a consumer that meets a message it can
// never act on may requeue it, which with one consumer is a loop, or
// acknowledge something it did not do. A dead letter is the third answer.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang/effect"
)

type lettering[A any] = effect.Effect[effect.Unit, amqp091.Fault, A]

func TestAMessageAConsumerDiscardsArrivesWhereTheQueueSaysItShould(t *testing.T) {
	address := os.Getenv("EFFECT_GOLANG_AMQP_URL")
	if address == "" {
		t.Skip("set EFFECT_GOLANG_AMQP_URL to run this against a broker")
	}
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	topology, working, dead := lettered(t.Name())

	program := effect.Scoped(func(scope effect.Scope) lettering[amqp091.Received[[]byte]] {
		return amqp091.Connect[effect.Unit](scope, address).
			FlatMap(func(connection *amqp091.Connection) lettering[amqp091.Received[[]byte]] {
				return amqp091.Open[effect.Unit](scope, connection).
					FlatMap(func(channel *amqp091.Channel) lettering[amqp091.Received[[]byte]] {
						return amqp091.Declare[effect.Unit](channel, topology).
							FlatMap(func(effect.Unit) lettering[amqp091.Received[[]byte]] {
								return discarding(channel, working, dead)
							})
					})
			})
	})

	within, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()
	exit := runtime.Run(within, effect.Unit{}, program)

	received, ok := exit.Value()
	if !ok {
		t.Fatalf("expected the discarded message to arrive, got %+v", exit)
	}
	body, err := received.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "unactionable" {
		t.Fatalf("expected the message itself, got %q", body)
	}
}

// discarding publishes one message, discards it, and reads what the broker did
// with it.
func discarding(
	channel *amqp091.Channel,
	working string,
	dead string,
) lettering[amqp091.Received[[]byte]] {
	return amqp091.Publish[effect.Unit](channel, amqp091.Target{Key: working},
		amqp091.Message{Body: []byte("unactionable")}).
		FlatMap(func(effect.Unit) lettering[amqp091.Received[[]byte]] {
			return first(channel, working, "reading the message to discard").
				FlatMap(func(received amqp091.Received[[]byte]) lettering[amqp091.Received[[]byte]] {
					return amqp091.Discard[effect.Unit](received).
						FlatMap(func(effect.Unit) lettering[amqp091.Received[[]byte]] {
							return acknowledged(channel, dead)
						})
				})
		})
}

func acknowledged(channel *amqp091.Channel, dead string) lettering[amqp091.Received[[]byte]] {
	return first(channel, dead, "reading the dead-lettered message").
		FlatMap(func(received amqp091.Received[[]byte]) lettering[amqp091.Received[[]byte]] {
			return amqp091.Ack[effect.Unit](received).As(received)
		})
}

// first is the next delivery on a queue, and a fault when the stream ended
// without one.
//
// Taking one rather than collecting, because a consumer's stream does not end:
// the subscription's release cancels it, so reading it whole would wait for a
// message nobody is going to send.
func first(
	channel *amqp091.Channel,
	queue string,
	doing string,
) lettering[amqp091.Received[[]byte]] {
	return effect.RunCollect(amqp091.Consume[effect.Unit](channel, queue).TakeStream(1)).
		FlatMap(func(held []amqp091.Received[[]byte]) lettering[amqp091.Received[[]byte]] {
			if len(held) == 0 {
				return effect.Fail[effect.Unit, amqp091.Received[[]byte]](
					amqp091.Fault{Doing: doing})
			}
			return effect.Succeed[effect.Unit, amqp091.Fault](held[0])
		})
}

// lettered is a working queue that sends its discards to a queue of its own.
func lettered(name string) (amqp091.Topology, string, string) {
	working := "effect-golang-" + name
	dead := working + "-dead"
	exchange := dead + "-exchange"
	return amqp091.Topology{
		Exchanges: []amqp091.Exchange{
			{Name: exchange, Routing: amqp091.Direct, Durability: amqp091.Transient},
		},
		Queues: []amqp091.Queue{
			{
				Name: working, Durability: amqp091.Transient, Access: amqp091.Owned,
				DeadLetter: amqp091.DeadLetter{Exchange: exchange, Key: dead},
			},
			{Name: dead, Durability: amqp091.Transient, Access: amqp091.Owned},
		},
		Bindings: []amqp091.Binding{
			{Exchange: exchange, Queue: dead, Key: dead},
		},
	}, working, dead
}
