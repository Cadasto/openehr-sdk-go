package webtemplate_test

// REQ-106 / PROBE-075 — per-node input (suffix, type) parity against the
// vendored EHRbase reference.

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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
	for _, stem := range parityFixtures {
		t.Run(stem, func(t *testing.T) { assertInputParity(t, stem) })
	}
}

func assertInputParity(t *testing.T, stem string) {
	t.Helper()
	refInputs := map[string]string{}
	walkRefTree(refTree(t, loadReferenceStem(t, stem)), func(m map[string]any) {
		refInputs[refStr(m, "aqlPath")] = refInputSig(m)
	})

	c := compileFixture(t, referenceDir+"/"+stem+".opt")
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
	slices.Sort(mismatch)

	t.Logf("INPUT PARITY: compared=%d matched=%d mismatch=%d", len(refInputs), matched, len(mismatch))
	if len(mismatch) > 0 {
		n := min(len(mismatch), 20)
		for _, m := range mismatch[:n] {
			t.Logf("  %s", m)
		}
	}
	// The REQ-116 oracles carry known input deltas (see deviations.md
	// § Input-level). A constrained DV_TEXT's C_STRING list now reaches its
	// TEXT input (REQ-106, REQ-103 list_open), so what is left is one node
	// in each oracle whose input shape differs from the reference's.
	// Structural parity is exact for both, so the count is pinned rather
	// than waived — it must shrink deliberately, never grow silently.
	if want := inputDeviationCounts[stem]; len(mismatch) != want {
		t.Errorf("input (suffix,type) parity: %d mismatches, want %d documented (see deviations.md)", len(mismatch), want)
	}
}

// inputDeviationCounts is the pinned number of documented input-signature
// deltas per parity fixture.
var inputDeviationCounts = map[string]int{
	referenceStem:     0,
	"Corona_Anamnese": 1,
	"GECCO_Diagnose":  1,
}

// REQ-106, REQ-103 — a DV_TEXT whose value C_STRING carries a list emits
// that list on its TEXT input, each entry's label the list string, and
// listOpen read from the C_STRING list_open flag, with a list or without
// one. The vendored fixture sets no list_open, so its list is closed; the
// open cases add the element to the same C_STRING, the last one in place
// of its list.
func TestTextInput_REQ106_REQ103_CStringList(t *testing.T) {
	t.Parallel()
	const fixture = "../../../testkit/corpus/templates/Test_dv_text_list_constraint.v0.opt"
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	const (
		lastEntry  = "<list>OPQ</list>"
		allEntries = "<list>XYZ</list>\n    " + lastEntry
		openFlag   = "<list_open>true</list_open>"
	)
	for _, s := range []string{lastEntry, allEntries} {
		if bytes.Count(raw, []byte(s)) != 1 {
			t.Fatalf("fixture %s does not hold %q exactly once", fixture, s)
		}
	}
	open := bytes.Replace(raw, []byte(lastEntry), []byte(lastEntry+openFlag), 1)
	openNoList := bytes.Replace(raw, []byte(allEntries), []byte(openFlag), 1)
	list := []webtemplate.InputListItem{{Value: "XYZ", Label: "XYZ"}, {Value: "OPQ", Label: "OPQ"}}
	tests := []struct {
		name string
		opt  []byte
		want []webtemplate.Input
	}{
		{name: "closed", opt: raw, want: []webtemplate.Input{{Type: "TEXT", List: list}}},
		{name: "open", opt: open, want: []webtemplate.Input{{Type: "TEXT", List: list, ListOpen: true}}},
		{name: "open, no list", opt: openNoList, want: []webtemplate.Input{{Type: "TEXT", ListOpen: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opt, err := template.ParseOPT(bytes.NewReader(tc.opt))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			wt, err := webtemplate.Build(c)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			var got *webtemplate.Node
			walkOurTree(wt.Tree, func(n *webtemplate.Node) {
				if n.NodeID == "at0031" && n.RMType == "DV_TEXT" {
					got = n
				}
			})
			if got == nil {
				t.Fatal("no DV_TEXT leaf at0031 in the built tree")
			}
			if !reflect.DeepEqual(got.Inputs, tc.want) {
				t.Errorf("DV_TEXT at0031 inputs = %+v, want %+v", got.Inputs, tc.want)
			}
		})
	}
}
