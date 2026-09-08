package amqp091

// The other of the two places this module holds a value it cannot name.
//
// A message's headers are an AMQP field table, which the protocol defines as a
// set of named values of a dozen kinds and amqp091-go represents as
// map[string]any. That is a genuine boundary rather than a shortcut, and it is
// confined to this file: everything above it works in the universal
// representation. The architecture test names this file, so the exemption is a
// decision on the record.
//
// It maps better than the database one does. A field table permits every kind
// the representation has, nesting included, so the two agree case for case and
// nothing is narrowed on the way through.

import (
	"fmt"
	"sort"
	"time"

	broker "github.com/rabbitmq/amqp091-go"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
)

// Table is the field table an object makes.
//
// Public because the two conversions are the boundary, and a boundary that
// cannot be tested from outside the package is a boundary nobody has checked.
// Its signature says map[string]any rather than the library's named type, so
// nothing above this package acquires the dependency by calling it.
func Table(named dynamic.Object) (map[string]any, error) {
	if len(named.Fields) == 0 {
		return nil, nil
	}
	made := make(map[string]any, len(named.Fields))
	for _, header := range named.Fields {
		crossed, err := fieldOf(header.Value)
		if err != nil {
			return nil, fmt.Errorf("header %q: %w", header.Name, err)
		}
		made[header.Name] = crossed
	}
	return made, nil
}

func fieldOf(value dynamic.Value) (any, error) {
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
		return Table(held)
	case dynamic.List:
		return fieldsOf(held)
	default:
		// Unreachable from outside: dynamic.Value is sealed by an unexported
		// method, so this guards against a case being added to the sum without
		// this file being taught it.
		return nil, fmt.Errorf("%T is not a value a header may hold", value)
	}
}

func fieldsOf(list dynamic.List) ([]any, error) {
	made := make([]any, 0, len(list.Elements))
	for index, element := range list.Elements {
		crossed, err := fieldOf(element)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", index, err)
		}
		made = append(made, crossed)
	}
	return made, nil
}

// Headers is the object a field table makes.
//
// A kind the protocol permits and the representation does not is a header this
// package cannot carry, and saying so beats dropping it: a consumer that acted
// on the headers it could see would be acting on half the message.
func Headers(received map[string]any) (dynamic.Object, error) {
	object := dynamic.Object{Fields: make([]dynamic.Field, 0, len(received))}
	for _, name := range sortedNames(received) {
		value, err := carried(received[name])
		if err != nil {
			return dynamic.Object{}, fmt.Errorf("header %q: %w", name, err)
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
	case float32:
		return dynamic.Number{Value: float64(value)}, nil
	case float64:
		return dynamic.Number{Value: value}, nil
	case string:
		return dynamic.Text{Value: value}, nil
	case []byte:
		return dynamic.Bytes{Value: value}, nil
	case time.Time:
		return dynamic.Timestamp{Value: value}, nil
	case broker.Table:
		// A nested table arrives as the library's own named type, which a type
		// switch does not match against the underlying one.
		return Headers(value)
	case map[string]any:
		return Headers(value)
	case []any:
		return elements(value)
	default:
		return nil, fmt.Errorf("a broker sent %T, which is not a value a header may hold", received)
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

// sortedNames is the order the headers are read in.
//
// A field table is a map and has no order; the representation's Object has one,
// because a description declares its members in an order. By name is the only
// order available here, and a deterministic one matters: a consumer that
// forwards the headers it received would otherwise send them differently each
// time.
func sortedNames(received map[string]any) []string {
	names := make([]string, 0, len(received))
	for name := range received {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
