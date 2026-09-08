// Package definitionprobes hosts the openEHR conformance probes
// for the openEHR REST Definition API (PROBE-067 today; more follow
// alongside ADL 2 and stored-AQL surfaces).
//
// Probes are plain functions returning (Result, error) and run
// against any compliant transport.Client backend. Today that is the
// sandbox; Cassette and Live are REQ-082 modes still to be wired up
// for this package.
package definitionprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome (REQ-082).
type Result = probe.Result
