package validation

// binding_trial_cost_internal_test.go: REQ-171 One walk — what binding by
// trial costs the walk. A verdict, once decided for a value under a child,
// is reused wherever the walk meets that value under that child again, so
// the trials of nested lists whose items validate under the first child
// they are tried under do not compound.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// chainCluster renders a CLUSTER at0001 child occurring at most once that
// declares attrs.
func chainCluster(attrs string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>CLUSTER</rm_type_name>
        <occurrences>` + existence01 + `</occurrences>
        <node_id>at0001</node_id>` + attrs + `
      </children>`
}

// chainLevel renders the children of a level of clusterChainOPT: a CLUSTER
// at0001 whose items hold the level below, above the last level, and, unless
// single, a second CLUSTER at0001 that constrains nothing below it. Both
// occur at most once and pin no name, so they rank alike.
func chainLevel(level int, single bool) string {
	items := ""
	if level > 1 {
		items = `
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>items</rm_attribute_name>` + chainLevel(level-1, single) + `
        </attributes>`
	}
	children := chainCluster(items)
	if !single {
		children += chainCluster("")
	}
	return children
}

// clusterChainOPT returns an OPT rooted at a CLUSTER whose items hold depth
// levels of chainLevel, each the items of the first child of the one above.
func clusterChainOPT(depth int, single bool) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>cluster-chain</value></template_id>
  <concept>cluster-chain</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>CLUSTER</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>items</rm_attribute_name>` + chainLevel(depth, single) + `
    </attributes>
    <archetype_id><value>openEHR-EHR-CLUSTER.chain.v1</value></archetype_id>
  </definition>
</template>`
}

// clusterChain returns a CLUSTER of clusterChainOPT holding a chain of depth
// CLUSTER at0001 items, each the one item of the one before, the last of
// which holds an ELEMENT.
func clusterChain(depth int) *rm.Cluster {
	var item rm.Item = &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "element"}, Value: &rm.DVText{Value: "x"}}
	for range depth {
		item = &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "cluster"}, Items: []rm.Item{item}}
	}
	return &rm.Cluster{
		ArchetypeNodeID:  "openEHR-EHR-CLUSTER.chain.v1",
		Name:             rm.DVText{Value: "chain"},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.chain.v1"}, RMVersion: "1.1.0"},
		Items:            []rm.Item{item},
	}
}

// REQ-171 One walk — the trials of nested lists do not compound where each
// item validates under the first child it is tried under. Each level of a
// chain of CLUSTERs offers its item two children of one node id, the second
// a leaf, so the item is tried under the first, validates there, and is
// walked under it again once its path is known. Each verdict is decided
// once: that walk binds the items below by the verdicts their trials
// decided, and a trial, whose findings are withdrawn, takes the verdict of
// an item below in place of walking it again. So the visits grow by 8 a
// level, against 4 a level where each level offers the first child alone,
// and chains 17 and 32 levels deep validate within the default limits.
// Without the reuse the visits of the first doubled with each level, and a
// chain 16 levels deep stopped at the visit limit.
func TestEngine_REQ171_NestedTrialsDoNotCompound(t *testing.T) {
	tests := []struct {
		depth int
		// single and pair are the visits with one child a level and with
		// two.
		single, pair int
	}{
		{depth: 1, single: 18, pair: 28},
		{depth: 2, single: 22, pair: 36},
		{depth: 4, single: 30, pair: 52},
		{depth: 8, single: 46, pair: 84},
		{depth: 16, single: 78, pair: 148},
		{depth: 17, single: 82, pair: 156},
		{depth: 32, single: 142, pair: 276},
	}
	for _, tc := range tests {
		for _, mode := range []struct {
			name   string
			single bool
			want   int
		}{{name: "one child a level", single: true, want: tc.single}, {name: "two children a level", want: tc.pair}} {
			r, e := engineWalk(clusterChain(tc.depth), compileOPTText(t, clusterChainOPT(tc.depth, mode.single)), Limits{})
			if !r.OK || len(r.Issues) != 0 {
				t.Errorf("a chain %d levels deep, %s: OK %t, issues %q, want OK with none within the default limits", tc.depth, mode.name, r.OK, keys(r.Issues))
			}
			if e.visits != mode.want {
				t.Errorf("a chain %d levels deep, %s: the walk visits %d values, want %d", tc.depth, mode.name, e.visits, mode.want)
			}
		}
	}
}
