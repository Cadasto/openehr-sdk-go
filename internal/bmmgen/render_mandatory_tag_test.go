package bmmgen

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ052_MandatoryFieldJSONTag pins the JSON tag the generator gives a
// mandatory field of each Go kind, on real RM and AOM 1.4 fields. A list,
// map, pointer or any field takes omitzero; an interface or type-parameter
// field takes omitempty; a field held by value keeps the bare tag. The flat
// wire struct of a class whose ancestor bears a marshaler copies its fields
// through the same rule, so one of its rows is pinned too.
//
// REQ-052 § An absent mandatory member: a mandatory member with no value is
// left out on encode, never written as null.
func TestREQ052_MandatoryFieldJSONTag(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	aomPlan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}

	for _, tc := range []struct {
		name     string
		plan     *Plan
		fileBase string
		render   func(*Plan, *PlannedFile) ([]byte, error)
		field    string
	}{
		{
			name: "slice, container property: CLUSTER.items", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "Items []Item `json:\"items,omitzero\"`",
		},
		{
			name: "slice of an interface: CONTRIBUTION.versions", plan: rmPlan, render: RenderFile,
			fileBase: "common_change_control",
			field:    "Versions []ObjectRefLike `json:\"versions,omitzero\"`",
		},
		{
			name: "map, generic property over Hash: RESOURCE_DESCRIPTION.details", plan: rmPlan, render: RenderFile,
			fileBase: "common_resource",
			field:    "Details map[string]ResourceDescriptionItem `json:\"details,omitzero\"`",
		},
		{
			name: "pointer that breaks a cycle: ARCHETYPE.ontology", plan: aomPlan, render: RenderFile,
			fileBase: "archetype",
			field:    "Ontology *ArchetypeOntology `json:\"ontology,omitzero\"`",
		},
		{
			name: "any: EXPR_LEAF.item", plan: aomPlan, render: RenderFile,
			fileBase: "archetype_assertion",
			field:    "Item any `json:\"item,omitzero\"`",
		},
		{
			name: "narrow Like interface: ELEMENT.name", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "Name DVTextLike `json:\"name,omitempty\"`",
		},
		{
			name: "abstract class interface: RESOURCE_DESCRIPTION.parent_resource", plan: rmPlan, render: RenderFile,
			fileBase: "common_resource",
			field:    "ParentResource AuthoredResource `json:\"parent_resource,omitempty\"`",
		},
		{
			name: "type parameter: EVENT.data", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_history",
			field:    "Data T `json:\"data,omitempty\"`",
		},
		{
			name: "generic struct by value: OBSERVATION.data", plan: rmPlan, render: RenderFile,
			fileBase: "composition_content_entry",
			field:    "Data History[ItemStructure] `json:\"data\"`",
		},
		{
			name: "struct by value: CONTRIBUTION.uid", plan: rmPlan, render: RenderFile,
			fileBase: "common_change_control",
			field:    "UID HierObjectID `json:\"uid\"`",
		},
		{
			name: "primitive by value: ELEMENT.archetype_node_id", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "ArchetypeNodeID string `json:\"archetype_node_id\"`",
		},
		{
			name: "interface in a flat wire struct: VERSIONED_FOLDER.owner_id", plan: rmPlan, render: RenderMarshalJSONFile,
			fileBase: "common_directory",
			field:    "OwnerID ObjectRefLike `json:\"owner_id,omitempty\"`",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file *PlannedFile
			for _, f := range tc.plan.Files {
				if f.FileBase == tc.fileBase {
					file = f
					break
				}
			}
			if file == nil {
				t.Fatalf("%s file not in plan", tc.fileBase)
			}
			got, err := tc.render(tc.plan, file)
			if err != nil {
				t.Fatalf("render %s: %v", tc.fileBase, err)
			}
			if !fieldDecl(tc.field).Match(got) {
				t.Errorf("the rendered %s source does not declare the field %q", tc.fileBase, tc.field)
			}
		})
	}
}

