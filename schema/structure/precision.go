package structure

// The Go representation a scalar is carried in, as distinct from what it is on
// the wire.

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
