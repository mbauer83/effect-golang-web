package schema

// Enforcing a constraint from the description alone.
//
// The typed combinators enforce their own bounds as they encode, because each
// one knows the Go type it measures. A description with no Go type has only
// what was recorded, so this reads the recorded vocabulary back -- which is
// what makes recording it worth doing, and what makes the choice to record a
// format's expression alongside its name pay for itself twice.
//
// A rule that could not be recorded cannot be enforced here. A format whose
// rule is grammar -- an address, a URI -- annotates and no more when the
// description is all there is. That is a real limit, and saying so is better
// than a dynamic path that silently admits what the typed path refuses.

import (
	"regexp"
	"strconv"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func checkConstraints(constraints []structure.Constraint, value dynamic.Value) error {
	for _, constraint := range constraints {
		if err := checkConstraint(constraint, value); err != nil {
			return err
		}
	}
	return nil
}

func checkConstraint(constraint structure.Constraint, value dynamic.Value) error {
	switch narrowed := constraint.(type) {
	case structure.AtLeast:
		return checkBound(value, func(measured float64) error {
			if measured < narrowed.Value {
				return fail("is less than "+number(narrowed.Value), nil)
			}
			return nil
		})
	case structure.AtMost:
		return checkBound(value, func(measured float64) error {
			if measured > narrowed.Value {
				return fail("is greater than "+number(narrowed.Value), nil)
			}
			return nil
		})
	case structure.Above:
		return checkBound(value, func(measured float64) error {
			if measured <= narrowed.Value {
				return fail("is not above "+number(narrowed.Value), nil)
			}
			return nil
		})
	case structure.Below:
		return checkBound(value, func(measured float64) error {
			if measured >= narrowed.Value {
				return fail("is not below "+number(narrowed.Value), nil)
			}
			return nil
		})
	case structure.MinLength:
		return checkText(value, func(text string) error {
			if length(text) < narrowed.Value {
				return fail("is shorter than "+strconv.Itoa(narrowed.Value)+" characters", nil)
			}
			return nil
		})
	case structure.MaxLength:
		return checkText(value, func(text string) error {
			if length(text) > narrowed.Value {
				return fail("is longer than "+strconv.Itoa(narrowed.Value)+" characters", nil)
			}
			return nil
		})
	case structure.Pattern:
		return checkText(value, func(text string) error {
			compiled, err := regexp.Compile(narrowed.Expression)
			if err != nil {
				return fail("the pattern does not compile", err)
			}
			if !compiled.MatchString(text) {
				return fail("does not match "+narrowed.Expression, nil)
			}
			return nil
		})
	case structure.MinItems:
		return checkList(value, func(count int) error {
			if count < narrowed.Value {
				return fail("has fewer than "+strconv.Itoa(narrowed.Value)+" items", nil)
			}
			return nil
		})
	case structure.MaxItems:
		return checkList(value, func(count int) error {
			if count > narrowed.Value {
				return fail("has more than "+strconv.Itoa(narrowed.Value)+" items", nil)
			}
			return nil
		})
	default:
		return nil
	}
}

// checkBound measures a number. A bound on something that is not a number is a
// description that disagrees with itself, and saying so beats passing it.
func checkBound(value dynamic.Value, check func(float64) error) error {
	switch held := value.(type) {
	case dynamic.Integer:
		return check(float64(held.Value))
	case dynamic.Number:
		return check(held.Value)
	default:
		return fail("is bounded and is not a number", nil)
	}
}

func checkText(value dynamic.Value, check func(string) error) error {
	held, isText := value.(dynamic.Text)
	if !isText {
		return fail("is constrained as text and is not text", nil)
	}
	return check(held.Value)
}

func checkList(value dynamic.Value, check func(int) error) error {
	held, isList := value.(dynamic.List)
	if !isList {
		return fail("is counted and is not a list", nil)
	}
	return check(len(held.Elements))
}
