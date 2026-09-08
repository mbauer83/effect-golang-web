package schema

// Decoding into a value whose Go type is not known. The description drives, as
// it always does; what it fills in is the universal representation rather than
// a struct's fields.

import (
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func decodeDynamic(node structure.Node, from Source) (dynamic.Value, error) {
	switch shape := node.(type) {
	case structure.Scalar:
		return decodeDynamicScalar(shape, from)
	case structure.Object:
		return decodeDynamicObject(shape, from)
	case structure.Sequence:
		return decodeDynamicSequence(shape, from)
	case structure.Mapping:
		return decodeDynamicMapping(shape, from)
	case structure.Union:
		return decodeDynamicUnion(shape, from)
	case structure.Nullable:
		absent, err := from.Null()
		if err != nil {
			return nil, err
		}
		if absent {
			return dynamic.Absent{}, nil
		}
		return decodeDynamic(shape.Inner, from)
	case structure.Reference:
		if shape.Resolve == nil {
			return nil, fail("refers to a shape that cannot be resolved", nil)
		}
		return decodeDynamic(shape.Resolve(), from)
	default:
		return nil, fail("has a shape this schema cannot decode", nil)
	}
}

func decodeDynamicScalar(shape structure.Scalar, from Source) (dynamic.Value, error) {
	read, err := readScalar(shape.Kind, from)
	if err != nil {
		return nil, err
	}
	if err := checkConstraints(shape.Constraints, read); err != nil {
		return nil, err
	}
	return read, nil
}

func readScalar(kind structure.Kind, from Source) (dynamic.Value, error) {
	switch kind {
	case structure.Integer:
		value, err := from.Integer()
		return dynamic.Integer{Value: value}, err
	case structure.Number:
		value, err := from.Number()
		return dynamic.Number{Value: value}, err
	case structure.Boolean:
		value, err := from.Boolean()
		return dynamic.Boolean{Value: value}, err
	case structure.Bytes:
		value, err := from.Bytes()
		return dynamic.Bytes{Value: value}, err
	case structure.Timestamp:
		value, err := from.Timestamp()
		return dynamic.Timestamp{Value: value}, err
	default:
		value, err := from.Text()
		return dynamic.Text{Value: value}, err
	}
}

// decodeDynamicObject reads the members a document carries and then puts them
// in the order the description declares, so two documents that differ only in
// member order decode to the same value.
func decodeDynamicObject(shape structure.Object, from Source) (dynamic.Value, error) {
	byName := map[string]dynamic.Value{}
	declared := map[string]structure.Field{}
	for _, field := range shape.Fields {
		declared[field.Name] = field
	}

	err := from.ReadObject(func(name string) error {
		field, known := declared[name]
		if !known {
			// Tolerated, as it is for a struct: a decoder that refused one
			// could not read a document from a newer producer.
			return from.Skip()
		}
		read, err := decodeDynamic(field.Node, from)
		if err != nil {
			return within(name, err)
		}
		byName[name] = read
		return nil
	})
	if err != nil {
		return nil, err
	}
	return orderedObject(shape, byName)
}

func orderedObject(shape structure.Object, byName map[string]dynamic.Value) (dynamic.Value, error) {
	object := dynamic.Object{Fields: make([]dynamic.Field, 0, len(shape.Fields))}
	for _, field := range shape.Fields {
		read, present := byName[field.Name]
		switch {
		case !present && field.Optional:
			continue
		case !present:
			return nil, within(field.Name, fail("required member is missing", nil))
		}
		object.Fields = append(object.Fields, dynamic.Field{Name: field.Name, Value: read})
	}
	return object, nil
}

func decodeDynamicSequence(shape structure.Sequence, from Source) (dynamic.Value, error) {
	list := dynamic.List{Elements: []dynamic.Value{}}
	err := from.ReadList(func() error {
		read, err := decodeDynamic(shape.Element, from)
		if err != nil {
			return within(listIndex(len(list.Elements)), err)
		}
		list.Elements = append(list.Elements, read)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := checkConstraints(shape.Constraints, list); err != nil {
		return nil, err
	}
	return list, nil
}

func decodeDynamicMapping(shape structure.Mapping, from Source) (dynamic.Value, error) {
	mapping := dynamic.Object{Fields: []dynamic.Field{}}
	err := from.ReadObject(func(key string) error {
		read, err := decodeDynamic(shape.Value, from)
		if err != nil {
			return within(key, err)
		}
		mapping.Fields = append(mapping.Fields, dynamic.Field{Name: key, Value: read})
		return nil
	})
	if err != nil {
		return nil, err
	}
	mapping.Fields = sortedFields(mapping.Fields)
	return mapping, nil
}

func decodeDynamicUnion(shape structure.Union, from Source) (dynamic.Value, error) {
	declared := map[string]structure.Variant{}
	for _, variant := range shape.Variants {
		declared[variant.Name] = variant
	}

	chosen := dynamic.Field{}
	err := from.ReadObject(func(name string) error {
		if chosen.Name != "" {
			return fail("a union names one variant, and both "+chosen.Name+
				" and "+name+" are present", nil)
		}
		variant, known := declared[name]
		if !known {
			// Refused rather than skipped: there is no value to build without
			// it, as there is none for a typed union.
			return fail("no variant is named "+name, nil)
		}
		read, err := decodeDynamic(variant.Node, from)
		if err != nil {
			return within(name, err)
		}
		chosen = dynamic.Field{Name: name, Value: read}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if chosen.Name == "" {
		return nil, fail("a union names one variant, and none is present", nil)
	}
	return dynamic.Object{Fields: []dynamic.Field{chosen}}, nil
}
