package jsonschema

import (
	"bytes"
	"encoding/json/jsontext"
)

// Rendering is explicit and ordered rather than reflective, so the same
// document always produces the same bytes: a reader benefits from the author's
// field order, and a diff benefits from it being stable.

// Dialect is the JSON Schema dialect this projection emits. OpenAPI 3.1 uses
// the same one, which is why one projection serves both a standalone document
// and an OpenAPI component.
const Dialect = "https://json-schema.org/draft/2020-12/schema"

// Render writes the root schema as JSON, with its components under $defs, which
// is where the 2020-12 dialect expects them.
func (document Document) Render() ([]byte, error) {
	var written bytes.Buffer
	encoder := jsontext.NewEncoder(&written)
	if err := document.writeRoot(encoder); err != nil {
		return nil, err
	}
	return written.Bytes(), nil
}

func (document Document) writeRoot(encoder *jsontext.Encoder) error {
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	// The dialect is declared, so a validator reading this document does not
	// have to be told which one to apply. A component embedded in an OpenAPI
	// document is rendered from its Node instead and carries no $schema.
	if err := writeString(encoder, "$schema", Dialect); err != nil {
		return err
	}
	// The root's own members and $defs share one object, so the root is written
	// inline rather than nested.
	if err := writeMembers(encoder, document.Root); err != nil {
		return err
	}
	if len(document.Components) > 0 {
		if err := document.writeDefinitions(encoder); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func (document Document) writeDefinitions(encoder *jsontext.Encoder) error {
	if err := encoder.WriteToken(jsontext.String("$defs")); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, name := range document.ComponentNames() {
		if err := encoder.WriteToken(jsontext.String(name)); err != nil {
			return err
		}
		if err := writeNode(encoder, document.Components[name]); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndObject)
}

// Render writes one schema as JSON, without a dialect declaration.
//
// It is what an enclosing document embeds: an OpenAPI component is a schema on
// its own terms but not a document of its own, so it carries no $schema.
func (node Node) Render() ([]byte, error) {
	var written bytes.Buffer
	encoder := jsontext.NewEncoder(&written)
	if err := writeNode(encoder, node); err != nil {
		return nil, err
	}
	return written.Bytes(), nil
}

func writeNode(encoder *jsontext.Encoder, node Node) error {
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := writeMembers(encoder, node); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

// writeMembers writes a node's members without the surrounding braces, so the
// root can share an object with $defs.
func writeMembers(encoder *jsontext.Encoder, node Node) error {
	if node.Ref != "" {
		return writeString(encoder, "$ref", node.Ref)
	}

	if err := writeType(encoder, node); err != nil {
		return err
	}
	for _, member := range []struct{ name, value string }{
		{"format", node.Format},
		{"description", node.Description},
		{"const", node.Const},
	} {
		if err := writeString(encoder, member.name, member.value); err != nil {
			return err
		}
	}
	if err := writeBounds(encoder, node.Bounds); err != nil {
		return err
	}
	if err := writeProperties(encoder, node); err != nil {
		return err
	}
	if err := writeChild(encoder, "items", node.Items); err != nil {
		return err
	}
	if err := writeChild(encoder, "additionalProperties", node.Values); err != nil {
		return err
	}
	if err := writeVariants(encoder, node); err != nil {
		return err
	}
	if err := writeMembersOf(encoder, "allOf", node.AllOf); err != nil {
		return err
	}
	return writeDiscriminator(encoder, node.Discriminator)
}

func writeType(encoder *jsontext.Encoder, node Node) error {
	if node.Type == "" {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String("type")); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.String(node.Type))
}

func writeProperties(encoder *jsontext.Encoder, node Node) error {
	if len(node.Properties) == 0 {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String("properties")); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, property := range node.Properties {
		if err := encoder.WriteToken(jsontext.String(property.Name)); err != nil {
			return err
		}
		if err := writeNode(encoder, property.Schema); err != nil {
			return err
		}
	}
	if err := encoder.WriteToken(jsontext.EndObject); err != nil {
		return err
	}
	if len(node.Required) == 0 {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String("required")); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, name := range node.Required {
		if err := encoder.WriteToken(jsontext.String(name)); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}

func writeVariants(encoder *jsontext.Encoder, node Node) error {
	if len(node.OneOf) == 0 {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String("oneOf")); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, variant := range node.OneOf {
		if err := writeNode(encoder, variant); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}

func writeChild(encoder *jsontext.Encoder, name string, child *Node) error {
	if child == nil {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String(name)); err != nil {
		return err
	}
	return writeNode(encoder, *child)
}

func writeString(encoder *jsontext.Encoder, name string, value string) error {
	if value == "" {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String(name)); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.String(value))
}
