//go:build race

package validation_test

// raceEnabled reports a build with the race detector, which slows the walk
// enough that a deadline sized for a normal build would be missed.
const raceEnabled = true
