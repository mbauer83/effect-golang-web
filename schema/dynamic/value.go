// Package dynamic is the value a schema describes when there is no Go type to
// describe it as.
//
// A Schema[A] moves a Go value in and out of a format. A description loaded
// from elsewhere, or written before the type it will become exists, has no A --
// and it is still worth validating, transcoding, inspecting and composing. This
// is the A it uses instead.
//
// It is a sealed sum rather than a top type, so a walker switches over it
// exhaustively and knows it has covered everything, and so nothing in this
// module has to reach for erasure to hold a value it cannot name. The nine
// cases are the twelve calls of a Sink seen from the other side, which is why a
// format needs to learn nothing new to carry one.
package dynamic

import "time"

// Value is a value of unknown Go type but known shape.
type Value interface {
	value()
}

// Text is a string.
type Text struct {
	Value string
}

// Integer is a whole number.
type Integer struct {
	Value int64
}

// Number is a floating-point number.
type Number struct {
	Value float64
}

// Boolean is true or false.
type Boolean struct {
	Value bool
}

// Bytes is an opaque byte string.
type Bytes struct {
	Value []byte
}

// Timestamp is an instant.
type Timestamp struct {
	Value time.Time
}

// Absent is the explicit absence of a value: present and null, as distinct from
// a member that is not there at all, which is simply missing from an Object.
type Absent struct{}

// Object is a set of named members, in the order its description declares them
// rather than the order a document happened to carry them.
//
// A struct, a string-keyed mapping and a union all appear as one, because that
// is what all three are on the wire: an object. Which of them a value is meant
// to be is the description's business, not the value's -- a dynamic value is
// only meaningful beside its node, and giving it three near-identical shapes
// would be recording the same fact twice.
type Object struct {
	Fields []Field
}

// Field is one member of an Object.
type Field struct {
	Name  string
	Value Value
}

// List is an ordered sequence.
type List struct {
	Elements []Value
}

func (Text) value()      {}
func (Integer) value()   {}
func (Number) value()    {}
func (Boolean) value()   {}
func (Bytes) value()     {}
func (Timestamp) value() {}
func (Absent) value()    {}
func (Object) value()    {}
func (List) value()      {}

// Member returns the value of a named member, and whether the object has one.
// It is here because looking one up is the thing every caller does first, and
// scanning a slice by hand at each use would be tedious rather than explicit.
func (object Object) Member(name string) (Value, bool) {
	for _, field := range object.Fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
}

// Only returns the single member of an object, which is how a union's value is
// read: the wire form of a chosen variant is an object with one member, and its
// name is the variant's.
func (object Object) Only() (Field, bool) {
	if len(object.Fields) != 1 {
		return Field{}, false
	}
	return object.Fields[0], true
}
