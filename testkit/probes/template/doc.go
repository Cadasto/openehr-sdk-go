// Package templateprobes hosts the openEHR conformance probes for
// the openehr/template/ surface: local ADL 1.4 operational template
// parsing, path resolution and primitive-constraint validation
// (PROBE-022, PROBE-024). Probes are plain functions returning
// (Result, error); the template probes are sandbox-only (no
// transport), since the package is a local building block.
package templateprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
