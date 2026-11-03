package bmmgen

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmlists"
	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ052_ContainerFieldShapeKeepsEmpty pins the JSON tag
// [containerFieldShape] gives a container field, by the class that declares
// it and its cardinality. An optional list takes omitempty, so a nil and an
// empty one are both left out, except the item-structure lists
// [rmlists.KeepsEmpty] names (issue 341), which take omitzero like a
// mandatory list, so a present empty one is written as [].
//
// REQ-052 § An absent mandatory member: a present empty optional list is left
// out where the field is the list itself, except ITEM_LIST.items,
// ITEM_TREE.items and ITEM_TABLE.rows.
func TestREQ052_ContainerFieldShapeKeepsEmpty(t *testing.T) {
	list := &bmm.ContainerType{ContainerType: "List"}
	hash := &bmm.ContainerType{ContainerType: "Hash"}
	optional := &bmm.Cardinality{Lower: 0}
	mandatory := &bmm.Cardinality{Lower: 1}

	for _, tc := range []struct {
		name      string
		td        *bmm.ContainerType
		owner     string
		attr      string
		card      *bmm.Cardinality
		wantType  string
		wantTagOn string
	}{
		{name: "ITEM_LIST.items, kept", td: list, owner: "ITEM_LIST", attr: "items", card: optional, wantType: "[]X", wantTagOn: "omitzero"},
		{name: "ITEM_TREE.items, kept", td: list, owner: "ITEM_TREE", attr: "items", card: optional, wantType: "[]X", wantTagOn: "omitzero"},
		{name: "ITEM_TABLE.rows, kept with no cardinality", td: list, owner: "ITEM_TABLE", attr: "rows", card: nil, wantType: "[]X", wantTagOn: "omitzero"},
		{name: "ITEM_LIST.links, another optional list on a kept class", td: list, owner: "ITEM_LIST", attr: "links", card: optional, wantType: "[]X", wantTagOn: "omitempty"},
		{name: "DV_TEXT.mappings, collapses", td: list, owner: "DV_TEXT", attr: "mappings", card: optional, wantType: "[]X", wantTagOn: "omitempty"},
		{name: "SECTION.items, same attribute name on another class", td: list, owner: "SECTION", attr: "items", card: optional, wantType: "[]X", wantTagOn: "omitempty"},
		{name: "optional Hash", td: hash, owner: "X_HOLDER", attr: "entries", card: optional, wantType: "map[string]X", wantTagOn: "omitempty"},
		{name: "CLUSTER.items, mandatory", td: list, owner: "CLUSTER", attr: "items", card: mandatory, wantType: "[]X", wantTagOn: "omitzero"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotTag := containerFieldShape(tc.td, "X", tc.owner, tc.attr, tc.card)
			wantTag := "`json:\"" + tc.attr + "," + tc.wantTagOn + "\"`"
			if gotType != tc.wantType || gotTag != wantTag {
				t.Errorf("containerFieldShape(%s, X, %q, %q, %v) = (%s, %s), want (%s, %s)",
					tc.td.ContainerType, tc.owner, tc.attr, tc.card, gotType, gotTag, tc.wantType, wantTag)
			}
		})
	}
}

// TestREQ052_KeepsEmptyListFieldsRendered renders the item-structure and
// DV_TEXT files from the pinned RM and requires the three listed lists to
// carry omitzero while the optional lists beside them keep omitempty. It then
// renders every optional container property the RM and AOM 1.4 plans declare
// and requires omitzero on exactly the pairs [rmlists.KeepsEmpty] names, so a
// blanket change of the optional-container tag fails here.
//
// REQ-052 § An absent mandatory member, issue 341.
func TestREQ052_KeepsEmptyListFieldsRendered(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(t.Context(), TargetRM, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	aomPlan, err := BuildPlanForTarget(t.Context(), TargetAOM14, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}

	for _, tc := range []struct {
		fileBase string
		fields   []string
	}{
		{
			fileBase: "data_structures_item_structure",
			fields: []string{
				"Items []Element `json:\"items,omitzero\"`",
				"Items []Item `json:\"items,omitzero\"`",
				"Rows []Cluster `json:\"rows,omitzero\"`",
				"Links []Link `json:\"links,omitempty\"`",
			},
		},
		{
			fileBase: "data_types_text",
			fields:   []string{"Mappings []TermMapping `json:\"mappings,omitempty\"`"},
		},
		{
			fileBase: "composition_content_navigation",
			fields:   []string{"Items []ContentItem `json:\"items,omitempty\"`"},
		},
	} {
		t.Run(tc.fileBase, func(t *testing.T) {
			var file *PlannedFile
			for _, f := range rmPlan.Files {
				if f.FileBase == tc.fileBase {
					file = f
					break
				}
			}
			if file == nil {
				t.Fatalf("%s file not in the RM plan", tc.fileBase)
			}
			got, err := RenderFile(rmPlan, file)
			if err != nil {
				t.Fatalf("RenderFile(RM, %s): %v", tc.fileBase, err)
			}
			for _, field := range tc.fields {
				if !fieldDecl(field).Match(got) {
					t.Errorf("the rendered %s source does not declare the field %q", tc.fileBase, field)
				}
			}
		})
	}

	var optionalLists int
	var kept []string
	for _, plan := range []*Plan{rmPlan, aomPlan} {
		for className, pc := range plan.Classes {
			if pc.External || pc.IsPrimitive {
				continue
			}
			sc, ok := pc.Class.(*bmm.SimpleClass)
			if !ok {
				continue
			}
			for attr, prop := range sc.Properties {
				cp, ok := prop.(*bmm.ContainerProperty)
				if !ok || (cp.Cardinality != nil && cp.Cardinality.Lower > 0) {
					continue
				}
				optionalLists++
				line, err := renderField(plan, sc, className, prop)
				if err != nil {
					t.Fatalf("renderField(%s.%s): %v", className, attr, err)
				}
				omitZero := strings.Contains(line, ",omitzero\"`")
				if want := rmlists.KeepsEmpty(className, attr); omitZero != want {
					t.Errorf("renderField(%s.%s) = %q: omitzero %t, want %t (rmlists.KeepsEmpty)", className, attr, line, omitZero, want)
				}
				if omitZero {
					kept = append(kept, className+"."+attr)
				}
			}
		}
	}
	// The control: the walk reaches the optional container properties, the
	// listed ones among them.
	if optionalLists < 10 || len(kept) != 3 {
		t.Errorf("walked %d optional container properties, %d kept (%v); want at least 10, and the 3 rmlists.KeepsEmpty names", optionalLists, len(kept), kept)
	}
}
