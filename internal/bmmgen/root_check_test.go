package bmmgen

// REQ-041 § SDK-authored root schemas, Gate — the root check. Every root
// schema under resources/bmm/sdk/ is held, with the pinned schemas it
// includes, to four rules:
//
//  1. every type name a root's declarations use resolves after the merge;
//  2. every class and property a root declares carries, in its
//     documentation, the reason and the upstream reference it rests on;
//  3. each class a root declares again still differs from the class it
//     shadows, so a re-pin that lands the upstream fix fails here until the
//     shadow goes;
//  4. no root's id is a pinned schema's, and a root's header composes the id
//     its file name carries.
//
// The rules run on the vendored roots in TestRootSchemas_REQ041_PassTheRootCheck
// and on scratch schemas in TestRootCheck_REQ041_RefusesWhatARootMustNotDo,
// one case per rule that only that rule refuses.

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// rootIDsIn lists the ids of the schemas in dir, every *.bmm.json file with
// its suffix stripped, sorted.
func rootIDsIn(t *testing.T, dir string) []string {
	t.Helper()
	ids, err := vendoredSchemaRoots(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	return ids
}

// checkRoot runs the root check on the root schema id names under
// resourcesDir/sdk, over the pinned schemas in resourcesDir, and returns what
// the root does wrong, one sentence each. An error is a schema that does not
// load at all.
func checkRoot(ctx context.Context, resourcesDir, id string) ([]string, error) {
	resolver := newSchemaResolver(resourcesDir)
	var problems []string

	// Rule 4: the id.
	pinned, err := vendoredSchemaRoots(resourcesDir)
	if err != nil {
		return nil, err
	}
	if slices.Contains(pinned, id) {
		problems = append(problems, fmt.Sprintf("root %s takes a pinned schema's id", id))
	}
	own, err := loadSchemaFile(filepath.Join(resolver.roots, id+schemaFileSuffix))
	if err != nil {
		return nil, err
	}
	if got := own.SchemaID(); got != id {
		problems = append(problems, fmt.Sprintf("root %s composes the id %q from its header, not its file name", id, got))
	}
	if len(problems) > 0 {
		// The resolver refuses a root that takes a pinned id, and an id the
		// header does not compose cannot be included by it: nothing else
		// can be checked.
		return problems, nil
	}

	merged, err := bmm.LoadAll(id, resolver) //nolint:contextcheck // bmm.LoadAll takes no context
	if err != nil {
		return nil, err
	}
	chain, _, err := loadSchemaChain(ctx, id, resolver)
	if err != nil {
		return nil, err
	}
	included := chain[1:]
	shadowed, err := bmm.LoadAll(id, withoutDeclarations{Resolver: resolver, id: id, header: own}) //nolint:contextcheck // bmm.LoadAll takes no context
	if err != nil {
		return nil, err
	}

	for _, section := range []struct {
		name     string
		own      map[string]bmm.Class
		shadowed map[string]bmm.Class
	}{
		{"class_definitions", own.ClassDefinitions, shadowed.ClassDefinitions},
		{"primitive_types", own.PrimitiveTypes, shadowed.PrimitiveTypes},
	} {
		for _, name := range slices.Sorted(maps.Keys(section.own)) {
			form, err := classForm(section.own[name])
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", section.name, name, err)
			}
			// Rule 1: every type name resolves.
			params := map[string]bool{}
			if defs, ok := form["generic_parameter_defs"].(map[string]any); ok {
				for p := range defs {
					params[p] = true
				}
			}
			for _, typeName := range typeNamesIn(form) {
				_, isClass := merged.ClassDefinitions[typeName]
				_, isPrimitive := merged.PrimitiveTypes[typeName]
				if !isClass && !isPrimitive && !params[typeName] {
					problems = append(problems, fmt.Sprintf("root %s: %s names the type %s, which no schema of the merge declares", id, name, typeName))
				}
			}
			// Rule 2: the reason and the upstream reference.
			if p := noteProblem(section.own[name].Documentation(), included); p != "" {
				problems = append(problems, fmt.Sprintf("root %s: class %s %s", id, name, p))
			}
			props, _ := form["properties"].(map[string]any)
			for _, prop := range slices.Sorted(maps.Keys(props)) {
				doc, _ := props[prop].(map[string]any)["documentation"].(string)
				if p := noteProblem(doc, included); p != "" {
					problems = append(problems, fmt.Sprintf("root %s: property %s.%s %s", id, name, prop, p))
				}
			}
			// Rule 3: a shadow still differs from what it shadows.
			prev, ok := section.shadowed[name]
			if !ok {
				continue
			}
			prevForm, err := classForm(prev)
			if err != nil {
				return nil, fmt.Errorf("%s %s of the includes: %w", section.name, name, err)
			}
			if reflect.DeepEqual(withoutDocumentation(form), withoutDocumentation(prevForm)) {
				problems = append(problems, fmt.Sprintf("root %s: %s is the class it shadows, apart from its documentation; the includes now carry the fix, so remove the shadow", id, name))
			}
		}
	}
	return problems, nil
}

