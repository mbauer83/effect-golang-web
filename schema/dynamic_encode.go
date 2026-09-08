package schema

// Encoding a value whose Go type is not known: the description drives, as it
// always does, and the value is read out of the universal representation
// instead of out of a struct's fields.

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func encodeDynamic(node structure.Node, value dynamic.Value, into Sink) error {
	switch shape := node.(type) {
	case structure.Scalar:
		return encodeDynamicScalar(shape, value, into)
	case structure.Object:
		return encodeDynamicObject(shape, value, into)
	case structure.Sequence:
		return encodeDynamicSequence(shape, value, into)
	case structure.Mapping:
		return encodeDynamicMapping(shape, value, into)
	case structure.Union:
		return encodeDynamicUnion(shape, value, into)
	case structure.Nullable:
		if _, absent := value.(dynamic.Absent); absent {
			return into.Null()
		}
		return encodeDynamic(shape.Inner, value, into)
	case structure.Reference:
		return encodeDynamicReference(shape, value, into)
	default:
		return fail("has a shape this schema cannot encode", nil)
	}
}

func encodeDynamicScalar(shape structure.Scalar, value dynamic.Value, into Sink) error {
	if err := checkConstraints(shape.Constraints, value); err != nil {
		return err
	}
	switch held := value.(type) {
	case dynamic.Text:
		return into.Text(held.Value)
	case dynamic.Integer:
		return into.Integer(held.Value)
	case dynamic.Number:
		return into.Number(held.Value)
	case dynamic.Boolean:
		return into.Boolean(held.Value)
	case dynamic.Bytes:
		return into.Bytes(held.Value)
	case dynamic.Timestamp:
		return into.Timestamp(held.Value)
	default:
		return fail("is not "+article(shape.Kind), nil)
	}
}

func encodeDynamicObject(shape structure.Object, value dynamic.Value, into Sink) error {
	object, isObject := value.(dynamic.Object)
	if !isObject {
		return fail("is not an object", nil)
	}
	if err := into.BeginObject(); err != nil {
		return err
	}
	for _, field := range shape.Fields {
		held, present := object.Member(field.Name)
		switch {
		case !present && field.Optional:
			// An absent optional member is omitted, as it is when the value
			// came from a struct.
			continue
		case !present:
			return within(field.Name, fail("required member is missing", nil))
		}
		if err := into.FieldName(field.Name); err != nil {
			return err
		}
		if err := encodeDynamic(field.Node, held, into); err != nil {
			return within(field.Name, err)
		}
	}
	return into.EndObject()
}

func encodeDynamicSequence(shape structure.Sequence, value dynamic.Value, into Sink) error {
	if err := checkConstraints(shape.Constraints, value); err != nil {
		return err
	}
	list, isList := value.(dynamic.List)
	if !isList {
		return fail("is not a list", nil)
	}
	if err := into.BeginList(); err != nil {
		return err
	}
	for index, element := range list.Elements {
		if err := encodeDynamic(shape.Element, element, into); err != nil {
			return within(listIndex(index), err)
		}
	}
	return into.EndList()
}

// encodeDynamicMapping writes a mapping's keys in sorted order, so a document
// written from a dynamic value is as comparable as one written from a Go map.
func encodeDynamicMapping(shape structure.Mapping, value dynamic.Value, into Sink) error {
	mapping, isObject := value.(dynamic.Object)
	if !isObject {
		return fail("is not a mapping", nil)
	}
	if err := into.BeginObject(); err != nil {
		return err
	}
	for _, entry := range sortedFields(mapping.Fields) {
		if err := into.FieldName(entry.Name); err != nil {
			return err
		}
		if err := encodeDynamic(shape.Value, entry.Value, into); err != nil {
			return within(entry.Name, err)
		}
	}
	return into.EndObject()
}

func encodeDynamicUnion(shape structure.Union, value dynamic.Value, into Sink) error {
	object, isObject := value.(dynamic.Object)
	if !isObject {
		return fail("is not a choice between variants", nil)
	}
	chosen, only := object.Only()
	if !only {
		return fail("a union names one variant, and this names "+
			strconv.Itoa(len(object.Fields)), nil)
	}
	for _, variant := range shape.Variants {
		if variant.Name != chosen.Name {
			continue
		}
		if err := into.BeginObject(); err != nil {
			return err
		}
		if err := into.FieldName(variant.Name); err != nil {
			return err
		}
		if err := encodeDynamic(variant.Node, chosen.Value, into); err != nil {
			return within(variant.Name, err)
		}
		return into.EndObject()
	}
	return fail("names the variant "+chosen.Name+", which this union does not have", nil)
}

func encodeDynamicReference(shape structure.Reference, value dynamic.Value, into Sink) error {
	if shape.Resolve == nil {
		return fail("refers to a shape that cannot be resolved", nil)
	}
	return encodeDynamic(shape.Resolve(), value, into)
}

func sortedFields(fields []dynamic.Field) []dynamic.Field {
	ordered := append([]dynamic.Field{}, fields...)
	slices.SortStableFunc(ordered, func(first dynamic.Field, second dynamic.Field) int {
		return strings.Compare(first.Name, second.Name)
	})
	return ordered
}

// article names a kind the way a message should read.
func article(kind structure.Kind) string {
	switch kind {
	case structure.Integer, structure.Number:
		return "a " + kind.String()
	case structure.Boolean:
		return "a boolean"
	case structure.Bytes:
		return "a byte string"
	case structure.Timestamp:
		return "a timestamp"
	default:
		return "text"
	}
}
