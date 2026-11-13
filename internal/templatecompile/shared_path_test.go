package templatecompile_test

// shared_path_test.go: nodes that share an AQL path with an earlier node,
// which Compiled.NodeAt cannot tell apart (§ REQ-111 Path lookup): the
// alternatives under one attribute, same-id siblings, and every node below
// either.

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	templateprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

// sharedPathTypeExceptions lists, by corpus template and path, the nodes
// below an earlier node's sibling that share its path with a node of another
// RM type. Each value is the reason the census tolerates it.
var sharedPathTypeExceptions = map[[2]string]string{
	{"templates/sdk_crosscheck_siblings.v1.opt", "/context/other_context/items[at0002]/value"}: "two ELEMENTs of one node id that pin no name, a DV_TEXT value " +
		"under the first and a DV_COUNT value under the second, the synthetic shape the corpus README describes; " +
		"the composition builder's Set reaches the first use alone there, as its skeleton policy says, which the composition tests pin",
}

// TestREQ111_NodesBelowSharedPathsShareRMType is a census over the corpus
// templates of the nodes NodeAt cannot reach: for each node whose path
// NodeAt resolves to an earlier node with another parent, a node below an
// alternative or below a same-id sibling, it fails when the two have
// different RM types, except where sharedPathTypeExceptions names the path.
// Alternatives under one attribute share a parent and are left out: Set
// checks a value against each of them, as its type enforcement says. It also
// logs a shared path whose first node the template prohibits while it
// allows a later one, where NodeAt returns a node the skeleton does not
// build.
func TestREQ111_NodesBelowSharedPathsShareRMType(t *testing.T) {
	names, err := templateprobes.Probe111OperationalTemplates()
	if err != nil {
		t.Fatalf("Probe111OperationalTemplates: %v", err)
	}
	root := fixtures.CorpusRoot()
	var shared, below int
	met := map[[2]string]bool{}
	for _, name := range names {
		c, err := templatecompile.Compile(mustReadModelFile(t, filepath.Join(root, filepath.FromSlash(name))))
		if err != nil {
			t.Errorf("Compile(%s) = %v, want a compiled template to census", name, err)
			continue
		}
		var walk func(n *templatecompile.CompiledNode)
		walk = func(n *templatecompile.CompiledNode) {
			for _, a := range n.Attributes() {
				for _, child := range templatecompile.ChildNodes(a) {
					walk(child)
				}
			}
			first, err := c.NodeAt(n.AQLPath())
			if err != nil {
				t.Errorf("%s: NodeAt(%q) of a compiled node = %v, want the node or one sharing its path", name, n.AQLPath(), err)
				return
			}
			if first == n {
				return
			}
			shared++
			if !allowedChain(first) && allowedChain(n) {
				t.Logf("%s: %s: NodeAt returns a node the template prohibits, %s, while it allows a later one, %s",
					name, n.AQLPath(), first.RMTypeName(), n.RMTypeName())
			}
			if first.Parent() == n.Parent() {
				return
			}
			below++
			if first.RMTypeName() == n.RMTypeName() {
				return
			}
			key := [2]string{name, n.AQLPath()}
			if _, ok := sharedPathTypeExceptions[key]; ok {
				met[key] = true
				return
			}
			t.Errorf("%s: %s: NodeAt returns a %s, but a node below a later alternative or same-id sibling at that path is a %s",
				name, n.AQLPath(), first.RMTypeName(), n.RMTypeName())
		}
		walk(c.Root())
	}
	t.Logf("%d templates: %d nodes share a path with an earlier node, %d of them below an alternative or a same-id sibling", len(names), shared, below)
	if below == 0 {
		t.Errorf("no corpus node sits below an alternative or a same-id sibling, want some: the census checks nothing")
	}
	for key, reason := range sharedPathTypeExceptions {
		if !met[key] {
			t.Errorf("%s: %s is excused (%s), but the census no longer meets it there: drop the exception", key[0], key[1], reason)
		}
	}
}

// allowedChain reports whether the template allows n and each node above
// it: none has occurrences 0..0, and none sits under an attribute the
// template prohibits.
func allowedChain(n *templatecompile.CompiledNode) bool {
	for ; n.Parent() != nil; n = n.Parent() {
		attr := attributeHolding(n.Parent(), n)
		if attr == nil || templatecompile.Prohibited(attr) || !slices.Contains(templatecompile.AllowedChildNodes(attr), n) {
			return false
		}
	}
	return true
}

// attributeHolding returns the attribute of parent that holds child, or nil.
func attributeHolding(parent, child *templatecompile.CompiledNode) *templatecompile.CompiledAttribute {
	for _, a := range parent.Attributes() {
		if slices.Contains(templatecompile.ChildNodes(a), child) {
			return a
		}
	}
	return nil
}

// TestREQ111_NodeAtKeepsFirstAlternativeResolveReachesLater pins, on the
// ACTION of the cross-format test_all_types template, what each lookup
// returns below three ISM_TRANSITION alternatives that share one path:
// NodeAt, by the exact text the compile wrote, returns the careflow_step
// under the first alternative, at0003, whichever alternative the path was
// read from; Resolve, with an alternative's node id as the predicate,
// reaches the careflow_step under that alternative, whose code list is its
// own step.
func TestREQ111_NodeAtKeepsFirstAlternativeResolveReachesLater(t *testing.T) {
	c, err := templatecompile.Compile(mustReadModelFile(t, filepath.Join(fixtures.CrossFormatRoot(), "test_all_types", "template.opt")))
	if err != nil {
		t.Fatalf("Compile(test_all_types): %v", err)
	}
	alternatives := c.AllByRMType("ISM_TRANSITION")
	ids := make([]string, len(alternatives))
	for i, alt := range alternatives {
		ids[i] = alt.NodeID()
	}
	if want := []string{"at0003", "at0004", "at0005"}; !slices.Equal(ids, want) {
		t.Fatalf("AllByRMType(ISM_TRANSITION) node ids = %q, want %q", ids, want)
	}
	shared := alternatives[0].AQLPath()
	for _, alt := range alternatives {
		if alt.AQLPath() != shared {
			t.Fatalf("ISM_TRANSITION %s has path %q, want the path the alternatives share, %q", alt.NodeID(), alt.AQLPath(), shared)
		}
		step, err := c.NodeAt(alt.AQLPath() + "/careflow_step")
		if err != nil {
			t.Fatalf("NodeAt(%s/careflow_step) = %v", alt.AQLPath(), err)
		}
		if got := step.Parent().NodeID(); got != "at0003" {
			t.Errorf("NodeAt(%s/careflow_step), read from ISM_TRANSITION %s, returns the careflow_step of %s, want the first alternative's, at0003",
				alt.AQLPath(), alt.NodeID(), got)
		}

		path := alt.Parent().AQLPath() + "/ism_transition[" + alt.NodeID() + "]/careflow_step/defining_code"
		code, err := c.Resolve(mustParsePath(t, path))
		if err != nil {
			t.Fatalf("Resolve(%s) = %v", path, err)
		}
		cp, ok := code.PrimitiveConstraint().(constraints.CodePhrase)
		if !ok {
			t.Fatalf("Resolve(%s).PrimitiveConstraint() = %#v, want a CodePhrase", path, code.PrimitiveConstraint())
		}
		if want := []string{alt.NodeID()}; !slices.Equal(cp.CodeList, want) {
			t.Errorf("Resolve(%s) reaches a careflow_step coded %q, want its own step, %q", path, cp.CodeList, want)
		}
	}
}
