// Package transportprobes hosts the openEHR conformance probes for
// cross-cutting transport behaviour: properties the `transport/` layer
// enforces on behalf of every `openehr/client` leaf, as opposed to
// properties of one openEHR resource (PROBE-091 and PROBE-101).
//
// Probes are plain functions returning (Result, error) and run against
// any compliant transport.Client backend. Today that is the sandbox;
// Cassette and Live modes are not yet wired up for this package.
package transportprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
