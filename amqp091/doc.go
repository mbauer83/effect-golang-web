// Package amqp091 carries messages over AMQP 0-9-1, the protocol RabbitMQ
// speaks natively.
//
// The version is in the name because AMQP 1.0 shares the name and almost
// nothing else: it has no exchanges, no routing keys and no bindings, it
// addresses nodes directly, and it settles a delivery by disposition rather
// than by acknowledgement. Package amqp10 is that protocol, and a port covering
// both would be a port covering neither. A plain "amqp" here would have read as
// the general one.
//
// A connection and a channel are scoped resources, a consumer is a Stream,
// publishing is an effect, and acknowledgement is explicit -- because
// at-least-once delivery is a property the caller has to decide about, not one
// a library should decide for it. The same Schema that describes a request body
// describes a message, so a producer and a consumer agree about a shape without
// anyone writing it twice.
//
// There is a port here, and unlike the one in sql it is not because there are
// two implementations worth having: amqp091-go is the only serious one. It is
// because a program that publishes and consumes should be testable without a
// broker, and because Publishing and Consuming are what an application actually
// depends on -- three operations, one of which most programs never call. The
// adapter over amqp091-go is in this package and nothing above it sees that
// library.
package amqp091
