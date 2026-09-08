package amqp10

// The third place this module holds a value it cannot name.
//
// A message's application properties and its annotations are AMQP maps, which
// the protocol defines as sets of named values and the library represents as
// map[string]any. That is a genuine boundary rather than a shortcut, and it is
// confined to this file: everything above it works in the universal
// representation. The architecture test names this file.
//
// It is deliberately not shared with the AMQP 0-9-1 boundary, which does the
// same-looking thing. The two protocols permit different value sets -- 1.0 has
// UUIDs, symbols and described types that 0-9-1 has no encoding for, and 0-9-1
// has decimals that 1.0 expresses differently -- and the nested map is a
// different named type in each library. Merging them would invent a third
// concept, "a broker's untyped map", that neither specification has, and the
// first value one protocol accepted and the other did not would split it again.

import (
	"fmt"
	"sort"
	"time"

	broker "github.com/Azure/go-amqp"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
)

// Properties is the application-property map an object makes.
//
// Public because the conversions are the boundary, and a boundary that cannot
// be tested from outside the package is a boundary nobody has checked. Its
// signature says map[string]any rather than the library's named type, so
// nothing above this package acquires the dependency by calling it.
func Properties(named dynamic.Object) (map[string]any, error) {
	if len(named.Fields) == 0 {
		return nil, nil
	}
	made := make(map[string]any, len(named.Fields))
	for _, property := range named.Fields {
		crossed, err := propertyOf(property.Value)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", property.Name, err)
		}
		made[property.Name] = crossed
	}
	return made, nil
}

// Annotations is the annotation map an object makes.
//
// Annotations are keyed by symbol or by ulong rather than by string, which is
// why this is a second function and not a cast: the library's Annotations type
// is keyed by the top type, and the keys this produces are the strings the
// representation had.
func Annotations(named dynamic.Object) (broker.Annotations, error) {
	if len(named.Fields) == 0 {
		return nil, nil
	}
	made := make(broker.Annotations, len(named.Fields))
	for _, annotation := range named.Fields {
		crossed, err := propertyOf(annotation.Value)
		if err != nil {
			return nil, fmt.Errorf("annotation %q: %w", annotation.Name, err)
		}
		made[annotation.Name] = crossed
	}
	return made, nil
}

func propertyOf(value dynamic.Value) (any, error) {
	switch held := value.(type) {
	case dynamic.Absent:
		return nil, nil
	case dynamic.Boolean:
		return held.Value, nil
	case dynamic.Integer:
		return held.Value, nil
	case dynamic.Number:
		return held.Value, nil
	case dynamic.Text:
		return held.Value, nil
	case dynamic.Bytes:
		return held.Value, nil
	case dynamic.Timestamp:
		return held.Value, nil
	case dynamic.Object:
		return Properties(held)
	case dynamic.List:
		return propertiesOf(held)
	default:
		// Unreachable from outside: dynamic.Value is sealed by an unexported
		// method, so this guards against a case being added to the sum without
		// this file being taught it.
		return nil, fmt.Errorf("%T is not a value a property may hold", value)
	}
}

func propertiesOf(list dynamic.List) ([]any, error) {
	made := make([]any, 0, len(list.Elements))
	for index, element := range list.Elements {
		crossed, err := propertyOf(element)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", index, err)
		}
		made = append(made, crossed)
	}
	return made, nil
}

// Named is the object an application-property map makes.
//
// A kind the protocol permits and the representation does not is a property
// this package cannot carry, and saying so beats dropping it: a consumer that
// acted on the properties it could see would be acting on half the message.
func Named(received map[string]any) (dynamic.Object, error) {
	object := dynamic.Object{Fields: make([]dynamic.Field, 0, len(received))}
	for _, name := range sortedNames(received) {
		value, err := carried(received[name])
		if err != nil {
			return dynamic.Object{}, fmt.Errorf("property %q: %w", name, err)
		}
		object.Fields = append(object.Fields, dynamic.Field{Name: name, Value: value})
	}
	return object, nil
}

func carried(received any) (dynamic.Value, error) {
	switch value := received.(type) {
	case nil:
		return dynamic.Absent{}, nil
	case bool:
		return dynamic.Boolean{Value: value}, nil
	case int8:
		return dynamic.Integer{Value: int64(value)}, nil
	case int16:
		return dynamic.Integer{Value: int64(value)}, nil
	case int32:
		return dynamic.Integer{Value: int64(value)}, nil
	case int64:
		return dynamic.Integer{Value: value}, nil
	case int:
		return dynamic.Integer{Value: int64(value)}, nil
	case uint8:
		return dynamic.Integer{Value: int64(value)}, nil
	case uint16:
		return dynamic.Integer{Value: int64(value)}, nil
	case uint32:
		return dynamic.Integer{Value: int64(value)}, nil
	case float32:
		return dynamic.Number{Value: float64(value)}, nil
	case float64:
		return dynamic.Number{Value: value}, nil
	case string:
		return dynamic.Text{Value: value}, nil
	case broker.UUID:
		// A UUID is text everywhere above this: the representation has no case
		// for one, and its string form is what a schema's uuid format reads.
		return dynamic.Text{Value: value.String()}, nil
	case []byte:
		return dynamic.Bytes{Value: value}, nil
	case time.Time:
		return dynamic.Timestamp{Value: value}, nil
	case map[string]any:
		return Named(value)
	case []any:
		return elements(value)
	default:
		return nil, fmt.Errorf("a broker sent %T, which is not a value a property may hold", received)
	}
}

func elements(received []any) (dynamic.Value, error) {
	list := dynamic.List{Elements: make([]dynamic.Value, 0, len(received))}
	for index, element := range received {
		value, err := carried(element)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", index, err)
		}
		list.Elements = append(list.Elements, value)
	}
	return list, nil
}

// sortedNames is the order the properties are read in.
//
// A property map has none of its own; the representation's Object has one,
// because a description declares its members in an order. By name is the only
// order available here, and a deterministic one matters: a consumer that
// forwards the properties it received would otherwise send them differently
// each time.
func sortedNames(received map[string]any) []string {
	names := make([]string, 0, len(received))
	for name := range received {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
