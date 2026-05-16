package versionedprobes

import (
	"context"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe012ETagRoundTrip implements PROBE-012: a GET followed by a PUT
// that carries the captured ETag/version_uid as If-Match succeeds and
// returns a fresh version identifier distinct from the input.
//
// The probe exercises the [openehrclient.VersionMetadata] round-trip
// contract — Location-derived VersionUID on read becomes If-Match on
// the follow-up write without consumer-side string surgery. Closes
// the read-modify-write loop that every leaf client in
// `openehr/client/ehr/*` is shaped around.
//
// Inputs:
//   - ehrID and voID identify an existing versioned Composition.
//   - update is the modification body. The probe is opaque about
//     content semantics; the wire shape is what matters.
func Probe012ETagRoundTrip(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, update *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-012"}
	if c == nil || ehrID == "" || voID == "" || update == nil {
		return r, fmt.Errorf("PROBE-012: missing required inputs (client/ehr/voID/update)")
	}
	_, meta, err := composition.Get(ctx, c, ehrID, openehrclient.LatestOf(voID))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("initial Get failed: %v", err)
		return r, nil
	}
	if meta == nil || meta.VersionUID == "" {
		r.Status = "fail"
		r.Detail = "initial Get returned no VersionUID (Location header missing or unparseable)"
		return r, nil
	}
	initialVUID := meta.VersionUID

	_, putMeta, err := composition.Update(ctx, c, ehrID, voID, string(initialVUID), update)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Update with captured If-Match failed: %v", err)
		return r, nil
	}
	if putMeta == nil {
		r.Status = "fail"
		r.Detail = "Update returned no metadata"
		return r, nil
	}
	if putMeta.VersionUID == "" {
		r.Status = "fail"
		r.Detail = "Update response did not advertise a new VersionUID"
		return r, nil
	}
	if putMeta.VersionUID == initialVUID {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("new VersionUID equals initial: %q (server did not bump version)", initialVUID)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("round-trip OK: %q → %q", initialVUID, putMeta.VersionUID)
	return r, nil
}
