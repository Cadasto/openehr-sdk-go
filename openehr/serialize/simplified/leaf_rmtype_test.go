package simplified

// REQ-053 / REQ-140 — the codec reads a Web Template node's RM type through one
// normaliser, so a spelling padded with white space classifies exactly like the
// clean one: the value, interval, party and CODE_PHRASE leaf predicates on
// encode, the datatype dispatch on decode, and the container types decode
// materialises. Before, the value-leaf test compared the untrimmed name, so a
// padded leaf was read as structure and every one of its keys was dropped on
// encode with no error.

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// leafTypes names the RM type of every node in [leafTypesWT], so one table row
// can pad any of them.
type leafTypes struct {
	comp, entry, interval, openInterval, quantity, text, party, code string
}

// cleanLeafTypes is the spelling the SDK's own Web Template builder writes.
var cleanLeafTypes = leafTypes{
	comp:         "COMPOSITION",
	entry:        "EVALUATION",
	interval:     "DV_INTERVAL<DV_QUANTITY>",
	openInterval: "DV_INTERVAL<DV_COUNT>",
	quantity:     "DV_QUANTITY",
	text:         "DV_TEXT",
	party:        "PARTY_PROXY",
	code:         "CODE_PHRASE",
}

// leafTypesWT is a Web Template with one leaf of each classification the codec
// makes: two interval leaves (one bounded, one open), a plain DV_ leaf with no input descriptors (classified
// by its type alone), a DV_ leaf with inputs (classified by its inputs, then
// dispatched by its type), a party leaf and a CODE_PHRASE leaf.
func leafTypesWT(ty leafTypes) *webtemplate.WebTemplate {
	const entry = "/content[openEHR-EHR-EVALUATION.test.v1]"
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: ty.comp, NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "ev", RMType: ty.entry, NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
			AQLPath: entry,
			Children: []*webtemplate.Node{
				{ID: "iv", RMType: ty.interval, NodeID: "at0002", Max: 1, AQLPath: entry + "/data[at0001]/items[at0002]/value"},
				{ID: "open", RMType: ty.openInterval, NodeID: "at0005", Max: 1, AQLPath: entry + "/data[at0001]/items[at0005]/value"},
				{ID: "q", RMType: ty.quantity, NodeID: "at0003", Max: 1, AQLPath: entry + "/data[at0001]/items[at0003]/value"},
				{
					ID: "t", RMType: ty.text, NodeID: "at0004", Max: 1, AQLPath: entry + "/data[at0001]/items[at0004]/value",
					Inputs: []webtemplate.Input{{Type: "TEXT"}},
				},
				{ID: "subject", RMType: ty.party, Max: 1, AQLPath: entry + "/subject"},
				{ID: "language", RMType: ty.code, Max: 1, AQLPath: entry + "/language"},
			},
		}},
	}}
}

// leafTypesComp populates every leaf of [leafTypesWT], the quantity with a
// `normal_range` so the decoder's value-decoration families are judged against
// the leaf's type too.
func leafTypesComp() *rm.Composition {
	mm := func(m rm.Real) rm.DVQuantity { return rm.DVQuantity{Magnitude: m, Units: "mm"} }
	name := "Dr. Leaf"
	element := func(id string, v rm.DataValue) rm.Item {
		return &rm.Element{ArchetypeNodeID: id, Name: rm.DVText{Value: id}, Value: v}
	}
	return &rm.Composition{
		Name:      rm.DVText{Value: "t"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content: []rm.ContentItem{&rm.Evaluation{
			ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1", Name: rm.DVText{Value: "ev"},
			Language: rm.CodePhrase{CodeString: "nl", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}},
			Subject:  &rm.PartyIdentified{Name: &name},
			Data: &rm.ItemTree{
				ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"},
				Items: []rm.Item{
					// Both ends excluded, so both |*_included flags are written.
					element("at0002", &rm.DVInterval[rm.DVQuantity]{
						Lower: mm(1.5), Upper: mm(8),
					}),
					element("at0003", &rm.DVQuantity{
						Magnitude: 65.5, Units: "mm",
						NormalRange: &rm.DVInterval[rm.DVQuantity]{
							Lower: mm(60), Upper: mm(70), LowerIncluded: true, UpperIncluded: true,
						},
					}),
					element("at0004", &rm.DVText{Value: "free text"}),
					// No lower bound, so the |lower_unbounded flag is written.
					element("at0005", &rm.DVInterval[rm.DVCount]{
						LowerUnbounded: true, Upper: rm.DVCount{Magnitude: 12}, UpperIncluded: true,
					}),
				},
			},
		}},
	}
}

