package schema

// A schema for a shape with no Go type.
//
// Schema[A] moves a Go value in and out of a format. A description written
// before the type it will become exists -- or loaded from somewhere else, or
// read back out of another schema -- has no A, and it is still worth
// validating, transcoding, inspecting and composing. Dynamic gives it one: the
// universal representation, which every combinator here already works over
// because a Schema does not care what A is.
//
// Nothing else changes. The same constructors build it, the same codecs read
// and write it, the same projections describe it, and the same Validate reports
// a mistake in it. That is the point: one vocabulary, used with a Go type or
// without one.

import (
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Dynamic derives a schema over the universal representation from a
// description.
//
// It is the door between the two ways of using this package: a typed schema's
// Structure passed through here is usable without its type, and a description
// assembled by hand or loaded from elsewhere is usable at all.
//
// What it can enforce is what the description records. A bound, a length, a
// pattern and an item count are all there; a rule that could only be code --
// an address, a URI, a refinement -- annotates and no more, because there was
// nothing to record. That is the honest limit of describing a shape rather than
// writing one.
func Dynamic(node structure.Node) Schema[dynamic.Value] {
	if node == nil {
		return faulted[dynamic.Value](nil, fail("a description is required", nil))
	}
	return of(
		node,
		func(value dynamic.Value, into Sink) error {
			if value == nil {
				return fail("is missing", nil)
			}
			return encodeDynamic(node, value, into)
		},
		func(from Source) (dynamic.Value, error) {
			return decodeDynamic(node, from)
		},
	)
}

// Record describes an object with no Go type, which is the shape a generator
// reads and a loaded description arrives as.
//
// It is Struct without the accessors: a struct's getters and setters are the
// only part of a field declaration that needs the Go type, so leaving them out
// is exactly the difference between describing a shape and binding one.
func Record(name string, members ...structure.Field) Schema[dynamic.Value] {
	node := structure.Object{Name: name, Fields: members}
	if fault := firstMemberFault(members); fault != nil {
		return faulted[dynamic.Value](node, fault)
	}
	return Dynamic(node)
}

// MemberOf describes one member of a Record, taking its shape from a schema of
// any type: the shape is what a description needs, and the type is what it does
// not.
func MemberOf[B any](name string, shape Schema[B]) structure.Field {
	return structure.Field{Name: name, Node: describedNode(shape)}
}

// OptionalMemberOf describes a member that may be absent.
func OptionalMemberOf[B any](name string, shape Schema[B]) structure.Field {
	return structure.Field{Name: name, Node: describedNode(shape), Optional: true}
}

// DocumentedMember attaches prose a projection can carry into its output.
func DocumentedMember(doc string, member structure.Field) structure.Field {
	member.Doc = doc
	return member
}

// Choice describes a union with no Go type.
func Choice(name string, alternatives ...structure.Variant) Schema[dynamic.Value] {
	node := structure.Union{Name: name, Variants: alternatives}
	if fault := firstAlternativeFault(alternatives); fault != nil {
		return faulted[dynamic.Value](node, fault)
	}
	return Dynamic(node)
}

// AlternativeOf describes one alternative of a Choice.
func AlternativeOf[B any](name string, shape Schema[B]) structure.Variant {
	return structure.Variant{Name: name, Node: describedNode(shape)}
}

// DocumentedAlternative attaches prose a projection can carry into its output.
func DocumentedAlternative(doc string, alternative structure.Variant) structure.Variant {
	alternative.Doc = doc
	return alternative
}

// describedNode is a schema's shape, or a node that says the schema was
// unusable -- so a mistake in a member's own schema is reported by the record
// rather than disappearing into a description that looks complete.
func describedNode[B any](shape Schema[B]) structure.Node {
	if Validate(shape) != nil {
		return nil
	}
	return shape.node
}

func firstMemberFault(members []structure.Field) error {
	if len(members) == 0 {
		return fail("a record has at least one member", nil)
	}
	seen := make(map[string]bool, len(members))
	for _, member := range members {
		switch {
		case strings.TrimSpace(member.Name) == "":
			return fail("a member has no name", nil)
		case seen[member.Name]:
			return fail("two members are named "+member.Name, nil)
		case member.Node == nil:
			return within(member.Name, fail("has no usable shape", nil))
		}
		seen[member.Name] = true
	}
	return nil
}

func firstAlternativeFault(alternatives []structure.Variant) error {
	if len(alternatives) == 0 {
		return fail("a union has no variants", nil)
	}
	seen := make(map[string]bool, len(alternatives))
	for _, alternative := range alternatives {
		switch {
		case strings.TrimSpace(alternative.Name) == "":
			return fail("a variant has no name", nil)
		case seen[alternative.Name]:
			return fail("two variants are named "+alternative.Name, nil)
		case alternative.Node == nil:
			return within(alternative.Name, fail("has no usable shape", nil))
		}
		seen[alternative.Name] = true
	}
	return nil
}
