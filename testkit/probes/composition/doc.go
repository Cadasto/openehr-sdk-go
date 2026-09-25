// Package compositionprobes hosts the openEHR conformance probes
// for openehr/composition/, the OPT-driven composition builder.
// Probes are plain functions returning (Result, error); they are
// sandbox-only (no transport) since the builder is a local
// building block.
//
// PROBE-023 closes the authoring round-trip: NewBuilder + Set →
// Build → canjson.Marshal → containment checks on selected paths.
// Downstream SDKs implementing the same builder against the same
// fixtures must reproduce the same pass outcome.
package compositionprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
