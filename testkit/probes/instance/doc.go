// Package instanceprobes hosts the openEHR conformance probes for
// openehr/instance/, template-driven RM instance synthesis.
// Probes are plain functions returning (Result, error); they are
// sandbox-only (no transport) since the generator is a local
// building block.
//
// PROBE-027 closes the loop: Generate followed by
// ValidateComposition on the same compiled OPT must surface
// Result.OK = true for both Minimal and Example policies. Downstream
// SDKs implementing the same generator and validator against the
// same fixtures must reproduce the same OK outcome.
package instanceprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
