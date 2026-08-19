// Package transportprobes hosts the openEHR conformance probes for
// cross-cutting transport behaviour — properties the `transport/` layer
// enforces on behalf of every `openehr/client` leaf, rather than
// properties of one openEHR resource (PROBE-091 today).
//
// Probes are plain functions returning (Result, error) and run against
// any compliant transport.Client backend — sandbox, cassette, or live
// deployment.
package transportprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
