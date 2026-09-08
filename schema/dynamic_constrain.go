package schema

// Applying the constraints a description recorded, using the combinators they
// came from.
//
// There is one function per kind rather than one generic function, for the
// reason the combinators themselves are per kind: measuring a value is
// type-dependent, and Go cannot dispatch a generic call on what A happens to
// be. Each of these is called from a place that knows the type, which is what
// makes reusing the real combinator possible -- and reusing it is what keeps
// one opinion about what a shape admits.

import (
	"math"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func constrainText(inner Schema[string], constraints []structure.Constraint) Schema[string] {
	for _, constraint := range constraints {
		switch narrowed := constraint.(type) {
		case structure.MinLength:
			inner = MinLength(inner, narrowed.Value)
		case structure.MaxLength:
			inner = MaxLength(inner, narrowed.Value)
		case structure.Pattern:
			inner = Matching(inner, narrowed.Expression)
		default:
			return faulted[string](inner.node, misplacedConstraint("text"))
		}
	}
	return inner
}

func constrainInteger(inner Schema[int64], constraints []structure.Constraint) Schema[int64] {
	for _, constraint := range constraints {
		switch narrowed := constraint.(type) {
		case structure.AtLeast:
			inner = AtLeast(inner, wireBound(narrowed.Value))
		case structure.AtMost:
			inner = AtMost(inner, wireBound(narrowed.Value))
		case structure.Above:
			inner = Above(inner, wireBound(narrowed.Value))
		case structure.Below:
			inner = Below(inner, wireBound(narrowed.Value))
		default:
			return faulted[int64](inner.node, misplacedConstraint("a whole number"))
		}
	}
	return inner
}

func constrainNumber(inner Schema[float64], constraints []structure.Constraint) Schema[float64] {
	for _, constraint := range constraints {
		switch narrowed := constraint.(type) {
		case structure.AtLeast:
			inner = AtLeast(inner, narrowed.Value)
		case structure.AtMost:
			inner = AtMost(inner, narrowed.Value)
		case structure.Above:
			inner = Above(inner, narrowed.Value)
		case structure.Below:
			inner = Below(inner, narrowed.Value)
		default:
			return faulted[float64](inner.node, misplacedConstraint("a number"))
		}
	}
	return inner
}

func constrainList[A any](inner Schema[[]A], constraints []structure.Constraint) Schema[[]A] {
	for _, constraint := range constraints {
		switch narrowed := constraint.(type) {
		case structure.MinItems:
			inner = MinItems(inner, narrowed.Value)
		case structure.MaxItems:
			inner = MaxItems(inner, narrowed.Value)
		default:
			return faulted[[]A](inner.node, misplacedConstraint("a list"))
		}
	}
	return inner
}

// wireBound converts a recorded bound back to the width the wire carries,
// saturating rather than wrapping.
//
// A bound is recorded as a float64, which cannot hold the largest int64
// exactly: the nearest float to MaxInt64 is one above it. Saturating keeps the
// meaning -- a bound at the edge of the range means "no further" -- where
// converting would overflow into a negative number.
func wireBound(bound float64) int64 {
	switch {
	case bound >= math.MaxInt64:
		return math.MaxInt64
	case bound <= math.MinInt64:
		return math.MinInt64
	default:
		return int64(bound)
	}
}

func misplacedConstraint(kind string) error {
	return fail("carries a constraint that does not narrow "+kind, nil)
}
