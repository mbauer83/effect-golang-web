package notanumber

//schema:generate
type Record struct {
	Pages int `json:"pages" schema:"min=many"`
}
