package schema_test

import (
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema"
)

// Shape is a sum modelled the way Go models one: an interface with an
// unexported marker and a concrete type per alternative.
type Shape interface{ shape() }

type Circle struct{ Radius float64 }

type Rectangle struct{ Width, Height float64 }

func (Circle) shape()    {}
func (Rectangle) shape() {}

var circleSchema = schema.Struct[Circle]("Circle",
	schema.FieldOf("radius", schema.Float64(),
		func(circle Circle) float64 { return circle.Radius },
		func(circle *Circle, radius float64) { circle.Radius = radius }),
)

var rectangleSchema = schema.Struct[Rectangle]("Rectangle",
	schema.FieldOf("width", schema.Float64(),
		func(rectangle Rectangle) float64 { return rectangle.Width },
		func(rectangle *Rectangle, width float64) { rectangle.Width = width }),
	schema.FieldOf("height", schema.Float64(),
		func(rectangle Rectangle) float64 { return rectangle.Height },
		func(rectangle *Rectangle, height float64) { rectangle.Height = height }),
)

// A union names the variant it carries as the object's single member, so a
// decoder knows which shape follows before it reads it.
func ExampleOneOf() {
	shapes := schema.OneOf[Shape]("Shape",
		schema.VariantOf("circle", circleSchema,
			func(shape Shape) (Circle, bool) { circle, is := shape.(Circle); return circle, is },
			func(circle Circle) Shape { return circle }),
		schema.VariantOf("rectangle", rectangleSchema,
			func(shape Shape) (Rectangle, bool) { rectangle, is := shape.(Rectangle); return rectangle, is },
			func(rectangle Rectangle) Shape { return rectangle }),
	)

	document, err := schema.EncodeJSON[Shape](shapes, Circle{Radius: 2})
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.TrimSpace(string(document)))

	loaded, err := schema.DecodeJSON(shapes, []byte(`{"rectangle":{"width":3,"height":4}}`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v\n", loaded)

	_, err = schema.DecodeJSON(shapes, []byte(`{"triangle":{"base":1}}`))
	fmt.Println(err)
	// Output:
	// {"circle":{"radius":2}}
	// schema_test.Rectangle{Width:3, Height:4}
	// schema: no variant is named triangle
}

// Deferred is how a recursive type is declared, because Go cannot refer to a
// variable in its own initialiser.
func ExampleDeferred() {
	type Node struct {
		Label    string
		Children []Node
	}

	var nodeSchema schema.Schema[Node]
	nodeSchema = schema.Struct[Node]("Node",
		schema.FieldOf("label", schema.Text(),
			func(node Node) string { return node.Label },
			func(node *Node, label string) { node.Label = label }),
		schema.FieldOf("children",
			schema.List(schema.Deferred(func() schema.Schema[Node] { return nodeSchema })),
			func(node Node) []Node { return node.Children },
			func(node *Node, children []Node) { node.Children = children }),
	)

	document, err := schema.EncodeJSON(nodeSchema, Node{
		Label:    "root",
		Children: []Node{{Label: "leaf", Children: []Node{}}},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.TrimSpace(string(document)))
	// Output:
	// {"label":"root","children":[{"label":"leaf","children":[]}]}
}
