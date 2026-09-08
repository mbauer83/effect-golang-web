package protobuf

// Encoding and decoding a Go value on the protobuf wire.
//
// This codec works over a whole message rather than through the streaming Sink
// and Source the other formats use, and that is the format's requirement rather
// than a shortcut. A nested message's length precedes it, so nothing can be
// written until the whole of it is known; and a repeated field may appear under
// its number more than once and in any order, so nothing can be read in place
// either. Since a protobuf message is always framed -- a gRPC request is a
// length-prefixed frame -- there is no streaming to give up.
//
// What crosses in between is the universal representation, for the reason the
// SQL package reads a row that way: a protobuf message is a set of named
// values, which is an object, so ToDynamic and FromDynamic do the crossing and
// this package adds only the numbers.

import (
	"github.com/mbauer83/effect-golang-web/schema"
)

// Encode writes a value as a protobuf message.
func Encode[A any](shape schema.Schema[A], value A) ([]byte, error) {
	crossed, err := schema.ToDynamic(shape, value)
	if err != nil {
		return nil, err
	}
	return written(shape.Structure(), crossed)
}

// Decode reads a protobuf message as a value.
//
// A field the wire carried nothing for is the zero of its kind when it has
// implicit presence, and absent when it has explicit presence -- because that
// is what proto3 means rather than a default invented here: an ordinary proto3
// field writes no bytes for its zero, so absent and zero are the same message.
// The description's own rules then apply to the result, which is where a zero
// gets refused: a count of at least one rejects the zero the wire implied
// exactly as it would reject one the wire spelled out.
func Decode[A any](shape schema.Schema[A], message []byte) (A, error) {
	crossed, err := read(shape.Structure(), message)
	if err != nil {
		var absent A
		return absent, err
	}
	return schema.FromDynamic(shape, crossed)
}
