package unit

// The in-process broker, which is a shipped capability and so needs its own
// coverage.
//
// What matters about a test double is where it refuses. One that answered
// everything would let a program pass here and fail against RabbitMQ, so the
// rules a real broker enforces -- declare before you bind, declare before you
// consume -- are enforced, and the ones it does not implement say so instead of
// pretending.

import (
	"context"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/amqp091/inprocess"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang/effect"
)

type queueing[A any] = effect.Effect[effect.Unit, amqp091.Fault, A]

// ran interprets one effect against a broker.
func ran[A any](t *testing.T, work queueing[A]) effect.Exit[amqp091.Fault, A] {
	t.Helper()
	return effect.Run(context.Background(), effect.Unit{}, work)
}

func TestTheBrokerRefusesWhatHasNotBeenDeclared(t *testing.T) {
	// Declaring first is the broker's rule, not this package's: a real one
	// refuses a binding or a consumer on a name it does not hold, and a fake
	// that invented them would hide a missing declaration until deployment.
	//
	// Each case leaves exactly one name undeclared, so a refusal can only be
	// the rule under test. A case missing both would be refused either way, and
	// would still pass with the rule removed.
	broker := inprocess.NewBroker()
	declared := amqp091.Declare[effect.Unit](broker, amqp091.Topology{
		Exchanges: []amqp091.Exchange{{Name: "known", Routing: amqp091.Direct}},
		Queues:    []amqp091.Queue{{Name: "known"}},
	})
	if exit := ran(t, declared); !exit.IsSuccess() {
		t.Fatalf("unexpected exit: %+v", exit)
	}

	refusals := map[string]struct {
		work   queueing[effect.Unit]
		reason string
	}{
		"publishing to an unknown exchange": {
			work: amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Exchange: "nowhere", Key: "known"}, amqp091.Message{}),
			reason: "no exchange of that name",
		},
		"binding an unknown exchange": {
			work: amqp091.BindQueue[effect.Unit](broker,
				amqp091.Binding{Exchange: "nowhere", Queue: "known", Key: "k"}),
			reason: "no exchange of that name",
		},
		"binding an unknown queue": {
			work: amqp091.BindQueue[effect.Unit](broker,
				amqp091.Binding{Exchange: "known", Queue: "nowhere", Key: "k"}),
			reason: "no queue of that name",
		},
	}
	for named, refused := range refusals {
		exit := ran(t, refused.work)
		cause, failed := exit.Cause()
		if !failed {
			t.Errorf("expected %s to be refused", named)
			continue
		}
		failures := cause.Failures()
		if len(failures) != 1 || !strings.Contains(failures[0].Error(), refused.reason) {
			t.Errorf("%s: expected %q, got %+v", named, refused.reason, cause)
		}
	}

	// And a consumer, which is the same rule read the other way.
	consuming := effect.RunCollect(amqp091.Consume[effect.Unit](broker, "absent").TakeStream(1))
	if exit := ran(t, consuming); exit.IsSuccess() {
		t.Error("expected consuming an undeclared queue to be refused")
	}
}

func TestTheDefaultExchangeRoutesByQueueName(t *testing.T) {
	// Which is why a program with no topology of its own can publish straight
	// to a queue: Target{Key: "work"} and nothing else.
	broker := inprocess.NewBroker()
	program := amqp091.DeclareQueue[effect.Unit](broker, amqp091.Queue{Name: "work"}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Key: "work"}, amqp091.Message{Body: []byte("one")})
		}).
		FlatMap(func(effect.Unit) queueing[[]amqp091.Delivery] {
			return effect.RunCollect(amqp091.Consume[effect.Unit](broker, "work").TakeStream(1))
		})

	exit := ran(t, program)
	arrived, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(arrived) != 1 || string(arrived[0].Body) != "one" {
		t.Fatalf("unexpected deliveries: %#v", arrived)
	}
	// The delivery says where it came from, which a consumer bound to several
	// keys needs in order to tell which one this was.
	if arrived[0].Key != "work" || arrived[0].Exchange != "" {
		t.Fatalf("unexpected origin: %#v", arrived[0])
	}
}

