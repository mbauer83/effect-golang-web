// Package inprocess is a broker that runs inside the test that uses it.
//
// It satisfies the port, so a producer and a consumer written against
// Publishing and Consuming run against this exactly as they run against
// RabbitMQ -- which is what the port is for. What it gives a test that a real
// broker cannot is a question: which deliveries were accepted, which were
// discarded, and which came back. Acknowledgement is the part of at-least-once
// delivery the caller decides, so it is the part worth asserting on.
//
// It is not a broker. It routes directly and by fanout, holds everything in
// memory, and forgets it when the test ends. Topic routing, dead-lettering,
// publisher confirms and durability are absent, and it says so rather than
// pretending: a fake that answered every question the way the real one does
// would have to be the real one.
//
// Prefetch is absent too, and it is the absence worth knowing about, because
// Prefetch is an operation a program calls and this has no answer for it. A
// real broker sends at most that many unsettled deliveries and then waits, so
// a consumer that never acknowledges stalls after that many; here it reads
// everything. A test that turns on acknowledgement behaviour needs a broker --
// which is what the acceptance job with a RabbitMQ service container is for.
package inprocess
