package bmm_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// fixture is one in-memory schema of an include graph. Its resolver id
// is "<name>_1.0.0". A non-empty form declares class X and primitive
// type P, each documented as form; includes name other fixtures.
type fixture struct {
	form     string
	includes []string
}

func fixtureID(name string) string { return name + "_1.0.0" }

// fixtureResolver turns an include graph into a resolver over minimal
// P_BMM documents.
func fixtureResolver(graph map[string]fixture) bmm.MapResolver {
	r := make(bmm.MapResolver, len(graph))
	for name, f := range graph {
		var b strings.Builder
		fmt.Fprintf(&b, `{"schema_name":%q,"rm_release":"1.0.0"`, name)
		if len(f.includes) > 0 {
			refs := make([]string, 0, len(f.includes))
			for _, inc := range f.includes {
				refs = append(refs, fmt.Sprintf(`%q:{"id":%q}`, fixtureID(inc), fixtureID(inc)))
			}
			fmt.Fprintf(&b, `,"includes":{%s}`, strings.Join(refs, ","))
		}
		if f.form != "" {
			fmt.Fprintf(&b, `,"primitive_types":{"P":{"name":"P","documentation":%q}}`, f.form)
			fmt.Fprintf(&b, `,"class_definitions":{"X":{"name":"X","documentation":%q}}`, f.form)
		}
		b.WriteString("}")
		r[fixtureID(name)] = []byte(b.String())
	}
	return r
}

// mergedForms reads back the documentation LoadAll kept for class X and
// for primitive type P, so each test checks both kinds of entry.
var mergedForms = []struct {
	kind    string
	entries func(*bmm.Schema) map[string]bmm.Class
	name    string
}{
	{kind: "class", entries: func(s *bmm.Schema) map[string]bmm.Class { return s.ClassDefinitions }, name: "X"},
	{kind: "primitive type", entries: func(s *bmm.Schema) map[string]bmm.Class { return s.PrimitiveTypes }, name: "P"},
}

// TestLoadAll_includeDiamondKeepsDescendantForm pins REQ-045: when two
// includes of a schema meet again with differing forms of a class or
// primitive type, and the schema that declared one form transitively
// includes the schema that declared the other, LoadAll keeps the
// descendant's form instead of failing. Identical forms merge once.
func TestLoadAll_includeDiamondKeepsDescendantForm(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		graph map[string]fixture
		want  string
	}{
		{
			name: "descendant include merged last",
			graph: map[string]fixture{
				"root": {includes: []string{"am", "rm"}},
				"am":   {includes: []string{"base"}},
				"rm":   {form: "from rm", includes: []string{"base"}},
				"base": {form: "from base"},
			},
			want: "from rm",
		},
		{
			name: "descendant include merged first",
			graph: map[string]fixture{
				"root": {includes: []string{"rm", "zam"}},
				"rm":   {form: "from rm", includes: []string{"base"}},
				"zam":  {includes: []string{"base"}},
				"base": {form: "from base"},
			},
			want: "from rm",
		},
		{
			name: "descendant includes the ancestor through an intermediate schema",
			graph: map[string]fixture{
				"root": {includes: []string{"am", "rm"}},
				"am":   {includes: []string{"base"}},
				"rm":   {form: "from rm", includes: []string{"mid"}},
				"mid":  {includes: []string{"base"}},
				"base": {form: "from base"},
			},
			want: "from rm",
		},
		{
			name: "descendant form reached through a further include",
			graph: map[string]fixture{
				"root": {includes: []string{"am", "ehr"}},
				"am":   {includes: []string{"base"}},
				"ehr":  {includes: []string{"rm"}},
				"rm":   {form: "from rm", includes: []string{"base"}},
				"base": {form: "from base"},
			},
			want: "from rm",
		},
		{
			name: "identical form reached through a shared include",
			graph: map[string]fixture{
				"root": {includes: []string{"base", "lang"}},
				"lang": {includes: []string{"base"}},
				"base": {form: "from base"},
			},
			want: "from base",
		},
		{
			name: "identical forms from unrelated includes",
			graph: map[string]fixture{
				"root": {includes: []string{"p1", "p2"}},
				"p1":   {form: "same"},
				"p2":   {form: "same"},
			},
			want: "same",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := bmm.LoadAll(fixtureID("root"), fixtureResolver(tc.graph))
			if err != nil {
				t.Fatalf("LoadAll(%q) error = %v, want nil", fixtureID("root"), err)
			}
			for _, m := range mergedForms {
				c, ok := m.entries(s)[m.name]
				if !ok {
					t.Errorf("LoadAll(%q): %s %s missing from the merged schema", fixtureID("root"), m.kind, m.name)
					continue
				}
				if got := c.Documentation(); got != tc.want {
					t.Errorf("LoadAll(%q): %s %s documentation = %q, want %q", fixtureID("root"), m.kind, m.name, got, tc.want)
				}
			}
		})
	}
}

