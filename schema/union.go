package schema

import (
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Variant describes one alternative of A: the name that selects it on the wire,
// its shape, and how to move between that shape and an A.
//
// narrow is a type assertion when A is an interface, which is how Go models a
// sum, and a discriminator test when A is a struct with a kind field.
type Variant[A any] struct {
	name  string
	doc   string
	node  structure.Node
	fault error
	// matches reports whether the value is this alternative, so the name is
	// written only for the variant that will then write the value. It is the
	// same separation an optional field makes between presence and encoding.
	matches func(A) bool
	encode  func(A, Sink) error
	decode  func(Source) (A, error)
}

// VariantOf describes one alternative of a union.
func VariantOf[A, B any](
	name string,
	shape Schema[B],
	narrow func(A) (B, bool),
	widen func(B) A,
) Variant[A] {
	return Variant[A]{
		name:  name,
		node:  shape.node,
		fault: Validate(shape),
		matches: func(value A) bool {
			_, matched := narrow(value)
			return matched
		},
		encode: func(value A, into Sink) error {
			held, matched := narrow(value)
			if !matched {
				return fail("the value is not the "+name+" variant", nil)
			}
			return Encode(shape, held, into)
		},
		decode: func(from Source) (A, error) {
			decoded, err := Decode(shape, from)
			if err != nil {
				var missing A
				return missing, err
			}
			return widen(decoded), nil
		},
	}
}

// DocumentedVariant attaches prose a projection can carry into its output.
func DocumentedVariant[A any](doc string, variant Variant[A]) Variant[A] {
	variant.doc = doc
	return variant
}

// OneOf describes A as a choice between named variants.
//
// The wire form names the variant as the single member of an object:
//
//	{"circle": {"radius": 2}}
//
// The alternative REST idiom, a discriminator field beside the variant's own
// fields, is not available here and the reason is structural rather than a
// preference: a value passes through a token stream in one pass, so a decoder
// that met the fields before the discriminator would have to buffer or rewind
// to know what it had been reading. Naming the variant as the key means the
// selection always arrives first. The same holds for an envelope with separate
// tag and payload members, whose order a producer is free to choose.
//
// Variants are tried in declared order, so a narrower variant belongs before a
// wider one that would also match.
func OneOf[A any](name string, variants ...Variant[A]) Schema[A] {
	node := structure.Union{Name: name, Variants: describeVariants(variants)}
	if fault := firstVariantFault(variants); fault != nil {
		return faulted[A](node, fault)
	}

	byName := make(map[string]Variant[A], len(variants))
	for _, variant := range variants {
		byName[variant.name] = variant
	}
	return of[A](
		node,
		func(value A, into Sink) error {
			return encodeVariant(value, variants, into)
		},
		func(from Source) (A, error) {
			return decodeVariant(from, byName)
		},
	)
}

func describeVariants[A any](variants []Variant[A]) []structure.Variant {
	described := make([]structure.Variant, 0, len(variants))
	for _, variant := range variants {
		described = append(described, structure.Variant{
			Name: variant.name,
			Doc:  variant.doc,
			Node: variant.node,
		})
	}
	return described
}

// firstVariantFault reports a union that could never encode or decode a value:
// one with no variants at all, a nameless or repeated variant name, or a
// variant whose own schema is unusable.
func firstVariantFault[A any](variants []Variant[A]) error {
	if len(variants) == 0 {
		return fail("a union has no variants", nil)
	}
	seen := make(map[string]bool, len(variants))
	for _, variant := range variants {
		switch {
		case strings.TrimSpace(variant.name) == "":
			return fail("a variant has no name", nil)
		case seen[variant.name]:
			return fail("two variants are named "+variant.name, nil)
		case variant.fault != nil:
			return within(variant.name, variant.fault)
		}
		seen[variant.name] = true
	}
	return nil
}

func encodeVariant[A any](value A, variants []Variant[A], into Sink) error {
	for _, variant := range variants {
		if !variant.matches(value) {
			continue
		}
		if err := into.BeginObject(); err != nil {
			return err
		}
		if err := into.FieldName(variant.name); err != nil {
			return err
		}
		if err := variant.encode(value, into); err != nil {
			return within(variant.name, err)
		}
		return into.EndObject()
	}
	return fail("no variant matches the value", nil)
}

func decodeVariant[A any](from Source, byName map[string]Variant[A]) (A, error) {
	var built A
	selected := ""

	err := from.ReadObject(func(name string) error {
		if selected != "" {
			return fail("a union names one variant, and both "+selected+" and "+name+" are present", nil)
		}
		variant, known := byName[name]
		if !known {
			// An unknown variant is refused rather than skipped, unlike an
			// unknown field: there is no value to build without it.
			return fail("no variant is named "+name, nil)
		}
		selected = name
		decoded, err := variant.decode(from)
		if err != nil {
			return within(name, err)
		}
		built = decoded
		return nil
	})
	if err != nil {
		var missing A
		return missing, err
	}
	if selected == "" {
		var missing A
		return missing, fail("a union names one variant, and none is present", nil)
	}
	return built, nil
}
