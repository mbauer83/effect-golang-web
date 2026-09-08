package schemagen

// The schema expression a described shape becomes.
//
// It is the inverse of reading a struct: the same combinator calls, derived
// from the description rather than from a Go type. A generated schema and a
// written one stay interchangeable in both directions, which is what keeps
// there being one vocabulary to learn.

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func schemaExpression(node structure.Node) (string, error) {
	switch shape := node.(type) {
	case structure.Scalar:
		return scalarExpression(shape)
	case structure.Sequence:
		return sequenceExpression(shape)
	case structure.Mapping:
		return wrappedExpression("schema.Map", shape.Value)
	case structure.Nullable:
		return wrappedExpression("schema.Nullable", shape.Inner)
	case structure.Object:
		return namedExpression(shape.Name, "an object")
	case structure.Union:
		return namedExpression(shape.Name, "a union")
	case structure.Reference:
		return namedExpression(shape.Name, "a reference")
	default:
		return "", errors.New("this shape has no schema expression")
	}
}

// scalarExpression names the base shape and then the constraints that narrow
// it.
//
// A known format is emitted as its own constructor, and the constraints that
// constructor already carries are not emitted again -- which is worked out by
// asking the constructor what it records, rather than by a second table that
// could fall out of step with it.
func scalarExpression(shape structure.Scalar) (string, error) {
	base, carried := baseExpression(shape)
	return constrainExpression(base, shape.Constraints[carried:])
}

func baseExpression(shape structure.Scalar) (string, int) {
	if known, checked := schema.FormatConstructors[shape.Format]; checked {
		return known.Call, known.Carries
	}
	if shape.Format != "" {
		return "schema.Formatted(" + strconv.Quote(shape.Format) + ")", 0
	}
	if known, precise := schema.PrecisionConstructors[shape.Precision]; precise {
		return known.Call, known.Carries
	}
	return kindExpression(shape.Kind), 0
}

func kindExpression(kind structure.Kind) string {
	switch kind {
	case structure.Integer:
		return "schema.Int64()"
	case structure.Number:
		return "schema.Float64()"
	case structure.Boolean:
		return "schema.Bool()"
	case structure.Bytes:
		return "schema.Bytes()"
	case structure.Timestamp:
		return "schema.Time()"
	default:
		return "schema.Text()"
	}
}

func sequenceExpression(shape structure.Sequence) (string, error) {
	listed, err := wrappedExpression("schema.List", shape.Element)
	if err != nil {
		return "", err
	}
	return constrainExpression(listed, shape.Constraints)
}

func constrainExpression(shape string, constraints []structure.Constraint) (string, error) {
	for _, constraint := range constraints {
		call, value, err := constraintCall(constraint)
		if err != nil {
			return "", err
		}
		shape = call + "(" + shape + ", " + value + ")"
	}
	return shape, nil
}

func constraintCall(constraint structure.Constraint) (call string, value string, err error) {
	switch narrowed := constraint.(type) {
	case structure.AtLeast:
		return "schema.AtLeast", bound(narrowed.Value), nil
	case structure.AtMost:
		return "schema.AtMost", bound(narrowed.Value), nil
	case structure.Above:
		return "schema.Above", bound(narrowed.Value), nil
	case structure.Below:
		return "schema.Below", bound(narrowed.Value), nil
	case structure.MinLength:
		return "schema.MinLength", strconv.Itoa(narrowed.Value), nil
	case structure.MaxLength:
		return "schema.MaxLength", strconv.Itoa(narrowed.Value), nil
	case structure.Pattern:
		return "schema.Matching", strconv.Quote(narrowed.Expression), nil
	case structure.MinItems:
		return "schema.MinItems", strconv.Itoa(narrowed.Value), nil
	case structure.MaxItems:
		return "schema.MaxItems", strconv.Itoa(narrowed.Value), nil
	default:
		return "", "", errors.New("this constraint has no combinator")
	}
}

// bound renders a numeric bound as a Go literal. An untyped constant is used so
// the same text serves an int64 and a float64 shape.
func bound(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func wrappedExpression(combinator string, inner structure.Node) (string, error) {
	within, err := schemaExpression(inner)
	if err != nil {
		return "", err
	}
	return combinator + "(" + within + ")", nil
}

func namedExpression(name string, what string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s has no name, and a schema is referred to by name", what)
	}
	return name + "Schema", nil
}
