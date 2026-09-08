package main

// The aggregate, its three derived shapes, and the tables it becomes.
//
// One description does four jobs, and this prints all four so the point is
// visible rather than described: a column that changed would change every one
// of them together.

import (
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/examples/warehouse"
	"github.com/mbauer83/effect-golang-web/schema/ddl"
	"github.com/mbauer83/effect-golang-web/schema/structure"
	"github.com/mbauer83/effect-golang-web/schema/variant"
)

func runWarehouse() {
	shape := warehouse.PalletSchema.Structure()

	fmt.Println("warehouse: the shapes one description has")
	for _, derived := range []struct {
		named string
		of    func(structure.Node) (structure.Node, error)
	}{
		{"create", variant.Create},
		{"create, with the items", variant.CreateWithEntities},
		{"update", variant.Update},
	} {
		node, err := derived.of(shape)
		if err != nil {
			fail(err)
		}
		fmt.Printf("  %-22s %s\n", derived.named, fieldsOf(node))
	}

	for _, dialect := range []ddl.Dialect{ddl.Postgres, ddl.MySQL} {
		statements, err := ddl.Create(dialect, shape)
		if err != nil {
			fail(err)
		}
		fmt.Printf("\nwarehouse: the tables, in %s\n", dialect.Name())
		for _, statement := range statements {
			fmt.Println(statement + ";")
		}
	}
}

// fieldsOf names an object's fields, with the optional ones marked -- because
// an update shape asking for nothing is the whole difference between it and a
// create shape with the identity removed.
func fieldsOf(node structure.Node) string {
	object, isObject := node.(structure.Object)
	if !isObject {
		return "(not an object)"
	}
	named := make([]string, 0, len(object.Fields))
	for _, field := range object.Fields {
		if field.Optional {
			named = append(named, field.Name+"?")
			continue
		}
		named = append(named, field.Name)
	}
	return strings.Join(named, ", ")
}