// leafTypesKeys is what the clean spelling writes for [leafTypesComp] beyond
// ctx/: every interval key (both bounds' suffixes and the flags), the plain
// DV_ leaves, the quantity's decoration, the party and the CODE_PHRASE. Pinned
// so the equality below cannot pass on two empty outputs.
var leafTypesKeys = []string{
	"root/ev/iv/lower|magnitude", "root/ev/iv/lower|unit",
	"root/ev/iv/upper|magnitude", "root/ev/iv/upper|unit",
	"root/ev/iv|lower_included", "root/ev/iv|upper_included",
	"root/ev/open|lower_unbounded", "root/ev/open/upper",
	"root/ev/q|magnitude", "root/ev/q|unit",
	"root/ev/q/_normal_range/lower|magnitude", "root/ev/q/_normal_range/upper|magnitude",
	"root/ev/t",
	"root/ev/subject|name",
	"root/ev/language|code", "root/ev/language|terminology",
}

// paddedLeafTypes pads each node's RM type a different way: around the whole
// name, inside the generic parameter list, and with non-ASCII white space.
var paddedLeafTypes = []struct {
	name  string
	types leafTypes
}{
	{
		name: "leading and trailing spaces",
		types: leafTypes{
			comp: " COMPOSITION", entry: "EVALUATION ", interval: " DV_INTERVAL<DV_QUANTITY>",
			openInterval: "DV_INTERVAL<DV_COUNT> ", quantity: "DV_QUANTITY ", text: " DV_TEXT ", party: " PARTY_PROXY", code: "CODE_PHRASE ",
		},
	},
	{
		name: "spaces inside the generic parameters",
		types: leafTypes{
			comp: "COMPOSITION", entry: "EVALUATION", interval: "DV_INTERVAL< DV_QUANTITY >",
			openInterval: "DV_INTERVAL <DV_COUNT>", quantity: "DV_QUANTITY", text: "DV_TEXT", party: "PARTY_PROXY", code: "CODE_PHRASE",
		},
	},
	{
		name: "tabs, newlines and a no-break space",
		types: leafTypes{
			comp: "\tCOMPOSITION", entry: "EVALUATION\n", interval: "\u00a0DV_INTERVAL <\tDV_QUANTITY\n> ",
			openInterval: "\tDV_INTERVAL<DV_COUNT>", quantity: "\tDV_QUANTITY", text: "DV_TEXT\u00a0",
			party: "PARTY_PROXY\t", code: "\nCODE_PHRASE",
		},
	},
}

