package instance_test

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// compileFixtureMode compiles the corpus template name, with the implicit
// RM attributes the compile step adds for every mandatory field the OPT
// leaves out, or without them.
func compileFixtureMode(t *testing.T, name string, implicit bool) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt(name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	var opts []templatecompile.Option
	if !implicit {
		opts = append(opts, templatecompile.WithoutImplicitAttributes())
	}
	c, err := templatecompile.Compile(opt, opts...)
	if err != nil {
		t.Fatalf("Compile %s (implicit=%t): %v", name, implicit, err)
	}
	return c
}

// fixedOptions are Options whose output depends on the template alone: a
// fixed clock and uids from a counter that starts again on every call.
func fixedOptions(policy instance.Policy) instance.Options {
	uid := 0
	return instance.Options{
		Policy:    policy,
		Territory: "NL",
		Composer:  testComposer(),
		Now:       time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		UIDSource: func() *rm.HierObjectID {
			uid++
			return &rm.HierObjectID{Value: "uid-" + strconv.Itoa(uid)}
		},
	}
}

// TestREQ107_CompileModesGenerateTheSameInstance is the REQ-107 Visit rule
// check that a mandatory attribute a compiled node does not carry, as under
// WithoutImplicitAttributes, is visited as if the node carried it with no
// OPT child: a corpus template compiled with and without the implicit
// attributes generates the same canonical JSON, under both policies. A
// locatable's name, which the identity rule stamps from its node's term,
// is the same in both.
func TestREQ107_CompileModesGenerateTheSameInstance(t *testing.T) {
	for _, name := range []string{"vital_signs"} {
		implicit := compileFixtureMode(t, name, true)
		uncarried := compileFixtureMode(t, name, false)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(fmt.Sprintf("%s/%v", name, policy), func(t *testing.T) {
				var out [2][]byte
				for i, c := range []*templatecompile.Compiled{implicit, uncarried} {
					v, err := instance.Generate(t.Context(), c, fixedOptions(policy))
					if err != nil {
						t.Fatalf("Generate (implicit=%t): %v", i == 0, err)
					}
					if out[i], err = canjson.MarshalIndent(v, "", " "); err != nil {
						t.Fatalf("MarshalIndent (implicit=%t): %v", i == 0, err)
					}
				}
				if !bytes.Equal(out[0], out[1]) {
					t.Errorf("Generate(%s, %v) differs between the compile modes:\n%s", name, policy, firstDifference(out[0], out[1]))
				}
			})
		}
	}
}

// firstDifference shows the first line on which a and b differ, with the
// line before it, so a failure names the attribute.
func firstDifference(a, b []byte) string {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := range min(len(la), len(lb)) {
		if !bytes.Equal(la[i], lb[i]) {
			prev := ""
			if i > 0 {
				prev = string(la[i-1])
			}
			return fmt.Sprintf("line %d, after %s\n  with implicit attributes:    %s\n  without implicit attributes: %s", i+1, prev, la[i], lb[i])
		}
	}
	return fmt.Sprintf("one output is a prefix of the other (%d and %d lines)", len(la), len(lb))
}

// TestREQ107_ArchetypeRootIsNamedByItsNodeTerm is the REQ-107 Identity
// check that an archetype root the OPT gives no name constraint is named by
// the text of its own node's term (at0000), as any other node is, not by
// its RM type: the vital_signs composition and each observation in its
// content, in both compile modes and under both policies.
func TestREQ107_ArchetypeRootIsNamedByItsNodeTerm(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		c := compileFixtureMode(t, "vital_signs", implicit)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(fmt.Sprintf("implicit=%t/%v", implicit, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, fixedOptions(policy))
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				checkRootName(t, c, comp)
				if len(comp.Content) == 0 {
					t.Fatal("COMPOSITION.content is empty; vital_signs pins observations")
				}
				for _, item := range comp.Content {
					loc, ok := item.(rm.Locatable)
					if !ok {
						t.Fatalf("COMPOSITION.content member %T is not a locatable", item)
					}
					checkRootName(t, c, loc)
				}
			})
		}
	}
}

// checkRootName checks that the archetype root loc is named by the text of
// the at0000 term of its archetype in c.
func checkRootName(t *testing.T, c *templatecompile.Compiled, loc rm.Locatable) {
	t.Helper()
	details := loc.GetArchetypeDetails()
	if details == nil {
		t.Fatalf("%T has no archetype_details; want an archetype root", loc)
	}
	archetypeID := details.ArchetypeID.Value
	nodes := c.AllByArchetypeID(archetypeID)
	if len(nodes) == 0 {
		t.Fatalf("the compiled template has no node for %s", archetypeID)
	}
	term, ok := nodes[0].Term(nodes[0].NodeID(), "")
	want := term.Items["text"]
	if !ok || want == "" {
		t.Fatalf("%s has no text for its node %q; the check needs one", archetypeID, nodes[0].NodeID())
	}
	name := loc.GetName()
	if name == nil || rm.IsTypedNil(name) {
		t.Fatalf("%s has no name, want %q", archetypeID, want)
	}
	if got := name.GetValue(); got != want {
		t.Errorf("%s is named %q, want %q, the text of its %s term", archetypeID, got, want, nodes[0].NodeID())
	}
}
