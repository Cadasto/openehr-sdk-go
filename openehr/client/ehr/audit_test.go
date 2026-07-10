package ehr

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
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

func TestMarshalAuditDetails_OmitsExternalRefWithoutID(t *testing.T) {
	name := "Alice"
	a := &rm.AuditDetails{
		Committer: rm.PartyIdentified{
			Name: &name,
			// external_ref present but its id has no value — must not emit
			// an orphan namespace/type with no id.
			ExternalRef: &rm.PartyRef{ObjectRef: rm.ObjectRef{
				ID:        rm.HierObjectID{Value: ""},
				Namespace: "demographic",
				Type:      "PERSON",
			}},
		},
	}
	got, err := MarshalAuditDetails(a)
	if err != nil {
		t.Fatalf("MarshalAuditDetails: %v", err)
	}
	if strings.Contains(got, "external_ref") {
		t.Errorf("emitted an external_ref group with no id: %q", got)
	}
	if !strings.Contains(got, `committer.name="Alice"`) {
		t.Errorf("missing committer.name: %q", got)
	}
}

func TestMarshalAuditDetails_RejectsControlChars(t *testing.T) {
	a := &rm.AuditDetails{SystemID: "bad\r\nInjected: header"}
	if _, err := MarshalAuditDetails(a); err == nil {
		t.Fatal("expected error for control characters in audit detail, got nil")
	}
}

func TestMarshalAuditDetails_RejectsTypedNilCommitter(t *testing.T) {
	var committer rm.PartyProxy = (*rm.PartyIdentified)(nil)
	a := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "249"}},
		Committer:  committer,
	}
	_, err := MarshalAuditDetails(a)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig for typed-nil committer, got %v", err)
	}
}

func TestMarshalAuditDetails_RejectsTypedNilObjectID(t *testing.T) {
	name := "Alice"
	var id rm.ObjectID = (*rm.HierObjectID)(nil)
	a := &rm.AuditDetails{
		ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "249"}},
		Committer: rm.PartyIdentified{
			Name: &name,
			ExternalRef: &rm.PartyRef{ObjectRef: rm.ObjectRef{
				ID:        id,
				Namespace: "demographic",
				Type:      "PERSON",
			}},
		},
	}
	_, err := MarshalAuditDetails(a)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig for typed-nil external_ref id, got %v", err)
	}
}