func TestFanoutReachesEveryBoundQueueAndDirectOnlyTheMatchingOne(t *testing.T) {
	broker := inprocess.NewBroker()
	topology := amqp091.Topology{
		Exchanges: []amqp091.Exchange{
			{Name: "everywhere", Routing: amqp091.Fanout},
			{Name: "somewhere", Routing: amqp091.Direct},
		},
		Queues: []amqp091.Queue{{Name: "first"}, {Name: "second"}},
		Bindings: []amqp091.Binding{
			{Exchange: "everywhere", Queue: "first", Key: "ignored"},
			{Exchange: "everywhere", Queue: "second", Key: "also ignored"},
			{Exchange: "somewhere", Queue: "first", Key: "this one"},
			{Exchange: "somewhere", Queue: "second", Key: "not this one"},
		},
	}

	program := amqp091.Declare[effect.Unit](broker, topology).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Exchange: "everywhere", Key: "anything"},
				amqp091.Message{Body: []byte("to all")})
		}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Exchange: "somewhere", Key: "this one"},
				amqp091.Message{Body: []byte("to one")})
		})

	if exit := ran(t, program); !exit.IsSuccess() {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	// Fanout ignored the key and reached both; direct reached the one bound by
	// the key that was published.
	if waiting := broker.Waiting("first"); waiting != 2 {
		t.Errorf("expected two waiting on first, got %d", waiting)
	}
	if waiting := broker.Waiting("second"); waiting != 1 {
		t.Errorf("expected one waiting on second, got %d", waiting)
	}
	// A queue that was never declared holds nothing rather than reporting that
	// there is no such queue: asking is not publishing.
	if waiting := broker.Waiting("third"); waiting != 0 {
		t.Errorf("expected nothing waiting on an unknown queue, got %d", waiting)
	}
}

func TestTheBrokerSaysWhatItDoesNotImplement(t *testing.T) {
	// A fake that answered every question the way a real one does would have to
	// be a real one. What it must not do is answer differently and quietly.
	broker := inprocess.NewBroker()
	program := amqp091.DeclareExchange[effect.Unit](broker,
		amqp091.Exchange{Name: "patterned", Routing: amqp091.Topic}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.DeclareQueue[effect.Unit](broker, amqp091.Queue{Name: "matched"})
		}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.BindQueue[effect.Unit](broker,
				amqp091.Binding{Exchange: "patterned", Queue: "matched", Key: "orders.#"})
		}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Exchange: "patterned", Key: "orders.placed"},
				amqp091.Message{})
		})

	exit := ran(t, program)
	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected topic routing to be refused, got %+v", exit)
	}
	failures := cause.Failures()
	if len(failures) != 1 || !strings.Contains(failures[0].Error(), "routes directly and by fanout") {
		t.Fatalf("expected the reason to say so, got %+v", cause)
	}
}

func TestHeadersReachTheConsumerAsTheyWereSent(t *testing.T) {
	// The port carries headers in the universal representation, so a producer
	// and a consumer that never meet the broker still agree about them.
	broker := inprocess.NewBroker()
	sent := headersOf("trace", "a4f9")

	program := amqp091.DeclareQueue[effect.Unit](broker, amqp091.Queue{Name: "traced"}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker, amqp091.Target{Key: "traced"},
				amqp091.Message{Body: []byte("{}"), Headers: sent})
		}).
		FlatMap(func(effect.Unit) queueing[[]amqp091.Delivery] {
			return effect.RunCollect(amqp091.Consume[effect.Unit](broker, "traced").TakeStream(1))
		})

	exit := ran(t, program)
	arrived, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	held, present := arrived[0].Headers.Member("trace")
	if !present || held != dynamic.OfText("a4f9") {
		t.Fatalf("unexpected headers: %#v", arrived[0].Headers)
	}
}

func headersOf(name string, value string) dynamic.Object {
	return dynamic.Object{Fields: []dynamic.Field{{Name: name, Value: dynamic.OfText(value)}}}
}
