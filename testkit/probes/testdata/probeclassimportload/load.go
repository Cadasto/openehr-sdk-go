// Package probeclassimportload is a fixture for the probe classifier: it
// imports a package that imports another, whose package-level var starts a
// test server when that package loads.
package probeclassimportload

import "github.com/cadasto/openehr-sdk-go/testkit/probes/testdata/probeclassloader"

// Probe917AtImportLoad calls a function that opens no connection, but a
// package it imports through probeclassloader starts a test server when it
// loads.
func Probe917AtImportLoad() string {
	return probeclassloader.Greeting()
}
