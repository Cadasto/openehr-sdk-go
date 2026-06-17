// Package demographicprobes hosts the openEHR conformance probes for the
// Demographic REST client (openehr/client/demographic). Probes are plain
// functions returning (Result, error) so they can run in Sandbox mode (a
// fake server) today and Cassette / Live modes later.
//
// PROBE-073 asserts the PARTY polymorphic round-trip: a create → get →
// get-version cycle for each concrete PARTY type decodes the wire `_type`
// discriminator back into the same concrete Go type (REQ-040), across the
// typed-resource body (Phase 1) and the ORIGINAL_VERSION<PARTY> envelope
// (Phase 2). Any conformant SDK and CDR pair MUST preserve the concrete type
// across the round-trip.
package demographicprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary text for
// failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
