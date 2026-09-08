package openapi

import (
	"encoding/json/jsontext"

	"github.com/mbauer83/effect-golang-schema/schema/jsonschema"
)

func writeOperation(encoder *jsontext.Encoder, operation Operation) error {
	if err := begin(encoder); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{
		{"operationId", operation.ID},
		{"summary", operation.Summary},
		{"description", operation.Description},
	} {
		if err := text(encoder, field.name, field.value); err != nil {
			return err
		}
	}
	if err := writeParameters(encoder, operation.Parameters); err != nil {
		return err
	}
	if err := writeRequestBody(encoder, operation.RequestBody); err != nil {
		return err
	}
	if err := writeResponses(encoder, operation.Responses); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func writeParameters(encoder *jsontext.Encoder, parameters []Parameter) error {
	if len(parameters) == 0 {
		return nil
	}
	if err := member(encoder, "parameters"); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, parameter := range parameters {
		if err := writeParameter(encoder, parameter); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}

func writeParameter(encoder *jsontext.Encoder, parameter Parameter) error {
	if err := begin(encoder); err != nil {
		return err
	}
	if err := text(encoder, "name", parameter.Name); err != nil {
		return err
	}
	if err := text(encoder, "in", parameter.In); err != nil {
		return err
	}
	if err := text(encoder, "description", parameter.Description); err != nil {
		return err
	}
	// Required is written even when false, because absent and false mean the
	// same thing here and a reader should not have to know that.
	if err := member(encoder, "required"); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.Bool(parameter.Required)); err != nil {
		return err
	}
	if err := member(encoder, "schema"); err != nil {
		return err
	}
	if err := writeSchema(encoder, parameter.Schema); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func writeRequestBody(encoder *jsontext.Encoder, body *RequestBody) error {
	if body == nil {
		return nil
	}
	if err := member(encoder, "requestBody"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	if err := member(encoder, "required"); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.Bool(body.Required)); err != nil {
		return err
	}
	if err := writeContent(encoder, body.MediaType, &body.Schema); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

func writeResponses(encoder *jsontext.Encoder, responses []Response) error {
	if err := member(encoder, "responses"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	for _, response := range responses {
		if err := member(encoder, status(response.Status)); err != nil {
			return err
		}
		if err := begin(encoder); err != nil {
			return err
		}
		if err := text(encoder, "description", response.Description); err != nil {
			return err
		}
		if err := writeContent(encoder, response.MediaType, response.Schema); err != nil {
			return err
		}
		if err := encoder.WriteToken(jsontext.EndObject); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndObject)
}

// writeContent writes a content map, or nothing when there is no entity. A
// media type with no schema is written as itself: the entity is that type and
// nothing here can say more about its shape.
func writeContent(encoder *jsontext.Encoder, mediaType string, schema *jsonschema.Node) error {
	if mediaType == "" {
		return nil
	}
	if err := member(encoder, "content"); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	if err := member(encoder, mediaType); err != nil {
		return err
	}
	if err := begin(encoder); err != nil {
		return err
	}
	if schema != nil {
		if err := member(encoder, "schema"); err != nil {
			return err
		}
		if err := writeSchema(encoder, *schema); err != nil {
			return err
		}
	}
	if err := encoder.WriteToken(jsontext.EndObject); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}