// TestREQ052_MandatoryJSONTagByGoType pins [mandatoryJSONTag] on each Go type
// shape it tells apart, including the ones no pinned schema emits today.
func TestREQ052_MandatoryJSONTagByGoType(t *testing.T) {
	for _, tc := range []struct {
		goType      string
		polymorphic bool
		want        string
	}{
		{goType: "[]Item", want: "`json:\"m,omitzero\"`"},
		{goType: "map[string]string", want: "`json:\"m,omitzero\"`"},
		{goType: "*ArchetypeOntology", want: "`json:\"m,omitzero\"`"},
		{goType: "*Archetype", polymorphic: true, want: "`json:\"m,omitzero\"`"},
		{goType: "any", want: "`json:\"m,omitzero\"`"},
		{goType: "any /* TODO: unmapped X */", want: "`json:\"m,omitzero\"`"},
		{goType: "any /* TODO: unknown type X */", want: "`json:\"m,omitzero\"`"},
		{goType: "DVTextLike", polymorphic: true, want: "`json:\"m,omitempty\"`"},
		{goType: "T", polymorphic: true, want: "`json:\"m,omitempty\"`"},
		{goType: "HierObjectID", want: "`json:\"m\"`"},
		{goType: "History[ItemStructure]", want: "`json:\"m\"`"},
		{goType: "string", want: "`json:\"m\"`"},
	} {
		if got := mandatoryJSONTag("m", tc.goType, tc.polymorphic); got != tc.want {
			t.Errorf("mandatoryJSONTag(%q, %q, %t) = %s, want %s", "m", tc.goType, tc.polymorphic, got, tc.want)
		}
	}
}

// TestREQ052_IsOwnTypeParameter pins [isOwnTypeParameter], which makes a
// mandatory single property typed by its class's own generic parameter
// polymorphic, so its field takes omitempty. No field generated today
// depends on it alone. These rows hold what the helper returns, and
// [TestREQ052_RenderFieldPolymorphicMandatoryTag] holds that [renderField]
// asks it.
func TestREQ052_IsOwnTypeParameter(t *testing.T) {
	// holder builds a class named name with a mandatory property "item"
	// typed T, declaring the generic parameters params.
	holder := func(name string, params ...string) *bmm.SimpleClass {
		item := &bmm.SingleProperty{TypeName: "T"}
		item.Name = "item"
		item.IsMandatory = true
		c := &bmm.SimpleClass{}
		c.Name = name
		c.Properties = map[string]bmm.Property{"item": item}
		for _, p := range params {
			if c.GenericParameterDefs == nil {
				c.GenericParameterDefs = map[string]*bmm.GenericParameterDef{}
			}
			c.GenericParameterDefs[p] = &bmm.GenericParameterDef{Name: p}
		}
		return c
	}

	for _, tc := range []struct {
		owner *bmm.SimpleClass
		want  bool
	}{
		{owner: holder("GENERIC_HOLDER", "T"), want: true},
		{owner: holder("PLAIN_HOLDER"), want: false},
	} {
		typeName := tc.owner.Properties["item"].(*bmm.SingleProperty).TypeName
		if got := isOwnTypeParameter(tc.owner, typeName); got != tc.want {
			t.Errorf("isOwnTypeParameter(%s with generic parameters %v, %q) = %t, want %t",
				tc.owner.Name, slices.Sorted(maps.Keys(tc.owner.GenericParameterDefs)), typeName, got, tc.want)
		}
	}
}

