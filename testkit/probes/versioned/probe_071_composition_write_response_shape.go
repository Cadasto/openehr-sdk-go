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
// Composition write under `Prefer: return=representation` — whether
// POST (Save) or PUT (Update) — decodes its response body as a bare
// `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` /
// `200_COMPOSITION_updated` schemas, not as an
// `ORIGINAL_VERSION<COMPOSITION>` envelope. The full version envelope
// is exposed only at
// `GET /versioned_composition/{vo_uid}/version/{version_uid}`
// (`UVersionOfComposition`).
//
// Pins [SDK-GAP-09]. A deployment that returns ORIGINAL_VERSION on
// these paths is non-conformant; the SDK surfaces the mismatch as a
// decode error (strict-against-spec). The probe exercises both halves:
// POST then PUT, each with a fresh round-trip. When `voID` or `ifMatch`
// is empty, the PUT arm is skipped and the probe still passes on the
// POST arm — preconditions reflect deployments that don't expose a
// preconfigured family for the test caller. A pass requires the POST
// arm at minimum; when both inputs are present, both arms must succeed.
func Probe071CompositionWriteResponseShape(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-071"}
	if c == nil || ehrID == "" || comp == nil {
		return r, fmt.Errorf("PROBE-071: missing required inputs (client/ehr/comp)")
	}
	// POST arm — Save with Prefer=representation must decode bare.
	postOut, postMeta, err := composition.Save(ctx, c, ehrID, comp,
		composition.WithPrefer(transport.PreferRepresentation),
	)
	if msg := assertBareCompositionResult(postOut, postMeta, err, "POST"); msg != "" {
		r.Status = "fail"
		r.Detail = msg
		return r, nil
	}
	// PUT arm — Update with Prefer=representation, optional based on
	// caller-provided preconditions.
	if voID != "" && ifMatch != "" {
		putOut, putMeta, err := composition.Update(ctx, c, ehrID, voID, ifMatch, comp,
			composition.WithPrefer(transport.PreferRepresentation),
		)
		if msg := assertBareCompositionResult(putOut, putMeta, err, "PUT"); msg != "" {
			r.Status = "fail"
			r.Detail = msg
			return r, nil
		}
		r.Status = "pass"
		r.Detail = fmt.Sprintf("bare-body decode OK on POST (vuid=%q) and PUT (vuid=%q)", postMeta.VersionUID, putMeta.VersionUID)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("bare-body decode OK on POST (vuid=%q); PUT arm skipped (no voID/ifMatch supplied)", postMeta.VersionUID)
	return r, nil
}

// assertBareCompositionResult inspects a Save/Update outcome under
// Prefer=representation. Returns a non-empty failure detail when the
// response is not a well-formed bare COMPOSITION; an empty string
// signals pass. Method names the verb in the failure detail so a
// caller of the merged POST+PUT probe can pinpoint which arm failed.
func assertBareCompositionResult(out *rm.Composition, meta *openehrclient.VersionMetadata, err error, method string) string {
	if err != nil {
		return fmt.Sprintf("%s with Prefer=representation failed: %v", method, err)
	}
	if out == nil {
		return fmt.Sprintf("%s returned nil Composition under Prefer=representation (body missing or empty)", method)
	}
	// Bare-body sanity: a Composition payload exposes ArchetypeNodeID
	// at the top level. An ORIGINAL_VERSION envelope decoded as a
	// Composition leaves this empty (the envelope wraps the
	// Composition under `data`), which catches the asymmetry even if
	// `_type` discrimination is lenient on a given codec.
	if out.ArchetypeNodeID == "" {
		return fmt.Sprintf("%s decoded *rm.Composition has empty archetype_node_id — body shape looks like ORIGINAL_VERSION<COMPOSITION> not bare COMPOSITION", method)
	}
	if meta == nil || meta.VersionUID == "" {
		return fmt.Sprintf("%s returned no VersionUID metadata (Location/ETag missing)", method)
	}
	return ""
}
