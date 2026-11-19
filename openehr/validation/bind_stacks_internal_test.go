package validation

// bind_stacks_internal_test.go: the engine binds the items of a list with
// runs it pushes onto stacks, as it reads a list's items onto its items
// stack (items_stack_internal_test.go): the child each item binds to,
// whether it binds by its archetype_node_id, whether the template admits it,
// and, for a list whose items it rebinds by trial, the items bound to each
// child so far. A run is zeroed when pushed and capped at its own end, and
// the visit that pushed it drops it on every path it returns by. A run held
// while a trial below grows its stack stays on the old array, where its
// holder writes the binding the trial decided and reads it back.

import (
	"slices"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// A run is zeroed over what a dropped run left in the stack's array, and
// ends at its own last value, so an append to it cannot write into a run
// pushed above it.
func TestPushRun_ZeroedAndCapped(t *testing.T) {
	t.Parallel()
	var stack []int
	outer := pushRun(&stack, 2)
	if len(outer) != 2 || cap(outer) != 2 {
		t.Fatalf("pushRun(empty stack, 2): len %d, cap %d, want 2, 2", len(outer), cap(outer))
	}
	outer[0], outer[1] = 7, 8
	inner := pushRun(&stack, 1)
	appended := append(outer, 9)
	if inner[0] != 0 {
		t.Errorf("appending %d to the outer run wrote into the inner one: it holds %d, want 0", appended[2], inner[0])
	}
	stack = stack[:0]
	if again := pushRun(&stack, 3); !slices.Equal(again, []int{0, 0, 0}) {
		t.Errorf("pushRun(stack of dropped runs 7 8 0, 3) = %v, want [0 0 0]", again)
	}
}

// bindStackLens returns the length of each of the engine's binding stacks.
func bindStackLens(e *engine) []int {
	return []int{len(e.bindings), len(e.byNodeID), len(e.admitted), len(e.counts)}
}

// REQ-171 One walk — a walk drops every run it pushed onto the binding
// stacks, so it ends with them empty, whether it ends at the end of the
// composition or the visit limit ends it inside the trials of a list item,
// where each list it is inside returns early. The chain of CLUSTERs offers
// each item two children of one rank, so each level's item is bound by
// trial.
func TestEngine_REQ171_BindStacksDroppedOnEveryReturn(t *testing.T) {
	t.Parallel()
	const depth = 4
	c := compileOPTText(t, clusterChainOPT(depth, false))
	for _, tc := range []struct {
		name    string
		lim     Limits
		stopped bool
	}{
		{name: "the walk ends", lim: Limits{}},
		{name: "the visit limit ends the walk inside a trial", lim: Limits{Nodes: 20}, stopped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, e := engineWalk(clusterChain(depth, false), c, tc.lim)
			if got := slices.ContainsFunc(r.Issues, func(iss Issue) bool { return iss.Code == "max_nodes" }); got != tc.stopped {
				t.Fatalf("precondition: a chain %d levels deep within %+v reports max_nodes %t, want %t; issues %q", depth, tc.lim, got, tc.stopped, keys(r.Issues))
			}
			if got := bindStackLens(e); slices.ContainsFunc(got, func(n int) bool { return n != 0 }) {
				t.Errorf("binding stacks hold %v values after the walk (bindings, byNodeID, admitted, counts), want none", got)
			}
		})
	}
}

// itemsHolding renders an items attribute holding an ELEMENT child of the
// node id occurring at most once.
func itemsHolding(nodeID string) string {
	return `
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>items</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>ELEMENT</rm_type_name>
            <occurrences>` + existence01 + `</occurrences>
            <node_id>` + nodeID + `</node_id>
          </children>
        </attributes>`
}

// REQ-102 Structural — binding items — the root's one item is tried under
// two CLUSTER at0001 children of one rank: under the first, whose items
// admit an ELEMENT at0003 alone, its ELEMENT at0002 binds to nothing, so it
// binds to the second. The root's binding run fills its stack, so the
// trial's own list grows the stack while bindRanked holds that run: the
// binding it writes after the trial is the one the walk reads, and the
// item, walked under the second child, reports nothing.
func TestEngine_REQ102_BindStacksGrowthLeavesTheHeldRunOnItsArray(t *testing.T) {
	t.Parallel()
	root := clusterChain(1, false)
	if r, _ := engineWalk(root, compileOPTText(t, clusterChainOPTWith(chainCluster(itemsHolding("at0003")))), Limits{}); len(r.Issues) == 0 {
		t.Fatal("precondition: the item validates under the first child alone, want it refused there")
	}
	c := compileOPTText(t, clusterChainOPTWith(chainCluster(itemsHolding("at0003"))+chainCluster(itemsHolding("at0002"))))
	e := newEngine(Limits{})
	e.bindings = make([]*tcimpl.CompiledNode, 0, len(root.Items))
	e.walk(engineNode{value: root, rmType: "CLUSTER", bound: c.Root()})
	if cap(e.bindings) <= len(root.Items) {
		t.Fatalf("precondition: the bindings stack kept its capacity %d, want it grown by the trial", cap(e.bindings))
	}
	if got := keys(e.result().Issues); len(got) != 0 {
		t.Errorf("issues %q, want none: the item binds to the second child", got)
	}
}

// clusterChainOPTWith returns clusterChainOPT's template with children as
// the root's items.
func clusterChainOPTWith(children string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>cluster-chain</value></template_id>
  <concept>cluster-chain</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>CLUSTER</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>items</rm_attribute_name>` + children + `
    </attributes>
    <archetype_id><value>openEHR-EHR-CLUSTER.chain.v1</value></archetype_id>
  </definition>
</template>`
}
