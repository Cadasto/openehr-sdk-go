// Package definitionprobes hosts the openEHR conformance probes
// for the openEHR REST Definition API (PROBE-067 and PROBE-093).
//
// Probes are plain functions returning (Result, error) and run
// against any compliant transport.Client backend. Today that is the
// sandbox; Cassette and Live modes are not yet wired up for this
// package.
package definitionprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
