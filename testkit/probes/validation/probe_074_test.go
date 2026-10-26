package validationprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// PROBE-074 — template-driven validation extends beyond COMPOSITION to
// the demographic PARTY hierarchy and the EHR-IM roots. The issue-code
// multiset per (OPT, root) shape is stable across conformant
// implementations (REQ-102).
func TestProbe074NonCompositionValidate(t *testing.T) {
	addr, err := probes.AddressFixture()
	if err != nil {
		t.Fatal(err)
	}
	addrOPT, err := os.ReadFile(fixtures.TemplateOpt("Address.v2"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []probes.RootCase{
		{
			// Real ADDRESS OPT + conformant instance validates clean.
			Name:      "address_fixture_clean",
			OPT:       addrOPT,
			Root:      addr,
			WantCodes: nil,
		},
		{
			// PERSON root under an ORGANISATION-rooted OPT → rm_type_mismatch.
			Name:      "person_under_organisation_opt",
			OPT:       []byte(probes.OrganisationOPT),
			Root:      probes.PersonUnderOrganisationTemplate(),
			WantCodes: []string{"rm_type_mismatch"},
		},
		{
			// PERSON whose identities is present and empty, under an OPT
			// pinning identities >= 1.
			Name: "person_empty_identities",
			OPT:  []byte(probes.PersonIdentitiesOPT),
			Root: probes.PersonEmptyIdentities(),
			// existence lower >= 1 with cardinality lower >= 1 → required
			// (an existence alone asks for no member); cardinality
			// interval lower >= 1 with zero children → cardinality (both
			// clauses independent, mirroring PROBE-026 empty-events).
			WantCodes: []string{"required", "cardinality"},
		},
		{
			// PERSON with no identities list at all under the same OPT.
			Name: "person_absent_identities",
			OPT:  []byte(probes.PersonIdentitiesOPT),
			Root: probes.PersonAbsentIdentities(),
			// identities is absent (REQ-171 § Structural access): existence
			// lower >= 1 → required, which the RM floor's required for the
			// RM-mandatory list repeats at the same path, so it is carried
			// once. An absent list has no item count to check, so no
			// cardinality (mirroring PROBE-026 absent-events).
			WantCodes: []string{"required"},
		},
		{
			// FOLDER whose archetype id differs from the OPT pin.
			Name:      "folder_archetype_id_mismatch",
			OPT:       []byte(probes.FolderOPT),
			Root:      probes.FolderOtherArchetype(),
			WantCodes: []string{"archetype_id_mismatch"},
		},
		{
			// Minimal EHR_STATUS with all BMM-mandatory channels present.
			Name:      "ehr_status_clean",
			OPT:       []byte(probes.EHRStatusOPT),
			Root:      probes.EHRStatusClean(),
			WantCodes: nil,
		},
	}

	r, err := probes.Probe074NonCompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe074: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe074 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-074" {
		t.Errorf("Probe id = %q, want PROBE-074", r.Probe)
	}
}
