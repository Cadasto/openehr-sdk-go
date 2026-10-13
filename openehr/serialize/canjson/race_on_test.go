//go:build race

package canjson_test

// raceEnabled reports a build with the race detector, under which sync.Pool
// drops pooled values at random, so allocation counts vary from run to run.
const raceEnabled = true
