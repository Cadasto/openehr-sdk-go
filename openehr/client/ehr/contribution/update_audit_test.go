package contribution_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// newUpdateAudit builds a fully-populated UpdateAudit for the unit tests.
func newUpdateAudit() contribution.UpdateAudit {
	name := "bob"
	desc := rm.DVText{Value: "initial commit"}
	return contribution.UpdateAudit{
		SystemID:    "cdr.example",
		Committer:   &rm.PartyIdentified{Name: &name},
		ChangeType:  rm.DVCodedText{DVText: rm.DVText{Value: "creation"}, DefiningCode: rm.CodePhrase{CodeString: "249"}},
		Description: desc,
	}
}

// TestUpdateAuditMarshalType checks that UpdateAudit serialises with
// _type:"AUDIT_DETAILS" as required by ITS-REST PR 131 / SPECITS-95.
func TestUpdateAuditMarshalType(t *testing.T) {
	b, err := json.Marshal(newUpdateAudit())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := m["_type"]; got != "AUDIT_DETAILS" {
		t.Errorf("_type = %v, want AUDIT_DETAILS", got)
	}
}

// TestUpdateAuditTypeFallback verifies the zero-value Type emits
// AUDIT_DETAILS (the SDK default) while AuditTypeUpdateAudit emits
// UPDATE_AUDIT — the fallback for non-conformant servers (SPECITS-95).
func TestUpdateAuditTypeFallback(t *testing.T) {
	cases := []struct {
		name string
		typ  contribution.AuditType
		want string
	}{
		{"default", "", "AUDIT_DETAILS"},
		{"explicit AUDIT_DETAILS", contribution.AuditTypeAuditDetails, "AUDIT_DETAILS"},
		{"UPDATE_AUDIT fallback", contribution.AuditTypeUpdateAudit, "UPDATE_AUDIT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newUpdateAudit()
			a.Type = tc.typ
			b, err := json.Marshal(a)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if got := m["_type"]; got != tc.want {
				t.Errorf("_type = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUpdateAuditMarshalChangeTypeDVCodedText checks that change_type has the
// nested defining_code shape (DV_CODED_TEXT), not a flat terminology-code
// triple.
func TestUpdateAuditMarshalChangeTypeDVCodedText(t *testing.T) {
	b, err := json.Marshal(newUpdateAudit())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	ct, ok := m["change_type"].(map[string]any)
	if !ok {
		t.Fatalf("change_type is not an object: %v", m["change_type"])
	}
	if _, has := ct["defining_code"]; !has {
		t.Errorf("change_type missing defining_code — want DV_CODED_TEXT shape, got %v", ct)
	}
	// Guard the marshal-by-pointer fix: the DV_CODED_TEXT discriminator is
	// only emitted when the (pointer-receiver) MarshalJSON fires.
	if got := ct["_type"]; got != "DV_CODED_TEXT" {
		t.Errorf("change_type._type = %v, want DV_CODED_TEXT (pointer-marshal regression?)", got)
	}
	// Committer carries its own discriminator when set as a pointer.
	if c, ok := m["committer"].(map[string]any); ok {
		if got := c["_type"]; got != "PARTY_IDENTIFIED" {
			t.Errorf("committer._type = %v, want PARTY_IDENTIFIED", got)
		}
	}
}

// TestUpdateAuditNoTimeCommitted verifies that time_committed is absent from
// the marshalled payload — it is a server-assigned field and MUST NOT appear
// on the write path.
func TestUpdateAuditNoTimeCommitted(t *testing.T) {
	b, err := json.Marshal(newUpdateAudit())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, has := m["time_committed"]; has {
		t.Errorf("time_committed must be absent from UpdateAudit JSON, got %v", m["time_committed"])
	}
}

// TestUpdateAuditSystemIDOmitWhenEmpty checks that system_id is absent when
// UpdateAudit.SystemID is the zero string.
func TestUpdateAuditSystemIDOmitWhenEmpty(t *testing.T) {
	a := newUpdateAudit()
	a.SystemID = ""
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, has := m["system_id"]; has {
		t.Errorf("system_id must be omitted when empty, got %v", m["system_id"])
	}
}

// TestUpdateAuditSystemIDPresentWhenSet checks that system_id appears when
// UpdateAudit.SystemID is non-empty.
func TestUpdateAuditSystemIDPresentWhenSet(t *testing.T) {
	b, err := json.Marshal(newUpdateAudit())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := m["system_id"]; got != "cdr.example" {
		t.Errorf("system_id = %v, want cdr.example", got)
	}
}

// TestUpdateAuditDescriptionOmitWhenNil checks that description is absent
// when UpdateAudit.Description is nil.
func TestUpdateAuditDescriptionOmitWhenNil(t *testing.T) {
	a := newUpdateAudit()
	a.Description = nil
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, has := m["description"]; has {
		t.Errorf("description must be omitted when nil, got %v", m["description"])
	}
}

// TestUpdateAuditFromAuditDetails verifies the adapter: copies
// change_type/committer/description/system_id and drops time_committed.
func TestUpdateAuditFromAuditDetails(t *testing.T) {
	name := "alice"
	desc := rm.DVText{Value: "reason"}
	ad := rm.AuditDetails{
		SystemID:      "cdr.example",
		Committer:     &rm.PartyIdentified{Name: &name},
		ChangeType:    rm.DVCodedText{DVText: rm.DVText{Value: "creation"}, DefiningCode: rm.CodePhrase{CodeString: "249"}},
		Description:   desc,
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	ua := contribution.UpdateAuditFromAuditDetails(ad)
	b, err := json.Marshal(ua)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	// _type must be AUDIT_DETAILS
	if got := m["_type"]; got != "AUDIT_DETAILS" {
		t.Errorf("_type = %v, want AUDIT_DETAILS", got)
	}
	// system_id must be copied
	if got := m["system_id"]; got != "cdr.example" {
		t.Errorf("system_id = %v, want cdr.example", got)
	}
	// time_committed must NOT appear
	if _, has := m["time_committed"]; has {
		t.Errorf("time_committed must be absent after UpdateAuditFromAuditDetails, got %v", m["time_committed"])
	}
	// change_type must have defining_code (DV_CODED_TEXT shape)
	ct, ok := m["change_type"].(map[string]any)
	if !ok {
		t.Fatalf("change_type is not an object: %v", m["change_type"])
	}
	if _, has := ct["defining_code"]; !has {
		t.Errorf("change_type missing defining_code — want DV_CODED_TEXT shape, got %v", ct)
	}
	// description must be present
	if _, has := m["description"]; !has {
		t.Errorf("description must be present after UpdateAuditFromAuditDetails, got nil")
	}
}

// TestSubmissionAuditNoTimeCommitted verifies that when a Submission is
// marshalled the top-level audit block has no time_committed key — the
// Submission.Audit is now UpdateAudit, not rm.AuditDetails.
func TestSubmissionAuditNoTimeCommitted(t *testing.T) {
	sub := &contribution.Submission{
		Audit:    newUpdateAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	b, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("json.Marshal Submission: %v", err)
	}
	var body struct {
		Audit map[string]any `json:"audit"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, has := body.Audit["time_committed"]; has {
		t.Errorf("Submission.audit must not contain time_committed, got %v", body.Audit["time_committed"])
	}
	if got := body.Audit["_type"]; got != "AUDIT_DETAILS" {
		t.Errorf("Submission.audit._type = %v, want AUDIT_DETAILS", got)
	}
}
