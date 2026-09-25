// Package demographicprobes hosts the openEHR conformance probes for the
// Demographic REST client (openehr/client/demographic). Probes are plain
// functions returning (Result, error) so they can run in Sandbox mode (a
// fake server) today and Cassette / Live modes later.
//
// PROBE-073 asserts the PARTY polymorphic round-trip: a create → get →
// get-version cycle for each concrete PARTY type decodes the wire `_type`
// discriminator back into the same concrete Go type, across both the
// typed-resource body and the ORIGINAL_VERSION<PARTY> envelope. Any
// conformant SDK and CDR pair must preserve the concrete type across the
// round-trip.
package demographicprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
