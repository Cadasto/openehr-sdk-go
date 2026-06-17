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

// Assertions match the whole archetype id, not a substring: a `…\.v1`
// pattern must reject both a longer version suffix (v10) and any
// leading/trailing garbage.
func TestSlotAssertion_AnchoredWholeString(t *testing.T) {
	a, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.device\.v1`)
	if err != nil {
		t.Fatalf("NewSlotAssertion: %v", err)
	}
	cases := []struct {
		id   string
		want bool
	}{
		{"openEHR-EHR-CLUSTER.device.v1", true},
		{"openEHR-EHR-CLUSTER.device.v10", false},
		{"openEHR-EHR-CLUSTER.device.v1x", false},
		{"xopenEHR-EHR-CLUSTER.device.v1", false},
		{"prefix openEHR-EHR-CLUSTER.device.v1 suffix", false},
	}
	for _, tc := range cases {
		if got := a.MatchesArchetypeID(tc.id); got != tc.want {
			t.Errorf("MatchesArchetypeID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// A catch-all `.*` exclude is the editor's auto-generated complement
// of an includes list; it must not reject the slot's own includes,
// but a non-catch-all exclude still narrows the set.
func TestSlotRules_CatchAllExcludeIgnoredWithIncludes(t *testing.T) {
	inc, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.anatomical_location\.v1|openEHR-EHR-CLUSTER\.device\.v1`)
	if err != nil {
		t.Fatal(err)
	}
	catchAll, err := constraints.NewSlotAssertion(`.*`)
	if err != nil {
		t.Fatal(err)
	}
	rules := constraints.SlotRules{
		RMTypeName: "CLUSTER",
		Includes:   []constraints.SlotAssertion{inc},
		Excludes:   []constraints.SlotAssertion{catchAll},
	}
	if !rules.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("catch-all exclude should not reject an included id")
	}
	if rules.AllowsArchetypeID("openEHR-EHR-CLUSTER.symptom.v1") {
		t.Error("non-included id should still be rejected")
	}

	// Without includes, a catch-all exclude genuinely excludes all.
	bare := constraints.SlotRules{
		RMTypeName: "CLUSTER",
		Excludes:   []constraints.SlotAssertion{catchAll},
	}
	if bare.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("catch-all exclude with no includes should reject everything")
	}

	// A specific exclude still narrows an includes list.
	specificEx, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.device\.v1`)
	if err != nil {
		t.Fatal(err)
	}
	narrowed := constraints.SlotRules{
		RMTypeName: "CLUSTER",
		Includes:   []constraints.SlotAssertion{inc},
		Excludes:   []constraints.SlotAssertion{specificEx},
	}
	if narrowed.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("specific exclude should reject the excluded id even when included")
	}
	if !narrowed.AllowsArchetypeID("openEHR-EHR-CLUSTER.anatomical_location.v1") {
		t.Error("specific exclude should not affect other included ids")
	}
}

func TestSlotRules_IncludesDroppedUnparsed(t *testing.T) {
	// RawIncludeCount > 0 but no compiled includes → fail-open to prefix.
	rules := constraints.SlotRules{
		RMTypeName:      "CLUSTER",
		RawIncludeCount: 2,
	}
	if !rules.IncludesDroppedUnparsed() {
		t.Error("expected IncludesDroppedUnparsed when raw includes existed but none compiled")
	}
	if !rules.AllowsArchetypeID("openEHR-EHR-CLUSTER.device.v1") {
		t.Error("fail-open: prefix fallback should allow a matching RM-type id")
	}

	inc, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.device\.v1`)
	if err != nil {
		t.Fatal(err)
	}
	ok := constraints.SlotRules{
		RMTypeName:      "CLUSTER",
		Includes:        []constraints.SlotAssertion{inc},
		RawIncludeCount: 1,
	}
	if ok.IncludesDroppedUnparsed() {
		t.Error("did not expect IncludesDroppedUnparsed when an include compiled")
	}
}

func TestSlotRules_ExampleArchetypeID(t *testing.T) {
	// Synthesisable alternation → a concrete id matching the include.
	inc, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\.device\.v1|openEHR-EHR-CLUSTER\.anatomical_location\.v1`)
	if err != nil {
		t.Fatal(err)
	}
	rules := constraints.SlotRules{RMTypeName: "CLUSTER", Includes: []constraints.SlotAssertion{inc}}
	got := rules.ExampleArchetypeID()
	if !inc.MatchesArchetypeID(got) {
		t.Errorf("ExampleArchetypeID()=%q does not satisfy its own include", got)
	}

	// Unsynthesisable pattern (unbounded wildcard) → bail out to the
	// RM-type-prefix example rather than emit a non-conforming id.
	wild, err := constraints.NewSlotAssertion(`openEHR-EHR-CLUSTER\..*`)
	if err != nil {
		t.Fatal(err)
	}
	wildRules := constraints.SlotRules{RMTypeName: "CLUSTER", Includes: []constraints.SlotAssertion{wild}}
	if got := wildRules.ExampleArchetypeID(); got != "openEHR-EHR-CLUSTER.example.v1" {
		t.Errorf("ExampleArchetypeID()=%q, want prefix-fallback example", got)
	}
}
