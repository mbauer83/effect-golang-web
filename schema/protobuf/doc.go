// Package protobuf projects a description into a proto3 file, and encodes a
// value on the protobuf wire.
//
// It is a projection like the JSON Schema one, and it carries no third-party
// dependency for the same reason that one does not: the wire format and the
// proto3 language are published specifications, and depending on an
// implementation of them would put a dependency in the schema layer that every
// user of the HTTP core would acquire. The canonical implementation appears in
// the tests instead, where what it is for is checking that what this emits is
// what protobuf reads -- which is exactly what a test dependency is for.
//
// Two things this refuses rather than guesses at, and they are the same
// decision twice. Protobuf identifies a field by its **number** and a type by
// its **name**, and both are part of the contract: renaming a Go field is safe
// and renumbering it is not, which is the opposite of every other projection
// here. So a field with no number is refused, because a number derived from
// declaration order would change when the declaration was reordered; and an
// unnamed object is refused, because a name synthesised from the field holding
// it would change when that field was renamed.
package protobuf
