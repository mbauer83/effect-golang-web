package grpc

// The one place this package holds a value it cannot name.
//
// Connect's Codec contract is untyped, because a codec is registered for a
// content type and has to marshal whatever message the procedure it serves
// happens to take. This package's port works in bytes, so what crosses here is
// always the same one type -- and the assertion is checked rather than assumed,
// which is what makes the boundary a boundary rather than a hole.
//
// The architecture test names this file.

import (
	"errors"
	"fmt"
)

// payload is one message's bytes, on their way through Connect.
//
// A type of its own rather than []byte, because Connect's generic handler needs
// a type to instantiate over and a named one makes the assertions below say
// what they mean.
type payload struct {
	bytes []byte
}

// passingThrough is the codec: it does nothing, because the schema layer has
// already done it.
//
// Its name is "proto", which is what puts "application/grpc+proto" on the wire
// -- so a gRPC client generated from the projected .proto file talks to this
// without knowing anything about it. Naming it otherwise would produce a
// content type no generated client asks for.
type passingThrough struct{}

func (passingThrough) Name() string { return "proto" }

func (passingThrough) Marshal(message any) ([]byte, error) {
	held, ours := message.(*payload)
	if !ours {
		return nil, fmt.Errorf("this codec carries bytes, and Connect offered a %T", message)
	}
	return held.bytes, nil
}

func (passingThrough) Unmarshal(bytes []byte, into any) error {
	target, ours := into.(*payload)
	if !ours {
		return fmt.Errorf("this codec carries bytes, and Connect offered a %T", into)
	}
	// Copied, because the buffer Connect read into is its own and may be
	// reused once this returns.
	target.bytes = append([]byte(nil), bytes...)
	return nil
}

// errorText is the error a message becomes, since Connect's own constructor
// takes one.
func errorText(message string) error {
	return errors.New(message)
}

var (
	errAnsweredTwice = errors.New(
		"two answers for one procedure means the author meant one thing and wrote two")
	errNoProcedures = errors.New("a transport with no procedures answers nothing")
)
