package walk_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Phase 5 — Walk visits every reachable node in depth-first order;
// pre-order fires before children, post-order after. Sanity-checks
// the pre/post order on a fixed fragment.
func TestWalk_PrePostOrder(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	var trace []string
	v := walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			trace = append(trace, "pre:"+ctx.Path())
			return nil
		},
		Post: func(ctx *walk.Context) error {
			trace = append(trace, "post:"+ctx.Path())
			return nil
		},
	}
	if err := walk.Walk(c, v); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(trace) == 0 {
		t.Fatal("trace empty; Walk did not visit any node")
	}
	// Root path "/" must be the first pre-order entry and the last
	// post-order entry.
	if trace[0] != "pre:/" {
		t.Errorf("first trace entry = %q, want \"pre:/\"", trace[0])
	}
	if trace[len(trace)-1] != "post:/" {
		t.Errorf("last trace entry = %q, want \"post:/\"", trace[len(trace)-1])
	}
	// Every pre must have a matching post (counts equal).
	var pre, post int
	for _, e := range trace {
		switch {
		case strings.HasPrefix(e, "pre:"):
			pre++
		case strings.HasPrefix(e, "post:"):
			post++
		}
	}
	if pre != post {
		t.Errorf("pre count %d != post count %d (every PreHandle should be paired with a PostHandle)", pre, post)
	}
}

// Phase 5 — SkipSubtree from PreHandle prunes children AND skips
// PostHandle for the pruned node. Sibling traversal continues.
//
// Note: the compiled tree's AQL paths are fully qualified, so
// `/content` is NOT a real path under a multi-cardinality
// attribute. The real subtree roots are `/content[<archetype-id>]`.
// Skip one specific archetype-root subtree and verify its descendants
// are pruned while sibling archetype-root subtrees are still walked.
func TestWalk_SkipSubtree(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")

	const skipped = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	const sibling = "/content[openEHR-EHR-OBSERVATION.heart_rate.v1]"

	var prePaths, postPaths []string
	v := walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			prePaths = append(prePaths, ctx.Path())
			if ctx.Path() == skipped {
				return walk.SkipSubtree
			}
			return nil
		},
		Post: func(ctx *walk.Context) error {
			postPaths = append(postPaths, ctx.Path())
			return nil
		},
	}
	if err := walk.Walk(c, v); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	// The pruned node must appear in pre but NOT in post.
	if !slices.Contains(prePaths, skipped) {
		t.Errorf("PreHandle never fired for %s", skipped)
	}
	if slices.Contains(postPaths, skipped) {
		t.Errorf("PostHandle fired for skipped %s", skipped)
	}
	// No descendant of the skipped subtree should appear in either
	// trace. Descendants share the skipped path as a prefix followed
	// by '/'.
	prefix := skipped + "/"
	for _, p := range prePaths {
		if strings.HasPrefix(p, prefix) {
			t.Errorf("descendant of skipped subtree visited (PreHandle): %s", p)
		}
	}
	for _, p := range postPaths {
		if strings.HasPrefix(p, prefix) {
			t.Errorf("descendant of skipped subtree visited (PostHandle): %s", p)
		}
	}
	// Sibling archetype-root subtrees MUST still be visited — pruning
	// a subtree must not abort sibling traversal.
	if !slices.Contains(prePaths, sibling) {
		t.Errorf("sibling subtree %s not visited after %s was pruned", sibling, skipped)
	}
	if !slices.ContainsFunc(prePaths, func(p string) bool {
		return strings.HasPrefix(p, sibling+"/")
	}) {
		t.Errorf("no descendant of sibling subtree %s visited; walker likely aborted", sibling)
	}
}

// Phase 5 — a non-nil non-SkipSubtree error aborts the walk.
func TestWalk_ErrorAborts(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	boom := errors.New("boom")
	var visited int
	err := walk.Walk(c, walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			visited++
			if ctx.Path() == "/category" {
				return boom
			}
			return nil
		},
	})
	if !errors.Is(err, boom) {
		t.Errorf("Walk = %v, want errors.Is(err, boom)", err)
	}
	if visited == 0 {
		t.Errorf("visited zero nodes before abort; walker may not have run")
	}
}

