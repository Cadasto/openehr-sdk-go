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

// inputSig is the comparable signature of a node's inputs: per input the
// "suffix:type" pair extended with the deep contents PROBE-075 pins —
// ordinal/coded list entries (value@ordinal), listOpen, terminology,
// temporal validation patterns, and numeric validation ranges. List
// labels and the remaining validation payloads (duration fields, quantity
// precision, per-unit ranges) stay outside the signature as documented
// deviations.
func inputSig(inputs []webtemplate.Input) string {
	parts := make([]string, 0, len(inputs))
	for _, in := range inputs {
		var p strings.Builder
		p.WriteString(in.Suffix + ":" + in.Type)
		for _, it := range in.List {
			p.WriteString("|" + it.Value)
			if it.Ordinal != nil {
				fmt.Fprintf(&p, "@%d", *it.Ordinal)
			}
		}
		p.WriteString(openSig(in.ListOpen) + termSig(in.Terminology))
		if v := in.Validation; v != nil && !isDurationField(in.Suffix) {
			p.WriteString(patternSig(v.Pattern) + rangeSig(v.Range))
		}
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ",")
}

// isDurationField reports whether an input suffix is a DV_DURATION
// component — their per-field ranges are a documented deviation
// (deviations.md) and stay outside the parity signature.
func isDurationField(suffix string) bool {
	switch suffix {
	case "year", "month", "week", "day", "hour", "minute", "second":
		return true
	}
	return false
}

func refInputSig(m map[string]any) string {
	raw, ok := m["inputs"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, entry := range raw {
		im, _ := entry.(map[string]any)
		suffix, _ := im["suffix"].(string)
		typ, _ := im["type"].(string)
		var p strings.Builder
		p.WriteString(suffix + ":" + typ)
		if list, ok := im["list"].([]any); ok {
			for _, e := range list {
				em, _ := e.(map[string]any)
				val, _ := em["value"].(string)
				p.WriteString("|" + val)
				if ord, ok := em["ordinal"].(float64); ok {
					fmt.Fprintf(&p, "@%d", int(ord))
				}
			}
		}
		open, _ := im["listOpen"].(bool)
		term, _ := im["terminology"].(string)
		p.WriteString(openSig(open) + termSig(term))
		if v, ok := im["validation"].(map[string]any); ok && !isDurationField(suffix) {
			pat, _ := v["pattern"].(string)
			p.WriteString(patternSig(pat))
			if rng, ok := v["range"].(map[string]any); ok {
				r := &webtemplate.Range{}
				if f, ok := rng["min"].(float64); ok {
					r.Min = &f
				}
				if f, ok := rng["max"].(float64); ok {
					r.Max = &f
				}
				r.MinOp, _ = rng["minOp"].(string)
				r.MaxOp, _ = rng["maxOp"].(string)
				p.WriteString(rangeSig(r))
			}
		}
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ",")
}

func openSig(open bool) string {
	if open {
		return "!open"
	}
	return ""
}

func termSig(terminology string) string {
	if terminology == "" {
		return ""
	}
	return "%" + terminology
}

func patternSig(pattern string) string {
	if pattern == "" {
		return ""
	}
	return "~" + pattern
}

func rangeSig(r *webtemplate.Range) string {
	if r == nil {
		return ""
	}
	sig := "#"
	if r.Min != nil {
		sig += fmt.Sprintf("%s%g", r.MinOp, *r.Min)
	}
	if r.Max != nil {
		sig += fmt.Sprintf("%s%g", r.MaxOp, *r.Max)
	}
	return sig
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
