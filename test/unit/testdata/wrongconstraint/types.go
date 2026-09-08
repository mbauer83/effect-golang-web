package wrongconstraint

//schema:generate
type Record struct {
	Pages int `json:"pages" schema:"minLength=2"`
}
