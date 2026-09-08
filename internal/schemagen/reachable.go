package schemagen

// Which shapes a description reaches.
//
// A description refers to its parts by name, and each named part needs a Go
// type of its own, so binding one shape means binding everything it reaches.
// Discovery follows the order the descriptions declare, so the output is
// stable.

import (
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func reachableShapes(roots []structure.Node) ([]structure.Node, error) {
	found := &discovery{seen: map[string]bool{}}
	for _, root := range roots {
		if err := found.walk(root); err != nil {
			return nil, err
		}
	}
	return found.shapes, nil
}

type discovery struct {
	shapes []structure.Node
	seen   map[string]bool
}

func (found *discovery) walk(node structure.Node) error {
	switch shape := node.(type) {
	case structure.Object:
		return found.named(shape.Name, shape, func() error { return found.members(shape) })
	case structure.Union:
		return found.named(shape.Name, shape, func() error { return found.variants(shape) })
	case structure.Sequence:
		return found.walk(shape.Element)
	case structure.Mapping:
		return found.walk(shape.Value)
	case structure.Nullable:
		return found.walk(shape.Inner)
	case structure.Reference:
		// A reference is a name, and the shape it names is reached from
		// wherever it was declared -- following it here would expand a
		// recursive description forever.
		if shape.Name == "" {
			return fmt.Errorf("a reference with no name cannot be bound")
		}
		return nil
	default:
		return nil
	}
}

// named records a shape once and then walks what it contains. The shape is
// recorded before its parts, so a type appears before the ones it refers to
// and a recursive description terminates.
func (found *discovery) named(name string, shape structure.Node, within func() error) error {
	if name == "" {
		return fmt.Errorf("a shape with no name cannot become a Go type")
	}
	if found.seen[name] {
		return nil
	}
	found.seen[name] = true
	found.shapes = append(found.shapes, shape)
	return within()
}

func (found *discovery) members(shape structure.Object) error {
	for _, member := range shape.Fields {
		if err := found.walk(member.Node); err != nil {
			return fmt.Errorf("%s.%s: %w", shape.Name, member.Name, err)
		}
	}
	return nil
}

func (found *discovery) variants(shape structure.Union) error {
	for _, variant := range shape.Variants {
		if _, isObject := variant.Node.(structure.Object); !isObject {
			return fmt.Errorf("%s.%s: a variant becomes a Go type, so it is an object",
				shape.Name, variant.Name)
		}
		if err := found.walk(variant.Node); err != nil {
			return fmt.Errorf("%s.%s: %w", shape.Name, variant.Name, err)
		}
	}
	return nil
}

// markersFor names the interfaces a type has to satisfy: one per union it is a
// variant of, which is how Go spells a sum.
func markersFor(name string, reachable []structure.Node) []string {
	markers := []string{}
	for _, shape := range reachable {
		union, isUnion := shape.(structure.Union)
		if !isUnion {
			continue
		}
		for _, variant := range union.Variants {
			if object, isObject := variant.Node.(structure.Object); isObject && object.Name == name {
				markers = append(markers, markerName(union.Name))
			}
		}
	}
	return markers
}

// markerName is the unexported method a union's members share. It is
// unexported so nothing outside the package can add a member to the sum, which
// is what makes the set closed.
func markerName(union string) string {
	return "is" + union
}