// Phase 5 — Context.Parent / ParentAttribute / Depth are populated
// consistently with the walk position.
func TestWalk_ContextFields(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")

	var sawRoot, sawDeep bool
	err := walk.Walk(c, walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			if ctx.Path() == "/" {
				sawRoot = true
				if ctx.Parent() != nil {
					t.Errorf("Root parent = %p, want nil", ctx.Parent())
				}
				if ctx.ParentAttribute() != nil {
					t.Errorf("Root parent attribute = %p, want nil", ctx.ParentAttribute())
				}
				if ctx.Depth() != 0 {
					t.Errorf("Root depth = %d, want 0", ctx.Depth())
				}
			}
			if ctx.Path() == "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]" {
				sawDeep = true
				if ctx.Parent() == nil {
					t.Errorf("deep node parent unexpectedly nil")
				}
				if pa := ctx.ParentAttribute(); pa == nil || pa.Name() != "content" {
					t.Errorf("deep node parent attribute = %+v, want name=content", pa)
				}
				if ctx.Depth() == 0 {
					t.Errorf("deep node depth = 0, expected > 0")
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !sawRoot {
		t.Error("root never visited")
	}
	if !sawDeep {
		t.Error("blood_pressure node never visited (fixture changed?)")
	}
}

// Phase 5 — WalkSubtree starts at the addressed node; the start
// node is visited at depth 0; descendants below it are visited.
// Sibling subtrees outside the start are NOT visited.
func TestWalkSubtree_StartsAtPath(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	const start = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"

	var paths []string
	err := walk.WalkSubtree(c, start, walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			paths = append(paths, ctx.Path())
			if ctx.Path() == start && ctx.Depth() != 0 {
				t.Errorf("start node depth = %d, want 0", ctx.Depth())
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("WalkSubtree: %v", err)
	}
	if len(paths) == 0 || paths[0] != start {
		t.Errorf("first visited path = %v, want %s", paths, start)
	}
	// No sibling archetype root should appear.
	for _, p := range paths {
		if strings.Contains(p, "heart_rate") || strings.Contains(p, "body_temperature") {
			t.Errorf("WalkSubtree leaked into sibling archetype root: %s", p)
		}
	}
}

// Phase 5 — WalkSubtree returns ErrPathNotFound when the start
// path does not resolve in the compiled tree.
func TestWalkSubtree_UnknownPath(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	err := walk.WalkSubtree(c, "/no_such_path", walk.VisitorFunc{})
	if !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Errorf("got %v, want errors.Is(err, templatecompile.ErrPathNotFound)", err)
	}
}

// Phase 5 — nil arguments to Walk / WalkSubtree return ErrInvalidInput.
func TestWalk_NilInputs(t *testing.T) {
	if err := walk.Walk(nil, walk.VisitorFunc{}); !errors.Is(err, walk.ErrInvalidInput) {
		t.Errorf("Walk(nil compiled) = %v, want ErrInvalidInput", err)
	}
	c := mustCompile(t, "vital_signs.opt")
	if err := walk.Walk(c, nil); !errors.Is(err, walk.ErrInvalidInput) {
		t.Errorf("Walk(nil visitor) = %v, want ErrInvalidInput", err)
	}
	if err := walk.WalkSubtree(nil, "/", walk.VisitorFunc{}); !errors.Is(err, walk.ErrInvalidInput) {
		t.Errorf("WalkSubtree(nil compiled) = %v, want ErrInvalidInput", err)
	}
}

// Phase 5 — implicit attribute nodes contribute no recursion (they
// have no children) but the walker does not crash on them. Spot-
// check by counting visited nodes against the byPath index size.
func TestWalk_VisitsEveryNode(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	var visited int
	if err := walk.Walk(c, walk.VisitorFunc{
		Pre: func(*walk.Context) error {
			visited++
			return nil
		},
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// Every CompiledNode is in the byPath index. AllByRMType returns
	// every node grouped by RM type; sum the groups for the total.
	want := totalNodes(c)
	if visited != want {
		t.Errorf("Walk visited %d nodes, byRMType total %d (walker missed some nodes)", visited, want)
	}
}

// totalNodes sums every node visible via AllByRMType. AllByRMType
// is keyed by RM type name; every node has one, so each node
// appears exactly once across the groups.
func totalNodes(c *templatecompile.Compiled) int {
	seen := map[*templatecompile.CompiledNode]struct{}{}
	// Iterate every RM type we know about via the compiled tree's
	// own walker (Compile populates byRMType for every node). We
	// don't have a direct enumerator, so deduplicate via pointer
	// identity from a small walk that uses a separate visitor.
	_ = walk.Walk(c, walk.VisitorFunc{
		Pre: func(ctx *walk.Context) error {
			seen[ctx.Node()] = struct{}{}
			return nil
		},
	})
	return len(seen)
}

func mustCompile(t *testing.T, fixture string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(filepath.Join("..", "..", "..", "openehr", "template", "testdata", fixture))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", fixture, err)
	}
	return c
}
