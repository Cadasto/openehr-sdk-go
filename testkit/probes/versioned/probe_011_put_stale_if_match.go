package versionedprobes

import (
	"context"
	"errors"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe011PutStaleIfMatch implements PROBE-011: a PUT with a stale
// If-Match (referencing an old version_uid) is rejected with 412
// Precondition Failed, which the SDK maps to
// [transport.ErrPreconditionFailed]. The vendored ITS-REST pin answers a
// stale If-Match on a PUT with 412 and lists no 409 for it, so a 409
// fails the probe.
//
// Inputs:
//   - ehrID is an existing EHR on the deployment / fixture under test.
//   - voID is an existing versioned-Composition family.
//   - staleIfMatch is a known-stale version identifier (e.g. the
//     initial version after at least one update has landed).
//   - comp is any well-formed update payload.
//
// The probe issues [composition.Update] with the stale If-Match and
// asserts the returned error matches [transport.ErrPreconditionFailed].
func Probe011PutStaleIfMatch(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, staleIfMatch string, comp *rm.Composition) (Result, error) { // PROBE-011 (REQ-054, REQ-093)
	r := Result{Probe: "PROBE-011"}
	if c == nil || ehrID == "" || voID == "" || staleIfMatch == "" || comp == nil {
		return r, errors.New("PROBE-011: missing required inputs (client/ehr/voID/staleIfMatch/comp)")
	}
	_, _, err := composition.Update(ctx, c, ehrID, voID, staleIfMatch, comp)
	if err == nil {
		r.Status = "fail"
		r.Detail = "Update with stale If-Match returned nil error"
		return r, nil
	}
	switch {
	case errors.Is(err, transport.ErrPreconditionFailed):
		r.Status = "pass"
		r.Detail = "mapped to ErrPreconditionFailed (412)"
	case errors.Is(err, transport.ErrVersionConflict):
		r.Status = "fail"
		r.Detail = "got ErrVersionConflict (409); the ITS-REST pin answers a stale If-Match on a PUT with 412"
	default:
		r.Status = "fail"
		r.Detail = fmt.Sprintf("expected ErrPreconditionFailed (412), got %v", err)
	}
	return r, nil
}
