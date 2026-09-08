// Package structure is the description of a value's shape, with the types
// erased.
//
// A schema's typed surface is a view over one of these nodes, exactly as the
// runtime's typed Cause is a view over an erased cause tree. Projections --
// JSON Schema, OpenAPI, protobuf descriptors, SQL column lists -- walk the
// node, so the description has to exist as data and not only as a pair of codec
// functions.
//
// It is public rather than internal because a projection is an extension point.
// The built-in ones have no privileged access: a projection to Avro, to a
// database migration or to a form renderer is written the same way.
package structure

// Kind identifies a scalar shape.
type Kind uint8

const (
	// Text is a UTF-8 string.
	Text Kind = iota
	// Integer is a signed whole number.
	Integer
	// Number is a floating-point number.
	Number
	// Boolean is true or false.
	Boolean
	// Bytes is an opaque byte string.
	Bytes
	// Timestamp is an instant in time.
	Timestamp
)

// String names a kind for diagnostics and for projections that use the name.
func (kind Kind) String() string {
	switch kind {
	case Integer:
		return "integer"
	case Number:
		return "number"
	case Boolean:
		return "boolean"
	case Bytes:
		return "bytes"
	case Timestamp:
		return "timestamp"
	default:
		return "text"
	}
}

// Node describes the shape of a value. The set is sealed: a projection can
// switch over it exhaustively and know it has covered everything.
type Node interface {
	node()
}

// Scalar is a single value. Format carries a refinement a projection may use --
// "uuid" or "date-time" for JSON Schema, for instance -- and is empty when
// there is none.
type Scalar struct {
	Kind Kind
	// Precision is the Go representation the value is carried in, where the
	// description pins one down. The wire has two numeric shapes and Go has
	// twelve; Kind is the first and this is the second.
	Precision Precision
	Format    string
	// Constraints narrow which values of the kind are admitted, in the order
	// they were declared. A projection that has keywords for them emits them;
	// one that has none describes the kind alone, which is still true.
	Constraints []Constraint
}

// Object is a fixed set of named fields.
//
// A named object becomes a reusable component in projections that have them, so
// a type used by ten endpoints is described once. An unnamed object is inlined.
type Object struct {
	Name   string
	Doc    string
	Fields []Field
}

// Identity is the field that distinguishes one of these from another, if the
// description names one.
func (object Object) Identity() (Field, bool) {
	for _, field := range object.Fields {
		if field.Identity {
			return field, true
		}
	}
	return Field{}, false
}

// IsEntity reports whether this object is a thing in its own right rather than
// a value belonging to whatever holds it.
//
// It is derived from having an identity rather than declared separately, which
// is a deliberate simplification: an object with an identity is an entity and
// one without is a value, so a separate marker could only ever agree with the
// identity or contradict it. An address inside a customer is a value and lives
// in the customer's row; an order line has an identity and lives in its own
// table.
func (object Object) IsEntity() bool {
	_, named := object.Identity()
	return named
}

// Field is one member of an object.
type Field struct {
	Name string
	Doc  string
	Node Node
	// Optional says the field may be absent.
	Optional bool
	// Number is the field's number on a wire that identifies fields by number
	// rather than by name -- protobuf, principally. Zero means the description
	// does not state one, and a projection that needs one says so rather than
	// inventing it: a number is what protobuf's compatibility rests on, and one
	// derived from declaration order would change when the declaration was
	// reordered.
	Number int
	// Identity says this field is what distinguishes one of these from
	// another. A projection to storage makes it the key; a projection to an
	// update shape leaves it out, because a key is what selects the row rather
	// than something the row's new value contains.
	Identity bool
	// Computed says the value comes from somewhere other than the caller: a
	// default, a trigger, a derivation. It is left out of every shape a caller
	// supplies, because asking for a value that will be overwritten is asking
	// a question with no answer.
	//
	// The two compose, and the composition is the distinction other libraries
	// spell with two separate concepts: an identity the application supplies is
	// Identity alone, and one the database generates is both.
	Computed bool
	// Default is what produces the value when the caller does not. It is nil
	// when the description does not say, and a projection that needs to know
	// says so rather than inventing one: a column with no value and no default
	// is a column no row can be written for, and a default invented here would
	// be a rule nobody asked for.
	//
	// A generated identity needs none, because the database's own key
	// generation is what produces it.
	Default Default
}

// Sequence is an ordered, variable-length list.
type Sequence struct {
	Element Node
	// Constraints narrow how many elements are admitted.
	Constraints []Constraint
}

// Mapping is a variable set of keys of one shape to values of another.
type Mapping struct {
	Key   Node
	Value Node
}

// Union is a choice between named variants.
//
// Discriminator says where the chosen variant's name is written. Empty means
// the variant names the object's single member -- {"circle": {...}} -- and a
// field name means the name is that field of the variant's own object --
// {"type": "circle", "radius": 2}. The two are different wire forms of one
// idea, and a projection has to know which, so the description says.
type Union struct {
	Name          string
	Doc           string
	Discriminator string
	Variants      []Variant
}

// Variant is one alternative of a union.
type Variant struct {
	Name string
	Doc  string
	Node Node
	// Number is the variant's number, for the same reason a Field has one: a
	// union becomes a oneof, and each of its members is numbered.
	Number int
}

// Nullable is a shape a format may carry as null.
//
// It is a node rather than a flag because it applies to any shape, and it is
// distinct from an optional field: a nullable value is present and null, an
// optional field is not there at all. Formats and readers treat those
// differently, so the description does too.
type Nullable struct {
	Inner Node
}

// Reference names a shape described elsewhere.
//
// It exists for two purposes that happen to need the same thing: a recursive
// type terminates, and a projection emits a reference rather than an inlined
// copy at every use. Resolve is how a walker reaches the referenced shape, and
// may be nil when the reference is to a component the walker already holds.
type Reference struct {
	Name    string
	Resolve func() Node
}

func (Scalar) node()    {}
func (Object) node()    {}
func (Sequence) node()  {}
func (Mapping) node()   {}
func (Union) node()     {}
func (Nullable) node()  {}
func (Reference) node() {}

// EntityBehind is the entity a node carries, through whatever wraps it, and
// whether it carries one at all.
//
// A field is a *relation* when it does: a thing with an identity of its own,
// which storage gives a table and which a derived shape treats as something
// created in its own act. A field that does not is a value, and lives wherever
// the thing holding it lives.
//
// It follows a list and a nullable, because a thing in a collection is still a
// thing, and a reference, because a name is not a shape. It deliberately does
// **not** follow a mapping: a map's key would need somewhere of its own to
// live, and the description does not say what to call it -- so a map of
// entities is one value rather than a relation, and inventing a name for the
// key would put it in a schema forever.
//
// Here rather than in a projection because three of them ask the same question
// and one answer is the point: a description says what a field is, and a
// projection reads it.
func EntityBehind(node Node) (Object, bool) {
	switch held := node.(type) {
	case Object:
		return held, held.IsEntity()
	case Reference:
		if held.Resolve == nil {
			return Object{}, false
		}
		return EntityBehind(held.Resolve())
	case Sequence:
		return EntityBehind(held.Element)
	case Nullable:
		return EntityBehind(held.Inner)
	default:
		return Object{}, false
	}
}
