package validation_test

// rmfloor_bytes_test.go: PROBE-081 — REQ-112. The
// presence-aware EHR_STATUS floor entry must flag an omitted (value-typed)
// mandatory `subject` from JSON-key presence, without false-positiving on
// a valid bare PARTY_SELF and without regressing the interface-typed
// mandatories the value-based floor already catches.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// A well-formed EHR_STATUS with `subject` supplied as a bare PARTY_SELF
// (no external_ref) — the common, valid "subject is the record patient"
// shape. It decodes to a zero rm.PartySelf, so a Go-value emptiness
// heuristic would wrongly flag it; presence-from-JSON must not. EHR_STATUS is
// an archetype root, so the body carries a complete archetype_details and is
// valid RM throughout (REQ-112).
const ehrStatusBareSubject = `{
	"_type": "EHR_STATUS",
	"name": {"_type": "DV_TEXT", "value": "EHR Status"},
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	"archetype_details": {"_type": "ARCHETYPED",
		"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"},
		"rm_version": "1.1.0"},
	"subject": {"_type": "PARTY_SELF"},
	"is_modifiable": true,
	"is_queryable": true
}`

// TestValidateRMEHRStatusBytes_MissingSubject is the PROBE-081 case: a
// decodable EHR_STATUS that omits the RM-mandatory `subject` key must
// surface `required` at /subject (the value-based floor cannot see it).
func TestValidateRMEHRStatusBytes_MissingSubject(t *testing.T) {
	data := []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"is_modifiable": true,
		"is_queryable": true
	}`)
	r := validation.ValidateRMEHRStatusBytes(data)
	if r.OK {
		t.Fatalf("EHR_STATUS omitting subject must not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/subject", "required") {
		t.Errorf("expected required @ /subject, got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_BareSubjectOK guards the false-positive:
// a present-but-minimal PARTY_SELF is valid RM and must report OK.
func TestValidateRMEHRStatusBytes_BareSubjectOK(t *testing.T) {
	r := validation.ValidateRMEHRStatusBytes([]byte(ehrStatusBareSubject))
	if !r.OK {
		t.Fatalf("bare-but-present subject must be OK; got %+v", r.Issues)
	}
	if containsIssue(r.Issues, "/subject", "required") {
		t.Errorf("present subject must not be flagged required; got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_MissingNameStillFlagged is the no-regression
// case: the interface-typed mandatory `name` is still flagged when absent
// even though `subject` is present.
func TestValidateRMEHRStatusBytes_MissingNameStillFlagged(t *testing.T) {
	data := []byte(`{
		"_type": "EHR_STATUS",
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"subject": {"_type": "PARTY_SELF"},
		"is_modifiable": true,
		"is_queryable": true
	}`)
	r := validation.ValidateRMEHRStatusBytes(data)
	if r.OK {
		t.Fatalf("missing name must not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/name", "required") {
		t.Errorf("expected required @ /name, got %+v", r.Issues)
	}
	if containsIssue(r.Issues, "/subject", "required") {
		t.Errorf("present subject must not be flagged; got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_MalformedSubject covers the second decode
// branch: a well-formed JSON object whose `subject` is the wrong shape
// passes the key-presence map decode but fails the typed EHR_STATUS decode,
// surfacing invalid_shape at "/".
func TestValidateRMEHRStatusBytes_MalformedSubject(t *testing.T) {
	data := []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"subject": "not-an-object",
		"is_modifiable": true,
		"is_queryable": true
	}`)
	r := validation.ValidateRMEHRStatusBytes(data)
	if r.OK {
		t.Fatalf("malformed subject must not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/", "invalid_shape") {
		t.Errorf("expected invalid_shape @ /, got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_SubjectNull covers an explicit JSON null:
// a null value does not satisfy the RM-mandatory `subject` (it decodes to
// the same zero PartySelf as an absent subject), so it is treated as absent
// — required @ /subject — rather than reading OK because the key exists.
func TestValidateRMEHRStatusBytes_SubjectNull(t *testing.T) {
	data := []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"subject": null,
		"is_modifiable": true,
		"is_queryable": true
	}`)
	r := validation.ValidateRMEHRStatusBytes(data)
	if r.OK {
		t.Fatalf("subject: null must not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/subject", "required") {
		t.Errorf("expected required @ /subject for a null subject, got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_InvalidShape covers non-object and
// malformed inputs: each surfaces a single invalid_shape issue at "/".
func TestValidateRMEHRStatusBytes_InvalidShape(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"malformed", `{`},
		{"array", `[]`},
		{"scalar", `42`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRMEHRStatusBytes([]byte(tc.data))
			if r.OK {
				t.Errorf("%s input must not be OK", tc.name)
			}
			if !containsIssue(r.Issues, "/", "invalid_shape") {
				t.Errorf("expected invalid_shape @ /, got %+v", r.Issues)
			}
		})
	}
}

// TestValidateRMEHRStatusBytes_DuplicateMemberName pins the canonical-JSON
// refusal of a repeated member name on the presence-aware floor entry
// (REQ-052, REQ-112). encoding/json/v2 refuses a duplicate name by default, so
// an EHR_STATUS that states is_queryable twice, with two different values, has
// no single defined value and is not OK: one invalid_shape issue at "/". This
// is a can-fail control for the v2 decode: on the v1 encoding/json decode the
// last duplicate wins silently and the result is OK.
func TestValidateRMEHRStatusBytes_DuplicateMemberName(t *testing.T) {
	const data = `{
	"_type": "EHR_STATUS",
	"name": {"_type": "DV_TEXT", "value": "EHR Status"},
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	"subject": {"_type": "PARTY_SELF"},
	"is_modifiable": true,
	"is_queryable": true,
	"is_queryable": false
}`
	r := validation.ValidateRMEHRStatusBytes([]byte(data))
	if r.OK {
		t.Errorf("ValidateRMEHRStatusBytes(duplicate is_queryable) OK = true; want not OK, got issues %+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/", "invalid_shape") {
		t.Errorf("ValidateRMEHRStatusBytes(duplicate is_queryable): want invalid_shape @ /, got %+v", r.Issues)
	}
}

// TestValidateRMEHRStatusBytes_V2DecodeSemantics pins what the move to
// encoding/json/v2 changed at this entry (REQ-052, REQ-112). There are two
// decodes: the top-level key map (map[string]jsontext.Value) and the body
// (rm.EHRStatus).
//
// The case-variant "Name" pins the body decode. v2 matches member names
// exactly, so "Name" is an unknown member, the decoded EHR_STATUS has no name,
// and the value floor reports `required` at /name. Can-fail (checked with go
// test -overlay on a copy of rmfloor_bytes.go whose body decode is v1
// encoding/json.Unmarshal): v1 matches "Name" case-insensitively, populates
// name, and the result turns OK, so this case goes red. A duplicate member
// cannot pin the body decode: the key-map decode runs first under v2 and, while
// it reads each value as a jsontext.Value, refuses a repeated name at every
// nesting level (a nested duplicate reports `within "/name"`), so the body
// decode is never reached.
//
// The other two cases, invalid UTF-8 even inside a member EHR_STATUS does not
// declare, and a repeated `subject` whose second value is null, are refused by
// both v2 decodes, the key map's and the body's, and either refusal is enough:
// relaxing only the key-map decode (AllowDuplicateNames, AllowInvalidUTF8)
// leaves them green, so they go red only when both decodes fall back to v1's
// options, that is on a full revert to v1. The duplicate rule wins over the
// presence rule: the input is refused as `invalid_shape` at / before the null
// `subject` could be read as absent.
func TestValidateRMEHRStatusBytes_V2DecodeSemantics(t *testing.T) {
	const tail = `"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"is_modifiable": true,
		"is_queryable": true
	}`
	cases := []struct {
		name, data, path, code string
	}{
		{
			name: "case-variant Name is an unknown member",
			data: `{"_type": "EHR_STATUS",
		"Name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"subject": {"_type": "PARTY_SELF"},
		` + tail,
			path: "/name", code: "required",
		},
		{
			name: "invalid UTF-8 inside an unknown member",
			data: `{"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"subject": {"_type": "PARTY_SELF"},
		"x_note": "a` + "\xff" + `b",
		` + tail,
			path: "/", code: "invalid_shape",
		},
		{
			name: "subject twice, the second null",
			data: `{"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"subject": {"_type": "PARTY_SELF"},
		"subject": null,
		` + tail,
			path: "/", code: "invalid_shape",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRMEHRStatusBytes([]byte(tc.data))
			if r.OK {
				t.Errorf("ValidateRMEHRStatusBytes(%s) OK = true; want not OK with %s @ %s (issues %+v)", tc.name, tc.code, tc.path, r.Issues)
			}
			if !containsIssue(r.Issues, tc.path, tc.code) {
				t.Errorf("ValidateRMEHRStatusBytes(%s): want %s @ %s, got %+v", tc.name, tc.code, tc.path, r.Issues)
			}
		})
	}
}

// completeArchetypeDetails is an ARCHETYPED carrying both RM-mandatory
// attributes, the shape an EHR_STATUS archetype root needs.
const completeArchetypeDetails = `{"_type": "ARCHETYPED",
		"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"},
		"rm_version": "1.1.0"}`

// ehrStatusWithArchetypeDetails returns a valid EHR_STATUS body whose
// archetype_details member is details, verbatim; an empty details leaves the
// member out.
func ehrStatusWithArchetypeDetails(details string) []byte {
	member := ""
	if details != "" {
		member = `"archetype_details": ` + details + `,`
	}
	return []byte(`{
	"_type": "EHR_STATUS",
	"name": {"_type": "DV_TEXT", "value": "EHR Status"},
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	` + member + `
	"subject": {"_type": "PARTY_SELF"},
	"is_modifiable": true,
	"is_queryable": true
}`)
}

// findingsOf returns each issue as "code path", sorted, so two results compare
// by their findings and not by the order the walk emits them in.
func findingsOf(r validation.Result) []string {
	out := make([]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, i.Code+" "+i.Path)
	}
	slices.Sort(out)
	return out
}

// TestValidateRMEHRStatus_ArchetypeDetails is PROBE-081's ARCHETYPED arm
// (REQ-112). EHR_STATUS is an archetype root (RM Is_archetype_root), so its
// archetype_details is mandatory, and an ARCHETYPED needs a non-empty
// archetype_id and rm_version. Each row gives one archetype_details shape and
// the exact findings of both entries:
//
//   - the value entry (ValidateRMEHRStatus on the decoded value) reads absent,
//     null and empty the same way, since they decode to one zero value: an
//     empty archetype_id is `required` at archetype_id/value, an empty
//     rm_version is `rm_version_valid`;
//   - the Bytes entry also sees the JSON keys of the root's archetype_details,
//     so an absent or null archetype_id or rm_version key is `required` at the
//     attribute path. That finding replaces the value walk's finding at the
//     same path (rm_version), and sits beside the deeper one it does not share
//     a path with (archetype_id/value). One finding per path.
func TestValidateRMEHRStatus_ArchetypeDetails(t *testing.T) {
	const (
		detailsPath = "/archetype_details"
		idPath      = "/archetype_details/archetype_id"
		idValuePath = "/archetype_details/archetype_id/value"
		versionPath = "/archetype_details/rm_version"
	)
	const validID = `"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"}`
	cases := []struct {
		name      string
		details   string // the archetype_details member; empty leaves it out
		wantValue []string
		wantBytes []string
	}{
		{
			name:      "archetype_details absent",
			wantValue: []string{"is_archetype_root " + detailsPath},
			wantBytes: []string{"is_archetype_root " + detailsPath},
		},
		{
			name:      "archetype_details null",
			details:   `null`,
			wantValue: []string{"is_archetype_root " + detailsPath},
			wantBytes: []string{"is_archetype_root " + detailsPath},
		},
		{
			name:      "ARCHETYPED with no attributes",
			details:   `{"_type": "ARCHETYPED"}`,
			wantValue: []string{"required " + idValuePath, "rm_version_valid " + versionPath},
			wantBytes: []string{"required " + idPath, "required " + idValuePath, "required " + versionPath},
		},
		{
			name:      "archetype_id value empty",
			details:   `{"_type": "ARCHETYPED", "archetype_id": {"_type": "ARCHETYPE_ID", "value": ""}, "rm_version": "1.1.0"}`,
			wantValue: []string{"required " + idValuePath},
			wantBytes: []string{"required " + idValuePath},
		},
		{
			name:      "archetype_id absent",
			details:   `{"_type": "ARCHETYPED", "rm_version": "1.1.0"}`,
			wantValue: []string{"required " + idValuePath},
			wantBytes: []string{"required " + idPath, "required " + idValuePath},
		},
		{
			name:      "archetype_id null",
			details:   `{"_type": "ARCHETYPED", "archetype_id": null, "rm_version": "1.1.0"}`,
			wantValue: []string{"required " + idValuePath},
			wantBytes: []string{"required " + idPath, "required " + idValuePath},
		},
		{
			name:      "rm_version absent",
			details:   `{"_type": "ARCHETYPED", ` + validID + `}`,
			wantValue: []string{"rm_version_valid " + versionPath},
			wantBytes: []string{"required " + versionPath},
		},
		{
			name:      "rm_version null",
			details:   `{"_type": "ARCHETYPED", ` + validID + `, "rm_version": null}`,
			wantValue: []string{"rm_version_valid " + versionPath},
			wantBytes: []string{"required " + versionPath},
		},
		{
			name:      "rm_version empty",
			details:   `{"_type": "ARCHETYPED", ` + validID + `, "rm_version": ""}`,
			wantValue: []string{"rm_version_valid " + versionPath},
			wantBytes: []string{"rm_version_valid " + versionPath},
		},
		{
			name:    "complete",
			details: completeArchetypeDetails,
		},
		{
			// template_id is optional in the RM; its presence changes nothing.
			name:    "complete with template_id",
			details: `{"_type": "ARCHETYPED", ` + validID + `, "template_id": {"_type": "TEMPLATE_ID", "value": "ehr_status.v1"}, "rm_version": "1.1.0"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := ehrStatusWithArchetypeDetails(tc.details)

			var status rm.EHRStatus
			if err := canjson.Unmarshal(data, &status); err != nil {
				t.Fatalf("canjson.Unmarshal: %v", err)
			}
			if got := findingsOf(validation.ValidateRMEHRStatus(&status)); !slices.Equal(got, tc.wantValue) {
				t.Errorf("ValidateRMEHRStatus(decoded %s) findings = %q, want %q", tc.name, got, tc.wantValue)
			}

			r := validation.ValidateRMEHRStatusBytes(data)
			if got := findingsOf(r); !slices.Equal(got, tc.wantBytes) {
				t.Errorf("ValidateRMEHRStatusBytes(%s) findings = %q, want %q", tc.name, got, tc.wantBytes)
			}
			if r.OK != (len(tc.wantBytes) == 0) {
				t.Errorf("ValidateRMEHRStatusBytes(%s).OK = %v, want %v", tc.name, r.OK, len(tc.wantBytes) == 0)
			}
			seen := map[string]bool{}
			for _, i := range r.Issues {
				if seen[i.Path] {
					t.Errorf("ValidateRMEHRStatusBytes(%s) reports %s twice; want one finding per path: %+v", tc.name, i.Path, r.Issues)
				}
				seen[i.Path] = true
			}
		})
	}
}

// TestValidateRMEHRStatusBytes_ArchetypeKeysRootOnly pins the scope of the
// Bytes entry's ARCHETYPED key presence (REQ-112): it reads the root
// EHR_STATUS's archetype_details only, as it does subject. An ARCHETYPED lower
// down, here under other_details, gets the value walk's findings alone: its
// absent rm_version is `rm_version_valid`, not `required`, and its absent
// archetype_id adds no finding at archetype_id itself.
func TestValidateRMEHRStatusBytes_ArchetypeKeysRootOnly(t *testing.T) {
	data := ehrStatusWithArchetypeDetails(completeArchetypeDetails)
	nested := `"other_details": {"_type": "ITEM_TREE", "name": {"_type": "DV_TEXT", "value": "tree"},
		"archetype_node_id": "openEHR-EHR-ITEM_TREE.status.v1",
		"archetype_details": {"_type": "ARCHETYPED"}},
	"is_queryable"`
	data = []byte(strings.Replace(string(data), `"is_queryable"`, nested, 1))

	want := []string{
		"required /other_details/archetype_details/archetype_id/value",
		"rm_version_valid /other_details/archetype_details/rm_version",
	}
	if got := findingsOf(validation.ValidateRMEHRStatusBytes(data)); !slices.Equal(got, want) {
		t.Errorf("ValidateRMEHRStatusBytes(nested ARCHETYPED with no attributes) findings = %q, want %q", got, want)
	}
}