// diffFlat reports every key the two FLAT maps disagree on, sorted.
func diffFlat(want, got map[string]any) []string {
	var diffs []string
	for _, k := range slices.Sorted(maps.Keys(want)) {
		g, ok := got[k]
		switch {
		case !ok:
			diffs = append(diffs, "missing "+k)
		case !reflect.DeepEqual(want[k], g):
			diffs = append(diffs, fmt.Sprintf("%s = %#v, want %#v", k, g, want[k]))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[k]; !ok {
			diffs = append(diffs, "extra "+k)
		}
	}
	return diffs
}

// TestPaddedLeafRMTypeEncodesLikeClean — REQ-053 / REQ-140. MarshalFlat with a
// padded Web Template writes exactly what the clean one writes, key for key,
// and so does MarshalStructured.
func TestPaddedLeafRMTypeEncodesLikeClean(t *testing.T) {
	comp := leafTypesComp()
	clean, err := MarshalFlat(comp, leafTypesWT(cleanLeafTypes))
	if err != nil {
		t.Fatalf("MarshalFlat(clean spelling): %v", err)
	}
	want := flatMap(t, clean)
	for _, k := range leafTypesKeys {
		if _, ok := want[k]; !ok {
			t.Fatalf("the clean spelling did not write %q — the fixture no longer covers that leaf; emitted: %v",
				k, slices.Sorted(maps.Keys(want)))
		}
	}
	cleanStructured, err := MarshalStructured(comp, leafTypesWT(cleanLeafTypes))
	if err != nil {
		t.Fatalf("MarshalStructured(clean spelling): %v", err)
	}
	for _, tc := range paddedLeafTypes {
		t.Run(tc.name, func(t *testing.T) {
			b, err := MarshalFlat(comp, leafTypesWT(tc.types))
			if err != nil {
				t.Fatalf("MarshalFlat(padded spelling): %v", err)
			}
			if diffs := diffFlat(want, flatMap(t, b)); len(diffs) > 0 {
				t.Errorf("MarshalFlat with a padded Web Template differs from the clean spelling:\n  %s",
					strings.Join(diffs, "\n  "))
			}
			s, err := MarshalStructured(comp, leafTypesWT(tc.types))
			if err != nil {
				t.Fatalf("MarshalStructured(padded spelling): %v", err)
			}
			var gotS, wantS any
			if err := json.Unmarshal(s, &gotS); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(cleanStructured, &wantS); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotS, wantS) {
				t.Errorf("MarshalStructured with a padded Web Template = %s, want %s", s, cleanStructured)
			}
		})
	}
}

// TestPaddedLeafRMTypeDecodesLikeClean — REQ-053 / REQ-140. UnmarshalFlat of the
// clean spelling's own output, through a padded Web Template, rebuilds the same
// canonical composition the clean one does, and re-encodes to the same keys.
func TestPaddedLeafRMTypeDecodesLikeClean(t *testing.T) {
	cleanWT := leafTypesWT(cleanLeafTypes)
	body, err := MarshalFlat(leafTypesComp(), cleanWT)
	if err != nil {
		t.Fatalf("MarshalFlat(clean spelling): %v", err)
	}
	cleanComp, err := UnmarshalFlat(body, cleanWT)
	if err != nil {
		t.Fatalf("UnmarshalFlat(clean spelling): %v", err)
	}
	wantCanon, err := canjson.Marshal(cleanComp)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range paddedLeafTypes {
		t.Run(tc.name, func(t *testing.T) {
			wt := leafTypesWT(tc.types)
			comp, err := UnmarshalFlat(body, wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat through a padded Web Template: %v", err)
			}
			gotCanon, err := canjson.Marshal(comp)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotCanon) != string(wantCanon) {
				t.Errorf("padded decode rebuilt\n  %s\nwant (clean decode)\n  %s", gotCanon, wantCanon)
			}
			again, err := MarshalFlat(comp, wt)
			if err != nil {
				t.Fatalf("MarshalFlat after a padded decode: %v", err)
			}
			if diffs := diffFlat(flatMap(t, body), flatMap(t, again)); len(diffs) > 0 {
				t.Errorf("padded round trip differs from the body it read:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

// TestWebTemplateRMTypeReadThroughNormaliser — REQ-053. The padded-name tests
// above cover the sites that exist today; this keeps a new one from reading the
// raw field. Every read of a Web Template node's RMType in the package's
// non-test code must sit inside the one normaliser, nodeRMType.
func TestWebTemplateRMTypeReadThroughNormaliser(t *testing.T) {
	const normaliser = "nodeRMType"
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var stray []string
	var normaliserReads int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "RMType" {
					return true
				}
				if isFunc && fn.Recv == nil && fn.Name.Name == normaliser {
					normaliserReads++
					return true
				}
				stray = append(stray, fset.Position(sel.Pos()).String())
				return true
			})
		}
	}
	if len(stray) > 0 {
		t.Errorf("Web Template RMType read outside %s — a padded type name would be misclassified there; "+
			"read it through %s instead:\n  %s", normaliser, normaliser, strings.Join(stray, "\n  "))
	}
	if normaliserReads == 0 {
		t.Errorf("no RMType read inside %s — the normaliser was renamed or removed, so this guard checks nothing", normaliser)
	}
}
