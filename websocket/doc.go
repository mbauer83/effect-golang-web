// Package websocket carries a conversation over an upgraded connection.
//
// It follows the shape every transport here follows, so learning one teaches
// the others:
//
//	a connection is a scoped resource
//	a conversation is a Stream in and a sink out
//	messages are encoded by a Schema
//	cancellation closes the connection through scope closure
//
// The upgrade happens inside a scope, so a closed scope closes the socket --
// which means a conversation cannot outlive the request that started it, and
// nothing has to remember to hang up.
//
// Both sides are here. A server accepts, a client dials, and both get the same
// Socket: a conversation is symmetrical once it has begun, and a library that
// only did the server half would leave every test to reach for something else.
package websocket