// TestREQ052_GenericRootIsInterface pins [genericRootIsInterface] on the
// real RM plan. A mandatory generic property whose root is EVENT is held as
// a Go interface and takes omitempty; one whose root is a generic struct
// (HISTORY) or a container (Hash) is not. No field generated today depends
// on it alone. These rows hold what the helper returns, and
// [TestREQ052_RenderFieldPolymorphicMandatoryTag] holds that [renderField]
// asks it.
func TestREQ052_GenericRootIsInterface(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(t.Context(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}

	for _, tc := range []struct {
		root string
		want bool
	}{
		{root: "EVENT", want: true},
		{root: "HISTORY", want: false},
		{root: "Hash", want: false},
	} {
		gt := &bmm.GenericType{
			RootType:          tc.root,
			GenericParameters: []bmm.Type{&bmm.SimpleType{TypeName: "ITEM_STRUCTURE"}},
		}
		if got := genericRootIsInterface(rmPlan, gt); got != tc.want {
			t.Errorf("genericRootIsInterface(RM plan, %s<ITEM_STRUCTURE>) = %t, want %t", tc.root, got, tc.want)
		}
	}
}

// TestREQ052_RenderFieldPolymorphicMandatoryTag renders a mandatory field
// with [renderField] itself, for the two property kinds whose polymorphic
// check lives in renderField rather than in [mandatoryJSONTag]: a
// P_BMM_SINGLE_PROPERTY that names its class's own generic parameter, and a
// P_BMM_GENERIC_PROPERTY whose root class is held as a Go interface. Each
// field takes omitempty, so a typed-nil pointer it holds, which encodes as
// null, is left out. No field generated today takes either path, so `make
// codegen-check` shows no diff when renderField stops asking.
//
// REQ-043: § Mapping rules, Property → Go field. REQ-052 § An absent
// mandatory member: a mandatory member with no value is left out on encode,
// never written as null.
func TestREQ052_RenderFieldPolymorphicMandatoryTag(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(t.Context(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}

	// item is a mandatory single property typed T, on a class that declares
	// the generic parameter T.
	item := &bmm.SingleProperty{TypeName: "T"}
	item.Name = "item"
	item.IsMandatory = true
	itemHolder := &bmm.SimpleClass{}
	itemHolder.Name = "GENERIC_HOLDER"
	itemHolder.GenericParameterDefs = map[string]*bmm.GenericParameterDef{"T": {Name: "T"}}
	itemHolder.Properties = map[string]bmm.Property{"item": item}

	// event is a mandatory generic property over EVENT<ITEM_STRUCTURE>, which
	// the RM plan holds as the Go interface Event.
	event := &bmm.GenericProperty{TypeDef: &bmm.GenericType{
		RootType:          "EVENT",
		GenericParameters: []bmm.Type{&bmm.SimpleType{TypeName: "ITEM_STRUCTURE"}},
	}}
	event.Name = "event"
	event.IsMandatory = true
	eventHolder := &bmm.SimpleClass{}
	eventHolder.Name = "EVENT_HOLDER"
	eventHolder.Properties = map[string]bmm.Property{"event": event}

	for _, tc := range []struct {
		name  string
		plan  *Plan
		owner *bmm.SimpleClass
		prop  bmm.Property
		want  string
	}{
		{
			name: "single property typed by its class's own generic parameter",
			plan: &Plan{}, owner: itemHolder, prop: item,
			want: "\tItem T `json:\"item,omitempty\"`\n",
		},
		{
			name: "generic property over EVENT<ITEM_STRUCTURE>",
			plan: rmPlan, owner: eventHolder, prop: event,
			want: "\tEvent Event `json:\"event,omitempty\"`\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderField(tc.plan, tc.owner, tc.owner.Name, tc.prop)
			if err != nil {
				t.Fatalf("renderField(%s.%s): %v", tc.owner.Name, tc.prop.PropertyName(), err)
			}
			if got != tc.want {
				t.Errorf("renderField(%s.%s) = %q, want %q", tc.owner.Name, tc.prop.PropertyName(), got, tc.want)
			}
		})
	}
}

// TestREQ052_GeneratedNilableFieldsCarryOmit type-checks every package that
// holds generated structs with JSON tags and requires each JSON-tagged field
// whose type can hold nil to carry the omit option its kind needs, so that
// its nil value is left out on encode and never written as null: omitempty
// on an interface or type-parameter field, so that one holding a nil
// pointer is left out too, and omitempty or omitzero on a pointer, slice,
// map or any field.
//
// [mandatoryJSONTag] tells field shapes apart by the text of the rendered Go
// type and gives a shape it does not know the bare tag, and the polymorphic
// flag its callers pass covers only some interface fields. This test reads
// each field's type from the type checker instead, so a new nilable field
// shape the generator misses fails here rather than encoding null. Every
// struct type in a *_gen.go file counts: the RM and AOM structs, the
// jsonWire* structs the marshalers encode through, and the anonymous
// wrapper structs inside generated functions.
//
// REQ-052 § An absent mandatory member: a mandatory member with no value is
// left out on encode, never written as null.
func TestREQ052_GeneratedNilableFieldsCarryOmit(t *testing.T) {
	root := moduleRootDir(t)
	modulePath, err := goModulePath(root)
	if err != nil {
		t.Fatalf("goModulePath(%s): %v", root, err)
	}
	dirs, err := generatedJSONTaggedDirs(root)
	if err != nil {
		t.Fatalf("generatedJSONTaggedDirs(%s): %v", root, err)
	}
	// The walk must find the two packages known to hold generated JSON
	// structs, or a mis-rooted or broken walk would leave nothing to check.
	for _, want := range []string{"openehr/aom/aom14", "openehr/rm"} {
		if !slices.Contains(dirs, want) {
			t.Fatalf("generatedJSONTaggedDirs(%s) = %v, want it to include %s", root, dirs, want)
		}
	}

	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			gen, info, err := typeCheckDir(fset, imp, modulePath+"/"+dir, filepath.Join(root, filepath.FromSlash(dir)))
			if err != nil {
				t.Fatal(err)
			}
			audit := auditNilableJSONOmit(fset, gen, info)
			t.Logf("%s: %d generated files, %d struct types, %d JSON-tagged fields, %d nilable",
				dir, len(gen), audit.structs, audit.tagged, audit.nilable)
			if audit.nilable == 0 {
				t.Errorf("%s: no JSON-tagged field of a nilable type found in %d generated files, so the audit checked nothing", dir, len(gen))
			}
			for _, f := range audit.missing {
				t.Errorf("%s: a nil value is written as JSON null; a field of kind %s must carry %s, see mandatoryJSONTag in render.go", f, f.rule.kind, strings.Join(f.rule.accepts, " or "))
			}
		})
	}
}

