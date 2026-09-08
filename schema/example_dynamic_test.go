package schema_test

import (
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
)

// A description with no Go type is written with the same combinators, minus the
// accessors a Go value would need. It validates, transcodes, describes itself
// and composes exactly as a typed schema does.
func ExampleDescribing() {
	reading := schema.Struct[dynamic.Value]("Reading",
		schema.Describing("code", schema.Matching(schema.Text(), `^[A-Z]{2}-[0-9]{4}$`)),
		schema.Describing("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 20000)),
	)

	value, err := schema.DecodeJSON(reading, []byte(`{"code":"AB-1234","pages":632}`))
	if err != nil {
		panic(err)
	}
	code, _ := value.(dynamic.Object).Member("code")
	fmt.Println(code.(dynamic.Text).Value)

	// Every rule the description records is enforced without the type that
	// stated it.
	_, err = schema.DecodeJSON(reading, []byte(`{"code":"ab-1234","pages":632}`))
	fmt.Println(err)

	written, err := schema.EncodeJSON(reading, value)
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.TrimSpace(string(written)))
	// Output:
	// AB-1234
	// schema: does not match ^[A-Z]{2}-[0-9]{4}$ at code
	// {"code":"AB-1234","pages":632}
}

// Dynamic is the door between the two ways of using this package: a typed
// schema's description is usable without its type.
func ExampleDynamic() {
	described := schema.Dynamic(bookSchema.Structure())

	value, err := schema.DecodeJSON(described,
		[]byte(`{"title":"Zionomicon","authors":["John A. De Goes"],"pages":632}`))
	if err != nil {
		panic(err)
	}
	title, _ := value.(dynamic.Object).Member("title")
	fmt.Println(title.(dynamic.Text).Value)

	// And a typed value crosses the other way.
	crossed, err := schema.ToDynamic(bookSchema, Book{Title: "T", Authors: []string{}, Pages: 1})
	if err != nil {
		panic(err)
	}
	back, err := schema.FromDynamic(bookSchema, crossed)
	if err != nil {
		panic(err)
	}
	fmt.Println(back.Title, back.Pages)
	// Output:
	// Zionomicon
	// T 1
}
