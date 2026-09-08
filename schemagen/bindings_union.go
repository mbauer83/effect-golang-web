package schemagen

// Binding a described union to Go: an interface with an unexported marker, and
// the schema that narrows to each variant. That is how Go spells a sum, and the
// marker being unexported is what keeps the set closed.

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func writeUnionBinding(written *bytes.Buffer, shape structure.Union) error {
	if len(shape.Variants) == 0 {
		return fmt.Errorf("%s has no variants", shape.Name)
	}
	marker := markerName(shape.Name)

	writeDoc(written, shape.Name, shape.Doc)
	fmt.Fprintf(written, "type %s interface{ %s() }\n", shape.Name, marker)

	fmt.Fprintf(written, "\n// %sSchema describes %s. It is generated from its description.\n",
		shape.Name, shape.Name)
	fmt.Fprintf(written, "var %sSchema = ", shape.Name)
	fmt.Fprintf(written, "schema.OneOf[%s](%s,\n", shape.Name, strconv.Quote(shape.Name))
	for _, variant := range shape.Variants {
		if err := writeVariant(written, shape.Name, variant); err != nil {
			return err
		}
	}
	fmt.Fprintf(written, ")")
	if shape.Doc != "" {
		fmt.Fprintf(written, ".Documented(%s)", strconv.Quote(shape.Doc))
	}
	fmt.Fprintf(written, "\n")
	return nil
}

func writeVariant(written *bytes.Buffer, union string, variant structure.Variant) error {
	object, isObject := variant.Node.(structure.Object)
	if !isObject {
		return fmt.Errorf("%s.%s: a variant becomes a Go type, so it is an object",
			union, variant.Name)
	}
	fmt.Fprintf(written, "schema.VariantOf(%s, %sSchema,\n",
		strconv.Quote(variant.Name), object.Name)
	fmt.Fprintf(written, "func(value %s) (%s, bool) { held, is := value.(%s); return held, is },\n",
		union, object.Name, object.Name)
	fmt.Fprintf(written, "func(held %s) %s { return held })", object.Name, union)
	if variant.Doc != "" {
		fmt.Fprintf(written, ".Documented(%s)", strconv.Quote(variant.Doc))
	}
	fmt.Fprintf(written, ",\n")
	return nil
}
