package wrongformat

//schema:generate
type Record struct {
	Pages int `json:"pages" schema:"format=uuid"`
}
