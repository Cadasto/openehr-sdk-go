package webtemplate_test

// REQ-106 / PROBE-075 — structural parity of the built tree against the
// vendored EHRbase reference (id / rmType / nodeId / aqlPath / min / max).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
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

// A nil compiled template must surface the sentinel, never panic (REQ-106).
func TestBuildNil(t *testing.T) {
	if _, err := webtemplate.Build(nil); !errors.Is(err, webtemplate.ErrEmptyTemplate) {
		t.Errorf("Build(nil) err = %v, want ErrEmptyTemplate", err)
	}
	if _, err := webtemplate.Marshal(nil); !errors.Is(err, webtemplate.ErrEmptyTemplate) {
		t.Errorf("Marshal(nil) err = %v, want ErrEmptyTemplate", err)
	}
}

// An OPT without a resolvable default language must error, never emit
// "defaultLanguage": "" (spec § REQ-106 Surface MUST).
func TestBuildNoDefaultLanguage(t *testing.T) {
	raw, err := os.ReadFile(referenceDir + "/" + referenceStem + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	// Strip the OPT-level <language> block; the lenient parser accepts
	// the document, leaving Compiled.Language() empty.
	before, rest, foundOpen := bytes.Cut(raw, []byte("<language>"))
	_, after, foundClose := bytes.Cut(rest, []byte("</language>"))
	if !foundOpen || !foundClose {
		t.Fatal("fixture has no <language> block to strip")
	}
	stripped := append(append([]byte{}, before...), after...)

	opt, err := template.ParseOPT(bytes.NewReader(stripped))
	if err != nil {
		t.Fatalf("ParseOPT (language stripped): %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := webtemplate.Build(c); !errors.Is(err, webtemplate.ErrNoDefaultLanguage) {
		t.Errorf("Build err = %v, want ErrNoDefaultLanguage", err)
	}
}

// Datatypes outside the core subset must emit their node without inputs
// and without error (spec § REQ-106 inputs MUST NOT error).
func TestExoticDatatypesEmitNoInputs(t *testing.T) {
	cases := map[string]string{ // fixture → expected input-less leaf rmType
		"Test_dv_multimedia_open_constraint.v0.opt":        "DV_MULTIMEDIA",
		"Test_dv_parsable_open_constraint.v0.opt":          "DV_PARSABLE",
		"Test_dv_uri_open_constraint.v0.opt":               "DV_URI",
		"Test_dv_ehr_uri_open_constraint.v0.opt":           "DV_EHR_URI",
		"Test_dv_interval_dv_count_open_constraint.v0.opt": "DV_INTERVAL",
	}
	for fixture, rmType := range cases {
		t.Run(rmType, func(t *testing.T) {
			c := compileFixture(t, "../../../testkit/cassettes/templates/"+fixture)
			wt, err := webtemplate.Build(c)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			found := false
			walkOurTree(wt.Tree, func(n *webtemplate.Node) {
				if strings.HasPrefix(n.RMType, rmType) {
					found = true
					if len(n.Inputs) != 0 {
						t.Errorf("%s node carries inputs %+v, want none", n.RMType, n.Inputs)
					}
				}
			})
			if !found {
				t.Errorf("no %s node emitted", rmType)
			}
		})
	}
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

// walkOurTree visits every node of our tree depth-first.
func walkOurTree(n *webtemplate.Node, visit func(*webtemplate.Node)) {
	visit(n)
	for _, ch := range n.Children {
		walkOurTree(ch, visit)
	}
}

// walkRefTree visits every node of the reference JSON tree depth-first.
func walkRefTree(m map[string]any, visit func(map[string]any)) {
	visit(m)
	if ch, ok := m["children"].([]any); ok {
		for _, c := range ch {
			if cm, ok := c.(map[string]any); ok {
				walkRefTree(cm, visit)
			}
		}
	}
}

func refStr(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// refTree fails the test when the reference document carries no tree.
func refTree(t *testing.T, ref map[string]any) map[string]any {
	t.Helper()
	tree, ok := ref["tree"].(map[string]any)
	if !ok {
		t.Fatal("reference has no object tree")
	}
	return tree
}

// Both parity tests index nodes by aqlPath; siblings sharing a path (one
// at-code cloned under a Multiple attribute) would silently collapse into
// one entry. constrain_test has no such duplicate; templates that do are
// the deferred archetype-reuse-under-slot class the compiler rejects.

func TestStructuralParity(t *testing.T) {
	refByPath := map[string]nodeFacts{}
	walkRefTree(refTree(t, loadReference(t)), func(m map[string]any) {
		num := func(k string) int {
			if v, ok := m[k].(float64); ok {
				return int(v)
			}
			return 0
		}
		refByPath[refStr(m, "aqlPath")] = nodeFacts{refStr(m, "rmType"), refStr(m, "nodeId"), refStr(m, "id"), num("min"), num("max")}
	})

	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ourByPath := map[string]nodeFacts{}
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		ourByPath[n.AQLPath] = nodeFacts{n.RMType, n.NodeID, n.ID, n.Min, n.Max}
	})

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
