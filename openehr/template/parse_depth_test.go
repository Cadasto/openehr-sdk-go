package template

// Tests for OPT node-tree depth limiting (Task 9 — security hardening).
//
// These tests are in package template (internal) so they can manipulate
// maxOPTDepth directly, following the same pattern as parse_cap_test.go.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// nestedOPTXML builds an OPT XML document with `levels` of nesting around
// a leaf. Each level adds one C_MULTIPLE_ATTRIBUTE → C_COMPLEX_OBJECT hop.
// levels=0 produces a flat document with only the root definition.
func nestedOPTXML(levels int) string {
	const envelope = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>depth-test</value></template_id>
  <concept>depth-test</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    %s
  </definition>
</template>`

	// Build the nested structure from the inside out.
	inner := "" // leaf: no attributes
	for i := range levels {
		inner = fmt.Sprintf(`<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>x%d</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>X</rm_type_name>
        <node_id>at0000</node_id>
        %s
      </children>
    </attributes>`, i, inner)
	}
	return fmt.Sprintf(envelope, inner)
}

// TestBuildNode_DepthExceeded verifies that ParseOPT returns an error
// wrapping ErrInvalidOPT when the node-tree nesting depth exceeds maxOPTDepth.
func TestBuildNode_DepthExceeded(t *testing.T) {
	orig := maxOPTDepth
	maxOPTDepth = 4
	t.Cleanup(func() { maxOPTDepth = orig })

	// maxOPTDepth+1 is the first failing depth (guard fires at
	// depth > maxOPTDepth), pinning the exact fence.
	xml := nestedOPTXML(maxOPTDepth + 1)
	_, err := ParseOPT(strings.NewReader(xml))
	if err == nil {
		t.Fatal("expected error for OPT nesting that exceeds maxOPTDepth, got nil")
	}
	if !errors.Is(err, ErrInvalidOPT) {
		t.Errorf("error should wrap ErrInvalidOPT, got: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "nesting") && !strings.Contains(msg, "exceeds") {
		t.Errorf("error message should mention 'nesting' or 'exceeds', got: %q", msg)
	}
}

// TestBuildNode_DepthAtCap verifies that ParseOPT succeeds when nesting is at
// or below the cap — no false positives.
func TestBuildNode_DepthAtCap(t *testing.T) {
	orig := maxOPTDepth
	maxOPTDepth = 4
	t.Cleanup(func() { maxOPTDepth = orig })

	// depth=0: root node only — depth counter starts at 0 and never
	// crosses the guard (which fires at depth > maxOPTDepth, i.e. > 4).
	xml := nestedOPTXML(0)
	if _, err := ParseOPT(strings.NewReader(xml)); err != nil {
		t.Fatalf("flat document (0 levels) should parse cleanly, got: %v", err)
	}

	// depth=maxOPTDepth-1: one level under the cap — must also be accepted.
	xmlUnder := nestedOPTXML(maxOPTDepth - 1)
	if _, err := ParseOPT(strings.NewReader(xmlUnder)); err != nil {
		t.Fatalf("document at maxOPTDepth-1 (%d) levels should parse cleanly, got: %v", maxOPTDepth-1, err)
	}

	// depth=maxOPTDepth exactly: the deepest buildNode sees depth ==
	// maxOPTDepth, and the guard fires only on depth > maxOPTDepth, so
	// this must parse — pins the off-by-one fence.
	xmlAtCap := nestedOPTXML(maxOPTDepth)
	if _, err := ParseOPT(strings.NewReader(xmlAtCap)); err != nil {
		t.Fatalf("document at exactly maxOPTDepth (%d) levels should parse cleanly, got: %v", maxOPTDepth, err)
	}
}

// TestWalkPath_DepthGuard verifies the defensive depth guard inside
// walkPath fires when descending a node tree deeper than maxOPTDepth.
// parseOPT already caps build depth, so to exercise the guard in
// isolation we construct a tree manually (deeper than the cap) and walk
// a fully-matching path through it — the case a non-parseOPT tree (or a
// cyclic one) could otherwise hit. The guard must return an error
// wrapping ErrPathNotFound mentioning the depth limit.
func TestWalkPath_DepthGuard(t *testing.T) {
	orig := maxOPTDepth
	maxOPTDepth = 4
	t.Cleanup(func() { maxOPTDepth = orig })

	// Build a chain of ComplexObjects maxOPTDepth+2 levels deep, each
	// linked by a single-cardinality attribute "aN" -> child.
	levels := maxOPTDepth + 2
	node := Node(&ComplexObject{rmTypeName: "X", nodeID: "at0000"})
	segs := make([]pathSegment, 0, levels)
	for i := range levels {
		name := fmt.Sprintf("a%d", i)
		node = &ComplexObject{
			rmTypeName: "X",
			nodeID:     "at0000",
			attributes: []*Attribute{{
				name:        name,
				cardinality: Single,
				children:    []Node{node},
			}},
		}
		// Prepend so the path reads root-to-leaf (a{levels-1}/.../a0).
		segs = append([]pathSegment{{name: name}}, segs...)
	}

	_, err := walkPath(node, segs, &resolveOpts{}, 0)
	if err == nil {
		t.Fatal("expected depth-guard error walking a tree deeper than maxOPTDepth, got nil")
	}
	if !errors.Is(err, ErrPathNotFound) {
		t.Errorf("error should wrap ErrPathNotFound, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nesting") && !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention the depth limit, got: %q", err.Error())
	}
}
