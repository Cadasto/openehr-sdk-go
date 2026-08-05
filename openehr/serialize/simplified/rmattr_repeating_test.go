package simplified

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// flatMap decodes MarshalFlat output for key-level assertions.
func flatMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// repeatingLeafWT is a minimal WebTemplate whose single DV_TEXT leaf sits on a
// repeatable ELEMENT (`Max: -1`) — the shape three nodes of the vendored
// `Corona_Anamnese` template and one of `constrain_test` already carry.
func repeatingLeafWT() *webtemplate.WebTemplate {
	return &webtemplate.WebTemplate{
		Tree: &webtemplate.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*webtemplate.Node{{
				ID: "ev", RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
				AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]",
				Children: []*webtemplate.Node{{
					ID: "x", RMType: "DV_TEXT", NodeID: "at0002", Max: -1,
					AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]/data[at0001]/items[at0002]/value",
					Inputs:  []webtemplate.Input{{Type: "TEXT"}},
				}},
			}},
		},
	}
}

// repeatingLeafComp builds a composition holding len(elements) instances of the
// repeatable ELEMENT the template above models.
func repeatingLeafComp(elements ...*rm.Element) *rm.Composition {
	items := make([]rm.Item, 0, len(elements))
	for _, el := range elements {
		items = append(items, el)
	}
	return &rm.Composition{
		Name:      rm.DVText{Value: "t"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content: []rm.ContentItem{
			&rm.Evaluation{
				ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1",
				Name:            rm.DVText{Value: "ev"},
				Data: &rm.ItemTree{
					ArchetypeNodeID: "at0001",
					Name:            rm.DVText{Value: "tree"},
					Items:           items,
				},
			},
		},
	}
}

// A repeatable collapsed ELEMENT holding two instances must encode. Before the
// owner walk, REQ-140's underscore emission resolved the hidden ELEMENT by the
// *unindexed* owner path, which matches every instance at once: rmpath returned
// ErrPathAmbiguous, skipNotFound passed it through (it only absorbs
// ErrPathNotFound), and the whole document failed to marshal — a regression
// against compositions that encoded before REQ-140 landed.
func TestMarshalFlatRepeatingElementEncodes(t *testing.T) {
	comp := repeatingLeafComp(
		&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}, Value: &rm.DVText{Value: "one"}},
		&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}, Value: &rm.DVText{Value: "two"}},
	)
	b, err := MarshalFlat(comp, repeatingLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat on a repeating ELEMENT: %v", err)
	}
	flat := flatMap(t, b)
	for key, want := range map[string]any{
		"root/ev/x:0": "one",
		"root/ev/x:1": "two",
	} {
		if got := flat[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// Each repeating instance keeps its *own* underscore attributes. Resolving the
// owner by the unindexed path cannot tell the instances apart, so whichever
// ELEMENT it landed on would have its `_uid` stamped onto every :index.
func TestMarshalFlatRepeatingElementOwnerAttrsPerInstance(t *testing.T) {
	comp := repeatingLeafComp(
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f50"},
			Value: &rm.DVText{Value: "one"},
		},
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f51"},
			Value: &rm.DVText{Value: "two"},
		},
	)
	b, err := MarshalFlat(comp, repeatingLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	flat := flatMap(t, b)
	for key, want := range map[string]any{
		"root/ev/x:0/_uid": "9fcc1c70-9349-444d-b9cb-8fa817697f50",
		"root/ev/x:1/_uid": "9fcc1c70-9349-444d-b9cb-8fa817697f51",
	} {
		if got := flat[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// An ELEMENT carrying only a `_null_flavour` has no value to resolve, so a walk
// keyed on `…/value` never sees it. Keyed on the owner it both survives and
// keeps its place in the :index sequence.
func TestMarshalFlatRepeatingElementNullFlavourOnlyInstance(t *testing.T) {
	comp := repeatingLeafComp(
		&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}, Value: &rm.DVText{Value: "one"}},
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			NullFlavour: &rm.DVCodedText{
				DVText:       rm.DVText{Value: "unknown"},
				DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			},
		},
	)
	b, err := MarshalFlat(comp, repeatingLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	flat := flatMap(t, b)
	if got := flat["root/ev/x:0"]; got != "one" {
		t.Errorf("root/ev/x:0 = %v, want one", got)
	}
	if got := flat["root/ev/x:1/_null_flavour|code"]; got != "253" {
		t.Errorf("root/ev/x:1/_null_flavour|code = %v, want 253", got)
	}
	if _, valued := flat["root/ev/x:1"]; valued {
		t.Error("the null-flavour-only instance emitted a value")
	}
}

// The repeating owner walk must round-trip: what MarshalFlat writes for several
// instances, UnmarshalFlat must read back into the same number of ELEMENTs with
// the same per-instance attributes.
func TestRepeatingElementOwnerAttrsRoundTrip(t *testing.T) {
	wt := repeatingLeafWT()
	comp := repeatingLeafComp(
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f50"},
			Value: &rm.DVText{Value: "one"},
		},
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f51"},
			Value: &rm.DVText{Value: "two"},
		},
	)
	b, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	back, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	again, err := MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (second pass): %v", err)
	}
	first, second := flatMap(t, b), flatMap(t, again)
	for key, want := range first {
		if got := second[key]; got != want {
			t.Errorf("round trip lost %s: %v, want %v", key, got, want)
		}
	}
	if len(second) != len(first) {
		t.Errorf("round trip changed key count: %d, want %d", len(second), len(first))
	}
}

