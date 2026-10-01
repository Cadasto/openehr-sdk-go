// Package probeclassimport is a fixture for the probe classifier: its probes
// reach, or do not reach, a backend through the declarations of other
// packages of this module.
package probeclassimport

import "github.com/cadasto/openehr-sdk-go/testkit/probes/testdata/probeclasshelper"

// Probe913ThroughPackage names no backend package itself; it reaches
// httptest only through a function of the package probeclasshelper.
func Probe913ThroughPackage() string {
	return probeclasshelper.StartServer()
}

// Probe914PureHelper calls a function of probeclasshelper that opens no
// connection, so it reaches no backend, although another function of that
// package does.
func Probe914PureHelper() string {
	return probeclasshelper.Greeting()
}

// Probe915ThroughForeignMethod reaches httptest through a method of a type
// that probeclasshelper declares.
func Probe915ThroughForeignMethod() string {
	return probeclasshelper.NewServer().Start()
}

// Probe916ForeignPureMethod calls only the method of that type that opens
// no connection.
func Probe916ForeignPureMethod() string {
	return probeclasshelper.NewServer().Name()
}

// Probe918ThroughTwoPackages reaches httptest through probeclasshelper,
// which reaches it through probeclassdeep.
func Probe918ThroughTwoPackages() string {
	return probeclasshelper.Relayed()
}
