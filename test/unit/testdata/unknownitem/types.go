package unknownitem

//schema:generate
type Record struct {
	Pages int `json:"pages" schema:"atLeast=2"`
}
