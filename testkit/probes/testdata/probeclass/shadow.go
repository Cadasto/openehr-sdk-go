package probeclass

import "net/http/httptest"

// newServer starts a test server, but no probe calls it.
func newServer() *httptest.Server {
	return httptest.NewServer(nil)
}

// Probe908LocalShadow declares a local named like the helper newServer. The
// local is not the helper, so the probe reaches no backend.
func Probe908LocalShadow() string {
	newServer := "local"
	return newServer
}
