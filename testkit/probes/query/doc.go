// Package queryprobes hosts the openEHR conformance probes for
// openehr/client/query — ad-hoc and stored AQL execution over the REST
// Query API (REQ-055, REQ-057). Probes are plain functions returning
// (Result, error); they are request-capture probes (they assert the shape
// of the request the SDK puts on the wire), driven in Sandbox mode against a
// sandbox scripted route.
package queryprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe result type (REQ-082). The alias keeps every
// probe package on one identity rather than a per-package copy.
type Result = probe.Result
