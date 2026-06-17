package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

func TestSlotAssertion_MatchesArchetypeID(t *testing.T) {
	a, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.device(-[a-zA-Z0-9_]+)*\.v1`)
	if err != nil {
		t.Fatalf("NewSlotAssertion: %v", err)
	}
	cases := []struct {
		id   string
		want bool
	}{
		{"openEHR-EHR-CLUSTER.device.v1", true},
		{"openEHR-EHR-CLUSTER.device-foo.v1", true},
		{"openEHR-EHR-OBSERVATION.blood_pressure.v1", false},
	}
	for _, tc := range cases {
		if got := a.MatchesArchetypeID(tc.id); got != tc.want {
			t.Errorf("MatchesArchetypeID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestSlotRules_AllowsArchetypeID(t *testing.T) {
	inc, err := constraints.NewSlotAssertion(`openEHR-EHR-OBSERVATION\.body_weight\..*`)
	if err != nil {
		t.Fatal(err)
	}
	rules := constraints.SlotRules{
		RMTypeName: "OBSERVATION",
		Includes:   []constraints.SlotAssertion{inc},
	}
	if !rules.AllowsArchetypeID("openEHR-EHR-OBSERVATION.body_weight.v2") {
		t.Error("expected include match")
	}
	if rules.AllowsArchetypeID("openEHR-EHR-OBSERVATION.heart_rate.v1") {
		t.Error("expected include miss")
	}
}

func TestSlotRules_PrefixFallback(t *testing.T) {
	rules := constraints.SlotRules{RMTypeName: "SECTION"}
	if !rules.AllowsArchetypeID("openEHR-EHR-SECTION.example.v1") {
		t.Error("expected prefix fallback")
	}
	if rules.HasParsedIncludes() {
		t.Error("expected no parsed includes")
	}
}

func TestSlotRules_ExcludeWins(t *testing.T) {
	ex, err := constraints.NewSlotAssertion(`openEHR-EHR-OBSERVATION\.body_weight\..*`)
	if err != nil {
		t.Fatal(err)
	}
	rules := constraints.SlotRules{
		RMTypeName: "OBSERVATION",
		Excludes:   []constraints.SlotAssertion{ex},
	}
	if rules.AllowsArchetypeID("openEHR-EHR-OBSERVATION.body_weight.v1") {
		t.Error("exclude should reject")
	}
}
