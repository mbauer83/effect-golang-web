package web

// Reading a request is per location and composes structurally, because Go
// cannot compute a type-level record: two codecs combine into a codec of a
// Product, exactly as two environments do in the runtime.

import (
	"errors"

	"github.com/mbauer83/effect-golang-web/schema/structure"
	"github.com/mbauer83/effect-golang/effect"
)

// Location is where a request carries a parameter.
type Location string

const (
	// InPath is a captured segment of the request path.
	InPath Location = "path"
	// InQuery is a query-string parameter.
	InQuery Location = "query"
	// InHeader is a request header.
	InHeader Location = "header"
)

// Parameter is one named value a codec reads, together with what a published
// document should say about it. Reading and describing come from one
// declaration, so they cannot disagree.
type Parameter struct {
	Name     string
	In       Location
	Doc      string
	Required bool
	Node     structure.Node
}

// Content describes an entity: the media type it arrives as and the shape it
// must have.
type Content struct {
	MediaType string
	Node      structure.Node
}

// Codec reads part of a request into A.
//
// It carries what it reads as well as how, because a projection has to walk the
// declaration -- the same reason Schema exposes its structure rather than only
// its codec.
type Codec[A any] struct {
	parameters []Parameter
	// entity is the request body the codec expects, or nil when it reads none.
	entity *Content
	decode func(Request) (A, error)
	// fault records a declaration mistake, reported by ValidateCodec and by the
	// first attempt to use the codec.
	fault error
}

// Parameters are the named values the codec reads, in declared order.
func (codec Codec[A]) Parameters() []Parameter {
	return codec.parameters
}

// Entity is the request body the codec expects, or nil when it reads none.
func (codec Codec[A]) Entity() *Content {
	return codec.entity
}

// ValidateCodec reports a declaration mistake in the codec, or nil.
func ValidateCodec[A any](codec Codec[A]) error {
	if codec.fault != nil {
		return codec.fault
	}
	if codec.decode == nil {
		return faulted("using a codec", errZeroCodec)
	}
	return nil
}

// Decode reads the part of the request the codec describes.
func Decode[A any](codec Codec[A], request Request) (A, error) {
	if err := ValidateCodec(codec); err != nil {
		var missing A
		return missing, err
	}
	return codec.decode(request)
}

// Nothing reads nothing, which is what a route with no input takes.
func Nothing() Codec[effect.Unit] {
	return Codec[effect.Unit]{
		decode: func(Request) (effect.Unit, error) { return effect.Unit{}, nil },
	}
}

// Both combines two codecs into one that reads both parts.
//
// The result is a Product because no information may be discarded and Go has no
// type-level record to widen; it is a package function because a method cannot
// grow the type parameters its own result needs.
func Both[A, B any](first Codec[A], second Codec[B]) Codec[effect.Product[A, B]] {
	combined := Codec[effect.Product[A, B]]{
		parameters: append(append([]Parameter{}, first.parameters...), second.parameters...),
		entity:     firstEntity(first.entity, second.entity),
		fault:      firstCodecFault(first, second),
	}
	if combined.fault != nil {
		return combined
	}
	combined.decode = func(request Request) (effect.Product[A, B], error) {
		read, err := first.decode(request)
		if err != nil {
			return effect.Product[A, B]{}, err
		}
		also, err := second.decode(request)
		if err != nil {
			return effect.Product[A, B]{}, err
		}
		return effect.ProductOf(read, also), nil
	}
	return combined
}

// Convert derives a codec for B from one for A, which is how a Product of
// parts becomes the struct a handler actually wants.
func Convert[A, B any](inner Codec[A], to func(A) (B, error)) Codec[B] {
	if fault := ValidateCodec(inner); fault != nil {
		return Codec[B]{parameters: inner.parameters, entity: inner.entity, fault: fault}
	}
	return Codec[B]{
		parameters: inner.parameters,
		entity:     inner.entity,
		decode: func(request Request) (B, error) {
			read, err := inner.decode(request)
			if err != nil {
				var missing B
				return missing, err
			}
			return to(read)
		},
	}
}

// firstEntity keeps the one body a request has. Two codecs that both read the
// entity are a declaration mistake, reported by firstCodecFault.
func firstEntity(first *Content, second *Content) *Content {
	if first != nil {
		return first
	}
	return second
}

func firstCodecFault[A, B any](first Codec[A], second Codec[B]) error {
	if fault := ValidateCodec(first); fault != nil {
		return fault
	}
	if fault := ValidateCodec(second); fault != nil {
		return fault
	}
	if first.entity != nil && second.entity != nil {
		return faulted("combining codecs", errTwoEntities)
	}
	return repeatedParameter(append(append([]Parameter{}, first.parameters...), second.parameters...))
}

// repeatedParameter reports two codecs reading the same parameter, which would
// describe it twice and leave a reader of the document guessing which applies.
func repeatedParameter(parameters []Parameter) error {
	// The key is a string rather than the Parameter itself: a Parameter carries
	// a structure node, and a node is not always comparable.
	seen := make(map[string]bool, len(parameters))
	for _, parameter := range parameters {
		key := string(parameter.In) + " " + parameter.Name
		if seen[key] {
			return faulted("combining codecs",
				errors.New("two codecs read the "+string(parameter.In)+" parameter "+parameter.Name))
		}
		seen[key] = true
	}
	return nil
}

var (
	errZeroCodec   = errors.New("the zero Codec reads nothing and cannot be used")
	errTwoEntities = errors.New("a request has one body, and two codecs both read it")
)
