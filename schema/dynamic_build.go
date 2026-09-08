package schema

// Rebuilding a description as the combinators it was written with.
//
// Every case here composes the ordinary constructors and then lifts the result
// into the universal representation, so there is one encoder, one decoder and
// one place each rule is enforced. A second interpreter over the description
// would be a second opinion about what a shape admits.

import (
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func dynamicScalar(shape structure.Scalar) Schema[dynamic.Value] {
	// The wire kind is what a scalar is read and written as; a recorded width
	// narrows which values are admitted, and that is carried in the
	// constraints, so nothing here needs to know about widths.
	switch shape.Kind {
	case structure.Integer:
		return liftScalar(constrainInteger(Int64(), shape.Constraints), toInteger, fromInteger)
	case structure.Number:
		return liftScalar(constrainNumber(Float64(), shape.Constraints), toNumber, fromNumber)
	case structure.Boolean:
		return liftScalar(Bool(), toBoolean, fromBoolean)
	case structure.Bytes:
		return liftScalar(Bytes(), toBytes, fromBytes)
	case structure.Timestamp:
		return liftScalar(Time(), toTimestamp, fromTimestamp)
	default:
		return liftScalar(
			constrainText(Formatted(shape.Format), shape.Constraints), toText, fromText)
	}
}

func dynamicObject(shape structure.Object) Schema[dynamic.Value] {
	fields := make([]Field[dynamic.Value], 0, len(shape.Fields))
	for _, member := range shape.Fields {
		described := DescribedField(member.Name, Dynamic(member.Node)).Documented(member.Doc)
		if member.Optional {
			described = described.Optional()
		}
		fields = append(fields, described)
	}
	return Struct[dynamic.Value](shape.Name, fields...).Documented(shape.Doc)
}

func dynamicUnion(shape structure.Union) Schema[dynamic.Value] {
	variants := make([]Variant[dynamic.Value], 0, len(shape.Variants))
	for _, alternative := range shape.Variants {
		variants = append(variants,
			DescribedVariant(alternative.Name, Dynamic(alternative.Node)).Documented(alternative.Doc))
	}
	return OneOf[dynamic.Value](shape.Name, variants...).Documented(shape.Doc)
}

func dynamicSequence(shape structure.Sequence) Schema[dynamic.Value] {
	listed := constrainList(List(Dynamic(shape.Element)), shape.Constraints)
	return liftScalar(listed,
		func(elements []dynamic.Value) (dynamic.Value, error) {
			return dynamic.List{Elements: elements}, nil
		},
		func(value dynamic.Value) ([]dynamic.Value, error) {
			list, isList := value.(dynamic.List)
			if !isList {
				return nil, fail("is not a list", nil)
			}
			return list.Elements, nil
		})
}

// dynamicMapping reads a variable set of keys. The keys come back sorted,
// because Map encodes them sorted and a value that read back in a different
// order would not round trip.
func dynamicMapping(shape structure.Mapping) Schema[dynamic.Value] {
	return liftScalar(Map(Dynamic(shape.Value)),
		func(entries map[string]dynamic.Value) (dynamic.Value, error) {
			object := dynamic.Object{Fields: make([]dynamic.Field, 0, len(entries))}
			for _, key := range sortedKeys(entries) {
				object.Fields = append(object.Fields,
					dynamic.Field{Name: key, Value: entries[key]})
			}
			return object, nil
		},
		func(value dynamic.Value) (map[string]dynamic.Value, error) {
			object, isObject := value.(dynamic.Object)
			if !isObject {
				return nil, fail("is not a mapping", nil)
			}
			entries := make(map[string]dynamic.Value, len(object.Fields))
			for _, field := range object.Fields {
				entries[field.Name] = field.Value
			}
			return entries, nil
		})
}

func dynamicNullable(shape structure.Nullable) Schema[dynamic.Value] {
	return liftScalar(Nullable(Dynamic(shape.Inner)),
		func(held *dynamic.Value) (dynamic.Value, error) {
			if held == nil {
				return dynamic.Absent{}, nil
			}
			return *held, nil
		},
		func(value dynamic.Value) (*dynamic.Value, error) {
			if _, absent := value.(dynamic.Absent); absent {
				return nil, nil
			}
			return &value, nil
		})
}

// dynamicReference defers, which is what makes a recursive description work:
// the shape it names is built when it is first needed rather than while the
// description that refers to it is still being assembled.
func dynamicReference(shape structure.Reference) Schema[dynamic.Value] {
	if shape.Resolve == nil {
		return faulted[dynamic.Value](shape,
			fail("refers to a shape that cannot be resolved", nil))
	}
	return Deferred(func() Schema[dynamic.Value] { return Dynamic(shape.Resolve()) })
}

// liftScalar carries a typed schema into the universal representation. The
// typed schema does the work; this only says which case of the sum holds it.
func liftScalar[A any](
	inner Schema[A],
	into func(A) (dynamic.Value, error),
	outOf func(dynamic.Value) (A, error),
) Schema[dynamic.Value] {
	return TransformOrFail(inner, into, outOf)
}
