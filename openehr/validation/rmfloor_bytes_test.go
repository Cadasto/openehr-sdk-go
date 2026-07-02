package validation_test

// rmfloor_bytes_test.go: PROBE-081 — REQ-112 / SDK-GAP-18. The
// presence-aware EHR_STATUS floor entry must flag an omitted (value-typed)
// mandatory `subject` from JSON-key presence, without false-positiving on
// a valid bare PARTY_SELF and without regressing the interface-typed
// mandatories the value-based floor already catches.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// A well-formed EHR_STATUS with `subject` supplied as a bare PARTY_SELF
// (no external_ref) — the common, valid "subject is the record patient"
// shape. It decodes to a zero rm.PartySelf, so a Go-value emptiness
// heuristic would wrongly flag it; presence-from-JSON must not.
const ehrStatusBareSubject = `{
	"_type": "EHR_STATUS",
	"name": {"_type": "DV_TEXT", "value": "EHR Status"},
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	"subject": {"_type": "PARTY_SELF"},
	"is_modifiable": true,
	"is_queryable": true
}`

// TestValidateRMEHRStatusBytes_MissingSubject is the SDK-GAP-18 case: a
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
	if !hasIssue(r.Issues, "/subject", "required") {
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
	if hasIssue(r.Issues, "/subject", "required") {
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
	if !hasIssue(r.Issues, "/name", "required") {
		t.Errorf("expected required @ /name, got %+v", r.Issues)
	}
	if hasIssue(r.Issues, "/subject", "required") {
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
	if !hasIssue(r.Issues, "/", "invalid_shape") {
		t.Errorf("expected invalid_shape @ /, got %+v", r.Issues)
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
			if !hasIssue(r.Issues, "/", "invalid_shape") {
				t.Errorf("expected invalid_shape @ /, got %+v", r.Issues)
			}
		})
	}
}