// TestLoadAll_unrelatedIncludesConflict pins REQ-045: two forms of a class
// or primitive type that differ, from schemas neither of which includes
// the other, have no winner, so LoadAll returns ErrSchemaConflict however
// deep the include paths that bring them together.
func TestLoadAll_unrelatedIncludesConflict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		graph map[string]fixture
	}{
		{
			name: "sibling includes",
			graph: map[string]fixture{
				"root": {includes: []string{"p1", "p2"}},
				"p1":   {form: "from p1"},
				"p2":   {form: "from p2"},
			},
		},
		{
			name: "unrelated schemas behind further includes",
			graph: map[string]fixture{
				"root": {includes: []string{"x", "y"}},
				"x":    {includes: []string{"p1"}},
				"y":    {includes: []string{"p2"}},
				"p1":   {form: "from p1"},
				"p2":   {form: "from p2"},
			},
		},
		{
			// c shadows a's form, but b declares the same form as a and
			// c does not include b.
			name: "descendant of only one of two identical forms",
			graph: map[string]fixture{
				"root": {includes: []string{"a", "b", "c"}},
				"a":    {form: "same"},
				"b":    {form: "same"},
				"c":    {form: "from c", includes: []string{"a"}},
			},
		},
		{
			name: "identical forms merged before the descendant of only one of them",
			graph: map[string]fixture{
				"root": {includes: []string{"c", "m"}},
				"m":    {includes: []string{"a", "b"}},
				"a":    {form: "same"},
				"b":    {form: "same"},
				"c":    {form: "from c", includes: []string{"a"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := bmm.LoadAll(fixtureID("root"), fixtureResolver(tc.graph))
			if !errors.Is(err, bmm.ErrSchemaConflict) {
				t.Fatalf("LoadAll(%q) error = %v, want errors.Is(err, bmm.ErrSchemaConflict)", fixtureID("root"), err)
			}
		})
	}
}

// overlayResolver serves the schemas in overlay and reads every other id
// from the pinned BMM files.
type overlayResolver struct {
	overlay bmm.MapResolver
	pinned  bmm.FSResolver
}

func (r overlayResolver) Resolve(ctx context.Context, id string) (io.ReadCloser, error) {
	if _, ok := r.overlay[id]; ok {
		return r.overlay.Resolve(ctx, id)
	}
	return r.pinned.Resolve(ctx, id)
}

// TestLoadAll_amAndRMShareBase pins REQ-045 on the pinned corpus: a root
// that includes both openehr_am_1.4.0 and openehr_rm_1.2.0 reaches
// openehr_base_1.3.0 twice, and the resource classes BASE and RM both
// declare take RM's form, because RM includes BASE.
func TestLoadAll_amAndRMShareBase(t *testing.T) {
	t.Parallel()
	pinned := bmm.FSResolver{Root: "../../" + bmm.DefaultResourcesDir}
	root := `{
		"schema_name": "root",
		"rm_release": "1.0.0",
		"includes": {
			"openehr_am_1.4.0": { "id": "openehr_am_1.4.0" },
			"openehr_rm_1.2.0": { "id": "openehr_rm_1.2.0" }
		}
	}`
	r := overlayResolver{overlay: bmm.MapResolver{"root_1.0.0": []byte(root)}, pinned: pinned}
	merged, err := bmm.LoadAll("root_1.0.0", r)
	if err != nil {
		t.Fatalf("LoadAll(root over AM 1.4 and RM) error = %v, want nil", err)
	}
	rm := loadPinned(t, pinned, "openehr_rm_1.2.0")
	base := loadPinned(t, pinned, "openehr_base_1.3.0")
	shared := 0
	for _, name := range slices.Sorted(maps.Keys(rm.ClassDefinitions)) {
		baseForm, inBase := base.ClassDefinitions[name]
		if !inBase {
			continue
		}
		rmForm := rm.ClassDefinitions[name]
		if reflect.DeepEqual(baseForm, rmForm) {
			continue
		}
		shared++
		if got := merged.ClassDefinitions[name]; !reflect.DeepEqual(got, rmForm) {
			t.Errorf("merged %s is not openehr_rm_1.2.0's form", name)
		}
	}
	// The pinned corpus declares AUTHORED_RESOURCE, CODE_PHRASE,
	// RESOURCE_DESCRIPTION, RESOURCE_DESCRIPTION_ITEM and
	// TRANSLATION_DETAILS in both RM and BASE with differing forms; a count
	// of zero would make the loop above check nothing.
	if shared == 0 {
		t.Fatal("no class differs between openehr_rm_1.2.0 and openehr_base_1.3.0; the test checks nothing")
	}
}

func loadPinned(t *testing.T, r bmm.FSResolver, id string) *bmm.Schema {
	t.Helper()
	rc, err := r.Resolve(t.Context(), id)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", id, err)
	}
	defer func() { _ = rc.Close() }()
	s, err := bmm.Load(rc)
	if err != nil {
		t.Fatalf("Load(%q): %v", id, err)
	}
	return s
}

