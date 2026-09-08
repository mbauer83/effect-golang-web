package structure

// Constraint narrows what a shape admits beyond what its kind says.
//
// A kind says a value is a number; a constraint says which numbers. The set is
// sealed, so a projection switches over it exhaustively and knows it has
// covered everything -- and the vocabulary is deliberately small, because a
// constraint only earns its place here if more than one projection can carry
// it. Anything narrower than this belongs in a refinement, which every
// projection can describe only as the shape underneath it.
type Constraint interface {
	constraint()
}

// AtLeast is an inclusive lower bound on a number.
type AtLeast struct {
	Value float64
}

// AtMost is an inclusive upper bound on a number.
type AtMost struct {
	Value float64
}

// Above is an exclusive lower bound on a number.
type Above struct {
	Value float64
}

// Below is an exclusive upper bound on a number.
type Below struct {
	Value float64
}

// MinLength is the fewest characters a string may carry.
type MinLength struct {
	Value int
}

// MaxLength is the most characters a string may carry.
type MaxLength struct {
	Value int
}

// Pattern is a regular expression a string must match. The syntax is Go's,
// which is RE2; a projection to a format expecting another dialect says so
// rather than translating.
type Pattern struct {
	Expression string
}

// MinItems is the fewest elements a sequence may carry.
type MinItems struct {
	Value int
}

// MaxItems is the most elements a sequence may carry.
type MaxItems struct {
	Value int
}

func (AtLeast) constraint()   {}
func (AtMost) constraint()    {}
func (Above) constraint()     {}
func (Below) constraint()     {}
func (MinLength) constraint() {}
func (MaxLength) constraint() {}
func (Pattern) constraint()   {}
func (MinItems) constraint()  {}
func (MaxItems) constraint()  {}
