package structure

// What produces a value the caller does not supply.
//
// Computed on a field says the value is not the caller's. That is what a
// derived shape needs and all it needs: a field nobody can supply is left out,
// whatever produces it. A projection to storage needs the other half, because a
// column that admits no value and has no default is a column no row can be
// written for -- so this is the other half, and it is a closed set rather than
// a SQL string, because a string would be one dialect's spelling in a
// description that is supposed to outlive the choice of dialect.

import (
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
)

// Default is what a field holds when nobody gives it a value. The set is
// sealed, so a projection switches over it and knows it has covered everything.
type Default interface {
	defaultValue()
}

// DefaultTo is a fixed value.
//
// It carries a dynamic.Value rather than a Go value because a description need
// not have a Go type at all, and because every projection already knows how to
// write one of those.
type DefaultTo struct {
	Value dynamic.Value
}

// DefaultNow is the moment the row is written.
//
// An expression rather than a value, which is why it is its own case: there is
// no instant to put in a description that would still be the right one when the
// row is written.
type DefaultNow struct{}

func (DefaultTo) defaultValue()  {}
func (DefaultNow) defaultValue() {}
