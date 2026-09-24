package validation

// rmfloor_bytes.go: PROBE-081 — REQ-112 — the presence-aware EHR_STATUS
// entry to the template-less RM floor. It closes the value-typed
// mandatory-attribute blind spot that the value-based [ValidateRMEHRStatus]
// structurally cannot: EHR_STATUS.subject is typed rm.PartySelf — a value
// struct whose only field (external_ref) is optional — so an omitted
// subject and a valid bare PARTY_SELF decode to the *identical* Go zero
// value. Presence therefore cannot be read from the decoded value; only
// the presence of the `subject` key in the source JSON carries it.

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// ValidateRMEHRStatusBytes validates a canonical-JSON EHR_STATUS against
// the template-less RM floor (REQ-112), consulting JSON-key presence so
// the value-typed mandatory `subject` is checked correctly (PROBE-081).
//
// It decodes data into a *rm.EHRStatus, runs the value-based
// [ValidateRMEHRStatus] floor, and additionally emits `required` at
// `/subject` when the top-level `subject` key is absent from the JSON —
// the one signal the Go value cannot carry (a valid bare
// `{"_type":"PARTY_SELF"}` and an omitted subject both decode to the zero
// rm.PartySelf). A present-but-null `subject` (`"subject": null`) is
// treated as absent: a null does not satisfy the mandatory attribute and
// decodes to the same zero rm.PartySelf. A supplied subject, even the bare
// form, yields no spurious `required`.
//
// Attributes the value-based floor already catches — the interface- /
// pointer- / slice-typed mandatories (e.g. `name`, typed rm.DVTextLike) —
// remain flagged when absent; the per-RM-type invariant catalogue is
// unchanged. Input that is not a well-formed JSON object (malformed, array,
// scalar, null, or an object repeating a member name), or that fails
// EHR_STATUS decode, surfaces a single
// `invalid_shape` issue at `/` and a not-OK [Result].
//
// The decode uses encoding/json/v2 directly rather than
// openehr/serialize/canjson, which REQ-013 forbids openehr/validation from
// importing: the generated UnmarshalJSONFrom methods on the RM types carry
// the codec, and typereg threads the polymorphic decode hooks, so no codec
// entry point is needed. v2's defaults refuse a duplicate member name and
// match member names case-sensitively, as the canonical-JSON contract
// requires (REQ-052); a duplicate therefore surfaces as `invalid_shape` at
// `/` like any other failed decode.
func ValidateRMEHRStatusBytes(data []byte) Result {
	// Top-level key presence — the only signal that separates an omitted
	// `subject` from a supplied bare PARTY_SELF. A non-object input
	// (array/scalar/null/malformed, or a repeated member name) fails to
	// unmarshal into the map (or yields a nil map for `null`) and is
	// reported as an invalid shape.
	var keys map[string]jsontext.Value
	if err := json.Unmarshal(data, &keys); err != nil || keys == nil {
		return resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "invalid_shape",
			Detail:   "ValidateRMEHRStatusBytes: input is not a well-formed JSON object",
			Severity: Error,
		}})
	}

	var status rm.EHRStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "invalid_shape",
			Detail:   "ValidateRMEHRStatusBytes: EHR_STATUS decode failed: " + err.Error(),
			Severity: Error,
		}})
	}

	r := ValidateRMEHRStatus(&status)

	if raw, present := keys["subject"]; !present || isJSONNull(raw) {
		// subject is RM-mandatory (rminfo) and value-typed (rm.PartySelf);
		// the value-based floor reads its zero value as present, so the
		// absence is decided here from JSON-key presence. A present-but-null
		// value is treated as absent — a null does not satisfy a mandatory
		// attribute and decodes to the same zero PartySelf as an omitted one.
		return resultFromIssues(append(r.Issues, Issue{
			Path:     "/subject",
			Code:     "required",
			Detail:   `RM-mandatory attribute "subject" is absent or null on EHR_STATUS`,
			Severity: Error,
		}))
	}
	return r
}

// isJSONNull reports whether a raw JSON value is the literal `null` token
// (tolerating surrounding whitespace).
func isJSONNull(raw jsontext.Value) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
