package web

// Reading a request into a struct, one field per codec, so a handler is given
// named fields rather than a nest of Products.

import "cmp"

// Field is one codec's value, assigned to a field of S.
type Field[S any] struct {
	parameters []Parameter
	entity     *Content
	fault      error
	decode     func(Request, *S) error
}

// FieldOf reads codec's value into a field of S. Any codec will do: a
// parameter, the entity, PageParams, or another Struct.
//
//	web.FieldOf(web.QueryParam("q", schema.Text()), func(search *Search, q string) { search.Text = q })
func FieldOf[S, A any](codec Codec[A], assign func(*S, A)) Field[S] {
	field := Field[S]{parameters: codec.parameters, entity: codec.entity, fault: ValidateCodec(codec)}
	field.decode = func(request Request, into *S) error {
		value, err := codec.decode(request)
		if err == nil {
			assign(into, value)
		}
		return err
	}
	return field
}

// Struct reads a request into S, field by field in the order given. It
// declares every field's parameters, so the published document and what is
// read cannot disagree; two fields reading one parameter, or two reading the
// body, are a declaration mistake.
func Struct[S any](fields ...Field[S]) Codec[S] {
	codec := Codec[S]{}
	for _, field := range fields {
		codec.parameters = append(codec.parameters, field.parameters...)
		codec.fault = cmp.Or(codec.fault, field.fault, secondEntityFault(codec.entity, field.entity))
		codec.entity = firstEntity(codec.entity, field.entity)
	}
	if codec.fault = cmp.Or(codec.fault, duplicateParameterFault(codec.parameters)); codec.fault != nil {
		return codec
	}
	codec.decode = func(request Request) (S, error) {
		var value S
		for _, field := range fields {
			if err := field.decode(request, &value); err != nil {
				var zero S
				return zero, err
			}
		}
		return value, nil
	}
	return codec
}

func secondEntityFault(first *Content, second *Content) error {
	if first != nil && second != nil {
		return faultOf("combine codecs", errTwoEntities)
	}
	return nil
}
