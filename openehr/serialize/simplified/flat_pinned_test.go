package simplified

// REQ-116 / REQ-053 — FLAT decode through a name-pinned container.
//
// Regression guard: after Phase 3 the WebTemplate carries name-predicated
// aqlPaths, but decode's placeLeaf rebuilds its lookup prefixes in the bare
// spelling (parseAQL strips predicates), so resolveLeaf must key predType /
// predIndex by the bare spelling too. Keyed by the predicated spelling, every
// lookup under a pinned container missed and concreteType fell back to a
// default RM type — GECCO's pinned EVALUATION decoded as OBSERVATION, whose
// `data` then failed as HISTORY. No prior FLAT fixture crossed a name-pinned
// container, which is why the suite stayed green through it.

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func TestUnmarshalFlatPinnedContainer(t *testing.T) {
	parsed, err := readOPTFile(fixtures.WebTemplateOpt("GECCO_Diagnose"))
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	flat := []byte(`{
		"ctx/language": "de",
		"ctx/territory": "DE",
		"ctx/composer_name": "Dr Test",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|code": "261665006",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|value": "Unknown",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|terminology": "SNOMED-CT"
	}`)

	comp, err := UnmarshalFlat(flat, wt, WithTemplate(c))
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}

	// The pinned container must decode as its own RM type, not a fallback.
	if n := len(comp.Content); n != 1 {
		t.Fatalf("content has %d entries, want 1", n)
	}
	eval, ok := comp.Content[0].(*rm.Evaluation)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Evaluation — predType lookup missed the pinned container", comp.Content[0])
	}
	if got := eval.ArchetypeNodeID; got != "openEHR-EHR-EVALUATION.absence.v2" {
		t.Errorf("archetype_node_id = %q, want openEHR-EHR-EVALUATION.absence.v2", got)
	}
	// WithTemplate repopulates LOCATABLE.name from the template-level pinned
	// name (REQ-116) — not the archetype concept term ("Absence of information").
	if got := eval.Name.GetValue(); got != "Unbekannte Diagnose" {
		t.Errorf("decoded name = %q, want the pinned %q", got, "Unbekannte Diagnose")
	}

	// Round-trip: the same leaf keys and values come back.
	out, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-decode emitted FLAT: %v", err)
	}
	for key, want := range map[string]string{
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|code":        "261665006",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|value":       "Unknown",
		"gecco_diagnose/unbekannte_diagnose/unbekannte_diagnose|terminology": "SNOMED-CT",
	} {
		if v, ok := got[key]; !ok || v != want {
			t.Errorf("round-trip key %q = %v (present=%v), want %q", key, v, ok, want)
		}
	}
}

