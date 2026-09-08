package schema

// A union whose variants are told apart by a field inside them.
//
//	{"type": "iso2768", "grade": "medium"}
//
// This is the common REST shape and it costs more than naming the variant as
// the object's key. The name may arrive after the fields whose meaning it
// settles, so a decoder has to read the object before it knows what it read --
// which is why it asks the format for that power rather than assuming the
// ordering, and why the other form is still the one to reach for when nothing
// external dictates the wire.

import (
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// OneOfBy describes A as a choice between variants told apart by a field.
//
// The field is written by the union and is not part of a variant's own shape,
// so a Go type does not carry a tag it never reads. A variant that declares a
// field of the same name is a declaration mistake: one of the two would win and
// which is not something to leave to chance.
//
// Every variant must be an object, because a field inside it is where the name
// goes. Variants are tried in declared order on encode, as OneOf's are.
func OneOfBy[A any](name string, discriminator string, variants ...Variant[A]) Schema[A] {
	node := structure.Union{
		Name:          name,
		Discriminator: discriminator,
		Variants:      describeVariants(variants),
	}
	if fault := firstTaggedFault(discriminator, variants); fault != nil {
		return faulted[A](node, fault)
	}

	byName := make(map[string]Variant[A], len(variants))
	for _, variant := range variants {
		byName[variant.name] = variant
	}
	return of[A](
		node,
		func(value A, into Sink) error {
			return encodeTagged(value, discriminator, variants, into)
		},
		func(from Source) (A, error) {
			return decodeTagged[A](from, discriminator, byName)
		},
	)
}

// firstTaggedFault reports what would make the union unusable, at the moment it
// is declared.
func firstTaggedFault[A any](discriminator string, variants []Variant[A]) error {
	if strings.TrimSpace(discriminator) == "" {
		return fail("a union told apart by a field needs the field's name", nil)
	}
	if fault := firstVariantFault(variants); fault != nil {
		return fault
	}
	for _, variant := range variants {
		object, isObject := variant.node.(structure.Object)
		if !isObject {
			return within(variant.name,
				fail("is not an object, and the name goes in a field of one", nil))
		}
		for _, field := range object.Fields {
			if field.Name == discriminator {
				return within(variant.name,
					fail("already has a field named "+discriminator, nil))
			}
		}
	}
	return nil
}

func encodeTagged[A any](
	value A,
	discriminator string,
	variants []Variant[A],
	into Sink,
) error {
	for _, variant := range variants {
		if !variant.matches(value) {
			continue
		}
		// Every variant is an object -- the declaration check above refuses
		// anything else -- so the name always has somewhere to go.
		tagging := &taggingSink{Sink: into, name: discriminator, value: variant.name}
		if err := variant.encode(value, tagging); err != nil {
			return within(variant.name, err)
		}
		return nil
	}
	return fail("no variant matches the value", nil)
}

// taggingSink writes the name into the object the variant writes, which is what
// puts the two in one object without the variant knowing about the union.
type taggingSink struct {
	Sink
	name  string
	value string
	depth int
}

func (sink *taggingSink) BeginObject() error {
	if err := sink.Sink.BeginObject(); err != nil {
		return err
	}
	sink.depth++
	if sink.depth > 1 {
		return nil
	}
	if err := sink.Sink.FieldName(sink.name); err != nil {
		return err
	}
	return sink.Sink.Text(sink.value)
}

func (sink *taggingSink) EndObject() error {
	sink.depth--
	return sink.Sink.EndObject()
}

// decodeTagged reads the object, finds the name, and then reads it again as the
// variant that name selects.
func decodeTagged[A any](
	from Source,
	discriminator string,
	byName map[string]Variant[A],
) (A, error) {
	var missing A
	buffering, can := from.(Buffering)
	if !can {
		return missing, fail(
			"this format cannot read a union told apart by a field, because the "+
				"name may arrive after the fields it settles", nil)
	}
	buffered, err := buffering.Buffer()
	if err != nil {
		return missing, err
	}

	object, isObject := buffered.(dynamic.Object)
	if !isObject {
		return missing, fail("is not an object", nil)
	}
	chosen, err := namedBy(object, discriminator)
	if err != nil {
		return missing, err
	}
	variant, known := byName[chosen]
	if !known {
		return missing, fail("no variant is named "+chosen, nil)
	}
	// The name is the union's, not the variant's, so the variant reads the
	// object it would have written: its own fields and nothing else.
	read, err := variant.decode(&dynamicSource{
		pending: []dynamic.Value{without(object, discriminator)},
	})
	if err != nil {
		return missing, within(chosen, err)
	}
	return read, nil
}

func namedBy(object dynamic.Object, discriminator string) (string, error) {
	held, present := object.Member(discriminator)
	if !present {
		return "", fail("carries no "+discriminator+" to say which variant it is", nil)
	}
	text, isText := held.(dynamic.Text)
	if !isText {
		return "", within(discriminator, fail("is not text", nil))
	}
	return text.Value, nil
}

func without(object dynamic.Object, name string) dynamic.Value {
	kept := dynamic.Object{Fields: make([]dynamic.Field, 0, len(object.Fields))}
	for _, field := range object.Fields {
		if field.Name != name {
			kept.Fields = append(kept.Fields, field)
		}
	}
	return kept
}
