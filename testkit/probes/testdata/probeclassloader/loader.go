// Package probeclassloader is a fixture for the probe classifier: its own
// code opens no connection, but it imports probeclassvar, whose
// package-level var starts a test server when that package loads.
package probeclassloader

import _ "github.com/cadasto/openehr-sdk-go/testkit/probes/testdata/probeclassvar"

// Greeting opens no connection.
func Greeting() string {
	return "hello"
}
