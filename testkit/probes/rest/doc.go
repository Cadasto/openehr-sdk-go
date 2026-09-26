// Package restprobes hosts openEHR REST-binding conformance probes for the
// System capabilities operation, the Admin bulk-delete surface, the Definition
// example endpoint, the openehr-audit-details header grammar, and the EHR
// creation round-trip. Probes are plain functions returning (Result, error);
// they are request-capture probes (they assert the shape of the request the SDK
// puts on the wire and the response it decodes), driven in Sandbox mode against
// a sandbox scripted route.
package restprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe result type. The alias keeps every
// probe package on one identity rather than a per-package copy.
type Result = probe.Result
