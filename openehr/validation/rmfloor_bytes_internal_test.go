package validation

// rmfloor_bytes_internal_test.go: REQ-171 § Coverage, REQ-112 — the
// presence-aware EHR_STATUS entry replaces the value walk's findings at a
// path whose key is absent, and keeps the walk conditions there, so a
// max_nodes or max_depth issue still matches its path in Coverage.Stopped.

import (
	"slices"
	"testing"
)

// REQ-171 — replaceAtPaths drops the value walk's findings at the path of a
// replacement and keeps every walk condition there, whatever the path:
// max_nodes, max_depth and cyclic_value. Issues at other paths stay, and the
// replacements follow, in order.
func TestReplaceAtPaths_REQ171_KeepsWalkConditions(t *testing.T) {
	t.Parallel()
	issues := []Issue{
		{Path: "/name", Code: "required", Layer: LayerRM, Rule: ruleMandatory},
		{Path: "/subject", Code: "rm_invariant", Layer: LayerRM, Rule: rulePartyBasicValidity},
		{Path: "/subject", Code: "max_nodes"},
		{Path: "/archetype_details/rm_version", Code: "rm_version_valid", Layer: LayerRM, Rule: ruleRMVersionValid},
		{Path: "/archetype_details/rm_version", Code: "max_depth"},
		{Path: "/archetype_details/archetype_id", Code: "cyclic_value"},
	}
	replacements := []Issue{
		{Path: "/subject", Code: "required", Layer: LayerRM, Rule: ruleMandatory},
		{Path: "/archetype_details/rm_version", Code: "required", Layer: LayerRM, Rule: ruleMandatory},
		{Path: "/archetype_details/archetype_id", Code: "required", Layer: LayerRM, Rule: ruleMandatory},
	}
	want := []string{
		"required /name",
		"max_nodes /subject",
		"max_depth /archetype_details/rm_version",
		"cyclic_value /archetype_details/archetype_id",
		"required /subject",
		"required /archetype_details/rm_version",
		"required /archetype_details/archetype_id",
	}
	got := make([]string, 0, len(want))
	for _, i := range replaceAtPaths(issues, replacements) {
		got = append(got, i.Code+" "+i.Path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("replaceAtPaths = %q, want %q", got, want)
	}
}
