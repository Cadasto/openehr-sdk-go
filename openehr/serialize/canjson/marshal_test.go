package canjson

import (
	"encoding/json"
	"strings"
	"testing"
)

// Phase 1 sanity tests for the public encode surface. These rely
// only on the codec orchestration layer — full RM-type round-trips
// live in roundtrip_test.go once the generator emits MarshalJSON on
// concrete RM types.

func TestMarshalDelegatesToEncodingJSON(t *testing.T) {
	type sample struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	got, err := Marshal(sample{A: 1, B: "x"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Verify the result is valid JSON.
	var back map[string]any
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("re-decode: %v (raw=%s)", err, got)
	}
	if back["a"].(float64) != 1 || back["b"].(string) != "x" {
		t.Errorf("round-trip mismatch: %v", back)
	}
}

func TestMarshalIndentEmitsIndent(t *testing.T) {
	type sample struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	out, err := MarshalIndent(sample{A: 1, B: 2}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !strings.Contains(string(out), "\n  \"a\"") {
		t.Errorf("MarshalIndent should add indent; got: %q", out)
	}
}
