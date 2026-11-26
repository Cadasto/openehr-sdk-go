package simplified

// REQ-053 — the codecs keep what they derive from a Web Template alone for
// every later call with it: one set of tables per Web Template, dropped with
// it, and never read for another Web Template or another tree.

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"weak"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// TestREQ053WebTemplateTablesBuiltOncePerWebTemplate pins that the calls
// sharing a Web Template share its tables, and that each Web Template gets its
// own, though both have the same template id. Build the tables per call again
// and the repeat call returns fresh ones; key the cache by anything coarser
// than the Web Template and the second one does not get tables of its own.
func TestREQ053WebTemplateTablesBuiltOncePerWebTemplate(t *testing.T) {
	t.Parallel()
	first, _ := conformanceWT(t)
	second, _ := conformanceWT(t)
	if first.TemplateID != second.TemplateID {
		t.Fatalf("template ids %q and %q differ: want one template built twice", first.TemplateID, second.TemplateID)
	}
	x := tablesFor(first)
	if again := tablesFor(first); again != x {
		t.Errorf("tablesFor(first) twice = %p, then %p: want one set of tables per Web Template", x, again)
	}
	y := tablesFor(second)
	if y == x {
		t.Errorf("tablesFor(second) = %p, the tables of first: want each Web Template its own", y)
	}
	if again := tablesFor(second); again != y {
		t.Errorf("tablesFor(second) twice = %p, then %p: want the second Web Template's tables kept too", y, again)
	}
	if x.sib != reusedSiblingsOf(first) {
		t.Errorf("reusedSiblingsOf(first) is not the reused siblings tablesFor(first) keeps")
	}
	if len(x.ranks()) == 0 || len(x.archetypeIDs()) == 0 {
		t.Errorf("tablesFor(first) has %d ranks and %d archetype ids: want the tables of the corpus template", len(x.ranks()), len(x.archetypeIDs()))
	}
}

// TestREQ053WebTemplateTablesDroppedWithTheirWebTemplate pins that the cache
// keeps no Web Template alive and keeps no entry for one once it is collected,
// so a process that builds Web Templates over and over does not grow it. Let
// the entry hold the Web Template, or key the cache by its pointer, and the
// Web Template is never collected; leave out the cleanup and its entry stays.
func TestREQ053WebTemplateTablesDroppedWithTheirWebTemplate(t *testing.T) {
	// The Web Template is reachable only inside this function, so once it
	// returns the cache's weak key is the only way left to it. The decode
	// reads every table, the decode-only ones included.
	key := func() weak.Pointer[webtemplate.WebTemplate] {
		wt, _ := conformanceWT(t)
		body := rmattrBody(mergedIsmStateOnly("524"))
		if _, err := UnmarshalFlat(mustJSON(t, body), wt); err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		return weak.Make(wt)
	}()
	if _, ok := webTemplateTablesCache.Load(key); !ok {
		t.Fatal("webTemplateTablesCache holds no entry for a Web Template a decode just used")
	}
	// A cleanup runs on its own goroutine after the collection that frees
	// the Web Template, so the entry goes some time after one.
	for range 100 {
		runtime.GC()
		if _, ok := webTemplateTablesCache.Load(key); !ok {
			if key.Value() != nil {
				t.Fatal("webTemplateTablesCache dropped the entry of a Web Template that is still alive")
			}
			return
		}
		runtime.Gosched()
	}
	if key.Value() != nil {
		t.Fatal("the Web Template was not collected after 100 collections: something keeps it alive, the cache's entry included")
	}
	t.Fatal("webTemplateTablesCache kept the entry of a Web Template that was collected")
}

// mergedIsmStateOnly is one transition of the corpus ACTION spelled at its
// merged in-context node with a current state and no careflow step, which
// decode infers from the careflow steps the node lists.
func mergedIsmStateOnly(state string) map[string]any {
	keys := inContextIsmKeys(state, "", "")
	for k := range keys {
		if strings.Contains(k, "/careflow_step|") {
			delete(keys, k)
		}
	}
	return keys
}

// listingStateTwice returns a copy of wt whose merged ISM transition node
// lists the current states 524 and 532 under both careflow steps, so no one
// step holds either.
func listingStateTwice(t *testing.T, wt *webtemplate.WebTemplate) *webtemplate.WebTemplate {
	t.Helper()
	return editWTNode(t, wt, conformanceIsm+"/careflow_step", func(n *webtemplate.Node) {
		n.Inputs = slices.Clone(n.Inputs)
		n.Inputs[0].List = slices.Clone(n.Inputs[0].List)
		for i := range n.Inputs[0].List {
			n.Inputs[0].List[i].CurrentStates = []string{"524", "532"}
		}
	})
}

// careflowStepOf decodes the state-only transition at the merged node with wt
// and returns the code of the careflow step decode inferred, "" for none.
func careflowStepOf(t *testing.T, wt *webtemplate.WebTemplate) string {
	t.Helper()
	comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(mergedIsmStateOnly("524"))), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if step := firstAction(t, comp).IsmTransition.CareflowStep; step != nil {
		return step.DefiningCode.CodeString
	}
	return ""
}

// TestREQ053WebTemplateTablesFollowEachWebTemplate pins that a call reads the
// tables of the Web Template it is given, not those of another one with the
// same template id that an earlier call used. The edited copy lists the
// current state under both careflow steps, so decode infers none there, where
// the template it was copied from infers at0005. Key the cache by the
// template id and the copy reads the template's ISM transition nodes.
func TestREQ053WebTemplateTablesFollowEachWebTemplate(t *testing.T) {
	t.Parallel()
	wt, _ := conformanceWT(t)
	edited := listingStateTwice(t, wt)
	for _, tc := range []struct {
		name string
		wt   *webtemplate.WebTemplate
		want string
	}{
		{name: "the template", wt: wt, want: "at0005"},
		{name: "an edited copy", wt: edited, want: ""},
		{name: "the template again", wt: wt, want: "at0005"},
	} {
		if got := careflowStepOf(t, tc.wt); got != tc.want {
			t.Errorf("%s: careflow_step = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestREQ053WebTemplateTablesFollowAReplacedTree pins that a call whose Web
// Template had its tree replaced after an earlier call reads the tree it is
// given. Replacing it breaks the read-only rule webtemplate.Build states, but
// it is the most visible way to, so the tables kept for the old tree are not
// read for the new one. Leave out the check of the tree and the decode
// infers the old tree's careflow step.
func TestREQ053WebTemplateTablesFollowAReplacedTree(t *testing.T) {
	t.Parallel()
	wt, _ := conformanceWT(t)
	edited := listingStateTwice(t, wt).Tree
	if got := careflowStepOf(t, wt); got != "at0005" {
		t.Fatalf("before the tree is replaced: careflow_step = %q, want %q", got, "at0005")
	}
	wt.Tree = edited
	if got := careflowStepOf(t, wt); got != "" {
		t.Errorf("after the tree is replaced: careflow_step = %q, want none, as the new tree lists the state under both steps", got)
	}
}
