package simplified

import (
	"encoding/json"
	"errors"
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
