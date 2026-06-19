package ehr

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestMarshalAuditDetails_DottedGrammar(t *testing.T) {
	name := "John Doe"
	a := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		Description: rm.DVText{Value: "initial commit"},
		Committer: rm.PartyIdentified{
			Name: &name,
			ExternalRef: &rm.PartyRef{ObjectRef: rm.ObjectRef{
				ID:        rm.HierObjectID{Value: "BC8132EA-8F4A-11E7-BB31-BE2E44B06B34"},
				Namespace: "demographic",
				Type:      "PERSON",
			}},
		},
		SystemID: "cdr.example",
	}

	got, err := MarshalAuditDetails(a)
	if err != nil {
		t.Fatalf("MarshalAuditDetails: %v", err)
	}
	// Dotted-attribute grammar (REQ-059), NOT a JSON object.
	want := `change_type.code_string="249",` +
		`description.value="initial commit",` +
		`committer.name="John Doe",` +
		`committer.external_ref.id="BC8132EA-8F4A-11E7-BB31-BE2E44B06B34",` +
		`committer.external_ref.namespace="demographic",` +
		`committer.external_ref.type="PERSON",` +
		`system_id="cdr.example"`
	if got != want {
		t.Errorf("audit header\n got = %q\nwant = %q", got, want)
	}
}

func TestMarshalAuditDetails_MinimalCommitter(t *testing.T) {
	name := "Alice"
	a := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "251"}},
		Committer:  rm.PartyIdentified{Name: &name},
	}
	got, err := MarshalAuditDetails(a)
	if err != nil {
		t.Fatalf("MarshalAuditDetails: %v", err)
	}
	want := `change_type.code_string="251",committer.name="Alice"`
	if got != want {
		t.Errorf("audit header\n got = %q\nwant = %q", got, want)
	}
}

func TestMarshalAuditDetails_Nil(t *testing.T) {
	got, err := MarshalAuditDetails(nil)
	if err != nil {
		t.Fatalf("MarshalAuditDetails(nil): %v", err)
	}
	if got != "" {
		t.Errorf("MarshalAuditDetails(nil) = %q, want empty", got)
	}
}

func TestMarshalAuditDetails_RejectsControlChars(t *testing.T) {
	a := &rm.AuditDetails{SystemID: "bad\r\nInjected: header"}
	if _, err := MarshalAuditDetails(a); err == nil {
		t.Fatal("expected error for control characters in audit detail, got nil")
	}
}
