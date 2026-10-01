// Package probeclassdeep is a fixture for the probe classifier:
// probeclasshelper reaches a test server through it.
package probeclassdeep

import "net/http/httptest"

// Start starts a test server and returns its URL.
func Start() string {
	s := httptest.NewServer(nil)
	defer s.Close()
	return s.URL
}
