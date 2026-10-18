package simplified

// REQ-053 / REQ-116 — the FLAT decode name index must keep answering to the
// bare (un-predicated) path spelling after REQ-116 Phase 3 started emitting
// name predicates on compiled paths.
//
// Decode composes its lookup key from the incoming FLAT segments, which key
// on archetype id / at-code and never carry a name. Indexing solely by the
// compiled path therefore made every named node's entry unreachable, and
// LOCATABLE.name silently stopped being repopulated for the 9 vendored
// templates that pin names. No FLAT fixture covers a name-pinning template,
// so the whole suite stayed green through the regression — hence this test.

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestBareAQLPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no predicate", "/context/other_context", "/context/other_context"},
		{"id only", "/content[openEHR-EHR-SECTION.adhoc.v1]", "/content[openEHR-EHR-SECTION.adhoc.v1]"},
		{
			"named archetype root",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']",
			"/content[openEHR-EHR-SECTION.adhoc.v1]",
		},
		{"named at-code", "/data/items[at0002,'Causative agent']", "/data/items[at0002]"},
		{
			"several segments",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']/items[at0009,'Reaction details']/value",
			"/content[openEHR-EHR-SECTION.adhoc.v1]/items[at0009]/value",
		},
		{
			// A comma inside the quoted name is literal — the corona golden
			// carries one — and must not end the predicate early.
			"comma inside the name",
			"/content[openEHR-EHR-SECTION.adhoc.v1,'Kontakt zu Menschen, die dort waren']",
			"/content[openEHR-EHR-SECTION.adhoc.v1]",
		},
		{
			"escaped quote inside the name",
			`/items[at0001,'O\'Brien score']/value`,
			"/items[at0001]/value",
		},
		{
			// NamePredicate escapes `\` as `\\`; the scanner must treat the
			// pair as one literal backslash, not as an escape of the closing
			// quote — otherwise the rest of the path is swallowed.
			"escaped trailing backslash",
			`/items[at0001,'a\\']/value`,
			"/items[at0001]/value",
		},
		{
			"escaped backslash-quote pair",
			`/items[at0001,'a\\\'b']/value`,
			"/items[at0001]/value",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bareAQLPath(tc.in); got != tc.want {
				t.Errorf("bareAQLPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A name-pinning template must expose every rubric under the bare spelling
// decode actually looks up, and additionally under the compiled one.
func TestBuildNameIndex_AnswersBareSpelling(t *testing.T) {
	parsed, err := template.ParseFile(fixtures.TemplateOpt("IDCR -  Adverse Reaction List.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	idx := buildNameIndex(c)

	var predicated int
	for key, want := range idx {
		if !strings.Contains(key, ",'") {
			continue
		}
		predicated++
		bare := bareAQLPath(key)
		got, ok := idx[bare]
		if !ok {
			t.Errorf("bare spelling %q missing — decode's lookup key would miss", bare)
			continue
		}
		if got != want {
			t.Errorf("idx[%q] = %q but idx[%q] = %q — rubric disagrees across spellings",
				key, want, bare, got)
		}
	}
	if predicated == 0 {
		t.Fatal("fixture pins no names any more — pick another name-pinning template")
	}
}

// buildNameIndexReference is buildNameIndex as it stood before it read the
// compiled template without copying: it reads each term through Term, which
// copies the term's items, and the tree through Attributes and Children,
// which copy their slices. Decode repopulates LOCATABLE.name from the
// index, so buildNameIndex must give the same index as this copy.
func buildNameIndexReference(c *templatecompile.Compiled) map[string]string {
	names := make(map[string]string)
	lang := c.Language()
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		if id := n.NodeID(); id != "" {
			var rubric string
			if t, ok := n.Term(id, lang); ok {
				rubric = t.Items["text"]
			}
			name := n.NodeName()
			if name == "" {
				name = rubric
			}
			if name != "" {
				path := n.AQLPath()
				names[path] = name
				if bare := bareAQLPath(path); bare != path {
					if prev, taken := names[bare]; taken && prev != name {
						names[bare] = rubric
					} else {
						names[bare] = name
					}
				}
			}
		}
		for _, a := range n.Attributes() {
			for _, ch := range a.Children() {
				walk(ch)
			}
		}
	}
	if root := c.Root(); root != nil {
		walk(root)
	}
	return names
}

// minNameIndexTemplates is a floor on how many corpus templates
// TestBuildNameIndexMatchesReference compares, so the comparison cannot
// shrink unnoticed when templates stop parsing or compiling. 61 of the 62
// corpus templates compile today.
const minNameIndexTemplates = 61

// buildNameIndex must give the index the earlier version gave on every
// template of the corpus that compiles, entry for entry.
func TestBuildNameIndexMatchesReference(t *testing.T) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	var compared int
	for _, ref := range refs {
		parsed, err := template.ParseFile(ref.Path)
		if err != nil {
			continue
		}
		c, err := templatecompile.Compile(parsed)
		if err != nil {
			continue
		}
		compared++
		got, want := buildNameIndex(c), buildNameIndexReference(c)
		if maps.Equal(got, want) {
			continue
		}
		for _, k := range slices.Sorted(maps.Keys(want)) {
			if g, ok := got[k]; !ok || g != want[k] {
				t.Errorf("%s: buildNameIndex()[%q] = %q (present %t), earlier version gave %q", ref.Name, k, g, ok, want[k])
			}
		}
		for _, k := range slices.Sorted(maps.Keys(got)) {
			if _, ok := want[k]; !ok {
				t.Errorf("%s: buildNameIndex() has %q = %q, earlier version has no such key", ref.Name, k, got[k])
			}
		}
	}
	if compared < minNameIndexTemplates {
		t.Fatalf("compared the name index of %d of the %d corpus templates, want at least %d", compared, len(refs), minNameIndexTemplates)
	}
	t.Logf("compared the name index of %d of the %d corpus templates", compared, len(refs))
}
