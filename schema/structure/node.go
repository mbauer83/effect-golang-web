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
	Kind Kind
	// Precision is the Go representation the value is carried in, where the
	// description pins one down. The wire has two numeric shapes and Go has
	// twelve; Kind is the first and this is the second.
	Precision Precision
	Format    string
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

// Precision is the Go representation a scalar is carried in.
//
// The wire has two numeric shapes and Go has twelve. A Kind says which of the
// two a value is on the wire; a Precision says which of the twelve it is in a
// program, so a generator emits the type the author meant rather than the
// widest one that would hold it, and a projection can state the range that
// choice implies.
//
// It is Go-side detail deliberately kept out of Kind: a format reads the Kind
// and needs to know nothing about this.
type Precision uint8

const (
	// Unstated is a scalar whose Go representation the description does not
	// pin down: the default for its kind.
	Unstated Precision = iota
	Int8Bits
	Int16Bits
	Int32Bits
	Int64Bits
	IntBits
	Uint8Bits
	Uint16Bits
	Uint32Bits
	Uint64Bits
	UintBits
	Float32Bits
	Float64Bits
)

// Numeric is the wire kind the precision belongs to, and whether it names a
// number at all.
//
// This is how the wire shape is derived rather than declared beside the width:
// there is one answer, so the two cannot disagree.
func (precision Precision) Numeric() (Kind, bool) {
	switch precision {
	case Unstated:
		return Text, false
	case Float32Bits, Float64Bits:
		return Number, true
	default:
		return Integer, true
	}
}

// String names a precision the way Go spells the type.
func (precision Precision) String() string {
	return [...]string{
		"", "int8", "int16", "int32", "int64", "int",
		"uint8", "uint16", "uint32", "uint64", "uint",
		"float32", "float64",
	}[precision]
}
