package web

// Middleware derives a handler from a handler.
//
// It is the shape of every wrapper that has to see the request as well as the
// response -- authentication, a request log, a rate limit -- where Transform
// only sees the response. Because a handler is a description, middleware
// composes with retries, races and timeouts rather than sitting outside them.
type Middleware[R, E any] func(Handler[R, E]) Handler[R, E]

// Wrap applies middleware to a handler, outermost first: the first given sees
// the request first and the response last, which is the order the list reads
// in.
func Wrap[R, E any](handler Handler[R, E], middleware ...Middleware[R, E]) Handler[R, E] {
	for index := len(middleware) - 1; index >= 0; index-- {
		if middleware[index] == nil {
			continue
		}
		handler = middleware[index](handler)
	}
	return handler
}
