package unsupported

import "net/url"

//schema:generate
type Record struct {
	Address url.URL `json:"address"`
}