// TestUnmarshalFlatNamedReusedSiblingsCorona — REQ-053, REQ-116: corona's four
// SECTION.adhoc.v1 siblings share one bare path and are told apart by the
// names they pin. A key through one of them decodes into a SECTION carrying
// that sibling's name, and keys through two of them into two SECTIONs, never
// one, re-encoding to the same keys.
func TestUnmarshalFlatNamedReusedSiblingsCorona(t *testing.T) {
	parsed, err := readOPTFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	sib := reusedSiblingsOf(wt)

	// For each SECTION.adhoc.v1 sibling, the FLAT key of the first DV_TEXT
	// leaf under it that no repeatable node holds, so the key is spelled
	// without an index, keyed by the name the sibling pins in its aqlPath.
	keys := make(map[string]string)
	var find func(n *webtemplate.Node, ids []string, name string)
	find = func(n *webtemplate.Node, ids []string, name string) {
		if n.Max != 1 && name != "" {
			return
		}
		ids = append(ids, n.ID)
		if name == "" && n.RMType == "SECTION" && n.NodeID == "openEHR-EHR-SECTION.adhoc.v1" {
			for seg := range rawSegments(n.AQLPath) {
				name = seg.name(n.AQLPath)
			}
		}
		if name != "" && keys[name] == "" && n.RMType == "DV_TEXT" && len(n.Children) == 0 {
			keys[name] = strings.Join(ids, "/")
		}
		for _, ch := range n.Children {
			find(ch, ids, name)
		}
	}
	find(wt.Tree, nil, "")
	if len(keys) < 2 {
		t.Fatalf("found DV_TEXT leaves under %d SECTION.adhoc.v1 siblings, want at least 2 — fixture changed?", len(keys))
	}
	if len(sib.claimed) == 0 {
		t.Fatal("corona has no siblings told apart by name — fixture changed?")
	}

	flat := map[string]any{
		"ctx/language":      "de",
		"ctx/territory":     "DE",
		"ctx/composer_name": "Dr Test",
	}
	for name, key := range keys {
		flat[key] = "value of " + name
	}
	for _, opts := range [][]Option{nil, {WithTemplate(c)}} {
		comp, err := UnmarshalFlat(mustJSON(t, flat), wt, opts...)
		if err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		sections := 0
		for _, item := range comp.Content {
			sec, ok := item.(*rm.Section)
			if !ok || sec.ArchetypeNodeID != "openEHR-EHR-SECTION.adhoc.v1" {
				continue
			}
			sections++
			if _, pinned := keys[sec.Name.GetValue()]; !pinned {
				t.Errorf("decoded a SECTION.adhoc.v1 named %q, want one of the pinned names %v", sec.Name.GetValue(), slices.Sorted(maps.Keys(keys)))
			}
		}
		if sections != len(keys) {
			t.Errorf("decoded %d SECTION.adhoc.v1, want %d: one per sibling a key reaches", sections, len(keys))
		}
		out, err := MarshalFlat(comp, wt)
		if err != nil {
			t.Fatalf("MarshalFlat: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("re-decode emitted FLAT: %v", err)
		}
		for name, key := range keys {
			if want := "value of " + name; got[key] != want {
				t.Errorf("round-trip %q = %v, want %q", key, got[key], want)
			}
		}
	}
}

// twoSiblingWT is a minimal WebTemplate with two reused siblings — one
// archetype id, pinning the names pinA and pinB, none where empty, escaped
// as the Web Template escapes them — each wrapping one DV_TEXT leaf. The
// smallest tree on which the reused-sibling cases show.
func twoSiblingWT(pinA, pinB string) *webtemplate.WebTemplate {
	pred := func(pin string) string {
		return tcimpl.NamePredicate("openEHR-EHR-EVALUATION.test.v1", pin)
	}
	leaf := func(pin string) *webtemplate.Node {
		return &webtemplate.Node{
			ID: "x", RMType: "DV_TEXT", NodeID: "at0002", Max: 1,
			AQLPath: "/content[" + pred(pin) + "]/data[at0001]/items[at0002]/value",
			Inputs:  []webtemplate.Input{{Type: "TEXT"}},
		}
	}
	eval := func(id, pin string) *webtemplate.Node {
		return &webtemplate.Node{
			ID: id, RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
			AQLPath:  "/content[" + pred(pin) + "]",
			Children: []*webtemplate.Node{leaf(pin)},
		}
	}
	return &webtemplate.WebTemplate{
		Tree: &webtemplate.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*webtemplate.Node{eval("a", pinA), eval("b", pinB)},
		},
	}
}

// siblingEvaluation is an EVALUATION of the twoSiblingWT archetype named
// name, holding text in its one ELEMENT.
func siblingEvaluation(name, text string) *rm.Evaluation {
	return &rm.Evaluation{
		ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1",
		Name:            rm.DVText{Value: name},
		Data: &rm.ItemTree{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "tree"},
			Items: []rm.Item{
				&rm.Element{
					ArchetypeNodeID: "at0002",
					Name:            rm.DVText{Value: "x"},
					Value:           &rm.DVText{Value: text},
				},
			},
		},
	}
}

