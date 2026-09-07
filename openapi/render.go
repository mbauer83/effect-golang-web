package openapi

// Rendering is explicit and ordered rather than reflective, so the same routes
// always produce the same bytes: a reader benefits from the author's order, and
// a diff benefits from it being stable.

import (
	"bytes"
	"encoding/json/jsontext"
	"strconv"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
)

// Version is the specification this projection emits.
const Version = "3.1.0"

// Render writes the document as JSON.
func (document Document) Render() ([]byte, error) {
	var written bytes.Buffer
	encoder := jsontext.NewEncoder(&written)
	if err := document.write(encoder); err != nil {
		return nil, err
	}
	return written.Bytes(), nil
}

func (document Document) write(encoder *jsontext.Encoder) error {
	if err := begin(encoder); err != nil {
		return err
	}
	if err := text(encoder, "openapi", Version); err != nil {
		return err
	}
	if err := document.writeInfo(encoder); err != nil {
		return err
	}
	if err := document.writeServers(encoder); err != nil {
		return err
	}
	if err := document.writePaths(encoder); err != nil {
		return err
	}
	if err := document.writeComponents(encoder); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func (document Document) writeInfo(encoder *jsontext.Encoder) error {
	if err := member(encoder, "info"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{
		{"title", document.Info.Title},
		{"version", document.Info.Version},
		{"description", document.Info.Description},
	} {
		if err := text(encoder, field.name, field.value); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func (document Document) writeServers(encoder *jsontext.Encoder) error {
	if len(document.Servers) == 0 {
		return nil
	}
	if err := member(encoder, "servers"); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, server := range document.Servers {
		if err := begin(encoder); err != nil {
			return err
		}
		if err := text(encoder, "url", server.URL); err != nil {
			return err
		}
		if err := text(encoder, "description", server.Description); err != nil {
			return err
		}
		if err := encoder.WriteToken(jsontext.EndObject); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}

func (document Document) writePaths(encoder *jsontext.Encoder) error {
	if err := member(encoder, "paths"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	for _, path := range document.Paths {
		if err := member(encoder, path.Path); err != nil {
			return err
		}
		if err := begin(encoder); err != nil {
			return err
		}
		for _, operation := range path.Operations {
			if err := member(encoder, strings.ToLower(operation.Method)); err != nil {
				return err
			}
			if err := writeOperation(encoder, operation); err != nil {
				return err
			}
		}
		if err := encoder.WriteToken(jsontext.EndObject); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func (document Document) writeComponents(encoder *jsontext.Encoder) error {
	if len(document.Components) == 0 {
		return nil
	}
	if err := member(encoder, "components"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	if err := member(encoder, "schemas"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	for _, name := range componentNames(document.Components) {
		if err := member(encoder, name); err != nil {
			return err
		}
		if err := writeSchema(encoder, document.Components[name]); err != nil {
			return err
		}
	}
	if err := encoder.WriteToken(jsontext.EndObject); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

// writeSchema embeds a projected schema. The schema layer renders it, so there
// is one renderer for a shape and this one never has to know what a shape can
// contain.
func writeSchema(encoder *jsontext.Encoder, node jsonschema.Node) error {
	rendered, err := node.Render()
	if err != nil {
		return err
	}
	return encoder.WriteValue(rendered)
}

func componentNames(components map[string]jsonschema.Node) []string {
	document := jsonschema.Document{Components: components}
	return document.ComponentNames()
}

func begin(encoder *jsontext.Encoder) error {
	return encoder.WriteToken(jsontext.BeginObject)
}

func member(encoder *jsontext.Encoder, name string) error {
	return encoder.WriteToken(jsontext.String(name))
}

// text writes a member, or nothing when there is nothing to say.
func text(encoder *jsontext.Encoder, name string, value string) error {
	if value == "" {
		return nil
	}
	if err := member(encoder, name); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.String(value))
}

func status(code int) string {
	return strconv.Itoa(code)
}
