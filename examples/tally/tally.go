// Package tally is a complete websocket program.
//
// A client sends changes and the server answers with the running total, which
// is the smallest exchange that is genuinely a conversation: the answer depends
// on everything said before it, so it could not be a request and a response.
//
// It is the transport shape the plan records, in one screen: the inbound side
// is a Stream, the outbound side is a sink, the messages are described by a
// Schema, and the socket belongs to a scope.
package tally

import (
	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang-web/websocket"
	"github.com/mbauer83/effect-golang/effect"
)

// Change is what a client asks for.
type Change struct {
	Add int32
}

// Total is what the server answers with.
type Total struct {
	Total int64
}

// ChangeSchema describes a change. The width is stated, so a client that sends
// a number too large to be one is told rather than silently wrapped.
var ChangeSchema = schema.Struct[Change]("Change",
	schema.FieldOf("add", schema.Int32(),
		func(change Change) int32 { return change.Add },
		func(change *Change, add int32) { change.Add = add }),
).Documented("a change to apply to the tally")

// TotalSchema describes an answer.
var TotalSchema = schema.Struct[Total]("Total",
	schema.FieldOf("total", schema.Int64(),
		func(total Total) int64 { return total.Total },
		func(total *Total, running int64) { total.Total = running }),
).Documented("the tally after the change")

type tallyEffect[A any] = effect.Effect[effect.Unit, websocket.Fault, A]

// Route is the conversation, mounted like any other route.
//
// It is an upgrading route rather than a handler mounted beside the others: a
// websocket endpoint is a GET that answers 101, and it should be dispatched by
// the same tree as everything else.
func Route(
	boundary web.Adapter[effect.Unit, websocket.Fault],
	running effect.Ref[int64],
) web.Route[effect.Unit, websocket.Fault] {
	return web.Upgrading("/tally", "Keep a running tally",
		websocket.Accept(boundary, conversing(running), websocket.Settings{}))
}

// conversing reads changes and answers each with the tally.
//
// The inbound side is a stream and the sink is the reply, which is the whole
// shape of a conversation: nothing here polls, nothing buffers, and the loop
// ends when the peer says it is finished.
func conversing(running effect.Ref[int64]) func(websocket.Socket) tallyEffect[effect.Unit] {
	return func(socket websocket.Socket) tallyEffect[effect.Unit] {
		return effect.RunForEach(
			websocket.Values[effect.Unit](socket, ChangeSchema),
			func(change Change) tallyEffect[effect.Unit] {
				return applying(running, change).
					FlatMap(func(now int64) tallyEffect[effect.Unit] {
						return websocket.SendValue[effect.Unit](socket, TotalSchema, Total{Total: now})
					})
			},
		).Named("tally")
	}
}

// applying adds one change. It is a Ref rather than a variable because a
// server may hold several conversations at once, and they share the tally.
func applying(running effect.Ref[int64], change Change) tallyEffect[int64] {
	return effect.For[effect.Unit, websocket.Fault]().WidenError(
		running.UpdateAndGet[effect.Unit](func(current int64) int64 {
			return current + int64(change.Add)
		}))
}

// Ask sends one change and reads the answer, which is what a client does.
func Ask[R any](socket websocket.Socket, add int32) effect.Effect[R, websocket.Fault, Total] {
	return websocket.SendValue[R](socket, ChangeSchema, Change{Add: add}).
		FlatMap(func(effect.Unit) effect.Effect[R, websocket.Fault, Total] {
			return websocket.ReceiveValue[R](socket, TotalSchema)
		}).
		Named("ask")
}
