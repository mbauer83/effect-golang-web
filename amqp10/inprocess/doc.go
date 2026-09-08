// Package inprocess is a broker that runs inside the test that uses it.
//
// It satisfies the port, so a sender and a receiver written against Sending and
// Receiving run against this exactly as they run against Service Bus or
// ActiveMQ. What it gives a test that a real broker cannot is the four
// dispositions as questions: which deliveries were accepted, which were
// rejected and why, which were released unchanged, and which were modified and
// with what said about them. Settlement is the part of at-least-once delivery
// the caller decides, so it is the part worth asserting on -- and the reason it
// is worth more here than in AMQP 0-9-1 is that there are four answers rather
// than three.
//
// It is not a broker. Nodes are declared by the test, messages are held in
// memory in the order they were sent, and a rejected message is dropped rather
// than dead-lettered. Filters, selectors, transactions and credit exhaustion
// are absent, and it says so rather than pretending.
package inprocess
