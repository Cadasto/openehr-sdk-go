package datamap

import (
	"reflect"
	"testing"
)

// REQ-058 — Cluster runtime-name encoding accepts both short and expanded
// `_code` forms per [docs/specifications/datamap.md § Terminology binding].
// Both wire-shapes MUST produce the same canonical-JSON DV_CODED_TEXT.

func TestClusterName_NoCodeFallsBackToLabel(t *testing.T) {
	got := clusterName(map[string]any{}, "Result")
	want := map[string]any{"_type": "DV_TEXT", "value": "Result"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestClusterName_ShortForm_ExternalTerminology(t *testing.T) {
	got := clusterName(map[string]any{
		"_code": "SNOMED-CT::386725007",
		"_name": "Body temperature",
	}, "Result")
	want := map[string]any{
		"_type": "DV_CODED_TEXT",
		"value": "Body temperature",
		"defining_code": map[string]any{
			"_type":          "CODE_PHRASE",
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "SNOMED-CT"},
			"code_string":    "386725007",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestClusterName_ShortForm_LocalAtCode(t *testing.T) {
	got := clusterName(map[string]any{
		"_code": "at0001",
		"_name": "Local label",
	}, "Result")
	want := map[string]any{
		"_type": "DV_CODED_TEXT",
		"value": "Local label",
		"defining_code": map[string]any{
			"_type":          "CODE_PHRASE",
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
			"code_string":    "at0001",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestClusterName_ExpandedForm_ExternalTerminology(t *testing.T) {
	got := clusterName(map[string]any{
		"_code": map[string]any{
			"code":        "386725007",
			"value":       "Body temperature",
			"terminology": "SNOMED-CT",
		},
	}, "Result")
	want := map[string]any{
		"_type": "DV_CODED_TEXT",
		"value": "Body temperature",
		"defining_code": map[string]any{
			"_type":          "CODE_PHRASE",
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "SNOMED-CT"},
			"code_string":    "386725007",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestClusterName_ExpandedForm_DefaultsTerminologyToLocal(t *testing.T) {
	got := clusterName(map[string]any{
		"_code": map[string]any{
			"code":  "KREA",
			"value": "Kreatinine",
			// terminology omitted on purpose
		},
	}, "Result")
	wantDC := map[string]any{
		"_type":          "CODE_PHRASE",
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
		"code_string":    "KREA",
	}
	if !reflect.DeepEqual(got["defining_code"], wantDC) {
		t.Errorf("defining_code = %#v, want %#v", got["defining_code"], wantDC)
	}
	if got["value"] != "Kreatinine" {
		t.Errorf("value = %v, want Kreatinine", got["value"])
	}
}

// Short and expanded form MUST produce identical canonical-JSON DV_CODED_TEXT
// for the same logical content (PROBE-058b — interchange contract).
func TestClusterName_ShortAndExpanded_AreEquivalent(t *testing.T) {
	short := clusterName(map[string]any{
		"_code": "LOINC::1975-2",
		"_name": "Bilirubin",
	}, "Result")
	expanded := clusterName(map[string]any{
		"_code": map[string]any{
			"code":        "1975-2",
			"value":       "Bilirubin",
			"terminology": "LOINC",
		},
	}, "Result")
	if !reflect.DeepEqual(short, expanded) {
		t.Errorf("short and expanded diverge:\n  short:    %#v\n  expanded: %#v", short, expanded)
	}
}

func TestClusterName_ExpandedForm_OuterNameOverridesInnerValue(t *testing.T) {
	// _name (sibling) wins over the inner value when both are set, so callers
	// can override the display without touching the code-block.
	got := clusterName(map[string]any{
		"_code": map[string]any{
			"code":        "KREA",
			"value":       "Kreatinine (gemeten)",
			"terminology": "local",
		},
		"_name": "Kreatinine",
	}, "Result")
	if got["value"] != "Kreatinine" {
		t.Errorf("value = %v, want Kreatinine (sibling _name should win)", got["value"])
	}
}

func TestClusterName_ExpandedForm_MissingCodeFallsThrough(t *testing.T) {
	// Expanded form without a `code` key should be treated as no-code → DV_TEXT.
	got := clusterName(map[string]any{
		"_code": map[string]any{
			"value":       "Bilirubin",
			"terminology": "LOINC",
		},
	}, "Result")
	want := map[string]any{"_type": "DV_TEXT", "value": "Result"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestClusterName_EmptyStringCode_FallsThrough(t *testing.T) {
	got := clusterName(map[string]any{"_code": ""}, "Result")
	want := map[string]any{"_type": "DV_TEXT", "value": "Result"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// --- parseCodeField unit-tests ---

func TestParseCodeField_NilInput(t *testing.T) {
	if _, _, _, ok := parseCodeField(nil); ok {
		t.Error("nil should yield ok=false")
	}
}

func TestParseCodeField_BareString_DefaultsToLocal(t *testing.T) {
	term, code, display, ok := parseCodeField("KREA")
	if !ok || term != "local" || code != "KREA" || display != "" {
		t.Errorf("got (%q, %q, %q, %v), want (local, KREA, \"\", true)", term, code, display, ok)
	}
}

func TestParseCodeField_UnknownType_FallsThrough(t *testing.T) {
	if _, _, _, ok := parseCodeField(42); ok {
		t.Error("int should yield ok=false")
	}
}