// Every `_identifier:N` fixture in the corpus and in PROBE-089 sits at `:0`, so
// nothing exercised the index beyond the first: mutating the `:0` collapse
// survived the whole suite. A second identifier pins the sequence.
func TestPartyIdentifiersBeyondIndexZero(t *testing.T) {
	str := func(s string) *string { return &s }
	party := &rm.PartyIdentified{
		Name: str("Dr Test"),
		Identifiers: []rm.DVIdentifier{
			{ID: "id-0", Issuer: str("iss-0"), Assigner: str("asg-0"), Type: str("typ-0")},
			{ID: "id-1", Issuer: str("iss-1"), Assigner: str("asg-1"), Type: str("typ-1")},
		},
	}
	out := map[string]any{}
	if err := partyProxyRMAttr(out, "x/_provider", party); err != nil {
		t.Fatalf("partyProxyRMAttr: %v", err)
	}
	for key, want := range map[string]any{
		"x/_provider/_identifier:0|id":     "id-0",
		"x/_provider/_identifier:0|issuer": "iss-0",
		"x/_provider/_identifier:1|id":     "id-1",
		"x/_provider/_identifier:1|issuer": "iss-1",
		"x/_provider/_identifier:1|type":   "typ-1",
	} {
		if got := out[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if _, collapsed := out["x/_provider/_identifier|id"]; collapsed {
		t.Error("a multi-identifier party collapsed an index away")
	}
}

// objectRefRMAttr must not panic on a typed-nil ObjectID: MarshalFlat is public
// API and a Go-constructed OBJECT_REF can hold one. AGENTS.md forbids panics in
// library code; uidRMAttr already guards the same shape.
func TestObjectRefTypedNilIDDoesNotPanic(t *testing.T) {
	for name, ref := range map[string]*rm.ObjectRef{
		"generic":    {ID: (*rm.GenericID)(nil)},
		"hier":       {ID: (*rm.HierObjectID)(nil)},
		"nil-holder": nil,
	} {
		t.Run(name, func(t *testing.T) {
			out := map[string]any{}
			err := objectRefRMAttr(out, "x/_work_flow_id", ref)
			if err != nil && !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want nil or ErrUnsupportedDatatype", err)
			}
		})
	}
}

// --- interval decode symmetry --------------------------------------------

// Decode must refuse exactly what encode refuses. A bounded end with no bound
// used to decode fine and then fail to re-encode, and a bound spelled beside
// `|*_unbounded: true` contradicts the RM equivalence outright.
func TestIntervalDecodeMirrorsEncodeRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, extra := range map[string]map[string]any{
		"bounded end with no bound": {
			rmattrElement + "/_normal_range/lower|magnitude": 20.5,
			rmattrElement + "/_normal_range/lower|unit":      "unit",
		},
		"bound beside an unbounded flag": {
			rmattrElement + "/_normal_range/lower|magnitude": 20.5,
			rmattrElement + "/_normal_range/lower|unit":      "unit",
			rmattrElement + "/_normal_range|lower_unbounded": true,
			rmattrElement + "/_normal_range|upper_unbounded": true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(extra))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// The half-open interval the corpus actually writes must still decode: a lower
// bound plus an explicit `|upper_unbounded`.
func TestIntervalHalfOpenDecodes(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(map[string]any{
		rmattrElement + "/_normal_range/lower|magnitude": 20.5,
		rmattrElement + "/_normal_range/lower|unit":      "unit",
		rmattrElement + "/_normal_range|upper_unbounded": true,
	}))
	q := elementValue[rm.DVQuantity](t, comp)
	if q.NormalRange == nil {
		t.Fatal("no normal range decoded")
	}
	if !q.NormalRange.UpperUnbounded {
		t.Error("upper_unbounded = false, want true")
	}
	if q.NormalRange.Lower.Magnitude != 20.5 {
		t.Errorf("lower magnitude = %v, want 20.5", q.NormalRange.Lower.Magnitude)
	}
}

// --- composer projection --------------------------------------------------

// The composer's `external_ref` and `identifiers` are dropped on encode by
// design (ADR 0015 — the ctx/ short form carries the name alone; registered in
// deviations.md). Prose alone left that untested, so a regression either way —
// a silent widening or an accidental refusal — would have gone unnoticed.
func TestComposerExtrasProjectedToNameOnly(t *testing.T) {
	name := "Dr Test"
	comp := repeatingLeafComp(
		&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}, Value: &rm.DVText{Value: "one"}},
	)
	comp.Composer = &rm.PartyIdentified{
		Name:        &name,
		ExternalRef: &rm.PartyRef{ObjectRef: rm.ObjectRef{ID: &rm.GenericID{Value: "p-1", Scheme: "local"}, Namespace: "demographic", Type: "PERSON"}},
		Identifiers: []rm.DVIdentifier{{ID: "id-0"}},
	}
	b, err := MarshalFlat(comp, repeatingLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	flat := flatMap(t, b)
	if got := flat["ctx/composer_name"]; got != name {
		t.Errorf("ctx/composer_name = %v, want %q", got, name)
	}
	for key := range flat {
		if strings.HasPrefix(key, "ctx/composer") && key != "ctx/composer_name" {
			t.Errorf("the ctx/ projection emitted %q; it carries the name alone", key)
		}
		if strings.Contains(key, "composer/") {
			t.Errorf("emitted a real-path composer key %q, which decode refuses", key)
		}
	}
}

