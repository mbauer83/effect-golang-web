package protobuf

// Projecting a description into a proto3 file.
//
// Every named shape becomes a message, declared once and referred to
// thereafter, the way the JSON Schema projection shares components. The order
// is the order the shapes were first reached, so the same description always
// produces the same file.

import (
	"errors"
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Timestamps is the well-known type an instant is carried as.
const Timestamps = "google/protobuf/timestamp.proto"

// Project projects a description into a proto3 file in the given package.
//
// It fails rather than approximating. Protobuf's compatibility rests on field
// numbers and message names, so a description missing either is a description
// that cannot be projected -- and saying so beats emitting a file that compiles
// and means something else next release.
func Project(node structure.Node, packageName string) (Document, error) {
	projection := &projector{
		declared: map[string]bool{},
		visiting: map[string]bool{},
	}
	root, err := projection.named(node)
	if err != nil {
		return Document{}, err
	}
	return Document{
		Package:  packageName,
		Imports:  projection.imports,
		Messages: projection.messages,
		Root:     root,
	}, nil
}

type projector struct {
	messages []Message
	declared map[string]bool
	visiting map[string]bool
	imports  []string
}

// named declares the message a node is, and returns its name.
//
// Only an object, a union or a reference has a name; anything else at the root
// is not a protobuf message, because a message is the unit protobuf transfers.
func (projection *projector) named(node structure.Node) (string, error) {
	switch shape := node.(type) {
	case structure.Object:
		return projection.message(shape)
	case structure.Union:
		return projection.oneOf(shape)
	case structure.Reference:
		return projection.reference(shape)
	default:
		return "", fmt.Errorf(
			"protobuf transfers messages, and %T is not one: wrap it in a named struct", node)
	}
}

func (projection *projector) message(object structure.Object) (string, error) {
	if object.Name == "" {
		return "", errUnnamed
	}
	if projection.declared[object.Name] || projection.visiting[object.Name] {
		return object.Name, nil
	}
	projection.visiting[object.Name] = true

	fields := make([]Field, 0, len(object.Fields))
	for _, member := range object.Fields {
		field, err := projection.field(member)
		if err != nil {
			return "", fmt.Errorf("field %q of %s: %w", member.Name, object.Name, err)
		}
		fields = append(fields, field)
	}

	delete(projection.visiting, object.Name)
	projection.declare(Message{Name: object.Name, Doc: object.Doc, Fields: fields})
	return object.Name, nil
}

// oneOf declares the message a union is.
//
// A union is a message holding a oneof rather than a oneof on its own, because
// proto3 has no standalone one: a oneof is a group of fields inside a message.
func (projection *projector) oneOf(union structure.Union) (string, error) {
	if union.Name == "" {
		return "", errUnnamed
	}
	if union.Discriminator != "" {
		return "", errDiscriminated
	}
	if projection.declared[union.Name] || projection.visiting[union.Name] {
		return union.Name, nil
	}
	projection.visiting[union.Name] = true

	fields := make([]Field, 0, len(union.Variants))
	for _, variant := range union.Variants {
		field, err := projection.variant(variant)
		if err != nil {
			return "", fmt.Errorf("variant %q of %s: %w", variant.Name, union.Name, err)
		}
		fields = append(fields, field)
	}

	delete(projection.visiting, union.Name)
	projection.declare(Message{
		Name:   union.Name,
		Doc:    union.Doc,
		Fields: fields,
		OneOf:  "value",
	})
	return union.Name, nil
}

func (projection *projector) reference(reference structure.Reference) (string, error) {
	if reference.Name == "" {
		return "", errUnnamed
	}
	if projection.declared[reference.Name] || projection.visiting[reference.Name] {
		return reference.Name, nil
	}
	if reference.Resolve == nil {
		// A reference the walker cannot follow is a name and nothing else,
		// which is all a proto file needs where the message is declared
		// elsewhere -- but not where this file is the whole contract.
		return "", fmt.Errorf("%q is referred to and not described", reference.Name)
	}
	return projection.named(reference.Resolve())
}

func (projection *projector) declare(message Message) {
	projection.declared[message.Name] = true
	projection.messages = append(projection.messages, message)
}

func (projection *projector) importing(path string) {
	for _, already := range projection.imports {
		if already == path {
			return
		}
	}
	projection.imports = append(projection.imports, path)
}

var (
	errUnnamed = errors.New(
		"a protobuf message has a name, and this shape has none: name it, because " +
			"a name taken from the field holding it would change when that field did")
	errDiscriminated = errors.New(
		"protobuf identifies the chosen member of a oneof by its field number, so a " +
			"union with a discriminating field would encode that choice twice and " +
			"could contradict itself; project the untagged form")
)
