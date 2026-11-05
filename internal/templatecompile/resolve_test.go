package templatecompile_test

// resolve_test.go: Compiled.Resolve and Compiled.ValidatePath, the path
// lookup that walks the compiled tree by the rules of § REQ-111 Path lookup.

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	templateprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

// mustParsePath parses s with [template.ParsePath], stopping the test when
// it does not parse.
func mustParsePath(t testing.TB, s string) template.Path {
	t.Helper()
	p, err := template.ParsePath(s)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", s, err)
	}
	return p
}

// resolveModel is a COMPOSITION whose content holds, in order, an
// OBSERVATION archetype root, a SECTION at0001 with no attributes, and a
// SECTION at0002 whose items hold a CLUSTER at0003, and whose name holds a
// DV_TEXT.
func resolveModel() *aom14.OperationalTemplate {
	observation := &aom14.CArchetypeRoot{
		CComplexObject: *complexObject("OBSERVATION", "at0000"),
		ArchetypeID:    rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.resolve.v1"},
	}
	return modelOf(multiple("content",
		observation,
		complexObject("SECTION", "at0001"),
		complexObject("SECTION", "at0002", multiple("items", complexObject("CLUSTER", "at0003"))),
	), single("name", complexObject("DV_TEXT", "")),
	)
}

// resolveModelWithSlot is resolveModel with the slot at0004 after the
// CLUSTER at0003 under the items of SECTION at0002.
func resolveModelWithSlot() *aom14.OperationalTemplate {
	m := resolveModel()
	content := m.Definition.Attributes[0].(*aom14.CMultipleAttribute)
	section := content.Children[2].(*aom14.CComplexObject)
	items := section.Attributes[0].(*aom14.CMultipleAttribute)
	items.Children = append(items.Children, &aom14.ArchetypeSlot{RMTypeName: "CLUSTER", NodeID: "at0004", Occurrences: once})
	return m
}

// TestREQ111_ResolveFollowsResolutionRules pins § REQ-111 Path lookup: a
// segment matches an attribute by name and its predicate a child's node id
// or archetype id, a segment without a predicate takes the first child, and
// a segment that matches nothing, an implicit attribute among them, a node
// that has no such attribute and a slot, which does not descend, gives an
// error that matches both not-found sentinels.
func TestREQ111_ResolveFollowsResolutionRules(t *testing.T) {
	c, err := templatecompile.Compile(resolveModelWithSlot())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	found := []struct {
		path, rmType, nodeID, archetypeID string
	}{
		{path: "/", rmType: "COMPOSITION", nodeID: "at0000", archetypeID: "openEHR-EHR-COMPOSITION.model.v1"},
		{path: "/content", rmType: "OBSERVATION", nodeID: "at0000", archetypeID: "openEHR-EHR-OBSERVATION.resolve.v1"},
		{path: "/content[openEHR-EHR-OBSERVATION.resolve.v1]", rmType: "OBSERVATION", nodeID: "at0000", archetypeID: "openEHR-EHR-OBSERVATION.resolve.v1"},
		{path: "/content[at0000]", rmType: "OBSERVATION", nodeID: "at0000", archetypeID: "openEHR-EHR-OBSERVATION.resolve.v1"},
		{path: "/content[at0002]", rmType: "SECTION", nodeID: "at0002"},
		{path: "/content[at0002]/items", rmType: "CLUSTER", nodeID: "at0003"},
		{path: "/content[at0002]/items[at0004]", rmType: "CLUSTER", nodeID: "at0004"},
		{path: "/name", rmType: "DV_TEXT"},
	}
	for _, tc := range found {
		n, err := c.Resolve(mustParsePath(t, tc.path))
		if err != nil {
			t.Errorf("Resolve(%q) = %v, want the %s node", tc.path, err, tc.rmType)
			continue
		}
		if n.RMTypeName() != tc.rmType || n.NodeID() != tc.nodeID || n.ArchetypeID() != tc.archetypeID {
			t.Errorf("Resolve(%q) = %s %q %q, want %s %q %q", tc.path,
				n.RMTypeName(), n.NodeID(), n.ArchetypeID(), tc.rmType, tc.nodeID, tc.archetypeID)
		}
		if err := c.ValidatePath(mustParsePath(t, tc.path)); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", tc.path, err)
		}
	}

	notFound := []string{
		"/no_such_attribute",
		"/Content", // a name matched as written, case and all
		"/content[at9999]",
		"/content[openEHR-EHR-OBSERVATION.other.v1]",
		"/content[at0001]/items",                // a node with no such attribute
		"/category",                             // an implicit attribute, which holds no child
		"/content[at0002]/items[at0004]/items",  // a slot, which does not descend
		"/content[at0002]/items[at0003]/name/x", // past a node the walk reaches
	}
	for _, path := range notFound {
		n, err := c.Resolve(mustParsePath(t, path))
		if !errors.Is(err, template.ErrPathNotFound) || !errors.Is(err, templatecompile.ErrPathNotFound) {
			t.Errorf("Resolve(%q) = %v, %v; want an error matching template.ErrPathNotFound and templatecompile.ErrPathNotFound", path, n, err)
		}
		if errors.Is(err, template.ErrAmbiguousPath) {
			t.Errorf("Resolve(%q) = %v, which matches template.ErrAmbiguousPath", path, err)
		}
		if n != nil {
			t.Errorf("Resolve(%q) returned node %s with its error", path, n.AQLPath())
		}
		if err := c.ValidatePath(mustParsePath(t, path)); !errors.Is(err, template.ErrPathNotFound) || !errors.Is(err, templatecompile.ErrPathNotFound) {
			t.Errorf("ValidatePath(%q) = %v, want an error matching both not-found sentinels", path, err)
		}
	}
}

