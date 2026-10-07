// Package noredirect builds the HTTP client that sends a request carrying a
// credential, such as a token, refresh or revocation request: one that
// returns a 3xx answer to the caller instead of following it, so a 307 or a
// 308 cannot take the request's form to another URL.
package noredirect

import "net/http"

// Client returns a shallow copy of c whose redirect policy returns every 3xx
// answer as the response of the call, whatever CheckRedirect c has. The
// caller reads the status code, and a body is not re-sent. c itself is never
// modified, and the copy shares c's Transport, Jar and Timeout.
//
// Client panics when c is nil.
func Client(c *http.Client) *http.Client {
	cp := *c
	cp.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cp
}
