// Package probeclassvar is a fixture for the probe classifier: a
// package-level var's initialiser starts a test server.
package probeclassvar

import "net/http/httptest"

var testServer = httptest.NewServer(nil)

// Probe911ThroughVar reads testServer, whose initialiser starts a server.
func Probe911ThroughVar() string {
	return testServer.URL
}

// Probe912AtLoad reads nothing, but testServer's initialiser runs when the
// package loads.
func Probe912AtLoad() string {
	return "load"
}
