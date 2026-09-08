package unit

// The precise numeric schemas. The wire carries two numeric shapes and Go has
// twelve; a description says which of the twelve, so a generator emits the type
// the author meant and a contract states the range that implies.

import (
	"math"
	"strconv"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func TestAPreciseIntegerRefusesWhatItsWidthCannotHold(t *testing.T) {
	// The wire carries a 64-bit integer whatever the program wants, so a value
	// that does not fit is refused rather than silently truncated.
	small := schema.Int8()

	for _, admitted := range []int64{-128, 0, 127} {
		if _, err := schema.DecodeJSON(small, []byte(strconv.FormatInt(admitted, 10))); err != nil {
			t.Errorf("expected %d to be admitted, got %v", admitted, err)
		}
	}
	for _, refused := range []int64{-129, 128, math.MaxInt64} {
		if _, err := schema.DecodeJSON(small, []byte(strconv.FormatInt(refused, 10))); err == nil {
			t.Errorf("expected %d to be refused", refused)
		}
	}
}

func TestAnUnsignedIntegerRefusesANegativeValue(t *testing.T) {
	for label, shape := range map[string]schema.Schema[uint16]{"uint16": schema.Uint16()} {
		if _, err := schema.DecodeJSON(shape, []byte("-1")); err == nil {
			t.Errorf("%s: expected a negative value to be refused", label)
		}
		if _, err := schema.DecodeJSON(shape, []byte("65535")); err != nil {
			t.Errorf("%s: expected the largest value to be admitted, got %v", label, err)
		}
		if _, err := schema.DecodeJSON(shape, []byte("65536")); err == nil {
			t.Errorf("%s: expected an over-large value to be refused", label)
		}
	}
}

func TestAPreciseFloatRefusesWhatItsWidthCannotHold(t *testing.T) {
	narrow := schema.Float32()

	if _, err := schema.DecodeJSON(narrow, []byte("1.5")); err != nil {
		t.Fatalf("expected 1.5 to be admitted, got %v", err)
	}
	// Turning an over-large value into an infinity would be worse than saying
	// it does not fit.
	if _, err := schema.DecodeJSON(narrow, []byte("1e39")); err == nil {
		t.Fatal("expected a value beyond float32 to be refused")
	}
}

func TestAWidthRecordsTheRangeItImplies(t *testing.T) {
	// Nobody writes these bounds down, and the contract still states them --
	// which is the whole reason a description carries the width.
	shape := schema.Uint8().Structure().(structure.Scalar)

	if shape.Precision != structure.Uint8Bits {
		t.Fatalf("expected the width recorded, got %v", shape.Precision)
	}
	if shape.Kind != structure.Integer {
		t.Fatalf("expected the wire kind derived, got %v", shape.Kind)
	}
	bounds := jsonschema.Project(shape).Root.Bounds
	if bounds.Minimum == nil || *bounds.Minimum != 0 {
		t.Fatalf("expected a lower bound of zero, got %#v", bounds.Minimum)
	}
	if bounds.Maximum == nil || *bounds.Maximum != 255 {
		t.Fatalf("expected an upper bound of 255, got %#v", bounds.Maximum)
	}
}

func TestOnlyTheWidthsTheSpecificationNamesBecomeAFormat(t *testing.T) {
	// A range a reader can act on is stated by minimum and maximum. Putting
	// "uint16" in a contract would be telling a client about Go.
	named := map[string]string{
		"int32":  projectedFormat(t, schema.Int32().Structure()),
		"int64":  projectedFormat(t, schema.Int64().Structure()),
		"float":  projectedFormat(t, schema.Float32().Structure()),
		"double": projectedFormat(t, schema.Float64().Structure()),
	}
	for expected, got := range named {
		if got != expected {
			t.Errorf("expected the format %q, got %q", expected, got)
		}
	}
	for label, node := range map[string]structure.Node{
		"uint16": schema.Uint16().Structure(),
		"int8":   schema.Int8().Structure(),
		"int":    schema.Int().Structure(),
	} {
		if got := projectedFormat(t, node); got != "" {
			t.Errorf("%s: expected no format, got %q", label, got)
		}
	}
}

func projectedFormat(t *testing.T, node structure.Node) string {
	t.Helper()
	return jsonschema.Project(node).Root.Format
}

func TestAWidthSurvivesADescriptionWithNoGoType(t *testing.T) {
	// The width is part of the description, so a schema used without its Go
	// type still refuses what the width cannot hold.
	described := schema.Dynamic(schema.Uint8().Structure())

	if _, err := schema.DecodeJSON(described, []byte("255")); err != nil {
		t.Fatalf("expected the largest value to be admitted, got %v", err)
	}
	if _, err := schema.DecodeJSON(described, []byte("256")); err == nil {
		t.Fatal("expected a value beyond the width to be refused without the type")
	}
}

func TestABoundOutsideTheWidthCannotBeWritten(t *testing.T) {
	// This is the compile-time half, and it cannot be tested at run time
	// because it does not compile: schema.AtMost(schema.Int8(), 200) is
	// rejected by the compiler, since 200 is not an int8. What is testable is
	// that a bound inside the width narrows further and one at the edge is
	// admitted.
	narrower := schema.AtMost(schema.Int8(), 100)

	if _, err := schema.DecodeJSON(narrower, []byte("100")); err != nil {
		t.Fatalf("expected the narrowed bound to admit its own edge, got %v", err)
	}
	if _, err := schema.DecodeJSON(narrower, []byte("101")); err == nil {
		t.Fatal("expected the narrowed bound to refuse a value the width allows")
	}
}

// widths pairs every precise constructor with the range it must record, so a
// constructor added without its bounds is caught here.
var widths = []struct {
	label     string
	node      structure.Node
	precision structure.Precision
	lowest    float64
	highest   float64
}{
	{"int8", schema.Int8().Structure(), structure.Int8Bits, math.MinInt8, math.MaxInt8},
	{"int16", schema.Int16().Structure(), structure.Int16Bits, math.MinInt16, math.MaxInt16},
	{"int32", schema.Int32().Structure(), structure.Int32Bits, math.MinInt32, math.MaxInt32},
	{"uint8", schema.Uint8().Structure(), structure.Uint8Bits, 0, math.MaxUint8},
	{"uint16", schema.Uint16().Structure(), structure.Uint16Bits, 0, math.MaxUint16},
	{"uint32", schema.Uint32().Structure(), structure.Uint32Bits, 0, math.MaxUint32},
	{"uint", schema.Uint().Structure(), structure.UintBits, 0, math.MaxInt64},
	{"uint64", schema.Uint64().Structure(), structure.Uint64Bits, 0, math.MaxInt64},
	{"float32", schema.Float32().Structure(), structure.Float32Bits, -math.MaxFloat32, math.MaxFloat32},
}

func TestEveryWidthRecordsItsOwnRange(t *testing.T) {
	for _, width := range widths {
		bounds := jsonschema.Project(width.node).Root.Bounds
		if bounds.Minimum == nil || *bounds.Minimum != width.lowest {
			t.Errorf("%s: expected a lower bound of %v, got %v",
				width.label, width.lowest, bounds.Minimum)
		}
		if bounds.Maximum == nil || *bounds.Maximum != width.highest {
			t.Errorf("%s: expected an upper bound of %v, got %v",
				width.label, width.highest, bounds.Maximum)
		}
		if scalar := width.node.(structure.Scalar); scalar.Precision != width.precision {
			t.Errorf("%s: expected the width recorded, got %v", width.label, scalar.Precision)
		}
	}
}

func TestAWidthAndAWireKindCannotBeMadeToDisagree(t *testing.T) {
	// The wire kind is derived from the width, so there is one answer and the
	// two cannot contradict each other. Unstated names no number at all.
	if kind, numeric := structure.Unstated.Numeric(); numeric {
		t.Errorf("expected Unstated to name no number, got %v", kind)
	}
	for _, width := range widths {
		scalar := width.node.(structure.Scalar)
		kind, numeric := scalar.Precision.Numeric()
		if !numeric || kind != scalar.Kind {
			t.Errorf("%s: the width and the wire kind disagree: %v against %v",
				width.label, scalar.Precision, scalar.Kind)
		}
	}
}

func TestEachConstructorKnowsHowMuchItAlreadyRecords(t *testing.T) {
	// A generator emitting a constructor must not emit the constraints that
	// constructor already carries. The count lives beside the constructors so
	// the two cannot fall out of step, and this checks it is right.
	for format, constructor := range schema.FormatConstructors {
		var known schema.Constructor = constructor
		if known.Call == "" {
			t.Errorf("%s: has no call", format)
		}
		if known.Carries < 0 {
			t.Errorf("%s: carries %d", format, known.Carries)
		}
	}
	for precision, constructor := range schema.PrecisionConstructors {
		if constructor.Call == "" {
			t.Errorf("%v: has no call", precision)
		}
	}
	// Every recorded width has a constructor, or a generator reading a
	// description would not know what to write.
	for _, width := range widths {
		if _, known := schema.PrecisionConstructors[width.precision]; !known {
			t.Errorf("%s: no constructor is named for it", width.label)
		}
	}
}