// TestLoadAll_conflictErrorIsReproducible pins REQ-045: when two includes
// disagree on several classes, LoadAll reports the same one on every run.
func TestLoadAll_conflictErrorIsReproducible(t *testing.T) {
	t.Parallel()
	classes := func(form string) string {
		defs := make([]string, 0, 5)
		for _, name := range []string{"A", "B", "C", "D", "E"} {
			defs = append(defs, fmt.Sprintf(`%q:{"name":%q,"documentation":%q}`, name, name, form))
		}
		return strings.Join(defs, ",")
	}
	r := bmm.MapResolver{
		"root_1.0.0": []byte(`{"schema_name":"root","rm_release":"1.0.0","includes":{"p1_1.0.0":{"id":"p1_1.0.0"},"p2_1.0.0":{"id":"p2_1.0.0"}}}`),
		"p1_1.0.0":   []byte(`{"schema_name":"p1","rm_release":"1.0.0","class_definitions":{` + classes("from p1") + `}}`),
		"p2_1.0.0":   []byte(`{"schema_name":"p2","rm_release":"1.0.0","class_definitions":{` + classes("from p2") + `}}`),
	}
	var first string
	for run := range 20 {
		_, err := bmm.LoadAll("root_1.0.0", r)
		if !errors.Is(err, bmm.ErrSchemaConflict) {
			t.Fatalf("run %d: LoadAll error = %v, want errors.Is(err, bmm.ErrSchemaConflict)", run, err)
		}
		if run == 0 {
			first = err.Error()
			continue
		}
		if got := err.Error(); got != first {
			t.Fatalf("run %d: LoadAll error = %q, want the first run's %q", run, got, first)
		}
	}
}
