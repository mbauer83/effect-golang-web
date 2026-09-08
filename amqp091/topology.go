package amqp091

// Declaring what the broker should hold.
//
// A program owns its topology, so it says so in one place, the way a program
// that owns its database schema does. Declaring is idempotent at the broker: a
// declaration that matches what is already there succeeds, and one that
// contradicts it is refused -- which is the broker telling you two programs
// disagree about the same name, and worth hearing at start-up rather than at
// the first message.

import (
	"context"

	"github.com/mbauer83/effect-golang/effect"
)

// Topology is what a program needs the broker to hold.
type Topology struct {
	Exchanges []Exchange
	Queues    []Queue
	Bindings  []Binding
}

// Declare states the whole topology.
//
// Exchanges and queues first, then the bindings, because a binding names both
// and the broker will not invent either. That ordering is the reason this
// exists rather than leaving a caller to write three loops in the right order.
func Declare[R any](channel Declaring, topology Topology) effect.Effect[R, Fault, effect.Unit] {
	return effect.ForEach(topology.Exchanges, func(exchange Exchange) effect.Effect[R, Fault, effect.Unit] {
		return DeclareExchange[R](channel, exchange)
	}).
		AndThen(effect.ForEach(topology.Queues, func(queue Queue) effect.Effect[R, Fault, effect.Unit] {
			return DeclareQueue[R](channel, queue)
		})).
		AndThen(effect.ForEach(topology.Bindings, func(binding Binding) effect.Effect[R, Fault, effect.Unit] {
			return BindQueue[R](channel, binding)
		})).
		As(effect.Unit{}).
		Named("declare")
}

// DeclareExchange states one exchange.
func DeclareExchange[R any](channel Declaring, exchange Exchange) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, channel.DeclareExchange(ctx, exchange)
		},
		func(err error) Fault { return faulted("declaring an exchange", exchange.Name, err) },
	).Named("declare-exchange")
}

// DeclareQueue states one queue.
func DeclareQueue[R any](channel Declaring, queue Queue) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, channel.DeclareQueue(ctx, queue)
		},
		func(err error) Fault { return faulted("declaring a queue", queue.Name, err) },
	).Named("declare-queue")
}

// BindQueue sends a queue the messages an exchange routes by a key.
func BindQueue[R any](channel Declaring, binding Binding) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, channel.Bind(ctx, binding)
		},
		func(err error) Fault { return faulted("binding a queue", binding.Queue, err) },
	).Named("bind-queue")
}
