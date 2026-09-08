// Package structure is the description of a value's shape, with the types
// erased.
//
// A schema's typed surface is a view over one of these nodes, exactly as the
// runtime's typed Cause is a view over an erased cause tree. Projections --
// JSON Schema, OpenAPI, protobuf descriptors, SQL column lists -- walk the
// node, so the description has to exist as data and not only as a pair of codec
// functions.
//
// It is public rather than internal because a projection is an extension point.
// The built-in ones have no privileged access: a projection to Avro, to a
// database migration or to a form renderer is written the same way.
package structure

// Kind identifies a scalar shape.
type Kind uint8

const (
	// Text is a UTF-8 string.
	Text Kind = iota
	// Integer is a signed whole number.
	Integer
	// Number is a floating-point number.
	Number
	// Boolean is true or false.
	Boolean
	// Bytes is an opaque byte string.
	Bytes
	// Timestamp is an instant in time.
	Timestamp
)

// String names a kind for diagnostics and for projections that use the name.
func (kind Kind) String() string {
	switch kind {
	case Integer:
		return "integer"
	case Number:
		return "number"
	case Boolean:
		return "boolean"
	case Bytes:
		return "bytes"
	case Timestamp:
		return "timestamp"
	default:
		return "text"
	}
}

// Node describes the shape of a value. The set is sealed: a projection can
// switch over it exhaustively and know it has covered everything.
type Node interface {
	node()
}

// Scalar is a single value. Format carries a refinement a projection may use --
// "uuid" or "date-time" for JSON Schema, for instance -- and is empty when
// there is none.
type Scalar struct {
	Kind   Kind
	Format string
	// Constraints narrow which values of the kind are admitted, in the order
	// they were declared. A projection that has keywords for them emits them;
	// one that has none describes the kind alone, which is still true.
	Constraints []Constraint
}

// Object is a fixed set of named fields.
//
// A named object becomes a reusable component in projections that have them, so
// a type used by ten endpoints is described once. An unnamed object is inlined.
type Object struct {
	Name   string
	Doc    string
	Fields []Field
}

// Field is one member of an object.
type Field struct {
	Name     string
	Doc      string
	Node     Node
	Optional bool
}

// Sequence is an ordered, variable-length list.
type Sequence struct {
	Element Node
	// Constraints narrow how many elements are admitted.
	Constraints []Constraint
}

// Mapping is a variable set of keys of one shape to values of another.
type Mapping struct {
	Key   Node
	Value Node
}

// Union is a choice between named variants.
type Union struct {
	Name     string
	Doc      string
	Variants []Variant
}

// Variant is one alternative of a union.
type Variant struct {
	Name string
	Doc  string
	Node Node
}

// Nullable is a shape a format may carry as null.
//
// It is a node rather than a flag because it applies to any shape, and it is
// distinct from an optional field: a nullable value is present and null, an
// optional field is not there at all. Formats and readers treat those
// differently, so the description does too.
type Nullable struct {
	Inner Node
}

// Reference names a shape described elsewhere.
//
// It exists for two purposes that happen to need the same thing: a recursive
// type terminates, and a projection emits a reference rather than an inlined
// copy at every use. Resolve is how a walker reaches the referenced shape, and
// may be nil when the reference is to a component the walker already holds.
type Reference struct {
	Name    string
	Resolve func() Node
}

func (Scalar) node()    {}
func (Object) node()    {}
func (Sequence) node()  {}
func (Mapping) node()   {}
func (Union) node()     {}
func (Nullable) node()  {}
func (Reference) node() {}
