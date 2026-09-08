package ddl

// The statements that take a table from one version to the next.
//
// This is where a declared change pays for itself. A rename is a rename --
// "alter table t rename column a to b" -- and the data stays where it is. A
// diff could only have seen a column gone and a column arrived, and the
// statements it wrote would have thrown the column's contents away.

import (
	"errors"
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/evolve"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Alter is the statements that carry an aggregate's root table from one version
// to another, in the order they have to be run.
//
// The root table only. A change to a nested entity is a change to the
// description of that entity, so it has a history of its own -- and a step that
// silently reached into a child would be a step whose effect depended on where
// the change happened to be written.
func Alter(
	dialect Dialect,
	history evolve.History,
	from int,
	to int,
) ([]string, error) {
	if err := history.Fault(); err != nil {
		return nil, err
	}
	changes, err := history.Between(from, to)
	if err != nil {
		return nil, err
	}
	before, err := history.At(from)
	if err != nil {
		return nil, err
	}
	table, isObject := before.(structure.Object)
	if !isObject || table.Name == "" {
		return nil, errUnnamed
	}

	statements := make([]string, 0, len(changes))
	for _, change := range changes {
		written, err := altered(dialect, table.Name, change)
		if err != nil {
			return nil, fmt.Errorf("version %d to %d of %s: %w",
				from, to, history.Name(), err)
		}
		statements = append(statements, written)
	}
	return statements, nil
}

func altered(dialect Dialect, table string, change evolve.Change) (string, error) {
	prefix := "alter table " + dialect.Quoted(table) + " "
	switch held := change.(type) {
	case evolve.Added:
		column, err := columnOf(dialect, held.Field, structure.Field{})
		if err != nil {
			return "", err
		}
		return prefix + "add column " + addedColumn(dialect, column), nil
	case evolve.Removed:
		return prefix + "drop column " + dialect.Quoted(held.Name), nil
	case evolve.Renamed:
		// The whole reason this is declared. Both dialects spell it the same
		// way, and both keep the data.
		return prefix + "rename column " + dialect.Quoted(held.From) +
			" to " + dialect.Quoted(held.To), nil
	case evolve.Retyped:
		return retyped(dialect, prefix, held)
	default:
		return "", fmt.Errorf("%T is not a change this projection can write", change)
	}
}

// addedColumn writes a column being added, without the comments.
//
// A comment belongs above a column in a create statement, where somebody reads
// the schema. In an alter it would be a comment in a migration nobody reads
// twice, so what is written here is the definition alone.
func addedColumn(dialect Dialect, column Column) string {
	written := dialect.Quoted(column.Name) + " " + column.Type
	if !column.Nullable && !column.Identity {
		written += " not null"
	}
	if column.Default != "" {
		written += " default " + column.Default
	}
	return written
}

// retyped writes a change of type, which is the one change the two dialects
// spell differently.
//
// Postgres says "alter column x type y" and MySQL says "modify column x y". The
// difference is not cosmetic: MySQL's form restates the whole definition, so a
// column that was not null stays not null only because this says so again,
// where Postgres's form changes the type and leaves everything else alone.
func retyped(dialect Dialect, prefix string, change evolve.Retyped) (string, error) {
	column, err := columnOf(dialect,
		structure.Field{Name: change.Name, Node: change.Node}, structure.Field{})
	if err != nil {
		return "", err
	}
	form, err := dialect.Retype()
	if err != nil {
		return "", err
	}
	switch form {
	case RetypeWhole:
		return prefix + "modify column " + addedColumn(dialect, column), nil
	default:
		return prefix + "alter column " + dialect.Quoted(column.Name) +
			" type " + column.Type, nil
	}
}

// RetypeForm is how a dialect spells a change of type.
type RetypeForm uint8

const (
	// RetypeTypeOnly changes the type and leaves the rest of the definition
	// alone, which is what Postgres does.
	RetypeTypeOnly RetypeForm = iota
	// RetypeWhole restates the whole definition, which is what MySQL requires
	// -- so anything the restatement leaves out is lost.
	RetypeWhole
)

var errNoRetype = errors.New(
	"this dialect cannot change a column's type in place: make a new column, copy, and drop")
