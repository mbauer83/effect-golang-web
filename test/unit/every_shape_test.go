package unit

// One value carrying every shape the schema layer can describe. It exists so a
// format author, or a new projection, has a single fixture that reaches the
// whole contract rather than the parts a realistic type happens to use.

import (
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

type everyShape struct {
	Text     string
	Whole    int
	Wide     int64
	Fraction float64
	Flag     bool
	Opaque   []byte
	Moment   time.Time
	Labels   map[string]string
	Missing  *string
	Nested   []int
	Detail   Detail
}

// Detail is named and documented, so a projection has something to describe
// once and refer to thereafter.
type Detail struct{ Note string }

var everyShapeSchema = schema.Struct[everyShape]("EveryShape",
	schema.FieldOf("text", schema.Formatted("free-text"),
		func(value everyShape) string { return value.Text },
		func(value *everyShape, text string) { value.Text = text }),
	schema.FieldOf("whole", schema.Int(),
		func(value everyShape) int { return value.Whole },
		func(value *everyShape, whole int) { value.Whole = whole }),
	schema.FieldOf("wide", schema.Int64(),
		func(value everyShape) int64 { return value.Wide },
		func(value *everyShape, wide int64) { value.Wide = wide }),
	schema.FieldOf("fraction", schema.Float64(),
		func(value everyShape) float64 { return value.Fraction },
		func(value *everyShape, fraction float64) { value.Fraction = fraction }),
	schema.FieldOf("flag", schema.Bool(),
		func(value everyShape) bool { return value.Flag },
		func(value *everyShape, flag bool) { value.Flag = flag }),
	schema.FieldOf("opaque", schema.Bytes(),
		func(value everyShape) []byte { return value.Opaque },
		func(value *everyShape, opaque []byte) { value.Opaque = opaque }),
	schema.FieldOf("moment", schema.Time(),
		func(value everyShape) time.Time { return value.Moment },
		func(value *everyShape, moment time.Time) { value.Moment = moment }),
	schema.FieldOf("labels", schema.Map(schema.Text()),
		func(value everyShape) map[string]string { return value.Labels },
		func(value *everyShape, labels map[string]string) { value.Labels = labels }),
	schema.FieldOf("missing", schema.Nullable(schema.Text()),
		func(value everyShape) *string { return value.Missing },
		func(value *everyShape, missing *string) { value.Missing = missing }),
	// A transform is what separates the wire shape from the domain type, so the
	// fixture carries one rather than describing every field directly.
	schema.FieldOf("nested", schema.List(countSchema),
		func(value everyShape) []int { return value.Nested },
		func(value *everyShape, nested []int) { value.Nested = nested }),
	schema.FieldOf("detail",
		schema.Documented("one note, described once and referred to thereafter",
			schema.Named("Detail", detailSchema)),
		func(value everyShape) Detail { return value.Detail },
		func(value *everyShape, detail Detail) { value.Detail = detail }),
)

// countSchema is a transform with no refinement: the wire carries an int64 and
// the domain wants an int, which is the ordinary case Transform exists for.
var countSchema = schema.Transform(schema.Int64(),
	func(wide int64) int { return int(wide) },
	func(narrow int) int64 { return int64(narrow) },
)

var detailSchema = schema.Struct[Detail]("",
	schema.FieldOf("note", schema.Text(),
		func(detail Detail) string { return detail.Note },
		func(detail *Detail, note string) { detail.Note = note }),
)
