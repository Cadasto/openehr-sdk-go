// Package templateprobes hosts the openEHR conformance probes for
// the openehr/template/ surface — local ADL 1.4 operational template
// parsing and path resolution (PROBE-022). Probes are plain functions
// returning (Result, error); the template probes are sandbox-only
// (no transport), since the package is a building-block per REQ-013.
package templateprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome (REQ-082).
type Result = probe.Result
