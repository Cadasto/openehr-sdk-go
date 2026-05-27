package rm_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestDVTextLikeGetValue pins the SDK-GAP-11 Phase 1 ergonomic method:
// `.GetValue()` on a DVTextLike returns the rendered text of the
// underlying concrete type — DVText OR DVCodedText. Equivalence with
// the legacy compat helper `rm.DVTextValueOf` is the migration
// guarantee.
func TestDVTextLikeGetValue(t *testing.T) {
	tests := []struct {
		name string
		v    rm.DVTextLike
		want string
	}{
		{
			name: "DVText value",
			v:    rm.DVText{Value: "blood pressure"},
			want: "blood pressure",
		},
		{
			name: "DVText pointer",
			v:    &rm.DVText{Value: "blood pressure"},
			want: "blood pressure",
		},
		{
			name: "DVCodedText value",
			v:    rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			want: "moderate",
		},
		{
			name: "DVCodedText pointer",
			v:    &rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			want: "moderate",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.GetValue(); got != tc.want {
				t.Errorf("GetValue() = %q, want %q", got, tc.want)
			}
			// Equivalent compat-helper output — the migration guarantee.
			if got := rm.DVTextValueOf(tc.v); got != tc.want {
				t.Errorf("DVTextValueOf = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDVTextLikeGetValueNilInterface guards against the v0.x.y pattern
// where a LOCATABLE-descended struct may have a nil Name field (rare
// but legal pre-construction). `DVTextValueOf(nil)` must return ""
// without panicking; calling `.GetValue()` directly on a nil interface
// is a runtime panic by Go's normal interface rules — exercised here
// only via the helper.
func TestDVTextLikeGetValueNilInterface(t *testing.T) {
	if got := rm.DVTextValueOf(nil); got != "" {
		t.Errorf("DVTextValueOf(nil) = %q, want \"\"", got)
	}
}

// TestDVURILikeGetValue pins the Phase 2 accessor: `.GetValue()` on a
// DVURILike returns the URI string of the underlying concrete type
// (DVURI or DVEHRURI).
func TestDVURILikeGetValue(t *testing.T) {
	tests := []struct {
		name string
		v    rm.DVURILike
		want string
	}{
		{"DVURI value", rm.DVURI{Value: "https://example.org/x"}, "https://example.org/x"},
		{"DVURI pointer", &rm.DVURI{Value: "https://example.org/x"}, "https://example.org/x"},
		{"DVEHRURI value", rm.DVEHRURI{DVURI: rm.DVURI{Value: "ehr://e1/abc"}}, "ehr://e1/abc"},
		{"DVEHRURI pointer", &rm.DVEHRURI{DVURI: rm.DVURI{Value: "ehr://e1/abc"}}, "ehr://e1/abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.GetValue(); got != tc.want {
				t.Errorf("GetValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAuditDetailsLikeAccessors pins Phase 2 accessors on AUDIT_DETAILS
// and its ATTESTATION subtype: GetSystemID / GetTimeCommitted /
// GetChangeType / GetCommitter / GetDescription. Attestation inherits
// the parent's audit fields via Go embedding.
func TestAuditDetailsLikeAccessors(t *testing.T) {
	change := rm.DVCodedText{DVText: rm.DVText{Value: "creation"}, DefiningCode: rm.CodePhrase{CodeString: "249"}}
	descName := "alice"
	committer := rm.PartyIdentified{Name: &descName}
	when := rm.DVDateTime{Value: "2026-05-26T10:00:00Z"}

	base := rm.AuditDetails{
		SystemID:      "cdr.example",
		TimeCommitted: when,
		ChangeType:    change,
		Committer:     committer,
	}
	att := rm.Attestation{AuditDetails: base, IsPending: false}

	for _, tc := range []struct {
		name string
		v    rm.AuditDetailsLike
	}{
		{"AuditDetails", base},
		{"Attestation (inherits base fields)", att},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.v.GetSystemID() != "cdr.example" {
				t.Errorf("GetSystemID = %q", tc.v.GetSystemID())
			}
			if tc.v.GetTimeCommitted().Value != "2026-05-26T10:00:00Z" {
				t.Errorf("GetTimeCommitted.Value = %q", tc.v.GetTimeCommitted().Value)
			}
			if tc.v.GetChangeType().Value != "creation" {
				t.Errorf("GetChangeType.Value = %q", tc.v.GetChangeType().Value)
			}
			c, ok := tc.v.GetCommitter().(rm.PartyIdentified)
			if !ok || c.Name == nil || *c.Name != "alice" {
				t.Errorf("GetCommitter = %#v (expected PartyIdentified Name=alice)", tc.v.GetCommitter())
			}
			if _, present := tc.v.GetDescription(); present {
				t.Errorf("GetDescription should be absent for empty description")
			}
		})
	}
}

// TestPartyIdentifiedLikeAccessors pins GetName / GetIdentifiers /
// GetExternalRef on both PartyIdentified and PartyRelated.
func TestPartyIdentifiedLikeAccessors(t *testing.T) {
	name := "Dr. Smith"
	issuer := "GMC"
	pid := rm.PartyIdentified{
		Name: &name,
		Identifiers: []rm.DVIdentifier{
			{ID: "12345", Issuer: &issuer},
		},
	}
	rel := rm.PartyRelated{PartyIdentified: pid, Relationship: rm.DVCodedText{DVText: rm.DVText{Value: "self"}}}

	for _, tc := range []struct {
		name string
		v    rm.PartyIdentifiedLike
	}{
		{"PartyIdentified", pid},
		{"PartyRelated (inherits PartyIdentified fields)", rel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := tc.v.GetName()
			if !ok || n != "Dr. Smith" {
				t.Errorf("GetName = (%q, %v), want (Dr. Smith, true)", n, ok)
			}
			ids := tc.v.GetIdentifiers()
			if len(ids) != 1 || ids[0].ID != "12345" {
				t.Errorf("GetIdentifiers = %#v", ids)
			}
			if _, present := tc.v.GetExternalRef(); present {
				t.Errorf("GetExternalRef should be absent for nil ExternalRef")
			}
		})
	}
}

// TestObjectRefLikeAccessors pins GetID / GetNamespace / GetType on
// ObjectRef + the three subtypes. LocatableRef shadows ObjectRef.ID
// with a typed UIDBasedID; the interface method returns the embedded
// OBJECT_REF parent's ID per the contract — subtype-specific id type
// is reached via type assertion.
func TestObjectRefLikeAccessors(t *testing.T) {
	base := rm.ObjectRef{
		ID:        rm.GenericID{Value: "abc-123"},
		Namespace: "local",
		Type:      "PARTY",
	}

	for _, tc := range []struct {
		name string
		v    rm.ObjectRefLike
	}{
		{"ObjectRef", base},
		{"AccessGroupRef", rm.AccessGroupRef{ObjectRef: base}},
		{"PartyRef", rm.PartyRef{ObjectRef: base}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if id, ok := tc.v.GetID().(rm.GenericID); !ok || id.Value != "abc-123" {
				t.Errorf("GetID = %#v, want GenericID{Value: abc-123}", tc.v.GetID())
			}
			if tc.v.GetNamespace() != "local" {
				t.Errorf("GetNamespace = %q", tc.v.GetNamespace())
			}
			if tc.v.GetType() != "PARTY" {
				t.Errorf("GetType = %q", tc.v.GetType())
			}
		})
	}
}

// TestLocatableRefGetIDFromShadow guards against the post-decode gap
// the v0.x.y reviewer flagged. The generated UnmarshalJSON populates
// LocatableRef's shadow `ID UIDBasedID` field, NOT the embedded
// `ObjectRef.ID`. GetID() MUST surface the shadow value, lifted to
// ObjectID. A regression here would silently return nil on every
// LOCATABLE_REF decoded from JSON.
func TestLocatableRefGetIDFromShadow(t *testing.T) {
	const body = `{"_type":"LOCATABLE_REF","namespace":"local","type":"VERSIONED_COMPOSITION","id":{"_type":"HIER_OBJECT_ID","value":"1234"},"path":"/data"}`
	var l rm.LocatableRef
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("Unmarshal LOCATABLE_REF: %v", err)
	}
	if l.ID == nil {
		t.Fatalf("shadow LocatableRef.ID is nil after decode — wire fixture broken?")
	}
	// Sanity: the embedded parent's ID is left zero on the generated
	// decode path; this is the bug surface GetID has to paper over.
	if l.ObjectRef.ID != nil {
		t.Errorf("ObjectRef.ID = %#v (expected nil; if this fires the generator changed and GetID can be simplified)", l.ObjectRef.ID)
	}
	got := l.GetID()
	if got == nil {
		t.Fatal("GetID returned nil after decode — regression of the post-decode shadow gap")
	}
	// Typereg ctors return pointers — the test asserts on *HierObjectID.
	if hid, ok := got.(*rm.HierObjectID); !ok || hid == nil || hid.Value != "1234" {
		t.Errorf("GetID = %#v, want *HierObjectID{Value: 1234}", got)
	}
	// Also exercise through the ObjectRefLike interface — same code
	// path, but documents that the accessor is reachable from the
	// generic polymorphic slot.
	var asLike rm.ObjectRefLike = l
	if asLike.GetID() == nil {
		t.Error("ObjectRefLike.GetID() returned nil after decode")
	}
}

// TestLocatableRefGetIDFallsBackToParent covers the hand-constructed
// path where a caller sets only ObjectRef.ID (no shadow). GetID should
// surface the parent value rather than returning nil.
func TestLocatableRefGetIDFallsBackToParent(t *testing.T) {
	l := rm.LocatableRef{
		ObjectRef: rm.ObjectRef{
			ID:        rm.GenericID{Value: "fallback"},
			Namespace: "local",
			Type:      "FOLDER",
		},
	}
	id, ok := l.GetID().(rm.GenericID)
	if !ok || id.Value != "fallback" {
		t.Errorf("GetID = %#v, want GenericID{Value: fallback} from embedded ObjectRef", l.GetID())
	}
}

// TestDVTextLikeGetDefiningCode pins the second Phase 1 accessor:
// (CodePhrase, true) when the concrete type is DVCodedText; (zero,
// false) when it is bare DVText.
func TestDVTextLikeGetDefiningCode(t *testing.T) {
	tests := []struct {
		name        string
		v           rm.DVTextLike
		wantCode    rm.CodePhrase
		wantPresent bool
	}{
		{
			name:        "DVText — no defining code",
			v:           rm.DVText{Value: "free text"},
			wantPresent: false,
		},
		{
			name:        "DVText pointer — no defining code",
			v:           &rm.DVText{Value: "free text"},
			wantPresent: false,
		},
		{
			name:        "DVCodedText — defining code present",
			v:           rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			wantCode:    rm.CodePhrase{CodeString: "at0001"},
			wantPresent: true,
		},
		{
			name:        "DVCodedText pointer — defining code present",
			v:           &rm.DVCodedText{DVText: rm.DVText{Value: "moderate"}, DefiningCode: rm.CodePhrase{CodeString: "at0001"}},
			wantCode:    rm.CodePhrase{CodeString: "at0001"},
			wantPresent: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, present := tc.v.GetDefiningCode()
			if present != tc.wantPresent {
				t.Errorf("present = %v, want %v", present, tc.wantPresent)
			}
			if code.CodeString != tc.wantCode.CodeString {
				t.Errorf("code.CodeString = %q, want %q", code.CodeString, tc.wantCode.CodeString)
			}
		})
	}
}