// --- decode-side party parity ---------------------------------------------

// Decode must refuse the party shapes encode refuses, or a body decodes and
// then fails to re-encode.
func TestPartyDecodeMirrorsEncodeRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, extra := range map[string]map[string]any{
		"present but empty name": {
			rmattrRoot + "/context/_health_care_facility|name": "",
		},
		"relationship without an identified half": {
			rmattrObs + "/subject/relationship|code":        "10",
			rmattrObs + "/subject/relationship|value":       "mother",
			rmattrObs + "/subject/relationship|terminology": "openehr",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(extra)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// partyRefToFlat carries the same typed-nil ObjectID hazard objectRefRMAttr
// does, on its own switch — pinned so a future arm cannot reintroduce it.
func TestPartyRefTypedNilIDDoesNotPanic(t *testing.T) {
	name := "Dr Test"
	for label, id := range map[string]rm.ObjectID{
		"generic": (*rm.GenericID)(nil),
		"hier":    (*rm.HierObjectID)(nil),
	} {
		t.Run(label, func(t *testing.T) {
			out := map[string]any{}
			party := &rm.PartyIdentified{
				Name:        &name,
				ExternalRef: &rm.PartyRef{ObjectRef: rm.ObjectRef{ID: id, Namespace: "demographic", Type: "PERSON"}},
			}
			err := partyRMAttr(out, "x/_provider", party)
			if err != nil && !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want nil or ErrUnsupportedDatatype", err)
			}
		})
	}
}

// The null-flavour-only repeating instance must survive the *decode* leg too,
// not just encode: it is the one instance the value walk cannot see, so a
// regression would silently renumber the surviving instances.
func TestRepeatingNullFlavourOnlyInstanceRoundTrips(t *testing.T) {
	wt := repeatingLeafWT()
	comp := repeatingLeafComp(
		&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"}, Value: &rm.DVText{Value: "one"}},
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			NullFlavour: &rm.DVCodedText{
				DVText:       rm.DVText{Value: "unknown"},
				DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			},
		},
	)
	b, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	back, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	again, err := MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (second pass): %v", err)
	}
	first, second := flatMap(t, b), flatMap(t, again)
	if len(first) != len(second) {
		t.Errorf("key count %d -> %d across the round trip", len(first), len(second))
	}
	for key, want := range first {
		if got := second[key]; got != want {
			t.Errorf("round trip lost %s: %v, want %v", key, got, want)
		}
	}
	if got := second["root/ev/x:1/_null_flavour|code"]; got != "253" {
		t.Errorf("the null-flavour-only instance did not survive decode: %v", got)
	}
}

// --- STRUCTURED leg -------------------------------------------------------

