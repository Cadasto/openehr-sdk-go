package webtemplate_test

// REQ-106 / PROBE-075 — per-node input (suffix, type) parity against the
// vendored EHRbase reference.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// inputSig is the comparable signature of a node's inputs: the ordered
// "suffix:type" list. PROBE-075 pins suffix and type, not deeper contents.
func inputSig(inputs []webtemplate.Input) string {
	parts := make([]string, 0, len(inputs))
	for _, in := range inputs {
		parts = append(parts, in.Suffix+":"+in.Type)
	}
	return strings.Join(parts, ",")
}

func refInputSig(m map[string]any) string {
	raw, ok := m["inputs"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, r := range raw {
		im, _ := r.(map[string]any)
		suffix, _ := im["suffix"].(string)
		typ, _ := im["type"].(string)
		parts = append(parts, suffix+":"+typ)
	}
	return strings.Join(parts, ",")
}

func TestInputParity(t *testing.T) {
	refInputs := map[string]string{}
	walkRefTree(refTree(t, loadReference(t)), func(m map[string]any) {
		refInputs[refStr(m, "aqlPath")] = refInputSig(m)
	})

	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ourInputs := map[string]string{}
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		ourInputs[n.AQLPath] = inputSig(n.Inputs)
	})

	var mismatch []string
	matched := 0
	for p, rs := range refInputs {
		os, ok := ourInputs[p]
		if !ok {
			continue // node-set parity is covered by TestStructuralParity
		}
		if os == rs {
			matched++
			continue
		}
		mismatch = append(mismatch, fmt.Sprintf("%s\n    ref:  [%s]\n    ours: [%s]", p, rs, os))
	}
	sort.Strings(mismatch)

	t.Logf("INPUT PARITY: compared=%d matched=%d mismatch=%d", len(refInputs), matched, len(mismatch))
	if len(mismatch) > 0 {
		n := min(len(mismatch), 20)
		for _, m := range mismatch[:n] {
			t.Logf("  %s", m)
		}
		t.Errorf("input (suffix,type) parity not reached: %d mismatches", len(mismatch))
	}
}
