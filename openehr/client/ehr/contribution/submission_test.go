package contribution_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// fakeNonVersion satisfies json.Marshaler + BMMName() but is not a
// member of the closed CommitVersion set — Validate must reject it.
type fakeNonVersion struct{}

func (fakeNonVersion) MarshalJSON() ([]byte, error) { return []byte(`{"_type":"WRONG"}`), nil }
func (fakeNonVersion) BMMName() string              { return "WRONG_TYPE" }

// newImportedVersion builds a minimal IMPORTED_VERSION<COMPOSITION> for
// the closed-set tests. ImportedVersion wraps an OriginalVersion under
// `Item`; both halves carry the spec-mandated discriminators.
func newImportedVersion() *rm.ImportedVersion[rm.Composition] {
	inner := newOriginalVersion()
	innerAny := rm.OriginalVersion[any]{
		Version:        rm.Version[any]{CommitAudit: inner.CommitAudit},
		UID:            inner.UID,
		LifecycleState: inner.LifecycleState,
	}
	return &rm.ImportedVersion[rm.Composition]{
		Version: rm.Version[rm.Composition]{CommitAudit: inner.CommitAudit},
		Item:    innerAny,
	}
}

// newOriginalVersionFolder / newOriginalVersionEHRAccess cover the two
// remaining versionable T's in the closed set so the type-switch is
// exercised for every documented case.
func newOriginalVersionFolder() *rm.OriginalVersion[rm.Folder] {
	folder := rm.Folder{Name: rm.DVText{Value: "Encounters"}}
	return &rm.OriginalVersion[rm.Folder]{
		Version:        rm.Version[rm.Folder]{CommitAudit: *newAudit()},
		UID:            rm.ObjectVersionID{Value: "3::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &folder,
	}
}

func newOriginalVersionEHRAccess() *rm.OriginalVersion[rm.EHRAccess] {
	access := rm.EHRAccess{}
	return &rm.OriginalVersion[rm.EHRAccess]{
		Version:        rm.Version[rm.EHRAccess]{CommitAudit: *newAudit()},
		UID:            rm.ObjectVersionID{Value: "4::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &access,
	}
}

func TestSubmissionValidate(t *testing.T) {
	tests := []struct {
		name    string
		sub     *contribution.Submission
		wantErr string
	}{
		{
			name: "ORIGINAL_VERSION<Composition> ok",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{newOriginalVersion()},
			},
		},
		{
			name: "ORIGINAL_VERSION<Folder> ok",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{newOriginalVersionFolder()},
			},
		},
		{
			name: "ORIGINAL_VERSION<EHRAccess> ok",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{newOriginalVersionEHRAccess()},
			},
		},
		{
			name: "IMPORTED_VERSION<Composition> ok",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{newImportedVersion()},
			},
		},
		{
			name:    "rejects empty Versions",
			sub:     &contribution.Submission{},
			wantErr: "empty",
		},
		{
			name: "rejects nil element",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{nil},
			},
			wantErr: "is nil",
		},
		{
			name: "rejects non-version type (wrong wrapper)",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{fakeNonVersion{}},
			},
			wantErr: `BMMName="WRONG_TYPE"`,
		},
		{
			name: "rejects OriginalVersion[T] with non-versionable T",
			sub: &contribution.Submission{
				// rm.PartyIdentified is NOT in the spec's
				// Contribution_create closed set — the wrapper's
				// BMMName is ORIGINAL_VERSION but the concrete
				// generic instantiation is rejected.
				Versions: []contribution.CommitVersion{
					&rm.OriginalVersion[rm.PartyIdentified]{
						Version:        rm.Version[rm.PartyIdentified]{CommitAudit: *newAudit()},
						UID:            rm.ObjectVersionID{Value: "x::cdr.example::1"},
						LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
					},
				},
			},
			wantErr: "OriginalVersion[",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sub.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v; want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestSubmissionMarshalJSONCanonical(t *testing.T) {
	sub := &contribution.Submission{
		Audit:    *newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	b, err := canjson.Marshal(sub)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, b)
	}
	if _, hasType := got["_type"]; hasType {
		t.Errorf("Submission must not emit top-level _type (Contribution_create has no class envelope): %s", b)
	}
	if _, hasAudit := got["audit"]; !hasAudit {
		t.Errorf("missing top-level audit: %s", b)
	}
	if _, hasVersions := got["versions"]; !hasVersions {
		t.Errorf("missing top-level versions: %s", b)
	}
}

func TestSubmissionMarshalJSONRejectsNonVersion(t *testing.T) {
	sub := &contribution.Submission{
		Versions: []contribution.CommitVersion{fakeNonVersion{}},
	}
	if _, err := canjson.Marshal(sub); err == nil {
		t.Error("expected validation error from MarshalJSON, got nil")
	}
}

// TestSubmissionMixesVersionable proves heterogeneous T's coexist —
// ORIGINAL_VERSION<COMPOSITION> + ORIGINAL_VERSION<EHR_STATUS> in the
// same submission round-trip without losing either discriminator.
func TestSubmissionMixesVersionable(t *testing.T) {
	status := rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		IsQueryable:     true,
		IsModifiable:    true,
	}
	statusVer := &rm.OriginalVersion[rm.EHRStatus]{
		Version:        rm.Version[rm.EHRStatus]{CommitAudit: *newAudit()},
		UID:            rm.ObjectVersionID{Value: "2::cdr.example::1"},
		LifecycleState: rm.DVCodedText{DVText: rm.DVText{Value: "complete"}, DefiningCode: rm.CodePhrase{CodeString: "532"}},
		Data:           &status,
	}
	sub := &contribution.Submission{
		Audit:    *newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion(), statusVer},
	}
	b, err := canjson.Marshal(sub)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	body := string(b)
	if !strings.Contains(body, `"_type":"COMPOSITION"`) {
		t.Errorf("COMPOSITION payload missing: %s", body)
	}
	if !strings.Contains(body, `"_type":"EHR_STATUS"`) {
		t.Errorf("EHR_STATUS payload missing: %s", body)
	}
}
