package simplified

// REQ-053 § Leaf datatypes, the STRING row. The Web Template gives one RM
// String attribute a leaf of its own, ACTIVITY `action_archetype_id`, and the
// FLAT form carries it as a bare JSON string at the leaf's key, with no suffix.
// Before, decode refused the key (`unsupported datatype: STRING`) and encode
// skipped it without a word.

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// stringLeafKey is the STRING leaf's FLAT key in [stringLeafWT].
const stringLeafKey = "root/ins/act/action_archetype_id"

// stringLeafValue is the reference's spelling of the attribute: a regular
// expression between slashes (the PROBE-086 corpus writes
// "/openEHR-EHR-CLUSTER.conformance_action.v0/").
const stringLeafValue = "/openEHR-EHR-ACTION.test.v1/"

// stringLeafWT models an INSTRUCTION whose ACTIVITY carries the in-context
// STRING leaf, the way the SDK's Web Template builder emits it.
func stringLeafWT() *webtemplate.WebTemplate {
	const ins = "/content[openEHR-EHR-INSTRUCTION.test.v1]"
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "ins", RMType: "INSTRUCTION", NodeID: "openEHR-EHR-INSTRUCTION.test.v1", Max: 1,
			AQLPath: ins,
			Children: []*webtemplate.Node{{
				ID: "act", RMType: "ACTIVITY", NodeID: "at0001", Max: 1,
				AQLPath: ins + "/activities[at0001]",
				Children: []*webtemplate.Node{{
					ID: "action_archetype_id", Name: "Action_archetype_id", RMType: "STRING", Max: 1,
					AQLPath: ins + "/activities[at0001]/action_archetype_id",
					Inputs:  []webtemplate.Input{{Type: "TEXT"}},
				}},
			}},
		}},
	}}
}

// stringLeafComp holds one ACTIVITY whose action_archetype_id is the given value.
func stringLeafComp(actionArchetypeID string) *rm.Composition {
	return &rm.Composition{
		Name:      rm.DVText{Value: "t"},
		Language:  rm.CodePhrase{CodeString: "en"},
		Territory: rm.CodePhrase{CodeString: "NL"},
		Content: []rm.ContentItem{&rm.Instruction{
			ArchetypeNodeID: "openEHR-EHR-INSTRUCTION.test.v1", Name: rm.DVText{Value: "ins"},
			Narrative: &rm.DVText{Value: "n"},
			Activities: []rm.Activity{{
				ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "act"},
				ActionArchetypeID: actionArchetypeID,
			}},
		}},
	}
}

// stringLeafBody is a decodable FLAT body carrying the STRING leaf plus extra.
func stringLeafBody(extra map[string]any) []byte {
	body := map[string]any{"ctx/language": "en", "ctx/territory": "NL"}
	maps.Copy(body, extra)
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return b
}

// firstActivity digs the one ACTIVITY out of a decoded composition.
func firstActivity(t *testing.T, comp *rm.Composition) rm.Activity {
	t.Helper()
	if len(comp.Content) != 1 {
		t.Fatalf("decoded %d content items, want 1", len(comp.Content))
	}
	ins, ok := comp.Content[0].(*rm.Instruction)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Instruction", comp.Content[0])
	}
	if len(ins.Activities) != 1 {
		t.Fatalf("decoded %d activities, want 1", len(ins.Activities))
	}
	return ins.Activities[0]
}

