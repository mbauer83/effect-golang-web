package schema

// A constraint narrows what a shape admits and says so in the description, so
// one declaration both refuses a value and appears in the published contract.
// Enforcement happens on encode as well as decode: a value this program built
// that breaks its own constraint is a mistake here, and finding it at the
// boundary is better than sending it.
//
// Each constraint is its own combinator rather than one generic Constrained,
// because measuring a value is type-dependent -- a number is compared, a string
// is counted, a list is counted differently -- and a generic one would have to
// take a measuring function nobody wants to write.

import (
	"regexp"
	"strconv"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// numeric is every Go type a bound can be stated over.
//
// It is the whole numeric breadth of the language rather than the two shapes
// the wire has, which is what makes a bound outside a type's range a compile
// error: AtMost(Int8(), 200) does not build, because 200 is not an int8.
type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// AtLeast admits values no less than minimum.
func AtLeast[A numeric](inner Schema[A], minimum A) Schema[A] {
	return constrained(inner, structure.AtLeast{Value: float64(minimum)},
		func(value A) error {
			if value < minimum {
				return fail("is less than "+number(minimum), nil)
			}
			return nil
		})
}

// AtMost admits values no greater than maximum.
func AtMost[A numeric](inner Schema[A], maximum A) Schema[A] {
	return constrained(inner, structure.AtMost{Value: float64(maximum)},
		func(value A) error {
			if value > maximum {
				return fail("is greater than "+number(maximum), nil)
			}
			return nil
		})
}

// Above admits values strictly greater than the bound.
func Above[A numeric](inner Schema[A], bound A) Schema[A] {
	return constrained(inner, structure.Above{Value: float64(bound)},
		func(value A) error {
			if value <= bound {
				return fail("is not above "+number(bound), nil)
			}
			return nil
		})
}

// Below admits values strictly less than the bound.
func Below[A numeric](inner Schema[A], bound A) Schema[A] {
	return constrained(inner, structure.Below{Value: float64(bound)},
		func(value A) error {
			if value >= bound {
				return fail("is not below "+number(bound), nil)
			}
			return nil
		})
}

// MinLength admits strings of at least the given length, counted in characters
// rather than bytes: a length a client can check is the one it can see.
func MinLength(inner Schema[string], atLeast int) Schema[string] {
	return constrained(inner, structure.MinLength{Value: atLeast},
		func(value string) error {
			if length(value) < atLeast {
				return fail("is shorter than "+strconv.Itoa(atLeast)+" characters", nil)
			}
			return nil
		})
}

// MaxLength admits strings of at most the given length, in characters.
func MaxLength(inner Schema[string], atMost int) Schema[string] {
	return constrained(inner, structure.MaxLength{Value: atMost},
		func(value string) error {
			if length(value) > atMost {
				return fail("is longer than "+strconv.Itoa(atMost)+" characters", nil)
			}
			return nil
		})
}

// Matching admits strings the expression matches. The syntax is Go's, which is
// RE2; a pattern that does not compile is a declaration mistake and is reported
// by Validate rather than panicking at the first request.
func Matching(inner Schema[string], expression string) Schema[string] {
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return faulted[string](inner.node, fail("the pattern does not compile", err))
	}
	return constrained(inner, structure.Pattern{Expression: expression},
		func(value string) error {
			if !compiled.MatchString(value) {
				return fail("does not match "+expression, nil)
			}
			return nil
		})
}

// MinItems admits sequences of at least the given length.
func MinItems[A any](inner Schema[[]A], atLeast int) Schema[[]A] {
	return constrained(inner, structure.MinItems{Value: atLeast},
		func(value []A) error {
			if len(value) < atLeast {
				return fail("has fewer than "+strconv.Itoa(atLeast)+" items", nil)
			}
			return nil
		})
}

// MaxItems admits sequences of at most the given length.
func MaxItems[A any](inner Schema[[]A], atMost int) Schema[[]A] {
	return constrained(inner, structure.MaxItems{Value: atMost},
		func(value []A) error {
			if len(value) > atMost {
				return fail("has more than "+strconv.Itoa(atMost)+" items", nil)
			}
			return nil
		})
}

// constrained records the constraint in the description and checks it in both
// directions.
func constrained[A any](
	inner Schema[A],
	constraint structure.Constraint,
	check func(A) error,
) Schema[A] {
	if fault := Validate(inner); fault != nil {
		return faulted[A](inner.node, fault)
	}
	node, applies := withConstraint(inner.node, constraint)
	if !applies {
		return faulted[A](inner.node,
			fail("a constraint applies to a scalar or a list, and this is neither", nil))
	}
	return of(
		node,
		func(value A, into Sink) error {
			if err := check(value); err != nil {
				return err
			}
			return Encode(inner, value, into)
		},
		func(from Source) (A, error) {
			decoded, err := Decode(inner, from)
			if err != nil {
				return decoded, err
			}
			if err := check(decoded); err != nil {
				var missing A
				return missing, err
			}
			return decoded, nil
		},
	)
}

// withConstraint attaches a constraint to the shape it belongs to.
//
// A constraint reaches through a refinement, because a refinement keeps the
// shape it was derived from and a bound on that shape is still a bound. It does
// not reach through an object or a union: a constraint on one of those would
// have to say which member it meant.
func withConstraint(node structure.Node, constraint structure.Constraint) (structure.Node, bool) {
	switch shape := node.(type) {
	case structure.Scalar:
		shape.Constraints = append(append([]structure.Constraint{}, shape.Constraints...), constraint)
		return shape, true
	case structure.Sequence:
		shape.Constraints = append(append([]structure.Constraint{}, shape.Constraints...), constraint)
		return shape, true
	default:
		return node, false
	}
}

// number renders a bound the way a message should read: 1 rather than 1e+00.
func number[A numeric](value A) string {
	return strconv.FormatFloat(float64(value), 'g', -1, 64)
}

// length counts characters rather than bytes, so a constraint on a string means
// what a reader of the contract would expect it to mean.
func length(value string) int {
	return len([]rune(value))
}
