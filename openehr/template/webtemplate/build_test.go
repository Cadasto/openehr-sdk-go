package webtemplate_test

// REQ-106 / PROBE-075 — structural parity of the built tree against the
// vendored EHRbase reference (id / rmType / nodeId / aqlPath / min / max).

import (
	"fmt"
	"sort"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

func compileFixture(t *testing.T, path string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return c
}

func TestBuildRootShape(t *testing.T) {
	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if wt.Version != "2.3" {
		t.Errorf("version = %q, want 2.3", wt.Version)
	}
	if wt.Tree == nil || wt.Tree.RMType != "COMPOSITION" {
		t.Fatalf("root = %+v, want COMPOSITION", wt.Tree)
	}
	if wt.Tree.AQLPath != "" {
		t.Errorf("root aqlPath = %q, want empty", wt.Tree.AQLPath)
	}
	if len(wt.Tree.Children) == 0 {
		t.Error("root has no children")
	}
}

// nodeFacts is the structural signature compared for parity.
type nodeFacts struct {
	rmType, nodeID, id string
	min, max           int
}

func (f nodeFacts) String() string {
	return fmt.Sprintf("rmType=%s nodeId=%s id=%s min=%d max=%d", f.rmType, f.nodeID, f.id, f.min, f.max)
}

// walkOurs indexes our tree by aqlPath.
func walkOurs(n *webtemplate.Node, out map[string]nodeFacts) {
	out[n.AQLPath] = nodeFacts{n.RMType, n.NodeID, n.ID, n.Min, n.Max}
	for _, ch := range n.Children {
		walkOurs(ch, out)
	}
}

// walkRef indexes the reference JSON tree by aqlPath.
func walkRef(m map[string]any, out map[string]nodeFacts) {
	str := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	num := func(k string) int {
		if v, ok := m[k].(float64); ok {
			return int(v)
		}
		return 0
	}
	out[str("aqlPath")] = nodeFacts{str("rmType"), str("nodeId"), str("id"), num("min"), num("max")}
	if ch, ok := m["children"].([]any); ok {
		for _, c := range ch {
			if cm, ok := c.(map[string]any); ok {
				walkRef(cm, out)
			}
		}
	}
}

func TestStructuralParity(t *testing.T) {
	ref := loadReference(t) // skips if fixture absent
	refByPath := map[string]nodeFacts{}
	walkRef(ref["tree"].(map[string]any), refByPath)

	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ourByPath := map[string]nodeFacts{}
	walkOurs(wt.Tree, ourByPath)

	var missing, extra, mismatch []string
	for p, rf := range refByPath {
		of, ok := ourByPath[p]
		if !ok {
			missing = append(missing, p)
			continue
		}
		if of != rf {
			mismatch = append(mismatch, fmt.Sprintf("%s\n    ref:  %s\n    ours: %s", p, rf, of))
		}
	}
	for p := range ourByPath {
		if _, ok := refByPath[p]; !ok {
			extra = append(extra, p)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(mismatch)

	t.Logf("PARITY: ref=%d ours=%d | matched=%d missing=%d extra=%d mismatch=%d",
		len(refByPath), len(ourByPath), len(refByPath)-len(missing)-len(mismatch), len(missing), len(extra), len(mismatch))
	show := func(label string, xs []string, n int) {
		if len(xs) == 0 {
			return
		}
		if len(xs) > n {
			t.Logf("--- %s (%d, first %d):", label, len(xs), n)
			xs = xs[:n]
		} else {
			t.Logf("--- %s (%d):", label, len(xs))
		}
		for _, x := range xs {
			t.Logf("  %s", x)
		}
	}
	show("MISSING (in ref, not ours)", missing, 15)
	show("EXTRA (ours, not ref)", extra, 15)
	show("MISMATCH", mismatch, 15)

	if len(missing) > 0 || len(extra) > 0 || len(mismatch) > 0 {
		t.Errorf("structural parity not yet reached (missing=%d extra=%d mismatch=%d)", len(missing), len(extra), len(mismatch))
	}
}
