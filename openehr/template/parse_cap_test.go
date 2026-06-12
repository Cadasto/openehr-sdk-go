package template

import (
	"errors"
	"strings"
	"testing"
)

func TestParseOPT_inputTooLarge(t *testing.T) {
	orig := maxOPTBytes
	maxOPTBytes = 64
	t.Cleanup(func() { maxOPTBytes = orig })

	// A syntactically-started XML document that exceeds the 64-byte cap.
	// The padding is enough to push it well over the limit.
	xml := "<template>" + strings.Repeat("x", 100) + "</template>"
	_, err := ParseOPT(strings.NewReader(xml))
	if err == nil {
		t.Fatal("expected error when input exceeds maxOPTBytes")
	}
	if !errors.Is(err, ErrInvalidOPT) {
		t.Errorf("error should wrap ErrInvalidOPT, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}

// TestParseOPT_capBoundary locks the off-by-one at the cap edge: a valid
// document of exactly maxOPTBytes bytes must parse, while the same
// document one byte over the cap must be rejected as too large. The
// earlier cappedReader implementation let an exactly-cap+1 document slip
// through because its "exceeded" flag lagged one Read behind.
func TestParseOPT_capBoundary(t *testing.T) {
	const valid = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>cap</value></template_id>
  <concept>cap</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
  </definition>
</template>`
	orig := maxOPTBytes
	t.Cleanup(func() { maxOPTBytes = orig })

	// Exactly at the cap: must parse cleanly (no false positive).
	maxOPTBytes = int64(len(valid))
	if _, err := ParseOPT(strings.NewReader(valid)); err != nil {
		t.Fatalf("doc of exactly maxOPTBytes bytes should parse, got: %v", err)
	}

	// One byte over the cap: must be rejected as too large.
	maxOPTBytes = int64(len(valid)) - 1
	_, err := ParseOPT(strings.NewReader(valid))
	if err == nil {
		t.Fatal("doc of maxOPTBytes+1 bytes must be rejected")
	}
	if !errors.Is(err, ErrInvalidOPT) || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("want ErrInvalidOPT 'exceeds', got: %v", err)
	}
}
