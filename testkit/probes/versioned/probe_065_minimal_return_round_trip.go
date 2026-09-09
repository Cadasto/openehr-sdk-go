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

// Probe065MinimalReturnRoundTrip implements PROBE-065: a Composition write
// under the SDK's default `Prefer: return=minimal` returns an empty body and a
// Location header, so the SDK surfaces only the version metadata (a nil
// Composition), and a follow-up GET returns the full Composition (REQ-094).
//
// It pins two facts a deployment must satisfy on the minimal path: the write's
// Location is recovered into the VersionUID — a write that names nothing leaves
// the caller unable to read what it just committed — and that committed version
// reads back in full. The "surfaces only metadata" half is the SDK's own
// contract (a minimal write never decodes a body); asserting it here catches a
// regression that started decoding one.
func Probe065MinimalReturnRoundTrip(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, comp *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-065"}
	if c == nil || ehrID == "" || comp == nil {
		return r, errors.New("PROBE-065: missing required inputs (client/ehr/comp)")
	}

	// POST arm — the default minimal write: empty body, Location only.
	out, meta, err := composition.Save(ctx, c, ehrID, comp)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("minimal Save failed: %v", err)
		return r, nil
	}
	if out != nil {
		r.Status = "fail"
		r.Detail = "minimal Save returned a Composition body; want metadata only (return=minimal)"
		return r, nil
	}
	if meta == nil || meta.VersionUID == "" {
		r.Status = "fail"
		r.Detail = "minimal Save surfaced no VersionUID; the write carried no usable Location"
		return r, nil
	}

	// GET arm — the committed version reads back in full.
	got, _, err := composition.Get(ctx, c, ehrID, openehrclient.VersionOf(meta.VersionUID))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("GET of the committed version %q failed: %v", meta.VersionUID, err)
		return r, nil
	}
	if got == nil {
		r.Status = "fail"
		r.Detail = "GET returned a nil Composition for the version the minimal write committed"
		return r, nil
	}
	if got.ArchetypeNodeID == "" {
		r.Status = "fail"
		r.Detail = "GET returned a Composition with empty archetype_node_id; body is not the full committed resource"
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("minimal write → metadata-only (vuid=%q); GET → full COMPOSITION", meta.VersionUID)
	return r, nil
}
