package validation

// rmfloor_bytes.go: PROBE-081 — REQ-112 — the presence-aware EHR_STATUS
// entry to the template-less RM floor. It closes the value-typed
// mandatory-attribute blind spot that the value-based [ValidateRMEHRStatus]
// structurally cannot, at the root EHR_STATUS:
//
//   - EHR_STATUS.subject is typed rm.PartySelf — a value struct whose only
//     field (external_ref) is optional — so an omitted subject and a valid
//     bare PARTY_SELF decode to the *identical* Go zero value;
//   - the root's ARCHETYPED archetype_id (rm.ArchetypeID) and rm_version
//     (a string) are value-typed too, so an omitted one decodes to the same
//     zero value as an empty one.
//
// Presence therefore cannot be read from the decoded value; only the
// presence of the key in the source JSON carries it.

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ValidateRMEHRStatusBytes validates a canonical-JSON EHR_STATUS against
// the template-less RM floor, consulting JSON-key presence so
// the value-typed mandatory `subject` is checked correctly.
//
// It decodes data into a *rm.EHRStatus, runs the value-based
// [ValidateRMEHRStatus] floor, and additionally emits `required` at
// `/subject` when the top-level `subject` key is absent from the JSON.
// That is the one signal the Go value cannot carry: a valid bare
// `{"_type":"PARTY_SELF"}` and an omitted subject both decode to the zero
// rm.PartySelf. A present-but-null `subject` (`"subject": null`) is
// treated as absent: a null does not satisfy the mandatory attribute and
// decodes to the same zero rm.PartySelf. A supplied subject, even the bare
// form, yields no spurious `required`.
//
// It decides the root's ARCHETYPED key presence the same way. When the
// top-level `archetype_details` is present, an absent or null `archetype_id`
// key in it is `required` at `/archetype_details/archetype_id`, and an absent
// or null `rm_version` key is `required` at `/archetype_details/rm_version`.
// Both attributes are value-typed, so the value-based floor sees only their
// zero values. A key-presence finding replaces the value-based finding at the
// same path, so each path carries one finding: an absent `rm_version` is
// `required`, not `rm_version_valid`, while an absent `archetype_id` is
// `required` at `archetype_id` beside the value-based `required` at
// `archetype_id/value`. Key presence is read at the root only: an
// ARCHETYPED lower in the tree gets the value-based findings alone.
//
// Attributes the value-based floor already catches (the interface-,
// pointer- and slice-typed mandatories, e.g. `name`, typed rm.DVTextLike)
// remain flagged when absent; the per-RM-type invariant catalogue is
// unchanged. Input that is not a well-formed JSON object (malformed, array,
// scalar, null, or an object repeating a member name), or that fails
// EHR_STATUS decode, surfaces a single
// `invalid_shape` issue at `/` and a not-OK [Result]. When the EHR_STATUS
// decode fails, the decode error, whose text may quote the input, is in
// that issue's Value and not in its Detail.
//
// The decode uses encoding/json/v2 directly rather than
// openehr/serialize/canjson, which openehr/validation does not import:
// the generated UnmarshalJSONFrom methods on the RM types carry the
// codec, and typereg threads the polymorphic decode hooks, so no codec
// entry point is needed. v2's defaults refuse a duplicate member name and
// match member names case-sensitively, as canonical JSON requires; a
// duplicate therefore surfaces as `invalid_shape` at `/` like any other
// failed decode.
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
		// The decode error's text may quote a literal from the input, so
		// it goes into Value and stays out of Detail (REQ-168).
		return resultFromIssues([]Issue{{
			Path:     "/",
			Code:     "invalid_shape",
			Detail:   "ValidateRMEHRStatusBytes: EHR_STATUS decode failed",
			Severity: Error,
			Value:    constraints.Redact(err),
		}})
	}

	r := ValidateRMEHRStatus(&status)

	absent := rootKeyAbsences(keys)
	if len(absent) == 0 {
		return r
	}
	return resultFromIssues(replaceAtPaths(r.Issues, absent))
}

// rootKeyAbsences returns the `required` findings for the value-typed
// mandatory attributes of the root EHR_STATUS whose key is absent or null:
// `subject`, and, when `archetype_details` is present, its `archetype_id`
// and `rm_version`. The value-based floor reads the zero value of each as
// present, so only the key set can tell an omitted attribute from a supplied
// one. A null does not satisfy a mandatory attribute and decodes to the same
// zero value as an omitted one, so it counts as absent.
func rootKeyAbsences(keys map[string]jsontext.Value) []Issue {
	var out []Issue
	if keyAbsent(keys, "subject") {
		out = append(out, Issue{
			Path:     "/subject",
			Code:     "required",
			Detail:   `RM-mandatory attribute "subject" is absent or null on EHR_STATUS`,
			Severity: Error,
		})
	}
	if keyAbsent(keys, "archetype_details") {
		// No ARCHETYPED to read keys from; the value-based floor reports
		// the absent archetype_details on the archetype root.
		return out
	}
	var details map[string]jsontext.Value
	if err := json.Unmarshal(keys["archetype_details"], &details); err != nil {
		// Not expected: the EHR_STATUS decode has already accepted the
		// member as an ARCHETYPED object. Report the shape rather than read
		// keys from a member that has none.
		return append(out, Issue{
			Path:     "/archetype_details",
			Code:     "invalid_shape",
			Detail:   "ValidateRMEHRStatusBytes: archetype_details is not a JSON object",
			Severity: Error,
		})
	}
	for _, attr := range []string{"archetype_id", "rm_version"} {
		if keyAbsent(details, attr) {
			out = append(out, Issue{
				Path:     "/archetype_details/" + attr,
				Code:     "required",
				Detail:   `RM-mandatory attribute "` + attr + `" is absent or null on ARCHETYPED`,
				Severity: Error,
			})
		}
	}
	return out
}

// replaceAtPaths returns issues without any issue at the path of one of
// replacements, followed by replacements. A key-presence finding thereby takes
// the place of the value-based finding at its path.
func replaceAtPaths(issues, replacements []Issue) []Issue {
	replaced := make(map[string]bool, len(replacements))
	for _, r := range replacements {
		replaced[r.Path] = true
	}
	out := make([]Issue, 0, len(issues)+len(replacements))
	for _, i := range issues {
		if !replaced[i.Path] {
			out = append(out, i)
		}
	}
	return append(out, replacements...)
}

// keyAbsent reports whether keys lacks key or holds it as JSON null.
func keyAbsent(keys map[string]jsontext.Value, key string) bool {
	raw, present := keys[key]
	return !present || isJSONNull(raw)
}

// isJSONNull reports whether a raw JSON value is the literal `null` token
// (tolerating surrounding whitespace).
func isJSONNull(raw jsontext.Value) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
