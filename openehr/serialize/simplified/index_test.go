package simplified

// REQ-053 — Web Template FLAT-path index engine (shared by both codecs).
import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// obsOPT is a small single-OBSERVATION operational template (proven to build
// a WebTemplate — it has a webtemplate golden).
const obsOPT = "../../../testkit/cassettes/templates/minimal_observation.en.v1.opt"

func buildWT(t *testing.T, optPath string) *webtemplate.WebTemplate {
	t.Helper()
	opt, err := template.ParseFile(optPath)
	if err != nil {
		t.Fatalf("parse %s: %v", optPath, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile %s: %v", optPath, err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build wt %s: %v", optPath, err)
	}
	return wt
}

func TestIndexRootAndValueLeaf(t *testing.T) {
	wt := buildWT(t, obsOPT)
	idx, err := indexTemplate(wt)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if len(idx) == 0 {
		t.Fatal("empty index")
	}
	// The root flatPath is the tree id and maps to the COMPOSITION entry.
	root, ok := idx[wt.Tree.ID]
	if !ok {
		t.Fatalf("no root entry for %q; %d keys", wt.Tree.ID, len(idx))
	}
	if root.rmType != "COMPOSITION" {
		t.Errorf("root rmType = %q, want COMPOSITION", root.rmType)
	}
	// At least one DV_* value leaf whose aqlPath ends in /value, rooted at the
	// tree id in FLAT space.
	var sawValueLeaf bool
	for fp, e := range idx {
		if strings.HasPrefix(e.rmType, "DV_") && strings.HasSuffix(e.aqlPath, "/value") {
			sawValueLeaf = true
			if !strings.HasPrefix(fp, wt.Tree.ID+"/") {
				t.Errorf("leaf flatPath %q not rooted at %q", fp, wt.Tree.ID)
			}
		}
	}
	if !sawValueLeaf {
		t.Error("index has no DV_* value leaf")
	}
}

func TestIndexNilTemplate(t *testing.T) {
	if _, err := indexTemplate(nil); err == nil {
		t.Error("indexTemplate(nil) = nil error, want ErrNoTemplate")
	}
}
