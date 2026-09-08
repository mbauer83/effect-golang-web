// Package amqp10 carries messages over AMQP 1.0, the protocol Azure Service
// Bus, ActiveMQ and Qpid speak.
//
// It is not a variant of [amqp091] and does not sit behind the same port. AMQP
// 1.0 has no exchanges, no routing keys and no bindings: a link is attached to
// a node by address, and the broker's own configuration decides what a node
// is. A delivery is settled by disposition -- accepted, rejected, released or
// modified -- which is four outcomes where 0-9-1 has three, and the fourth says
// something 0-9-1 cannot say. A port covering both would cover neither.
//
// What the two do share is the shape of the answer, because that shape is this
// runtime's rather than the protocol's: a connection, a session and a link are
// scoped resources, a receiver is a Stream, sending is an effect, settlement is
// explicit, and the same Schema that describes a request body describes a
// message.
//
// [amqp091]: https://pkg.go.dev/github.com/mbauer83/effect-golang-web/amqp091
package amqp10
