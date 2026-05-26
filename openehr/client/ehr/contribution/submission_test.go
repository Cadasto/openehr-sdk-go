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

func TestSubmissionValidate(t *testing.T) {
	tests := []struct {
		name    string
		sub     *contribution.Submission
		wantErr string
	}{
		{
			name: "ORIGINAL_VERSION ok",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{newOriginalVersion()},
			},
		},
		{
			name: "rejects nil element",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{nil},
			},
			wantErr: "is nil",
		},
		{
			name: "rejects non-version type",
			sub: &contribution.Submission{
				Versions: []contribution.CommitVersion{fakeNonVersion{}},
			},
			wantErr: `BMMName="WRONG_TYPE"`,
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