// MarshalStructured delegates to encodeFlat, so the repeating owner walk
// reaches it too — but STRUCTURED is a separate public surface and every other
// repeating test here is FLAT-only. It nests the instances as an array, each
// carrying its own `_uid`, and the OPT-free interconversion re-spells every
// segment with an explicit index (`x:1/_uid:0`) and back.
func TestRepeatingElementThroughStructured(t *testing.T) {
	wt := repeatingLeafWT()
	comp := repeatingLeafComp(
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f50"},
			Value: &rm.DVText{Value: "one"},
		},
		&rm.Element{
			ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "x"},
			UID:   &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f51"},
			Value: &rm.DVText{Value: "two"},
		},
	)
	s, err := MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	back, err := UnmarshalStructured(s, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured: %v", err)
	}
	viaStructured, err := MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat after STRUCTURED: %v", err)
	}
	direct, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	want, got := flatMap(t, direct), flatMap(t, viaStructured)
	if len(want) != len(got) {
		t.Errorf("key count %d -> %d through STRUCTURED", len(want), len(got))
	}
	for key, w := range want {
		if g := got[key]; g != w {
			t.Errorf("STRUCTURED round trip lost %s: %v, want %v", key, g, w)
		}
	}

	// OPT-free FLAT -> STRUCTURED -> FLAT keeps each instance's own owner attrs.
	sf, err := FlatToStructured(direct)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	fs, err := StructuredToFlat(sf)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	free := flatMap(t, fs)
	for key, want := range map[string]any{
		"root/ev:0/x:0/_uid:0": "9fcc1c70-9349-444d-b9cb-8fa817697f50",
		"root/ev:0/x:1/_uid:0": "9fcc1c70-9349-444d-b9cb-8fa817697f51",
	} {
		if got := free[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// --- underscore attrs on a composite leaf ---------------------------------

// intervalLeafOwnerWT models a collapsed ELEMENT whose value is a
// DV_INTERVAL leaf — a composite leaf that also owns REQ-140 underscore
// attributes one attribute up.
func intervalLeafOwnerWT() *webtemplate.WebTemplate {
	return &webtemplate.WebTemplate{
		Tree: &webtemplate.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*webtemplate.Node{{
				ID: "ev", RMType: "EVALUATION", NodeID: "openEHR-EHR-EVALUATION.test.v1", Max: 1,
				AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]",
				Children: []*webtemplate.Node{{
					ID: "iv", RMType: "DV_INTERVAL<DV_COUNT>", NodeID: "at0002", Max: 1,
					AQLPath: "/content[openEHR-EHR-EVALUATION.test.v1]/data[at0001]/items[at0002]/value",
				}},
			}},
		},
	}
}

// A composite leaf's key splitter used to fold *everything* after the leaf into
// that leaf's own tails — including the underscore attributes of the ELEMENT
// the Web Template collapsed it into. Those keys were then deleted from the
// `_` router's view and refused by the leaf grammar, so `MarshalFlat` emitted a
// body `UnmarshalFlat` rejected: a round-trip break on a shape this codec
// writes itself, and on the `<leaf>/_uid` the upstream corpus writes too.
func TestCompositeLeafKeepsItsOwnerUnderscoreAttrs(t *testing.T) {
	wt := intervalLeafOwnerWT()
	comp := &rm.Composition{
		Name:      rm.DVText{Value: "t"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content: []rm.ContentItem{&rm.Evaluation{
			ArchetypeNodeID: "openEHR-EHR-EVALUATION.test.v1", Name: rm.DVText{Value: "ev"},
			Data: &rm.ItemTree{
				ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"},
				Items: []rm.Item{&rm.Element{
					ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "iv"},
					UID: &rm.HierObjectID{Value: "9fcc1c70-9349-444d-b9cb-8fa817697f50"},
					Links: []rm.Link{{
						Meaning: rm.DVText{Value: "m"}, Type: rm.DVText{Value: "t"},
						Target: rm.DVEHRURI{DVURI: rm.DVURI{Value: "ehr://x"}},
					}},
					Value: &rm.DVInterval[rm.DVCount]{Interval: rm.Interval[rm.DVCount]{
						Lower: rm.DVCount{Magnitude: 1}, Upper: rm.DVCount{Magnitude: 8},
						LowerIncluded: true, UpperIncluded: true,
					}},
				}},
			},
		}},
	}
	b, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	first := flatMap(t, b)
	for _, key := range []string{"root/ev/iv/_uid", "root/ev/iv/_link:0|meaning", "root/ev/iv/lower"} {
		if _, ok := first[key]; !ok {
			t.Errorf("encode did not write %q", key)
		}
	}
	back, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat rejected a body MarshalFlat wrote: %v", err)
	}
	again, err := MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (second pass): %v", err)
	}
	second := flatMap(t, again)
	if len(first) != len(second) {
		t.Errorf("key count %d -> %d across the round trip", len(first), len(second))
	}
	for key, want := range first {
		if got := second[key]; got != want {
			t.Errorf("round trip lost %s: %v, want %v", key, got, want)
		}
	}
}
