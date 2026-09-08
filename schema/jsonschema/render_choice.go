package jsonschema

// The keywords a choice needs: the alternatives, what a variant is combined
// with, and the field that says which variant a value is.

import "encoding/json/jsontext"

// writeDiscriminator names the field that says which variant a value is.
func writeDiscriminator(encoder *jsontext.Encoder, discriminator string) error {
	if discriminator == "" {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String("discriminator")); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := writeString(encoder, "propertyName", discriminator); err != nil {
		return err
	}
	return encoder.WriteToken(jsontext.EndObject)
}

// writeMembersOf renders a list of schemas under one keyword.
func writeMembersOf(encoder *jsontext.Encoder, keyword string, members []Node) error {
	if len(members) == 0 {
		return nil
	}
	if err := encoder.WriteToken(jsontext.String(keyword)); err != nil {
		return err
	}
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, member := range members {
		if err := writeNode(encoder, member); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}
