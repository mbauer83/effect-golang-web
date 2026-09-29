package web

// An answer whose value decides its own response.

import (
	"errors"

	"github.com/mbauer83/effect-golang-schema/schema/naming"
)

// ReturnsResponse answers with a response the value renders itself as.
//
// For an answer whose status, headers or entity depend on the value: a
// sign-in that ends in a redirect and a cookie, a page rendered from a
// template, a token handed back in a header. Effect's HttpApi lets a handler
// return a raw response for the same reason; here the handler still returns
// its own typed value, and the rendering is declared beside the endpoint, so
// the handler stays about the application and the endpoint about HTTP.
//
// The status and media type are what the document says the ordinary answer
// is. Any other status a rendering answers with is declared with
// WithAlternative, so the document is not silent about a redirect.
//
// A rendered answer is not read back by a client: rendering is not
// invertible, so Call against such an endpoint answers with
// ErrUnreadableOutput rather than guessing at a decoding.
func ReturnsResponse[Out any](
	status int,
	mediaType string,
	render func(Out) (Response, error),
) Output[Out] {
	output := Output[Out]{status: status, content: &Content{MediaType: mediaType}}
	if render == nil {
		output.fault = faultOf("declare the response", errNoRendering)
		return output
	}
	output.encode = func(value Out, _ naming.Strategy) (Response, error) { return render(value) }
	output.decode = func([]byte, naming.Strategy) (Out, error) {
		var nothing Out
		return nothing, ErrUnreadableOutput
	}
	return output
}

// ErrUnreadableOutput is a response a client was asked to read back through an
// output that only renders.
var ErrUnreadableOutput = errors.New("web: a rendered response is not read back through its endpoint")

var errNoRendering = declarationMistake("a rendered output says how its value is rendered")