// TestREQ111_ResolveStrictPathsRefusesAmbiguity pins the strict-paths option
// of § REQ-111 Path lookup: a segment without a predicate on an attribute
// with more than one child gives an error wrapping template.ErrAmbiguousPath,
// where by default it takes the first child; an attribute with one child,
// and a segment with a predicate, resolve as without the option.
func TestREQ111_ResolveStrictPathsRefusesAmbiguity(t *testing.T) {
	c, err := templatecompile.Compile(resolveModel())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	strict := templatecompile.WithStrictPaths()

	for _, path := range []string{"/content", "/content/data"} {
		n, err := c.Resolve(mustParsePath(t, path), strict)
		if !errors.Is(err, template.ErrAmbiguousPath) {
			t.Errorf("Resolve(%q, WithStrictPaths()) = %v, %v; want an error wrapping template.ErrAmbiguousPath", path, n, err)
		}
		if errors.Is(err, template.ErrPathNotFound) {
			t.Errorf("Resolve(%q, WithStrictPaths()) = %v, which matches template.ErrPathNotFound", path, err)
		}
		if err := c.ValidatePath(mustParsePath(t, path), strict); !errors.Is(err, template.ErrAmbiguousPath) {
			t.Errorf("ValidatePath(%q, WithStrictPaths()) = %v, want an error wrapping template.ErrAmbiguousPath", path, err)
		}
	}
	if n, err := c.Resolve(mustParsePath(t, "/content")); err != nil || n.ArchetypeID() != "openEHR-EHR-OBSERVATION.resolve.v1" {
		t.Errorf("Resolve(/content) = %v, %v; want the first child, the OBSERVATION root", n, err)
	}
	for _, path := range []string{"/", "/content[at0002]/items", "/content[at0001]", "/name"} {
		if err := c.ValidatePath(mustParsePath(t, path), strict, nil); err != nil {
			t.Errorf("ValidatePath(%q, WithStrictPaths(), nil) = %v, want nil", path, err)
		}
	}
}

