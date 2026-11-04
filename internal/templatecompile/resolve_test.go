package templatecompile_test

// resolve_test.go: Compiled.Resolve and Compiled.ValidatePath, the path
// lookup that walks the compiled tree by the rules the deprecated parser's
// NodeAt applies to its own.

import (
	"bytes"
	"cmp"
	"errors"
	"os"
	"path/filepath"
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
// error that matches both not-found sentinels (REQ-100 § Resolution
// semantics).
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

// TestREQ111_ResolveMatchesParserNodeAt holds Resolve to the deprecated
// parser's NodeAt on every corpus template: for every node, the path built
// from the node ids and archetype ids above it resolves, in the default and
// the strict mode, to a compiled node of the RM type, node id and archetype
// id of the parser's NodeAt node, the one that stands where that node stands
// and so carries its AQL path, or both refuse it alike.
func TestREQ111_ResolveMatchesParserNodeAt(t *testing.T) {
	names, err := templateprobes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	root := fixtures.CorpusRoot()
	var checked int
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(name))
			c, err := templatecompile.Compile(mustReadModelFile(t, path))
			if err != nil {
				t.Skipf("does not compile: %v", err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := template.ParseOPT(bytes.NewReader(b))
			if err != nil {
				t.Skipf("the parser does not read it: %v", err)
			}
			counterpart := map[template.Node]*templatecompile.CompiledNode{}
			pairNodes(t, parsed.Root(), c.Root(), counterpart)

			var walkNodes func(n *templatecompile.CompiledNode, path string)
			walkNodes = func(n *templatecompile.CompiledNode, path string) {
				checked++
				p := mustParsePath(t, cmp.Or(path, "/"))
				for _, opts := range [][]template.ResolveOption{nil, {template.WithStrictPaths()}} {
					var compiledOpts []templatecompile.ResolveOption
					if opts != nil {
						compiledOpts = append(compiledOpts, templatecompile.WithStrictPaths())
					}
					want, wantErr := parsed.NodeAt(p, opts...)
					got, gotErr := c.Resolve(p, compiledOpts...)
					if !sameOutcome(wantErr, gotErr) {
						t.Errorf("%s (strict %t): NodeAt = %v, Resolve = %v", p, opts != nil, wantErr, gotErr)
						continue
					}
					if wantErr != nil {
						continue
					}
					if got.RMTypeName() != want.RMTypeName() || got.NodeID() != want.NodeID() || got.ArchetypeID() != archetypeIDOf(want) {
						t.Errorf("%s (strict %t): NodeAt reached %s %q %q, Resolve reached %s %q %q at %s",
							p, opts != nil, want.RMTypeName(), want.NodeID(), archetypeIDOf(want),
							got.RMTypeName(), got.NodeID(), got.ArchetypeID(), got.AQLPath())
						continue
					}
					if counterpart[want] != got {
						t.Errorf("%s (strict %t): NodeAt reached %s %q %q, Resolve reached %s %q %q at %s, not its counterpart at %s",
							p, opts != nil, want.RMTypeName(), want.NodeID(), archetypeIDOf(want),
							got.RMTypeName(), got.NodeID(), got.ArchetypeID(), got.AQLPath(), aqlPathOf(counterpart[want]))
					}
				}
				for _, a := range n.Attributes() {
					for _, child := range a.Children() {
						walkNodes(child, path+"/"+a.Name()+predicateOf(child))
					}
				}
			}
			walkNodes(c.Root(), "")
		})
	}
	if checked == 0 {
		t.Fatal("no corpus node was checked")
	}
}

// pairNodes records in counterpart the compiled node that stands where each
// node of the parser's tree stands: the same declared attribute, in the
// same order, and the same child. The compiled tree lists the declared
// attributes first, in document order, and the injected ones after them.
func pairNodes(t *testing.T, parsed template.Node, compiled *templatecompile.CompiledNode, counterpart map[template.Node]*templatecompile.CompiledNode) {
	t.Helper()
	counterpart[parsed] = compiled
	on, ok := parsed.(template.ObjectNode)
	if !ok {
		return
	}
	attrs := compiled.Attributes()
	for i, a := range on.Attributes() {
		if i >= len(attrs) || attrs[i].Name() != a.Name() || len(attrs[i].Children()) != len(a.Children()) {
			t.Fatalf("at %s, the parser's attribute %d (%s) has no counterpart in the compiled tree", compiled.AQLPath(), i+1, a.Name())
		}
		children := attrs[i].Children()
		for j, child := range a.Children() {
			pairNodes(t, child, children[j], counterpart)
		}
	}
}

// predicateOf returns the predicate of the path segment that reaches n from
// its holding attribute: n's archetype id, else its node id, else none.
func predicateOf(n *templatecompile.CompiledNode) string {
	if id := cmp.Or(n.ArchetypeID(), n.NodeID()); id != "" {
		return "[" + id + "]"
	}
	return ""
}

// sameOutcome reports whether the parser's error want and Resolve's error
// got are the same outcome: both nil, both not found, or both ambiguous.
func sameOutcome(want, got error) bool {
	for _, sentinel := range []error{template.ErrPathNotFound, template.ErrAmbiguousPath} {
		if errors.Is(want, sentinel) != errors.Is(got, sentinel) {
			return false
		}
	}
	return (want == nil) == (got == nil)
}

// archetypeIDOf returns the archetype id of a parser node that is an
// archetype root, else "".
func archetypeIDOf(n template.Node) string {
	if ar, ok := n.(*template.ArchetypeRoot); ok {
		return ar.ArchetypeID()
	}
	return ""
}

// aqlPathOf returns the AQL path of n, or "<none>" when n is nil.
func aqlPathOf(n *templatecompile.CompiledNode) string {
	if n == nil {
		return "<none>"
	}
	return n.AQLPath()
}