// loadSchemaFile reads one P_BMM schema from path, unmerged.
func loadSchemaFile(path string) (*bmm.Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := bmm.Load(f)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return s, nil
}

// withoutDeclarations serves the root schema id names as its header and
// includes alone, and every other id from the wrapped resolver, so loading it
// gives the classes the root's includes bring before the root shadows any.
type withoutDeclarations struct {
	bmm.Resolver
	id     string
	header *bmm.Schema
}

func (w withoutDeclarations) Resolve(ctx context.Context, id string) (io.ReadCloser, error) {
	if id != w.id {
		return w.Resolver.Resolve(ctx, id)
	}
	includes := map[string]any{}
	for incl := range w.header.Includes {
		includes[incl] = map[string]string{"id": incl}
	}
	b, err := json.Marshal(map[string]any{
		"rm_publisher": w.header.RMPublisher,
		"schema_name":  w.header.SchemaName,
		"rm_release":   w.header.RMRelease,
		"includes":     includes,
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// classForm returns c in its P_BMM JSON form, decoded into maps and lists.
func classForm(c bmm.Class) (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var form map[string]any
	if err := json.Unmarshal(b, &form); err != nil {
		return nil, err
	}
	return form, nil
}

// typeNamesIn lists every type name a class form uses: its ancestors, the
// bounds of its generic parameters, and the types of its properties and of
// its functions' parameters and results.
func typeNamesIn(form map[string]any) []string {
	var names []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				switch k {
				case "documentation", "invariants", "pre_conditions", "post_conditions", "item_names", "item_documentations", "item_values", "aliases":
					continue
				case "type", "root_type", "container_type", "conforms_to_type":
					if s, ok := x[k].(string); ok {
						names = append(names, s)
						continue
					}
				case "ancestors", "generic_parameters":
					items, _ := x[k].([]any)
					for _, item := range items {
						if s, ok := item.(string); ok {
							names = append(names, s)
						} else {
							walk(item)
						}
					}
					continue
				}
				walk(x[k])
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(form)
	return names
}

// withoutDocumentation returns form with every documentation member left out,
// at any depth.
func withoutDocumentation(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			if k != "documentation" {
				out[k] = withoutDocumentation(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = withoutDocumentation(item)
		}
		return out
	default:
		return v
	}
}

// noteProblem says what a root declaration's documentation lacks, or "" when
// it carries both parts of its note: a "Reason: " sentence, then an
// "Upstream: " reference, which is an https URL (a specification pull request
// or ticket) or the id of a schema the root includes (the schema it follows).
func noteProblem(doc string, included []string) string {
	_, rest, ok := strings.Cut(doc, "Reason: ")
	if !ok {
		return `gives no reason (a "Reason: " sentence in its documentation)`
	}
	reason, ref, ok := strings.Cut(rest, "Upstream: ")
	if strings.TrimSpace(reason) == "" {
		return `gives an empty reason`
	}
	if !ok {
		return `gives no upstream reference (an "Upstream: " reference after its reason)`
	}
	fields := strings.Fields(ref)
	if len(fields) == 0 {
		return `gives an empty upstream reference`
	}
	name := strings.TrimSuffix(fields[0], ".")
	if (strings.HasPrefix(name, "https://") && len(name) > len("https://")) || slices.Contains(included, name) {
		return ""
	}
	return fmt.Sprintf("gives the upstream reference %q, which is neither an https URL nor a schema the root includes", name)
}

// TestRootSchemas_REQ041_PassTheRootCheck runs the root check on every root
// schema the SDK authors, and pins that the RM target loads one of them.
func TestRootSchemas_REQ041_PassTheRootCheck(t *testing.T) {
	t.Parallel()
	roots := rootIDsIn(t, filepath.Join(testResources, rootsDirName))
	if len(roots) == 0 {
		t.Fatalf("no root schema under %s/%s, so the root check checked nothing", testResources, rootsDirName)
	}
	if !slices.Contains(roots, TargetRM.RootID) {
		t.Errorf("TargetRM.RootID = %q, which is not a root schema under %s/%s (%v)", TargetRM.RootID, testResources, rootsDirName, roots)
	}
	for _, id := range roots {
		problems, err := checkRoot(t.Context(), testResources, id)
		if err != nil {
			t.Errorf("root check on %s: %v", id, err)
			continue
		}
		for _, p := range problems {
			t.Error(p)
		}
	}
}

// rootCheckBase is the pinned schema of the scratch cases: one primitive type
// and one class.
const rootCheckBase = `{
	"rm_publisher": "test", "schema_name": "base", "rm_release": "1.0.0",
	"schema_revision": "1.0.0.1", "schema_lifecycle_state": "stable",
	"primitive_types": {"String": {"name": "String"}},
	"class_definitions": {"ITEM": {"name": "ITEM", "documentation": "An item.",
		"properties": {"label": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "label", "type": "String"}}}}
}`

// rootCheckRoot returns a scratch root over rootCheckBase whose
// class_definitions are classes and whose schema_name is name.
func rootCheckRoot(name, classes string) string {
	return `{
	"rm_publisher": "test", "schema_name": "` + name + `", "rm_release": "1.0.0",
	"schema_revision": "1.0.0.1", "schema_lifecycle_state": "stable",
	"includes": {"test_base_1.0.0": {"id": "test_base_1.0.0"}},
	"class_definitions": {` + classes + `}
}`
}

// rootCheckItem is a well-formed shadow of ITEM: it adds a property and notes
// both declarations.
const rootCheckItem = `"ITEM": {"name": "ITEM",
	"documentation": "An item. Reason: it lacks a title. Upstream: https://example.org/pull/1.",
	"properties": {
		"label": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "label", "type": "String",
			"documentation": "Its label. Reason: unchanged. Upstream: test_base_1.0.0."},
		"title": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "title", "type": "String",
			"documentation": "Its title. Reason: missing upstream. Upstream: https://example.org/pull/1."}}}`

// TestRootCheck_REQ041_RefusesWhatARootMustNotDo gives the root check one
// scratch root per rule, each breaking that rule alone, and a well-formed root
// it passes, so each case fails for its own rule and no other.
func TestRootCheck_REQ041_RefusesWhatARootMustNotDo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// files are the scratch resources directory beside rootCheckBase,
		// by path; the root checked is test_root_1.0.0 unless id says
		// otherwise.
		files map[string]string
		id    string
		// want is what the one problem reported must say; empty wants none.
		want string
	}{
		{
			name:  "a well-formed root",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", rootCheckItem)},
		},
		{
			name: "a root whose generic class uses its own parameter",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", `"BOX": {"name": "BOX",
				"documentation": "A box. Reason: none upstream. Upstream: https://example.org/pull/2.",
				"generic_parameter_defs": {"T": {"name": "T", "conforms_to_type": "ITEM"}},
				"properties": {"content": {"_type": "P_BMM_SINGLE_PROPERTY_OPEN", "name": "content", "type": "T",
					"documentation": "Its content. Reason: none upstream. Upstream: https://example.org/pull/2."}}}`)},
		},
		{
			name: "rule 1: a type name no schema declares",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
				`"name": "title", "type": "String"`, `"name": "title", "type": "NO_SUCH_TYPE"`, 1))},
			want: "names the type NO_SUCH_TYPE",
		},
		{
			name: "rule 2: a class with no reason",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
				`"An item. Reason: it lacks a title. Upstream: https://example.org/pull/1."`, `"An item."`, 1))},
			want: "class ITEM gives no reason",
		},
		{
			name: "rule 2: a property with no upstream reference",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
				`"Its title. Reason: missing upstream. Upstream: https://example.org/pull/1."`, `"Its title. Reason: missing upstream."`, 1))},
			want: "property ITEM.title gives no upstream reference",
		},
		{
			name: "rule 2: an upstream reference that names nothing",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
				"Upstream: test_base_1.0.0.", "Upstream: somewhere.", 1))},
			want: `property ITEM.label gives the upstream reference "somewhere"`,
		},
		{
			name: "rule 3: a shadow the includes have caught up with",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", `"ITEM": {"name": "ITEM",
				"documentation": "An item. Reason: it lacks a title. Upstream: https://example.org/pull/1.",
				"properties": {"label": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "label", "type": "String",
					"documentation": "Its label. Reason: unchanged. Upstream: test_base_1.0.0."}}}`)},
			want: "ITEM is the class it shadows",
		},
		{
			name: "rule 4: a root with a pinned schema's id",
			files: map[string]string{"sdk/test_base_1.0.0.bmm.json": strings.Replace(rootCheckRoot("base", rootCheckItem),
				`"includes": {"test_base_1.0.0": {"id": "test_base_1.0.0"}},`, "", 1)},
			id:   "test_base_1.0.0",
			want: "root test_base_1.0.0 takes a pinned schema's id",
		},
		{
			name:  "rule 4: a header that composes another id",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("other", rootCheckItem)},
			want:  `composes the id "test_other_1.0.0"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			files := map[string]string{"test_base_1.0.0.bmm.json": rootCheckBase}
			maps.Copy(files, tc.files)
			writeSchemaFiles(t, dir, files)
			id := tc.id
			if id == "" {
				id = "test_root_1.0.0"
			}
			problems, err := checkRoot(t.Context(), dir, id)
			if err != nil {
				t.Fatalf("checkRoot(%s) = %v", id, err)
			}
			switch {
			case tc.want == "" && len(problems) != 0:
				t.Errorf("checkRoot(%s) = %q, want no problem", id, problems)
			case tc.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], tc.want)):
				t.Errorf("checkRoot(%s) = %q, want one problem saying %q", id, problems, tc.want)
			}
		})
	}
}
