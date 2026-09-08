package protobuf

// One field of a message, and the proto type a shape is.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func (projection *projector) field(member structure.Field) (Field, error) {
	if member.Number < 1 {
		return Field{}, errUnnumbered
	}
	kind, err := projection.shape(member.Node)
	if err != nil {
		return Field{}, err
	}
	return Field{
		Name:   member.Name,
		Doc:    firstParagraph(member.Doc),
		Number: member.Number,
		Type:   kind.name,
		// A repeated field has no explicit presence in proto3, and an empty
		// list is how it says it has none -- so Optional and Repeated are
		// never both set, and the description asking for both is the
		// description saying the same thing twice.
		Repeated: kind.repeated,
		Optional: (member.Optional || kind.nullable) && !kind.repeated,
		Notes:    kind.notes,
	}, nil
}

func (projection *projector) variant(variant structure.Variant) (Field, error) {
	if variant.Number < 1 {
		return Field{}, errUnnumbered
	}
	kind, err := projection.shape(variant.Node)
	if err != nil {
		return Field{}, err
	}
	if kind.repeated {
		// A oneof member cannot be repeated: proto3 says so, because the
		// presence of a oneof member is what selects it and a repeated field
		// has no presence. A variant that is a list needs a message of its own
		// holding the list.
		return Field{}, errRepeatedVariant
	}
	return Field{
		Name:   variant.Name,
		Doc:    firstParagraph(variant.Doc),
		Number: variant.Number,
		Type:   kind.name,
		Notes:  kind.notes,
	}, nil
}

// shape is the proto type a node is, and what else the field has to say.
type shape struct {
	name     string
	repeated bool
	nullable bool
	notes    []string
}

func (projection *projector) shape(node structure.Node) (shape, error) {
	switch held := node.(type) {
	case structure.Scalar:
		return projection.scalar(held)
	case structure.Object:
		name, err := projection.message(held)
		return shape{name: name}, err
	case structure.Union:
		name, err := projection.oneOf(held)
		return shape{name: name}, err
	case structure.Reference:
		name, err := projection.reference(held)
		return shape{name: name}, err
	case structure.Sequence:
		return projection.sequence(held)
	case structure.Mapping:
		return projection.mapping(held)
	case structure.Nullable:
		inner, err := projection.shape(held.Inner)
		inner.nullable = true
		return inner, err
	default:
		return shape{}, fmt.Errorf("%T has no proto3 form", node)
	}
}

func (projection *projector) sequence(sequence structure.Sequence) (shape, error) {
	element, err := projection.shape(sequence.Element)
	if err != nil {
		return shape{}, err
	}
	if element.repeated {
		// proto3 has no repeated repeated. A list of lists is a list of a
		// message holding a list, which is a shape the description can state
		// and this cannot invent.
		return shape{}, errNestedList
	}
	return shape{
		name:     element.name,
		repeated: true,
		notes:    append(element.notes, noted(sequence.Constraints)...),
	}, nil
}

func (projection *projector) mapping(mapping structure.Mapping) (shape, error) {
	key, isScalar := mapping.Key.(structure.Scalar)
	if !isScalar || key.Kind != structure.Text {
		// proto3 permits an integral or string key and nothing else. The
		// description's mappings are string-keyed, so anything else here is a
		// description this projection was not built for rather than a
		// limitation worth working around.
		return shape{}, errMapKey
	}
	value, err := projection.shape(mapping.Value)
	if err != nil {
		return shape{}, err
	}
	if value.repeated {
		// A map of lists is a map to a message holding a list, for the reason
		// a list of lists is.
		return shape{}, errNestedList
	}
	return shape{name: "map<string, " + value.name + ">", notes: value.notes}, nil
}

var (
	errUnnumbered = errors.New(
		"protobuf identifies a field by its number, and this one has none: number it " +
			"with Numbered, because a number taken from declaration order would " +
			"change when the declaration was reordered")
	errRepeatedVariant = errors.New(
		"a oneof member cannot be repeated in proto3, because the presence of a " +
			"member is what selects it; give the variant a message of its own")
	errNestedList = errors.New(
		"proto3 has no repeated repeated and no map of repeated; describe the inner " +
			"list as a member of a named struct")
	errMapKey = errors.New("a proto3 map key is a string or an integer")
)

// firstParagraph is the part of a doc comment that belongs in a published
// contract.
//
// A description's prose may explain how the shape is used here as well as what
// it is; the first paragraph is what it is, and the rest is this program's own
// business rather than something to put in a file other languages generate
// from.
func firstParagraph(doc string) string {
	if split := strings.Index(doc, "\n\n"); split >= 0 {
		return strings.TrimSpace(doc[:split])
	}
	return strings.TrimSpace(doc)
}
