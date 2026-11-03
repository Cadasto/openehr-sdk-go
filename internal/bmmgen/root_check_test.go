package bmmgen

// REQ-041 § SDK-authored root schemas, Gate — the root check. Every root
// schema under resources/bmm/sdk/ is held, with the pinned schemas it
// includes, to five rules:
//
//  1. every type name a root's declarations use resolves after the merge;
//  2. every class and property a root declares carries, in its
//     documentation, the reason and the upstream reference it rests on, and
//     so does every entry of its SDK extension keys, in its reason and
//     upstream members; the reference is an https URL, a schema the root
//     includes, or a vendored ITS-XML schema by its path, with or without a
//     line;
//  3. each class a root declares again still differs from the class it
//     shadows, so a re-pin that lands the upstream fix fails here until the
//     shadow goes;
//  4. no root's id is a pinned schema's, and a root's header composes the id
//     its file name carries;
//  5. every top-level member of a root is a P_BMM schema member or
//     sdk_extensions: the loader and the generator both leave any other
//     member aside, so a misspelt extension key would drop its entries
//     unread.
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
	"strconv"
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
// the root does wrong, one sentence each. itsXMLDir is the directory of the
// vendored ITS-XML schemas, which an upstream reference names as
// resources/its-xml/<path>. An error is a schema that does not load at all.
func checkRoot(ctx context.Context, resourcesDir, itsXMLDir, id string) ([]string, error) {
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
	refs := upstreamRefs{included: chain[1:], itsXMLDir: itsXMLDir}
	shadowed, err := bmm.LoadAll(id, withoutDeclarations{Resolver: resolver, id: id, header: own}) //nolint:contextcheck // bmm.LoadAll takes no context
	if err != nil {
		return nil, err
	}

	// Rule 2 for the SDK extension keys: each entry's reason and upstream
	// reference, read from the root's own bytes, as the generator reads them.
	raw, err := os.ReadFile(filepath.Join(resolver.roots, id+schemaFileSuffix))
	if err != nil {
		return nil, err
	}
	extensions, err := parseSDKExtensions(raw)
	if err != nil {
		return nil, fmt.Errorf("root %s: %w", id, err)
	}

	// Rule 5: every top-level member is a P_BMM schema member or the SDK
	// extension keys' member.
	var members map[string]any
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("root %s: %w", id, err)
	}
	schemaMembers := pbmmSchemaMembers()
	for _, m := range slices.Sorted(maps.Keys(members)) {
		if !schemaMembers[m] && m != sdkExtensionsMember {
			problems = append(problems, fmt.Sprintf("root %s: top-level member %q is neither a P_BMM schema member nor %s, so no reader sees what it holds", id, m, sdkExtensionsMember))
		}
	}
	for _, e := range extensions.entries() {
		if p := entryNoteProblem(e, refs); p != "" {
			problems = append(problems, fmt.Sprintf("root %s: %s entry %s %s", id, e.key, e.subject, p))
		}
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
			if p := noteProblem(section.own[name].Documentation(), refs); p != "" {
				problems = append(problems, fmt.Sprintf("root %s: class %s %s", id, name, p))
			}
			props, _ := form["properties"].(map[string]any)
			for _, prop := range slices.Sorted(maps.Keys(props)) {
				doc, _ := props[prop].(map[string]any)["documentation"].(string)
				if p := noteProblem(doc, refs); p != "" {
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

// pbmmSchemaMembers returns the top-level members of a P_BMM schema: the JSON
// names of [bmm.Schema]'s fields.
func pbmmSchemaMembers() map[string]bool {
	members := map[string]bool{}
	for field := range reflect.TypeFor[bmm.Schema]().Fields() {
		if name, _, _ := strings.Cut(field.Tag.Get("json"), ","); name != "" && name != "-" {
			members[name] = true
		}
	}
	return members
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
// "Upstream: " reference that refs accepts.
func noteProblem(doc string, refs upstreamRefs) string {
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
	return refs.problem(ref)
}

// entryNoteProblem says what the note of e, an entry of an SDK extension key,
// lacks, or "" when it carries a reason and an upstream reference that refs
// accepts.
func entryNoteProblem(e extensionEntry, refs upstreamRefs) string {
	switch {
	case strings.TrimSpace(e.reason) == "":
		return `gives no reason (a "reason" member)`
	case strings.TrimSpace(e.upstream) == "":
		return `gives no upstream reference (an "upstream" member)`
	}
	return refs.problem(e.upstream)
}

// itsXMLRefPrefix starts an upstream reference to a vendored ITS-XML schema,
// named by its path in the tree.
const itsXMLRefPrefix = "resources/its-xml/"

// upstreamRefs is what an upstream reference may name besides an https URL (a
// specification pull request or ticket): the id of a schema the root includes
// (the schema it follows), or a vendored ITS-XML schema in itsXMLDir, by its
// path in the tree with an optional line, such as
// resources/its-xml/components/AM/Release-1.4/Template.xsd:115.
type upstreamRefs struct {
	included  []string
	itsXMLDir string
}

// problem says what is wrong with the upstream reference ref, whose first
// field, less a full stop that ends the sentence, names the reference, or ""
// when refs accepts it.
func (refs upstreamRefs) problem(ref string) string {
	fields := strings.Fields(ref)
	if len(fields) == 0 {
		return `gives an empty upstream reference`
	}
	name := strings.TrimSuffix(fields[0], ".")
	switch {
	case strings.HasPrefix(name, "https://") && len(name) > len("https://"):
		return ""
	case slices.Contains(refs.included, name):
		return ""
	case strings.HasPrefix(name, itsXMLRefPrefix):
		if p := refs.itsXMLProblem(strings.TrimPrefix(name, itsXMLRefPrefix)); p != "" {
			return fmt.Sprintf("gives the upstream reference %q, %s", name, p)
		}
		return ""
	}
	return fmt.Sprintf("gives the upstream reference %q, which is neither an https URL, a schema the root includes, nor a vendored ITS-XML schema under %s", name, itsXMLRefPrefix)
}

// itsXMLProblem says what is wrong with ref, a path under the vendored ITS-XML
// schemas with an optional ":line", or "" when the file exists there and holds
// that line. The path is opened through an [os.Root], so it cannot leave the
// schemas' directory.
func (refs upstreamRefs) itsXMLProblem(ref string) string {
	path, line, hasLine := strings.Cut(ref, ":")
	root, err := os.OpenRoot(refs.itsXMLDir)
	if err != nil {
		return fmt.Sprintf("but the ITS-XML schemas cannot be read: %v", err)
	}
	defer func() { _ = root.Close() }() // read only
	body, err := root.ReadFile(path)
	if err != nil {
		return "a file the vendored ITS-XML schemas do not hold"
	}
	if !hasLine {
		return ""
	}
	n, err := strconv.Atoi(line)
	lines := bytes.Count(body, []byte("\n"))
	if len(body) > 0 && body[len(body)-1] != '\n' {
		lines++
	}
	if err != nil || n < 1 || n > lines {
		return fmt.Sprintf("whose line is not one of the file's %d lines", lines)
	}
	return ""
}

// testITSXMLDir is the directory of the vendored ITS-XML schemas, beside the
// pinned BMM schemas.
var testITSXMLDir = filepath.Join(filepath.Dir(testResources), "its-xml")

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
		problems, err := checkRoot(t.Context(), testResources, testITSXMLDir, id)
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

// rootCheckRootWith returns rootCheckRoot(name, classes) with extensions, the
// members of its sdk_extensions object.
func rootCheckRootWith(name, classes, extensions string) string {
	return strings.Replace(rootCheckRoot(name, classes), `"class_definitions": {`,
		`"sdk_extensions": {`+extensions+`},
	"class_definitions": {`, 1)
}

// rootCheckAlias is a well-formed type_aliases entry.
const rootCheckAlias = `"type_aliases": [{"alias": "OLD_ITEM", "class": "ITEM", "reason": "older tools write it.", "upstream": "https://example.org/pull/9"}]`

// rootCheckItem is a well-formed shadow of ITEM: it adds a property and notes
// both declarations.
const rootCheckItem = `"ITEM": {"name": "ITEM",
	"documentation": "An item. Reason: it lacks a title. Upstream: https://example.org/pull/1.",
	"properties": {
		"label": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "label", "type": "String",
			"documentation": "Its label. Reason: unchanged. Upstream: test_base_1.0.0."},
		"title": {"_type": "P_BMM_SINGLE_PROPERTY", "name": "title", "type": "String",
			"documentation": "Its title. Reason: missing upstream. Upstream: https://example.org/pull/1."}}}`

// rootCheckSchemaFile is a vendored ITS-XML schema of the scratch cases, four
// lines long.
const rootCheckSchemaFile = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
	<xs:complexType name="ITEM"/>
</xs:schema>
`

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
			name: "rule 2: an upstream reference to a line of a vendored ITS-XML schema",
			files: map[string]string{
				"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
					"Upstream: test_base_1.0.0.", "Upstream: resources/its-xml/components/Item.xsd:3.", 1)),
				"its-xml/components/Item.xsd": rootCheckSchemaFile,
			},
		},
		{
			name: "rule 2: an upstream reference to a whole vendored ITS-XML schema",
			files: map[string]string{
				"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
					"Upstream: test_base_1.0.0.", "Upstream: resources/its-xml/components/Item.xsd.", 1)),
				"its-xml/components/Item.xsd": rootCheckSchemaFile,
			},
		},
		{
			name: "rule 2: an upstream reference to an ITS-XML schema the tree does not hold",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
				"Upstream: test_base_1.0.0.", "Upstream: resources/its-xml/components/Item.xsd:3.", 1))},
			want: `property ITEM.label gives the upstream reference "resources/its-xml/components/Item.xsd:3"`,
		},
		{
			name: "rule 2: an upstream reference past the last line of an ITS-XML schema",
			files: map[string]string{
				"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
					"Upstream: test_base_1.0.0.", "Upstream: resources/its-xml/components/Item.xsd:5.", 1)),
				"its-xml/components/Item.xsd": rootCheckSchemaFile,
			},
			want: `property ITEM.label gives the upstream reference "resources/its-xml/components/Item.xsd:5"`,
		},
		{
			name: "rule 2: an upstream reference that leaves the ITS-XML schemas",
			files: map[string]string{
				"sdk/test_root_1.0.0.bmm.json": rootCheckRoot("root", strings.Replace(rootCheckItem,
					"Upstream: test_base_1.0.0.", "Upstream: resources/its-xml/../test_base_1.0.0.bmm.json.", 1)),
				"its-xml/components/Item.xsd": rootCheckSchemaFile,
			},
			want: `property ITEM.label gives the upstream reference "resources/its-xml/../test_base_1.0.0.bmm.json"`,
		},
		{
			name:  "a well-formed root with an extension entry",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRootWith("root", rootCheckItem, rootCheckAlias)},
		},
		{
			name: "rule 2: an extension entry with no reason",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRootWith("root", rootCheckItem,
				strings.Replace(rootCheckAlias, `"reason": "older tools write it.", `, "", 1))},
			want: "type_aliases entry OLD_ITEM gives no reason",
		},
		{
			name: "rule 2: an extension entry with no upstream reference",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRootWith("root", rootCheckItem,
				strings.Replace(rootCheckAlias, `, "upstream": "https://example.org/pull/9"`, "", 1))},
			want: "type_aliases entry OLD_ITEM gives no upstream reference",
		},
		{
			name: "rule 2: an extension entry whose upstream reference names nothing",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": rootCheckRootWith("root", rootCheckItem,
				strings.Replace(rootCheckAlias, "https://example.org/pull/9", "somewhere", 1))},
			want: `type_aliases entry OLD_ITEM gives the upstream reference "somewhere"`,
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
		{
			// The loader and the generator both leave the misspelt member
			// aside, so nothing else sees the entries it holds.
			name: "rule 5: a misspelt extension key member",
			files: map[string]string{"sdk/test_root_1.0.0.bmm.json": strings.Replace(rootCheckRootWith("root", rootCheckItem, rootCheckAlias),
				`"sdk_extensions":`, `"sdk_extension":`, 1)},
			want: `top-level member "sdk_extension"`,
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
			problems, err := checkRoot(t.Context(), dir, filepath.Join(dir, "its-xml"), id)
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
