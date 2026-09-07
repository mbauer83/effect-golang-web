package schema

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Error reports why a value could not be encoded or decoded.
//
// It carries the path to the part that failed, because "expected an integer" is
// not something a caller can act on and "expected an integer at
// author.age" is. Decoders wrap as they unwind, so the path reads outermost
// first.
type Error struct {
	// Path names the parts traversed to reach the failure, outermost first.
	Path []string
	// Reason states what was wrong, in terms of the schema.
	Reason string
	// Err is the underlying format or conversion error, if any.
	Err error
}

func (failure *Error) Error() string {
	var rendered strings.Builder
	rendered.WriteString("schema: ")
	rendered.WriteString(failure.Reason)
	if len(failure.Path) > 0 {
		rendered.WriteString(" at ")
		rendered.WriteString(strings.Join(failure.Path, "."))
	}
	if failure.Err != nil {
		rendered.WriteString(": ")
		rendered.WriteString(failure.Err.Error())
	}
	return rendered.String()
}

// Unwrap exposes the underlying error, so errors.Is and errors.As keep working
// through a schema failure.
func (failure *Error) Unwrap() error {
	return failure.Err
}

// PathOf returns the path of the schema failure in err, and whether err was
// one. It is how a caller reports which field a request was rejected for.
func PathOf(err error) ([]string, bool) {
	var failure *Error
	if errors.As(err, &failure) {
		return failure.Path, true
	}
	return nil, false
}

// fail reports a failure with no path yet; an enclosing decoder adds one.
func fail(reason string, cause error) error {
	return &Error{Reason: reason, Err: cause}
}

// refused reports a value a conversion would not accept.
//
// The conversion's own message follows the reason rather than replacing it, so
// a path can still be prefixed as the failure unwinds and errors.Is still
// reaches whatever sentinel the refinement used.
func refused(err error) error {
	var failure *Error
	if errors.As(err, &failure) {
		return err
	}
	return &Error{Reason: "did not pass its refinement", Err: err}
}

// within prefixes a failure with the part it happened inside, so the path
// accumulates as decoding unwinds rather than being threaded through every
// call.
func within(part string, err error) error {
	if err == nil {
		return nil
	}
	var failure *Error
	if errors.As(err, &failure) {
		return &Error{
			Path:   append([]string{part}, failure.Path...),
			Reason: failure.Reason,
			Err:    failure.Err,
		}
	}
	return &Error{Path: []string{part}, Reason: "could not be processed", Err: err}
}

func zeroSchemaError[A any]() error {
	var missing A
	return fail(fmt.Sprintf("the zero Schema[%T] has no codec", missing), nil)
}

// listIndex names a position for a failure path, so a rejected element reads as
// "items.3" rather than as the whole list.
func listIndex(index int) string {
	return "items." + strconv.Itoa(index)
}