// TestREQ052_AuditNilableJSONOmitReportsBareTags runs the check behind
// [TestREQ052_GeneratedNilableFieldsCarryOmit] on a small source with one
// bare-tagged field of each nilable kind, a named slice type among them,
// inside a generic struct, a jsonWire* struct and an anonymous struct in a
// function body. Each one is reported, and so is an interface or
// type-parameter field that carries omitzero only, a named interface with no
// methods and an interface literal among them. A pointer, slice, map or
// any field that carries either omit option, an interface or type-parameter
// field that carries omitempty, and the fields held by value, with no json
// key or left out of JSON with "-" are not.
func TestREQ052_AuditNilableJSONOmitReportsBareTags(t *testing.T) {
	const src = `package fixture

type Item struct{ Name string }

// Items is a named slice type: its name hides the slice from the text.
type Items []Item

type Shape interface{ Area() float64 }

// Marker is a named interface with no methods: a Go interface, not any.
type Marker interface{}

type Holder[T any] struct {
	Pointer   *Item           "json:\"pointer\""
	Slice     []Item          "json:\"slice\""
	Named     Items           "json:\"named\""
	Map       map[string]Item "json:\"map\""
	Interface Shape           "json:\"interface\""
	Any       any             "json:\"any\""
	Param     T               "json:\"param\""

	InterfaceOmitZero Shape                      "json:\"interface_omit_zero,omitzero\""
	MarkerOmitZero    Marker                     "json:\"marker_omit_zero,omitzero\""
	LiteralOmitZero   interface{ Area() float64 } "json:\"literal_omit_zero,omitzero\""
	ParamOmitZero     T                          "json:\"param_omit_zero,omitzero\""

	OmitZero       *Item  "json:\"omit_zero,omitzero\""
	SliceOmitEmpty []Item "json:\"slice_omit_empty,omitempty\""
	AnyOmitZero    any    "json:\"any_omit_zero,omitzero\""
	OmitEmpty      Shape  "json:\"omit_empty,omitempty\""
	ParamOmitEmpty T      "json:\"param_omit_empty,omitempty\""
	Value          Item   "json:\"value\""
	Skipped        *Item  "json:\"-\""
	XMLOnly        *Item  "xml:\"xml_only\""
	Untagged       *Item
}

type jsonWireHolder struct {
	Hyperlink Shape "json:\"hyperlink\""
	Name      string "json:\"name\""
}

func (h Holder[T]) wrap() any {
	return struct {
		Wrapped *Item "json:\"wrapped\""
	}{Wrapped: h.Pointer}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture_gen.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse the fixture: %v", err)
	}
	info := newTypesInfo()
	if _, err := (&types.Config{}).Check("example.com/fixture", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check the fixture: %v", err)
	}

	audit := auditNilableJSONOmit(fset, []*ast.File{file}, info)
	got := make([]string, 0, len(audit.missing))
	for _, f := range audit.missing {
		got = append(got, fmt.Sprintf("%s.%s (%s: %s)", f.owner, f.field, f.rule.kind, strings.Join(f.rule.accepts, " or ")))
	}
	slices.Sort(got)
	want := []string{
		"Holder.Any (any: omitempty or omitzero)",
		"Holder.Interface (interface: omitempty)",
		"Holder.InterfaceOmitZero (interface: omitempty)",
		"Holder.LiteralOmitZero (interface: omitempty)",
		"Holder.Map (map: omitempty or omitzero)",
		"Holder.MarkerOmitZero (interface: omitempty)",
		"Holder.Named (slice: omitempty or omitzero)",
		"Holder.Param (type parameter: omitempty)",
		"Holder.ParamOmitZero (type parameter: omitempty)",
		"Holder.Pointer (pointer: omitempty or omitzero)",
		"Holder.Slice (slice: omitempty or omitzero)",
		"jsonWireHolder.Hyperlink (interface: omitempty)",
		"struct{…}.Wrapped (pointer: omitempty or omitzero)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("auditNilableJSONOmit(fixture) reported %q, want %q", got, want)
	}
	const passing = 5 // OmitZero, SliceOmitEmpty, AnyOmitZero, OmitEmpty, ParamOmitEmpty
	if audit.nilable != len(want)+passing {
		t.Errorf("auditNilableJSONOmit(fixture) counted %d nilable JSON-tagged fields, want %d (the %d reported and the %d that carry an accepted omit option)", audit.nilable, len(want)+passing, len(want), passing)
	}
}

// missingOmitField is a JSON-tagged field whose type can hold nil and
// whose tag carries none of the omit options its kind accepts.
type missingOmitField struct {
	pos          token.Position
	owner, field string
	goType       string
	tag          string
	rule         omitRule
}

func (f missingOmitField) String() string {
	return fmt.Sprintf("%s:%d: %s.%s %s %s", filepath.Base(f.pos.Filename), f.pos.Line, f.owner, f.field, f.goType, f.tag)
}

// nilableJSONAudit is what [auditNilableJSONOmit] counted and found.
type nilableJSONAudit struct {
	structs int // struct types, named or anonymous
	tagged  int // fields with a json key other than "-"
	nilable int // tagged fields whose type can hold nil
	missing []missingOmitField
}

// auditNilableJSONOmit walks every struct type in files, named or
// anonymous and inside function bodies too, and reports each field with a
// json key whose type, as info records it, can hold nil while its tag
// carries none of the omit options [nilableOmitRule] accepts for that type.
// An anonymous struct is reported under the owner name "struct{…}".
func auditNilableJSONOmit(fset *token.FileSet, files []*ast.File, info *types.Info) nilableJSONAudit {
	var audit nilableJSONAudit
	for _, file := range files {
		owners := map[*ast.StructType]string{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeSpec:
				if st, ok := n.Type.(*ast.StructType); ok {
					owners[st] = n.Name.Name
				}
			case *ast.StructType:
				audit.structs++
				owner, ok := owners[n]
				if !ok {
					owner = "struct{…}"
				}
				for _, field := range n.Fields.List {
					opts, ok := jsonTagOptions(field.Tag)
					if !ok {
						continue
					}
					names := make([]string, 0, len(field.Names))
					for _, name := range field.Names {
						names = append(names, name.Name)
					}
					if len(names) == 0 { // an embedded field
						names = append(names, types.ExprString(field.Type))
					}
					audit.tagged += len(names)
					rule, ok := nilableOmitRule(info.TypeOf(field.Type))
					if !ok {
						continue
					}
					audit.nilable += len(names)
					if slices.ContainsFunc(rule.accepts, func(opt string) bool { return slices.Contains(opts, opt) }) {
						continue
					}
					for _, name := range names {
						audit.missing = append(audit.missing, missingOmitField{
							pos: fset.Position(field.Pos()), owner: owner, field: name,
							goType: types.ExprString(field.Type), tag: field.Tag.Value,
							rule: rule,
						})
					}
				}
			}
			return true
		})
	}
	return audit
}

// omitRule names the kind of a nilable field type and the omit options
// that kind accepts.
type omitRule struct {
	kind    string   // pointer, slice, map, any, interface or type parameter
	accepts []string // a field of this kind must carry one of these
}

// nilableOmitRule returns the omit rule for a field of type t, and false
// when a value of type t cannot be nil. bmm-conformance.md § Mapping rules
// asks for omitempty on a Go interface and on a field typed by its class's
// own generic parameter: omitzero leaves out only a nil interface, so an
// interface that holds a nil pointer would still be written as null. A
// pointer, slice, map or any field may carry either option; the same rule
// gives an any field omitzero. An any field is one of the unnamed empty
// interface; a named interface type, even one with no methods, is a Go
// interface. A type parameter is told apart first, since its underlying
// type is its constraint, an interface.
func nilableOmitRule(t types.Type) (omitRule, bool) {
	either := []string{"omitempty", "omitzero"}
	if _, ok := types.Unalias(t).(*types.TypeParam); ok {
		return omitRule{kind: "type parameter", accepts: []string{"omitempty"}}, true
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return omitRule{kind: "pointer", accepts: either}, true
	case *types.Slice:
		return omitRule{kind: "slice", accepts: either}, true
	case *types.Map:
		return omitRule{kind: "map", accepts: either}, true
	case *types.Interface:
		if _, named := types.Unalias(t).(*types.Named); !named && u.Empty() {
			return omitRule{kind: "any", accepts: either}, true
		}
		return omitRule{kind: "interface", accepts: []string{"omitempty"}}, true
	}
	return omitRule{}, false
}

// jsonTagOptions returns the options after the name in the json key of a
// struct field tag, such as "omitzero". ok is false when the field has no
// tag, no json key, or the json key "-", which leaves the field out of JSON.
func jsonTagOptions(tag *ast.BasicLit) (opts []string, ok bool) {
	if tag == nil {
		return nil, false
	}
	raw, err := strconv.Unquote(tag.Value)
	if err != nil {
		return nil, false
	}
	value, ok := reflect.StructTag(raw).Lookup("json")
	if !ok || value == "-" {
		return nil, false
	}
	_, rest, _ := strings.Cut(value, ",")
	return strings.Split(rest, ","), true
}

// moduleRootDir returns the absolute path of the module root, two levels
// above this package.
func moduleRootDir(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at the resolved module root %s: %v", root, err)
	}
	return root
}

// goModulePath returns the module path the go.mod file in root declares.
func goModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(path), `"`), nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

// generatedJSONTaggedDirs returns, relative to the module root and sorted,
// the package directories in which some *_gen.go file declares a struct
// field with a json key. The walk skips what the go tool skips when it lists
// the module's packages: a directory holding its own go.mod, testdata and
// vendor, and every name that starts with "." or "_".
func generatedJSONTaggedDirs(root string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return fs.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, "_gen.go") || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		if hasJSONTaggedField(file) {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			seen[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return slices.Sorted(maps.Keys(seen)), err
}

// hasJSONTaggedField reports whether file declares a struct field with a
// json key in its tag.
func hasJSONTaggedField(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if field, ok := n.(*ast.Field); ok && field.Tag != nil {
			if raw, err := strconv.Unquote(field.Tag.Value); err == nil {
				if _, ok := reflect.StructTag(raw).Lookup("json"); ok {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// typeCheckDir parses the non-test Go files the default build context
// selects in dir, type-checks them as the package importPath with imp, and
// returns the generated files among them with the type information.
func typeCheckDir(fset *token.FileSet, imp types.Importer, importPath, dir string) ([]*ast.File, *types.Info, error) {
	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list the Go files of %s: %w", dir, err)
	}
	var all, gen []*ast.File
	for _, name := range pkg.GoFiles {
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, err
		}
		all = append(all, file)
		if strings.HasSuffix(name, "_gen.go") {
			gen = append(gen, file)
		}
	}
	info := newTypesInfo()
	conf := types.Config{Importer: imp}
	if _, err := conf.Check(importPath, fset, all, info); err != nil {
		return nil, nil, fmt.Errorf("type-check %s: %w", importPath, err)
	}
	return gen, info, nil
}

// newTypesInfo returns a [types.Info] that records the type of every
// expression and identifier, which [types.Info.TypeOf] reads.
func newTypesInfo() *types.Info {
	return &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
}