// TestREQ111_ResolveWithinTheDepthLimit pins the boundary of the depth limit
// on a compiled tree: a path that reaches the deepest node Compile admits,
// 128 levels below the definition, resolves (REQ-108).
func TestREQ111_ResolveWithinTheDepthLimit(t *testing.T) {
	c, err := templatecompile.Compile(nested(128))
	if err != nil {
		t.Fatalf("Compile(nested(128)): %v", err)
	}
	path := "/content[at0001]" + strings.Repeat("/items[at0001]", 127)
	n, err := c.Resolve(mustParsePath(t, path))
	if err != nil {
		t.Fatalf("Resolve of a path 128 levels deep = %v, want the deepest CLUSTER", err)
	}
	if n.AQLPath() != path {
		t.Errorf("Resolve of a path 128 levels deep reached %s, want the deepest CLUSTER", n.AQLPath())
	}
}

// TestREQ111_ResolveOnNoTemplate: a nil compiled template, and one that has
// no root, resolve no path, the root path included.
func TestREQ111_ResolveOnNoTemplate(t *testing.T) {
	for name, c := range map[string]*templatecompile.Compiled{"nil": nil, "zero": {}} {
		for _, path := range []string{"/", "/content"} {
			n, err := c.Resolve(mustParsePath(t, path))
			if n != nil || !errors.Is(err, template.ErrPathNotFound) || !errors.Is(err, templatecompile.ErrPathNotFound) {
				t.Errorf("%s Compiled: Resolve(%q) = %v, %v; want no node and both not-found sentinels", name, path, n, err)
			}
		}
	}
}

// TestREQ111_ResolveReachesLaterAlternative settles a backlog lead: the path
// index keeps the first of the alternatives under a single-valued attribute,
// so NodeAt cannot reach a node below a later one, but Resolve walks the
// tree, and a predicate naming the later alternative's node id reaches its
// descendants.
func TestREQ111_ResolveReachesLaterAlternative(t *testing.T) {
	c := mustCompile(t, "sdk_crosscheck_unwritten.v1")
	const action = "/content[openEHR-EHR-ACTION.sdk_unwritten.v1]"
	for _, transition := range []string{"at0003", "at0004"} {
		path := action + "/ism_transition[" + transition + "]/careflow_step"
		n, err := c.Resolve(mustParsePath(t, path))
		if err != nil {
			t.Fatalf("Resolve(%q) = %v", path, err)
		}
		if got := n.Parent().NodeID(); got != transition {
			t.Errorf("Resolve(%q) reached a careflow_step of the ISM_TRANSITION %q, want %q", path, got, transition)
		}
	}
	first, err := c.NodeAt(action + "/ism_transition/careflow_step")
	if err != nil {
		t.Fatalf("NodeAt: %v", err)
	}
	if got := first.Parent().NodeID(); got != "at0003" {
		t.Errorf("NodeAt kept the careflow_step of ISM_TRANSITION %q, want the first alternative's, at0003", got)
	}
}

// TestREQ111_ResolveFollowsResolutionRulesOverCorpus holds Resolve to the
// rules of § REQ-111 Path lookup on every corpus template: for every node,
// the path built from the node ids and archetype ids above it resolves, in
// the default and the strict mode, to the node resolveByRules reaches on the
// compiled tree's attributes and children, or both refuse it alike. The
// corpus holds paths on which the first match is another node than the one
// the path was built from, and paths the strict mode refuses as ambiguous, so
// the walk exercises both rules.
func TestREQ111_ResolveFollowsResolutionRulesOverCorpus(t *testing.T) {
	names, err := templateprobes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	root := fixtures.CorpusRoot()
	var checked, elsewhere, ambiguous int
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			c, err := templatecompile.Compile(mustReadModelFile(t, filepath.Join(root, filepath.FromSlash(name))))
			if err != nil {
				t.Skipf("does not compile: %v", err)
			}
			var walkNodes func(n *templatecompile.CompiledNode, steps []pathStep)
			walkNodes = func(n *templatecompile.CompiledNode, steps []pathStep) {
				checked++
				p := mustParsePath(t, pathText(steps))
				for _, strict := range []bool{false, true} {
					var opts []templatecompile.ResolveOption
					if strict {
						opts = append(opts, templatecompile.WithStrictPaths())
					}
					want, wantErr := resolveByRules(c.Root(), steps, strict)
					got, gotErr := c.Resolve(p, opts...)
					if !sameOutcome(wantErr, gotErr) || got != want {
						t.Errorf("Resolve(%s) strict %t = %s, %v; the rules reach %s, %v",
							p, strict, placeOf(got), gotErr, placeOf(want), wantErr)
					}
					if want != nil && want != n {
						elsewhere++
					}
					if errors.Is(wantErr, template.ErrAmbiguousPath) {
						ambiguous++
					}
				}
				for _, a := range n.Attributes() {
					for _, child := range a.Children() {
						step := pathStep{name: a.Name(), predicate: cmp.Or(child.ArchetypeID(), child.NodeID())}
						walkNodes(child, append(slices.Clip(steps), step))
					}
				}
			}
			walkNodes(c.Root(), nil)
		})
	}
	if checked == 0 || elsewhere == 0 || ambiguous == 0 {
		t.Errorf("the corpus walk checked %d nodes, %d lookups that reach another node and %d refused as ambiguous; want some of each",
			checked, elsewhere, ambiguous)
	}
}

