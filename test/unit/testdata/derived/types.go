// Package derived is a fixture holding every shape the generator can name, so
// the golden file below it is the whole of what generation produces.
package derived

import (
	"net/url"
	"time"
)

// Record is one of everything.
//
//schema:generate
type Record struct {
	// Text is a string.
	Text string `json:"text"`
	Whole    int                `json:"whole"`
	Wide     int64              `json:"wide"`
	Fraction float64            `json:"fraction"`
	Flag     bool               `json:"flag"`
	Opaque   []byte             `json:"opaque"`
	Moment   time.Time          `json:"moment"`
	Names    []string           `json:"names"`
	Labels   map[string]string  `json:"labels"`
	Maybe    *string            `json:"maybe"`
	Absent   *int               `json:"absent,omitempty"`
	Nested   Inner              `json:"nested"`
	Deep     []Inner            `json:"deep"`
	Address  url.URL            `json:"address" schema:"use=addressSchema"`
	Ignored  string             `json:"ignored" schema:"-"`
	hidden   string
}

// Inner is referred to by name, so its own schema is used.
//
//schema:generate
type Inner struct {
	Label string `json:"label"`
}

// Untagged takes its wire names from its fields.
//
// This second paragraph is addressed to whoever maintains the type and has no
// business in a published contract, so only the summary above becomes prose.
//
//schema:generate
type Untagged struct {
	FirstName string
	Age       int
}