// TestStringLeafEncodesBareValue — REQ-053. Encode writes the STRING leaf as a
// bare JSON string at its key, and nothing else under that key.
func TestStringLeafEncodesBareValue(t *testing.T) {
	b, err := MarshalFlat(stringLeafComp(stringLeafValue), stringLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	flat := flatMap(t, b)
	if got := flat[stringLeafKey]; got != stringLeafValue {
		t.Errorf("MarshalFlat wrote %s = %#v, want the bare string %q; emitted: %v",
			stringLeafKey, got, stringLeafValue, slices.Sorted(maps.Keys(flat)))
	}
	for k := range flat {
		if strings.HasPrefix(k, stringLeafKey) && k != stringLeafKey {
			t.Errorf("MarshalFlat wrote %q beside the STRING leaf; the leaf carries no suffix", k)
		}
	}
}

// TestStringLeafAbsentWhenEmpty — REQ-053. An RM String the composition leaves
// empty is absent, not an empty key: Go has no other spelling of "not set".
func TestStringLeafAbsentWhenEmpty(t *testing.T) {
	b, err := MarshalFlat(stringLeafComp(""), stringLeafWT())
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	if v, ok := flatMap(t, b)[stringLeafKey]; ok {
		t.Errorf("MarshalFlat wrote %s = %#v for an empty action_archetype_id, want no key", stringLeafKey, v)
	}
}

// TestStringLeafDecodesBareValue — REQ-053. Decode accepts the bare string and
// rebuilds the RM String, and the round trip gives back the key it read, through
// FLAT and through STRUCTURED.
func TestStringLeafDecodesBareValue(t *testing.T) {
	wt := stringLeafWT()
	body := stringLeafBody(map[string]any{stringLeafKey: stringLeafValue})
	comp, err := UnmarshalFlat(body, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if got := firstActivity(t, comp).ActionArchetypeID; got != stringLeafValue {
		t.Errorf("ACTIVITY.action_archetype_id = %q, want %q", got, stringLeafValue)
	}
	again, err := MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	if diffs := diffFlat(flatMap(t, body), flatMap(t, again)); len(diffs) > 0 {
		t.Errorf("FLAT round trip differs:\n  %s", strings.Join(diffs, "\n  "))
	}

	s, err := MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	back, err := UnmarshalStructured(s, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured(%s): %v", s, err)
	}
	if got := firstActivity(t, back).ActionArchetypeID; got != stringLeafValue {
		t.Errorf("STRUCTURED round trip: ACTIVITY.action_archetype_id = %q, want %q", got, stringLeafValue)
	}
}

// TestStringLeafRefusesSuffixes — REQ-053, REQ-140. The STRING leaf is "bare
// value only", so a suffix there, |raw included, is a typed refusal rather
// than a silent drop.
func TestStringLeafRefusesSuffixes(t *testing.T) {
	for name, extra := range map[string]map[string]any{
		"|value alone":         {stringLeafKey + "|value": stringLeafValue},
		"|raw alone":           {stringLeafKey + "|raw": map[string]any{"_type": "STRING", "value": stringLeafValue}},
		"bare value and |code": {stringLeafKey: stringLeafValue, stringLeafKey + "|code": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := UnmarshalFlat(stringLeafBody(extra), stringLeafWT())
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("UnmarshalFlat = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// TestStringLeafRefusesNonString — REQ-053. The bare value is a JSON string; a
// number there is a malformed body, refused with the key named.
func TestStringLeafRefusesNonString(t *testing.T) {
	_, err := UnmarshalFlat(stringLeafBody(map[string]any{stringLeafKey: 42}), stringLeafWT())
	if err == nil {
		t.Fatal("UnmarshalFlat accepted a number at the STRING leaf")
	}
	if !strings.Contains(err.Error(), stringLeafKey) {
		t.Errorf("UnmarshalFlat = %v, want the error to name %q", err, stringLeafKey)
	}
}

// TestStringLeafCorpusSpelling — REQ-053. The PROBE-086 corpus's own key and
// value, through the corpus template, decode and come back.
func TestStringLeafCorpusSpelling(t *testing.T) {
	wt, _ := conformanceWT(t)
	const key = rmattrSection + "/conformance_instruction/current_activity/action_archetype_id"
	const value = "/openEHR-EHR-CLUSTER.conformance_action.v0/"
	comp := decodeRMAttr(t, wt, rmattrBody(map[string]any{key: value}))
	if got := reencodeRMAttr(t, wt, comp)[key]; got != value {
		t.Errorf("re-encode of %s = %#v, want %q", key, got, value)
	}
}