// pathStep is one segment of a path the corpus walk builds: an attribute
// name and the predicate that selects a child, "" when it has none.
type pathStep struct {
	name, predicate string
}

// pathText returns the path of steps as template.ParsePath reads it.
func pathText(steps []pathStep) string {
	if len(steps) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range steps {
		b.WriteString("/" + s.name)
		if s.predicate != "" {
			b.WriteString("[" + s.predicate + "]")
		}
	}
	return b.String()
}

// resolveByRules walks root along steps by the resolution rules, read on the
// compiled tree's exported view: a step names an attribute of the node
// reached by its exact name, its predicate takes the first child whose node
// id or archetype id it is, and a step without one takes the first child,
// or in the strict mode refuses an attribute with more than one child as
// ambiguous. A slot does not descend. It returns the node reached, or the
// sentinel of the refusal.
func resolveByRules(root *templatecompile.CompiledNode, steps []pathStep, strict bool) (*templatecompile.CompiledNode, error) {
	n := root
	for _, s := range steps {
		if n.IsSlot() {
			return nil, template.ErrPathNotFound
		}
		attrs := n.Attributes()
		i := slices.IndexFunc(attrs, func(a *templatecompile.CompiledAttribute) bool { return a.Name() == s.name })
		if i < 0 {
			return nil, template.ErrPathNotFound
		}
		children := attrs[i].Children()
		switch {
		case len(children) == 0:
			return nil, template.ErrPathNotFound
		case s.predicate == "" && strict && len(children) > 1:
			return nil, template.ErrAmbiguousPath
		case s.predicate == "":
			n = children[0]
		default:
			j := slices.IndexFunc(children, func(c *templatecompile.CompiledNode) bool {
				return c.NodeID() == s.predicate || c.ArchetypeID() == s.predicate
			})
			if j < 0 {
				return nil, template.ErrPathNotFound
			}
			n = children[j]
		}
	}
	return n, nil
}

// sameOutcome reports whether the errors want and got are the same outcome
// of a lookup: both nil, both not found, or both ambiguous.
func sameOutcome(want, got error) bool {
	for _, sentinel := range []error{template.ErrPathNotFound, template.ErrAmbiguousPath} {
		if errors.Is(want, sentinel) != errors.Is(got, sentinel) {
			return false
		}
	}
	return (want == nil) == (got == nil)
}

// placeOf names n by its AQL path and, as siblings can share one, by its
// place among the children of its attribute; "<none>" when n is nil.
func placeOf(n *templatecompile.CompiledNode) string {
	if n == nil {
		return "<none>"
	}
	if parent := n.Parent(); parent != nil {
		for _, a := range parent.Attributes() {
			if i := slices.Index(a.Children(), n); i >= 0 {
				return fmt.Sprintf("%s (child %d of %s)", n.AQLPath(), i+1, a.Name())
			}
		}
	}
	return n.AQLPath()
}