// siblingComposition is a composition holding items.
func siblingComposition(items ...rm.ContentItem) *rm.Composition {
	return &rm.Composition{
		Name:      rm.DVText{Value: "A"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content:   items,
	}
}

// TestMarshalFlatNamedReusedSiblings — REQ-053, REQ-116: reused siblings that
// pin distinct names resolve by those names, so each instance is written under
// its own sibling's id alone, whether one sibling or both hold one, and decodes
// back into an instance of that name. An instance whose name none of them pins
// would be claimed by no sibling: it is refused with rmpath.ErrPathAmbiguous
// rather than dropped, and the refusal names no value.
func TestMarshalFlatNamedReusedSiblings(t *testing.T) {
	wt := twoSiblingWT("A", "B")
	for _, tc := range []struct {
		name  string
		items []rm.ContentItem
		want  map[string]any
	}{
		{"one sibling", []rm.ContentItem{siblingEvaluation("A", "hello")}, map[string]any{"root/a/x": "hello"}},
		{"the other sibling", []rm.ContentItem{siblingEvaluation("B", "hello")}, map[string]any{"root/b/x": "hello"}},
		{"both siblings", []rm.ContentItem{siblingEvaluation("B", "bee"), siblingEvaluation("A", "ay")}, map[string]any{"root/a/x": "ay", "root/b/x": "bee"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := MarshalFlat(siblingComposition(tc.items...), wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			got := flatMap(t, out)
			for _, k := range []string{"root/a/x", "root/b/x"} {
				if got[k] != tc.want[k] {
					t.Errorf("%s = %v, want %v", k, got[k], tc.want[k])
				}
			}
			back, err := UnmarshalFlat(out, wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			names := make(map[string]string)
			for _, item := range back.Content {
				ev, ok := item.(*rm.Evaluation)
				if !ok || ev.Data == nil {
					t.Fatalf("decoded content item %T", item)
				}
				tree, _ := ev.Data.(*rm.ItemTree)
				el, _ := tree.Items[0].(*rm.Element)
				names[ev.Name.GetValue()] = el.Value.(*rm.DVText).Value
			}
			want := make(map[string]string)
			for k, v := range tc.want {
				want[strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(k, "root/"), "/x"))] = v.(string)
			}
			if !maps.Equal(names, want) {
				t.Errorf("decoded evaluations by name = %v, want %v", names, want)
			}
		})
	}

	t.Run("a name no sibling pins", func(t *testing.T) {
		_, err := MarshalFlat(siblingComposition(siblingEvaluation("A", "ay"), siblingEvaluation("Secret name", "hello")), wt)
		if !errors.Is(err, rmpath.ErrPathAmbiguous) || !strings.Contains(err.Error(), "none of the reused siblings") {
			t.Fatalf("err = %v, want the ErrPathAmbiguous refusal of a name no sibling pins", err)
		}
		if strings.Contains(err.Error(), "Secret name") || strings.Contains(err.Error(), "hello") {
			t.Errorf("err = %v carries a value of the composition", err)
		}
	})
}

// TestFlatReusedSiblingEscapedNames — REQ-053, REQ-116: a pinned name holding
// a quote, a closing bracket or a backslash, which the Web Template escapes in
// its aqlPath, still tells reused siblings apart: each instance encodes under
// its own sibling's id and decodes back into an instance of that name.
func TestFlatReusedSiblingEscapedNames(t *testing.T) {
	for _, pin := range []string{"O'Brien", `a']b`, `back\slash`, `trailing\`} {
		t.Run(pin, func(t *testing.T) {
			wt := twoSiblingWT(pin, "B")
			out, err := MarshalFlat(siblingComposition(siblingEvaluation("B", "bee"), siblingEvaluation(pin, "ay")), wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			got := flatMap(t, out)
			for k, want := range map[string]any{"root/a/x": "ay", "root/b/x": "bee"} {
				if got[k] != want {
					t.Errorf("%s = %v, want %v", k, got[k], want)
				}
			}
			back, err := UnmarshalFlat(out, wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			texts := make(map[string]string)
			for _, item := range back.Content {
				ev, ok := item.(*rm.Evaluation)
				if !ok {
					t.Fatalf("decoded content item %T", item)
				}
				tree, _ := ev.Data.(*rm.ItemTree)
				el, _ := tree.Items[0].(*rm.Element)
				texts[ev.Name.GetValue()] = el.Value.(*rm.DVText).Value
			}
			if want := map[string]string{pin: "ay", "B": "bee"}; !maps.Equal(texts, want) {
				t.Errorf("decoded evaluations by name = %q, want %q", texts, want)
			}
		})
	}
}

// TestUnmarshalFlatNamedReusedSiblingSparseIndexRefused — REQ-053, REQ-116:
// placement fills the slots before a reused sibling's :index with members that
// carry its name, so a later key of that sibling finds them; one no key
// reaches is still a phantom of a sparse :index, refused as any other is.
func TestUnmarshalFlatNamedReusedSiblingSparseIndexRefused(t *testing.T) {
	wt := twoSiblingWT("A", "B")
	for _, ch := range wt.Tree.Children {
		ch.Max = -1
	}
	ctx := map[string]any{"ctx/language": "en", "ctx/territory": "NL", "ctx/composer_name": "Dr Test"}

	t.Run("sparse", func(t *testing.T) {
		flat := maps.Clone(ctx)
		flat["root/a:1/x"] = "second"
		flat["root/b:0/x"] = "bee"
		_, err := UnmarshalFlat(mustJSON(t, flat), wt)
		if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "sparse") {
			t.Fatalf("err = %v, want the sparse :index refusal", err)
		}
	})
	t.Run("dense, a later index first", func(t *testing.T) {
		flat := maps.Clone(ctx)
		for i := range 11 {
			flat["root/a:"+strconv.Itoa(i)+"/x"] = "a" + strconv.Itoa(i)
		}
		flat["root/b:0/x"] = "bee"
		comp, err := UnmarshalFlat(mustJSON(t, flat), wt)
		if err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		out, err := MarshalFlat(comp, wt)
		if err != nil {
			t.Fatalf("MarshalFlat: %v", err)
		}
		got := flatMap(t, out)
		for k, v := range flat {
			if strings.HasPrefix(k, "root/") && got[k] != v {
				t.Errorf("round-trip %s = %v, want %v", k, got[k], v)
			}
		}
	})
}

// TestFlatReusedSiblingPinningNoName — REQ-053, REQ-116: beside a reused
// sibling that pins a name, the one that pins none stands for every instance
// carrying no name its sibling pins. Encode writes each instance under the
// sibling it belongs to, and decode places each key back into an instance of
// that sibling, never into the named one.
func TestFlatReusedSiblingPinningNoName(t *testing.T) {
	wt := twoSiblingWT("A", "")
	wt.Tree.Children[1].Max = -1
	comp := siblingComposition(siblingEvaluation("Other", "o1"), siblingEvaluation("A", "ay"), siblingEvaluation("Another", "o2"))
	out, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	got := flatMap(t, out)
	want := map[string]any{"root/a/x": "ay", "root/b:0/x": "o1", "root/b:1/x": "o2"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if _, extra := got["root/b:2/x"]; extra {
		t.Errorf("root/b:2/x = %v: the instance named A was written under the sibling that pins no name too", got["root/b:2/x"])
	}

	back, err := UnmarshalFlat(out, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if n := len(back.Content); n != 3 {
		t.Fatalf("decoded %d content items, want 3", n)
	}
	if name := back.Content[0].(*rm.Evaluation).Name.GetValue(); name != "A" {
		t.Errorf("content[0] is named %q, want the sibling that pins A first", name)
	}
	again, err := MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat of the decoded composition: %v", err)
	}
	if g := flatMap(t, again); !maps.Equal(g, got) {
		t.Errorf("re-encoded FLAT = %v, want %v", g, got)
	}
}

// elementSiblingWT is a minimal WebTemplate whose one EVALUATION holds two
// reused ELEMENT siblings, at0002 pinning the names pinA and pinB, none where
// empty, each collapsed into its DV_TEXT leaf: a and b.
func elementSiblingWT(pinA, pinB string) *webtemplate.WebTemplate {
	leaf := func(id, pin string) *webtemplate.Node {
		return &webtemplate.Node{
			ID: id, RMType: "DV_TEXT", NodeID: "at0002", Max: 1,
			AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]/data[at0001]/items[" + tcimpl.NamePredicate("at0002", pin) + "]/value",
			Inputs:  []webtemplate.Input{{Type: "TEXT"}},
		}
	}
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "ev", RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
			AQLPath:  "/content[openEHR-EHR-EVALUATION.test.v1]",
			Children: []*webtemplate.Node{leaf("a", pinA), leaf("b", pinB)},
		}},
	}}
}

// stringLeafSiblingWT is a minimal WebTemplate whose one INSTRUCTION holds
// two STRING leaves, a and b, on the action_archetype_id of reused ACTIVITY
// siblings, at0001 pinning the names pinA and pinB, none where empty. The
// tree holds no node for the ACTIVITYs, so each leaf's path crosses its
// sibling's segment.
func stringLeafSiblingWT(pinA, pinB string) *webtemplate.WebTemplate {
	const ins = "/content[openEHR-EHR-INSTRUCTION.test.v1]"
	leaf := func(id, pin string) *webtemplate.Node {
		return &webtemplate.Node{
			ID: id, RMType: "STRING", Max: 1,
			AQLPath: ins + "/activities[" + tcimpl.NamePredicate("at0001", pin) + "]/action_archetype_id",
			Inputs:  []webtemplate.Input{{Type: "TEXT"}},
		}
	}
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "ins", RMType: "INSTRUCTION", NodeID: "openEHR-EHR-INSTRUCTION.test.v1", Max: 1,
			AQLPath:  ins,
			Children: []*webtemplate.Node{leaf("a", pinA), leaf("b", pinB)},
		}},
	}}
}

// elementSiblingComposition is a composition holding the elementSiblingWT
// EVALUATION with els, and namedElement an ELEMENT at0002 named name that
// holds text.
func elementSiblingComposition(els ...rm.Item) *rm.Composition {
	return siblingComposition(&rm.Evaluation{
		ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1",
		Name:            rm.DVText{Value: "ev"},
		Data:            &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}, Items: els},
	})
}

func namedElement(name, text string) *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: name}, Value: &rm.DVText{Value: text}}
}

// TestMarshalFlatUnnamedSiblingLeavesTheNamedInstance — REQ-053, REQ-116,
// REQ-140: beside a reused sibling that pins a name, the one that pins none
// does not take the named sibling's instance, whether it occurs at most once
// or repeats, has no FLAT spelling, or is reached by a STRING leaf whose path
// crosses it. The named
// instance, and the underscore attributes of the ELEMENT a collapsed leaf
// hides, are written under the sibling that pins its name alone; an instance
// carrying no name the siblings pin is written under the one that pins none.
func TestMarshalFlatUnnamedSiblingLeavesTheNamedInstance(t *testing.T) {
	const uid = "9fcc1c70-9349-444d-b9cb-8fa817697f50"
	for _, tc := range []struct {
		name string
		wt   *webtemplate.WebTemplate
		comp *rm.Composition
		want map[string]any
	}{
		{
			name: "EVALUATION siblings",
			wt:   twoSiblingWT("A", ""),
			comp: siblingComposition(siblingEvaluation("A", "ay")),
			want: map[string]any{"root/a/x": "ay"},
		},
		{
			name: "ELEMENT siblings",
			wt:   elementSiblingWT("A", ""),
			comp: siblingComposition(&rm.Evaluation{
				ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1",
				Name:            rm.DVText{Value: "ev"},
				Data: &rm.ItemTree{
					ArchetypeNodeID: "at0001",
					Name:            rm.DVText{Value: "tree"},
					Items: []rm.Item{&rm.Element{
						ArchetypeNodeID: "at0002",
						Name:            rm.DVText{Value: "A"},
						UID:             &rm.HierObjectID{Value: uid},
						Value:           &rm.DVText{Value: "ay"},
					}},
				},
			}),
			want: map[string]any{"root/ev/a": "ay", "root/ev/a/_uid": uid},
		},
		{
			name: "repeating ELEMENT sibling",
			wt: func() *webtemplate.WebTemplate {
				wt := elementSiblingWT("A", "")
				wt.Tree.Children[0].Children[1].Max = -1
				return wt
			}(),
			comp: elementSiblingComposition(namedElement("A", "ay"), namedElement("Other", "o1")),
			want: map[string]any{"root/ev/a": "ay", "root/ev/b:0": "o1"},
		},
		{
			// b is a childless ELEMENT node with no value inputs, which has
			// no FLAT spelling: the named instance is no value of its.
			name: "ELEMENT sibling with no FLAT spelling",
			wt: func() *webtemplate.WebTemplate {
				wt := elementSiblingWT("A", "")
				b := wt.Tree.Children[0].Children[1]
				b.RMType, b.Inputs, b.AQLPath = "ELEMENT", nil, strings.TrimSuffix(b.AQLPath, "/value")
				return wt
			}(),
			comp: elementSiblingComposition(namedElement("A", "ay")),
			want: map[string]any{"root/ev/a": "ay"},
		},
		{
			name: "STRING leaves on ACTIVITY siblings",
			wt:   stringLeafSiblingWT("A", ""),
			comp: siblingComposition(&rm.Instruction{
				ArchetypeNodeID: "openEHR-EHR-INSTRUCTION.test.v1",
				Name:            rm.DVText{Value: "ins"},
				Activities: []rm.Activity{{
					ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "A"}, ActionArchetypeID: "openEHR-EHR-ACTION.test.v1",
				}},
			}),
			want: map[string]any{"root/ins/a": "openEHR-EHR-ACTION.test.v1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := MarshalFlat(tc.comp, tc.wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			got := flatMap(t, out)
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
			for k, v := range got {
				if _, wanted := tc.want[k]; wanted {
					continue
				}
				if strings.HasPrefix(k, "root/b") || strings.HasPrefix(k, "root/ev/b") || strings.HasPrefix(k, "root/ins/b") {
					t.Errorf("%s = %v: the instance named A was written under the sibling that pins no name too", k, v)
				}
			}
		})
	}
}

// TestWalkAQLGivesNoSiblingsNameToTheUnnamedSibling — REQ-053, REQ-116: a
// member decode builds for a reused sibling that pins no name does not take,
// from the template, a name a sibling beside it pins: encode would read the
// member back as that sibling's. A name no sibling pins it does take.
func TestWalkAQLGivesNoSiblingsNameToTheUnnamedSibling(t *testing.T) {
	wt := twoSiblingWT("A", "")
	sib := reusedSiblingsOf(wt)
	leaf := wt.Tree.Children[1].Children[0]
	const bare = "/content[openEHR-EHR-EVALUATION.test.v1]"
	for _, tc := range []struct {
		template, want string
	}{
		{template: "A", want: ""},
		{template: "Assessment", want: "Assessment"},
	} {
		t.Run(tc.template, func(t *testing.T) {
			tmpl := &templateIndex{
				names:      map[string]string{bare: tc.template},
				rmTypes:    map[string]string{bare: "EVALUATION"},
				codedNames: map[string]codedName{},
			}
			_, predIndex, predType, err := resolveLeaf(wt, []flatSeg{{"root", -1}, {"b", 0}, {"x", -1}}, sib)
			if err != nil {
				t.Fatalf("resolveLeaf: %v", err)
			}
			compJSON := map[string]any{"_type": "COMPOSITION"}
			if _, err := placeLeaf(compJSON, sib.nodePath(leaf), leaf.AQLPath, predIndex, predType, textJSON("x"), &allocBudget{limit: maxTotalNodes}, tmpl, sib); err != nil {
				t.Fatalf("placeLeaf: %v", err)
			}
			eval := compJSON["content"].([]any)[0].(map[string]any)
			if got := nameValue(eval); got != tc.want {
				t.Errorf("the member's name = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestUnmarshalFlatNamedSiblingsOnOneSingleValuedAttribute — REQ-053,
// REQ-116: two named siblings that stand on one single-valued attribute, an
// OBSERVATION's protocol, cannot both be in a composition. A body with keys
// under both is refused rather than merged into one protocol, which encode
// would read back under one sibling only; a body with keys under one decodes
// into a protocol of that name and re-encodes to the same keys. The same
// holds for a sibling that pins no name beside one that pins a name, whichever
// of their keys sorts first.
func TestUnmarshalFlatNamedSiblingsOnOneSingleValuedAttribute(t *testing.T) {
	leaf := func(id, pin, nodeID string) *webtemplate.Node {
		pred := "at0010"
		if pin != "" {
			pred += ",'" + pin + "'"
		}
		return &webtemplate.Node{
			ID: id, RMType: "DV_TEXT", NodeID: nodeID, Max: 1,
			AQLPath: "/content[openEHR-EHR-OBSERVATION.test.v1]/protocol[" + pred + "]/items[" + nodeID + "]/value",
			Inputs:  []webtemplate.Input{{Type: "TEXT"}},
		}
	}
	observationWT := func(leaves ...*webtemplate.Node) *webtemplate.WebTemplate {
		return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*webtemplate.Node{{
				ID: "obs", RMType: "OBSERVATION", NodeID: "openEHR-EHR-OBSERVATION.test.v1", Max: 1,
				AQLPath:  "/content[openEHR-EHR-OBSERVATION.test.v1]",
				Children: leaves,
			}},
		}}
	}
	wt := observationWT(leaf("a", "A", "at0011"), leaf("b", "B", "at0012"))
	ctx := map[string]any{"ctx/language": "en", "ctx/territory": "NL", "ctx/composer_name": "Dr Test"}

	// The named sibling's id sorts first, then the unnamed one's.
	for _, ids := range []struct{ named, unnamed string }{{named: "a", unnamed: "b"}, {named: "b", unnamed: "a"}} {
		t.Run("named "+ids.named+", unnamed "+ids.unnamed, func(t *testing.T) {
			flat := maps.Clone(ctx)
			flat["root/obs/"+ids.named] = "named value"
			flat["root/obs/"+ids.unnamed] = "unnamed value"
			_, err := UnmarshalFlat(mustJSON(t, flat), observationWT(leaf(ids.named, "A", "at0011"), leaf(ids.unnamed, "", "at0012")))
			if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "single-valued") ||
				!strings.Contains(err.Error(), "[at0010,'A']") || !strings.Contains(err.Error(), "[at0010]") {
				t.Fatalf("err = %v, want the single-valued refusal naming [at0010,'A'] and [at0010]", err)
			}
			if strings.Contains(err.Error(), "named value") {
				t.Errorf("err = %v carries a value of the body", err)
			}
		})
	}

	both := maps.Clone(ctx)
	both["root/obs/a"] = "ay"
	both["root/obs/b"] = "bee"
	if _, err := UnmarshalFlat(mustJSON(t, both), wt); !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "single-valued") {
		t.Fatalf("err = %v, want the single-valued refusal", err)
	}

	one := maps.Clone(ctx)
	one["root/obs/b"] = "bee"
	comp, err := UnmarshalFlat(mustJSON(t, one), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	out, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	if got := flatMap(t, out); got["root/obs/b"] != "bee" || got["root/obs/a"] != nil {
		t.Errorf("round trip = %v, want root/obs/b alone", got)
	}
}

// Reused siblings that no pinned name tells apart share one resolution path,
// so a composition holding ONE instance would resolve it for BOTH sibling ids
// and emit the same data under `root/a/x` and `root/b/x` with no error: one
// sibling's data silently aliased as the other's. MarshalFlat must refuse
// instead, and UnmarshalFlat must refuse a key through either rather than
// merge them. A composition that never touches them still encodes — the guard
// fires only when data resolves at such a node.
func TestMarshalFlatReusedSiblingsRefused(t *testing.T) {
	wt := twoSiblingWT("", "")
	if _, err := MarshalFlat(siblingComposition(siblingEvaluation("A", "hello")), wt); err == nil {
		t.Fatal("MarshalFlat through a reused sibling succeeded — one instance would be emitted under every sibling id")
	} else if !errors.Is(err, rmpath.ErrPathAmbiguous) || !strings.Contains(err.Error(), "reused siblings") {
		t.Fatalf("err = %v, want the reused-siblings ErrPathAmbiguous refusal", err)
	}

	// Same template, no data at the reused siblings: still encodable.
	if _, err := MarshalFlat(siblingComposition(), wt); err != nil {
		t.Fatalf("MarshalFlat with no data at the reused siblings = %v, want nil (guard must fire only when data resolves there)", err)
	}

	flat := map[string]any{"ctx/language": "en", "ctx/territory": "NL", "ctx/composer_name": "Dr Test", "root/b/x": "x"}
	if _, err := UnmarshalFlat(mustJSON(t, flat), wt); err == nil {
		t.Fatal("UnmarshalFlat through a reused sibling succeeded — decode would silently merge siblings")
	} else if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "reused siblings") {
		t.Fatalf("err = %v, want the reused-siblings ErrUnknownPath refusal", err)
	}
}

// REQ-140 — a Web Template node that occurs at most once but that two items
// of the composition fill is refused with rmpath.ErrPathAmbiguous, not
// encoded from one of them: either choice would drop the other's data
// without a word. Both lookups a single-occurrence leaf makes refuse it, the
// one for its value and, where no value is there, the one for the ELEMENT
// that holds its null flavour. Read the second item as absent, or the first
// as the only one, and a row encodes.
func TestMarshalFlatSingleNodeFilledTwiceRefused(t *testing.T) {
	t.Parallel()
	wt := &webtemplate.WebTemplate{
		Tree: &webtemplate.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*webtemplate.Node{{
				ID: "a", RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
				AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]",
				Children: []*webtemplate.Node{{
					ID: "x", RMType: "DV_TEXT", NodeID: "at0002", Max: 1,
					AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]/data[at0001]/items[at0002]/value",
					Inputs:  []webtemplate.Input{{Type: "TEXT"}},
				}},
			}},
		},
	}
	element := func(value rm.DataValue) rm.Item {
		e := &rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}}
		if value != nil {
			e.Value = value
		} else {
			e.NullFlavour = &rm.DVCodedText{
				Value:        "unknown",
				DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			}
		}
		return e
	}
	for _, tc := range []struct {
		name  string
		items []rm.Item
	}{
		{name: "two values", items: []rm.Item{element(&rm.DVText{Value: "one"}), element(&rm.DVText{Value: "two"})}},
		{name: "two null flavours", items: []rm.Item{element(nil), element(nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp := &rm.Composition{
				Name:      rm.DVText{Value: "A"},
				Language:  rm.CodePhrase{CodeString: "en"},
				Territory: rm.CodePhrase{CodeString: "NL"},
				Content: []rm.ContentItem{&rm.Evaluation{
					ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1",
					Name:            rm.DVText{Value: "A"},
					Data:            &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}, Items: tc.items},
				}},
			}
			if flat, err := MarshalFlat(comp, wt); !errors.Is(err, rmpath.ErrPathAmbiguous) {
				t.Fatalf("MarshalFlat of a single-occurrence node two items fill = (%s, %v), want rmpath.ErrPathAmbiguous", flat, err)
			}
		})
	}
}
