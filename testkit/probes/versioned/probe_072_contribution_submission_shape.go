package versionedprobes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe072ContributionSubmissionShape implements PROBE-072: a
// Contribution submission body MUST be the ITS-REST `Contribution_create`
// schema — `versions[i]` is an inline `ORIGINAL_VERSION<T>` (or
// `IMPORTED_VERSION<T>`) with the payload under `data`, NOT the
// persisted `rm.Contribution` shape where `versions[]` is `[]OBJECT_REF`.
//
// Pins [SDK-GAP-10]. The persisted shape carries OBJECT_REFs pointing
// at versions that do not yet exist at submission time, so a spec-
// conformant CDR rejects it. The probe inspects the captured request
// body (Sandbox mode; the caller supplies a transport.Client wired to
// an httptest server) and asserts:
//
//   - `versions[i]._type` ∈ {"ORIGINAL_VERSION","IMPORTED_VERSION"}
//   - `versions[i].data._type` is present (the inline payload)
//   - `versions[i]._type` ≠ "OBJECT_REF" (the regression)
//   - the batch `audit` and each `versions[i].commit_audit` carry no
//     server-assigned `time_committed` and a `DV_CODED_TEXT`-shaped
//     `change_type` (SPECITS-95 / ITS-REST PR 131) — see [auditWriteShapeIssue]
//
// Symmetric to [Probe071CompositionWriteResponseShape] — both pin
// request/response shape asymmetries that the persisted RM shape would
// otherwise leak onto the wire.
//
// The caller supplies a non-nil Submission; the probe uses
// `Prefer: return=minimal` (the default) so the server's response shape
// is not part of this assertion — it's purely a request-body check.
func Probe072ContributionSubmissionShape(ctx context.Context, c *transport.Client, capturedBody *[]byte, ehrID openehrclient.EHRID, sub *contribution.Submission) (Result, error) {
	r := Result{Probe: "PROBE-072"}
	if c == nil || ehrID == "" || sub == nil || capturedBody == nil {
		return r, errors.New("PROBE-072: missing required inputs (client/ehr/submission/captured)")
	}
	if _, _, err := contribution.Commit(ctx, c, ehrID, sub); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Commit returned error: %v", err)
		return r, nil
	}
	if len(*capturedBody) == 0 {
		r.Status = "fail"
		r.Detail = "captured request body is empty — server fake did not record the request"
		return r, nil
	}
	var body struct {
		Type     string           `json:"_type"`
		Audit    map[string]any   `json:"audit"`
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(*capturedBody, &body); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("request body is not valid JSON: %v", err)
		return r, nil
	}
	if body.Type != "" {
		// Contribution_create has no class envelope — a top-level
		// `_type:"CONTRIBUTION"` would mean the SDK leaked the
		// persisted shape's discriminator.
		r.Status = "fail"
		r.Detail = fmt.Sprintf("top-level _type=%q (Contribution_create has no class envelope; persisted CONTRIBUTION shape leaked)", body.Type)
		return r, nil
	}
	if body.Audit == nil || body.Audit["_type"] != "AUDIT_DETAILS" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("audit._type = %v (want AUDIT_DETAILS)", body.Audit["_type"])
		return r, nil
	}
	if msg := auditWriteShapeIssue("audit", body.Audit); msg != "" {
		r.Status = "fail"
		r.Detail = msg
		return r, nil
	}
	if len(body.Versions) == 0 {
		r.Status = "fail"
		r.Detail = "versions[] is empty — every Contribution_create body must carry at least one version"
		return r, nil
	}
	for i, v := range body.Versions {
		switch t := v["_type"]; t {
		case "ORIGINAL_VERSION", "IMPORTED_VERSION":
			data, ok := v["data"].(map[string]any)
			if !ok || data["_type"] == nil {
				r.Status = "fail"
				r.Detail = fmt.Sprintf("versions[%d].data missing or has no _type (Contribution_create requires inline payload)", i)
				return r, nil
			}
			if ca, ok := v["commit_audit"].(map[string]any); ok {
				if msg := auditWriteShapeIssue(fmt.Sprintf("versions[%d].commit_audit", i), ca); msg != "" {
					r.Status = "fail"
					r.Detail = msg
					return r, nil
				}
			}
		case "OBJECT_REF":
			r.Status = "fail"
			r.Detail = fmt.Sprintf("versions[%d]._type=OBJECT_REF — persisted rm.Contribution shape leaked into the submission body (SDK-GAP-10 regression)", i)
			return r, nil
		default:
			r.Status = "fail"
			r.Detail = fmt.Sprintf("versions[%d]._type = %v (want ORIGINAL_VERSION or IMPORTED_VERSION)", i, t)
			return r, nil
		}
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("Contribution_create body: %d version(s), all inline ORIGINAL/IMPORTED_VERSION with data._type set", len(body.Versions))
	return r, nil
}

// auditWriteShapeIssue returns a non-empty description when a decoded
// commit-audit object violates the ITS-REST write shape (SPECITS-95 /
// ITS-REST PR 131): it MUST omit the server-assigned time_committed and
// carry a DV_CODED_TEXT-shaped change_type (nested defining_code), not a
// flat TERMINOLOGY_CODE triple. Returns "" when the audit is conformant.
func auditWriteShapeIssue(field string, audit map[string]any) string {
	if audit == nil {
		return field + " is missing"
	}
	if _, has := audit["time_committed"]; has {
		return field + " carries time_committed (server-assigned; MUST be omitted on write — SPECITS-95)"
	}
	ct, ok := audit["change_type"].(map[string]any)
	if !ok {
		return field + ".change_type is missing or not an object (want DV_CODED_TEXT)"
	}
	if _, has := ct["defining_code"]; !has {
		return field + ".change_type is not DV_CODED_TEXT-shaped (no defining_code)"
	}
	return ""
}
