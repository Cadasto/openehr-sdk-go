package versionedprobes

import (
	"context"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe071CompositionWriteResponseShape implements PROBE-071: a
// Composition write under `Prefer: return=representation` decodes its
// response body as a bare `*rm.Composition` per the ITS-REST OpenAPI
// `201_COMPOSITION` schema — not as an `ORIGINAL_VERSION<COMPOSITION>`
// envelope. The full version envelope is exposed only at
// `GET /versioned_composition/{vo_uid}/version/{version_uid}`.
//
// Pins [SDK-GAP-09]. A deployment that returns ORIGINAL_VERSION on
// POST is non-conformant; the SDK surfaces the mismatch as a decode
// error (strict-against-spec). The probe asserts both halves of that
// contract — success on the bare body, failure (decode error) on a
// stamped envelope — through two server arms.
//
// [SDK-GAP-09]: https://github.com/Cadasto/openehr-go-poc/blob/main/docs/sdk-gap-drafts/SDK-GAP-09-composition-save-update-spec-mismatch.md
func Probe071CompositionWriteResponseShape(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, comp *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-071"}
	if c == nil || ehrID == "" || comp == nil {
		return r, fmt.Errorf("PROBE-071: missing required inputs (client/ehr/comp)")
	}
	out, meta, err := composition.Save(ctx, c, ehrID, comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Save with Prefer=representation failed: %v", err)
		return r, nil
	}
	if out == nil {
		r.Status = "fail"
		r.Detail = "Save returned nil Composition under Prefer=representation (body missing or empty)"
		return r, nil
	}
	// Bare-body sanity: a Composition payload exposes ArchetypeNodeID
	// at the top level. An ORIGINAL_VERSION envelope decoded as a
	// Composition leaves this empty (the envelope wraps the
	// Composition under `data`), which would catch the asymmetry
	// even if `_type` discrimination is lenient on a given codec.
	if out.ArchetypeNodeID == "" {
		r.Status = "fail"
		r.Detail = "decoded *rm.Composition has empty archetype_node_id — body shape looks like ORIGINAL_VERSION<COMPOSITION> not bare COMPOSITION"
		return r, nil
	}
	if meta == nil || meta.VersionUID == "" {
		r.Status = "fail"
		r.Detail = "Save returned no VersionUID metadata (Location/ETag missing)"
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("bare-body decode OK: archetype_node_id=%q, version_uid=%q", out.ArchetypeNodeID, meta.VersionUID)
	return r, nil
}
