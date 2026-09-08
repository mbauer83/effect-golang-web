package jsonschema

// The constraint keywords. They are rendered in a fixed order, so a shape
// carrying two of them always produces the same bytes.

import "encoding/json/jsontext"

// writeBounds renders the constraint keywords in a fixed order, so a shape with
// two of them always produces the same bytes.
func writeBounds(encoder *jsontext.Encoder, bounds Bounds) error {
	for _, keyword := range []struct {
		name  string
		value *float64
	}{
		{"minimum", bounds.Minimum},
		{"maximum", bounds.Maximum},
		{"exclusiveMinimum", bounds.ExclusiveMinimum},
		{"exclusiveMaximum", bounds.ExclusiveMaximum},
	} {
		if err := writeNumber(encoder, keyword.name, keyword.value); err != nil {
			return err
		}
	}
	for _, keyword := range []struct {
		name  string
		value *int
	}{
		{"minLength", bounds.MinLength},
		{"maxLength", bounds.MaxLength},
		{"minItems", bounds.MinItems},
		{"maxItems", bounds.MaxItems},
	} {
		if err := writeCount(encoder, keyword.name, keyword.value); err != nil {
			return err
		}
	}
	return writeString(encoder, "pattern", bounds.Pattern)
}

func writeNumber(encoder *jsontext.Encoder, name string, value *float64) error {
	if value == nil {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String(name)); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.Float(*value))
}

func writeCount(encoder *jsontext.Encoder, name string, value *int) error {
	if value == nil {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String(name)); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.Int(int64(*value)))
}
