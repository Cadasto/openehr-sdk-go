package simplified

// REQ-140 — the composition's own name, `<root>/_name`: the one underscore
// family the grammar admits at the template root only. Decode sets
// COMPOSITION.name from it, in the DV_TEXT leaf grammar (a coded name rides
// REQ-053's `|code` substitution), in place of the Web Template root's name;
// encode writes it unless the name is a plain DV_TEXT whose value is that
// root's name. At any other node `_name` is unmatched content (REQ-170).

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// rootNameKey is the composition's own name at the corpus template root.
const rootNameKey = rmattrRoot + "/_name"

// canonicalName renders a composition's name as canonical JSON, the form two
// names are compared in.
func canonicalName(t *testing.T, name rm.DVTextLike) string {
	t.Helper()
	if name == nil {
		return "<nil>"
	}
	b, err := canjson.Marshal(name)
	if err != nil {
		t.Fatalf("canjson.Marshal(name): %v", err)
	}
	return string(b)
}

// nameKeys is the part of a FLAT map spelled under the root's `_name`.
func nameKeys(flat map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range flat {
		if k == rootNameKey || strings.HasPrefix(k, rootNameKey+"|") || strings.HasPrefix(k, rootNameKey+":") {
			out[k] = v
		}
	}
	return out
}

// TestREQ140CompositionNameDecodes — REQ-140: `<root>/_name` sets the
// composition's name in place of the Web Template root's, in the DV_TEXT leaf
// grammar: a bare string is a DV_TEXT, `|formatting` rides beside it, and a
// coded name rides the `|code` + `|value` + `|terminology` substitution. The
// root's indexed spelling `<root>:0/_name`, which `_uid` accepts too, reads
// the same. The decode reads every key, so strict refuses none, and the
// STRUCTURED form of the same body decodes to the same name. Re-encoding
// writes the keys back as they were.
func TestREQ140CompositionNameDecodes(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		keys map[string]any
		want rm.DVTextLike
	}{
		"plain": {
			keys: map[string]any{rootNameKey: "Routine checkup"},
			want: &rm.DVText{Value: "Routine checkup"},
		},
		"formatting": {
			keys: map[string]any{rootNameKey: "Routine checkup", rootNameKey + "|formatting": "plain"},
			want: &rm.DVText{Value: "Routine checkup", Formatting: new("plain")},
		},
		"coded": {
			keys: map[string]any{
				rootNameKey + "|code": "at0001", rootNameKey + "|value": "Routine checkup",
				rootNameKey + "|terminology": "local",
			},
			want: &rm.DVCodedText{Value: "Routine checkup", DefiningCode: rm.CodePhrase{
				CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"},
			}},
		},
		"coded, valued as the template root's name": {
			keys: map[string]any{
				rootNameKey + "|code": "at0000", rootNameKey + "|value": rmattrRoot,
				rootNameKey + "|terminology": "local",
			},
			want: &rm.DVCodedText{Value: rmattrRoot, DefiningCode: rm.CodePhrase{
				CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for form, comp := range decodeBothForms(t, wt, rmattrBody(tc.keys)) {
				if got, want := canonicalName(t, comp.Name), canonicalName(t, tc.want); got != want {
					t.Errorf("%s: name = %s, want %s", form, got, want)
				}
			}
			comp := assertRMAttrRoundTrip(t, wt, tc.keys)
			if got := nameKeys(reencodeRMAttr(t, wt, comp)); !maps.Equal(got, tc.keys) {
				t.Errorf("re-encoded name keys = %v, want %v", got, tc.keys)
			}
		})
	}
	t.Run("root indexed", func(t *testing.T) {
		comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(map[string]any{rmattrRoot + ":0/_name": "Routine checkup"})), wt, WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalFlat = %v", err)
		}
		if got, want := canonicalName(t, comp.Name), canonicalName(t, &rm.DVText{Value: "Routine checkup"}); got != want {
			t.Errorf("name = %s, want %s", got, want)
		}
	})
}

// TestREQ140CompositionNameSurvivesWithTemplate — REQ-140: with WithTemplate
// the decode names the nodes it rebuilds from the template, and the
// composition's own name decoded from `_name` stays as the body spells it.
func TestREQ140CompositionNameSurvivesWithTemplate(t *testing.T) {
	wt, compiled := conformanceWT(t)
	comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(map[string]any{rootNameKey: "Routine checkup"})), wt,
		WithTemplate(compiled), WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(WithTemplate) = %v", err)
	}
	if got, want := canonicalName(t, comp.Name), canonicalName(t, &rm.DVText{Value: "Routine checkup"}); got != want {
		t.Errorf("name = %s, want %s", got, want)
	}
}

// TestREQ140CompositionNameEncodes — REQ-140: encode writes the composition's
// name as `<root>/_name`, with the DV_TEXT leaf encoder, unless it is a plain
// DV_TEXT (no coded subtype, nothing set but its value) whose value is the
// name a decode of a body without `_name` gives: the Web Template root's name,
// or the template id where the root has none. So a composition named after its
// template writes no extra key, and every other name round-trips.
func TestREQ140CompositionNameEncodes(t *testing.T) {
	coded := func(value string) *rm.DVCodedText {
		return &rm.DVCodedText{Value: value, DefiningCode: rm.CodePhrase{
			CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"},
		}}
	}
	for name, tc := range map[string]struct {
		rootName string // the Web Template root's name; the template id is the corpus root id
		name     rm.DVTextLike
		want     map[string]any // nil: the keys are checked by suffix only
		raw      bool
	}{
		"a name that differs": {
			rootName: rmattrRoot, name: &rm.DVText{Value: "Routine checkup"},
			want: map[string]any{rootNameKey: "Routine checkup"},
		},
		"the root's own name": {
			rootName: rmattrRoot, name: &rm.DVText{Value: rmattrRoot}, want: map[string]any{},
		},
		"no name": {rootName: rmattrRoot, name: nil, want: map[string]any{}},
		"the root's own name, formatted": {
			rootName: rmattrRoot, name: &rm.DVText{Value: rmattrRoot, Formatting: new("plain")},
			want: map[string]any{rootNameKey: rmattrRoot, rootNameKey + "|formatting": "plain"},
		},
		"the root's own name, coded": {
			rootName: rmattrRoot, name: coded(rmattrRoot),
			want: map[string]any{
				rootNameKey + "|code": "at0000", rootNameKey + "|value": rmattrRoot,
				rootNameKey + "|terminology": "local",
			},
		},
		"a name the suffixes cannot carry": {
			rootName: rmattrRoot,
			name: &rm.DVText{Value: "Routine checkup", Mappings: []rm.TermMapping{{
				Match: "=", Target: rm.CodePhrase{CodeString: "21794005", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
			}}},
			raw: true,
		},
		"a renamed root's own name": {
			rootName: "Renamed root", name: &rm.DVText{Value: "Renamed root"}, want: map[string]any{},
		},
		"the template id beside a renamed root": {
			rootName: "Renamed root", name: &rm.DVText{Value: rmattrRoot},
			want: map[string]any{rootNameKey: rmattrRoot},
		},
		"the template id where the root has no name": {
			rootName: "", name: &rm.DVText{Value: rmattrRoot}, want: map[string]any{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			wt, _ := conformanceWT(t)
			wt.Tree.Name = tc.rootName
			comp := decodeRMAttr(t, wt, rmattrBody(nil))
			comp.Name = tc.name
			got := nameKeys(reencodeRMAttr(t, wt, comp))
			switch {
			case tc.raw:
				if _, ok := got[rootNameKey+"|raw"]; !ok || len(got) != 1 {
					t.Errorf("name keys = %v, want %q alone", got, rootNameKey+"|raw")
				}
			case !maps.Equal(got, tc.want):
				t.Errorf("name keys = %v, want %v", got, tc.want)
			}
			// The round trip is stable: what encode writes decodes to the name the
			// composition holds, or, for none, to the name decode gives a body
			// without `_name`.
			back := decodeRMAttr(t, wt, reencodeRMAttr(t, wt, comp))
			want := tc.name
			if want == nil {
				want = &rm.DVText{Value: rmattrRoot}
				if tc.rootName != "" {
					want = &rm.DVText{Value: tc.rootName}
				}
			}
			if gotName, wantName := canonicalName(t, back.Name), canonicalName(t, want); gotName != wantName {
				t.Errorf("round-tripped name = %s, want %s", gotName, wantName)
			}
		})
	}
}

// TestREQ140PlainTextIsADVTextWithAValueAlone — REQ-140: the name encode
// leaves unwritten is a "plain DV_TEXT": its type is DV_TEXT, not the coded
// subtype, and it sets nothing but its value. Each of the two halves is
// judged on its own, so a coded text spelled with a value alone is not plain.
func TestREQ140PlainTextIsADVTextWithAValueAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		m    map[string]any
		want bool
	}{
		"DV_TEXT with a value":          {map[string]any{"_type": "DV_TEXT", "value": "x"}, true},
		"DV_CODED_TEXT with a value":    {map[string]any{"_type": "DV_CODED_TEXT", "value": "x"}, false},
		"DV_TEXT with formatting":       {map[string]any{"_type": "DV_TEXT", "value": "x", "formatting": "plain"}, false},
		"no type":                       {map[string]any{"value": "x"}, false},
		"DV_TEXT with a mapping beside": {map[string]any{"_type": "DV_TEXT", "value": "x", "mappings": []any{}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := plainText(tc.m); got != tc.want {
				t.Errorf("plainText(%v) = %v, want %v", tc.m, got, tc.want)
			}
		})
	}
}

// TestREQ140CompositionNameWithoutKeyIsTheRootName — REQ-140: a body without
// `_name` decodes the composition's name from the Web Template root's name,
// or from the template id where the root has none, as it always has.
func TestREQ140CompositionNameWithoutKeyIsTheRootName(t *testing.T) {
	for rootName, want := range map[string]string{"Renamed root": "Renamed root", "": rmattrRoot} {
		t.Run(cmpLabel(rootName), func(t *testing.T) {
			wt, _ := conformanceWT(t)
			wt.Tree.Name = rootName
			comp := decodeRMAttr(t, wt, rmattrBody(nil))
			if got, w := canonicalName(t, comp.Name), canonicalName(t, &rm.DVText{Value: want}); got != w {
				t.Errorf("name = %s, want %s", got, w)
			}
		})
	}
}

// cmpLabel names a sub-test for a value that may be empty.
func cmpLabel(s string) string {
	if s == "" {
		return "<empty>"
	}
	return s
}

// TestREQ140CompositionNameNullIsAbsent — REQ-053: a `_name` key holding null
// reads as absent, so the composition takes the name decode gives a body
// without it, and the key is not reported: the decode reads it.
func TestREQ140CompositionNameNullIsAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, keys := range map[string]map[string]any{
		"bare":  {rootNameKey: nil},
		"coded": {rootNameKey + "|code": nil, rootNameKey + "|value": nil},
	} {
		t.Run(name, func(t *testing.T) {
			assertSameDecodeBothForms(t, wt, rmattrBody(keys), rmattrBody(nil))
		})
	}
}

// TestREQ140CompositionNameMustFit — REQ-053: a `_name` value that does not
// fit the DV_TEXT it decodes into is refused in every mode, wrapping
// ErrUnsupportedDatatype and naming the key; a `|code` that is not a string
// promotes the name to a DV_CODED_TEXT and then fails the fit check naming the
// `|code` key. The STRUCTURED form is refused the same way.
func TestREQ140CompositionNameMustFit(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		keys  map[string]any
		named string
	}{
		"a number":           {map[string]any{rootNameKey: 5}, rootNameKey},
		"a Boolean":          {map[string]any{rootNameKey: true}, rootNameKey},
		"a number for |code": {map[string]any{rootNameKey + "|code": 5, rootNameKey + "|value": "x"}, rootNameKey + "|code"},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(tc.keys)
			flat := refusedInEveryMode(t, wt, body)
			if !errors.Is(flat, ErrUnsupportedDatatype) || !strings.Contains(flat.Error(), `"`+tc.named+`"`) {
				t.Errorf("FLAT: decode = %v, want ErrUnsupportedDatatype naming %q", flat, tc.named)
			}
			if structured := structuredRefusedInEveryMode(t, wt, body); !errors.Is(structured, ErrUnsupportedDatatype) {
				t.Errorf("STRUCTURED: decode = %v, want ErrUnsupportedDatatype", structured)
			}
		})
	}
}

// TestREQ140CompositionNameSpelledOnce — REQ-140: `_name` addresses one
// single-valued RM attribute, so a second spelling of it, an index past :0 on
// it or on the root, is refused with ErrUnknownPath, as `_uid`'s is.
func TestREQ140CompositionNameSpelledOnce(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, keys := range map[string]map[string]any{
		"_name beside _name:0":         {rootNameKey: "a", rootNameKey + ":0": "a"},
		"_name:1":                      {rootNameKey + ":1": "a"},
		"_name beside the root's :0":   {rootNameKey: "a", rmattrRoot + ":0/_name": "a"},
		"_name under the root's :1":    {rmattrRoot + ":1/_name": "a"},
		"coded beside bare, two slots": {rootNameKey: "a", rootNameKey + ":0|code": "at0001", rootNameKey + ":0|value": "a"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := refusedInEveryMode(t, wt, rmattrBody(keys)); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// TestREQ140NameOffTheRootIsUnmatched — REQ-140 / REQ-170: `_name` is a family
// of the template root alone. At any other node, whether a SECTION, an ENTRY,
// a collapsed ELEMENT leaf or the EVENT_CONTEXT, it is unmatched content:
// ignored by default, reported, and refused with ErrUnknownPath under strict,
// in FLAT and in STRUCTURED.
func TestREQ140NameOffTheRootIsUnmatched(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, owner := range []string{rmattrSection, rmattrObs, rmattrElement, rmattrContext} {
		t.Run(owner, func(t *testing.T) {
			assertUnmatched(t, wt, rmattrBody(map[string]any{owner + "/_name": "x"}), owner+"/_name")
			coded := map[string]any{owner + "/_name|code": "at0001", owner + "/_name|value": "x"}
			assertUnmatched(t, wt, rmattrBody(coded), slices.Sorted(maps.Keys(coded))...)
		})
	}
	const secret = "secret-value-0451"
	for name, at := range map[string]string{
		"SECTION":           rmattrRoot + "/conformance_section/0",
		"OBSERVATION":       rmattrRoot + "/conformance_section/0/conformance_observation/0",
		"collapsed ELEMENT": rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity/0",
	} {
		t.Run("STRUCTURED "+name, func(t *testing.T) {
			ref, doc := structuredDoc(t, rmattrBody(nil))
			structuredObject(t, doc, at)["_name"] = []any{secret}
			assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, at+"/_name")
		})
	}
}

// TestREQ140CompositionNameInterconverts — REQ-140: STRUCTURED carries the
// root's `_name` as an array-valued member of the root object, the spelling
// the reference writes, and OPT-free FLAT ↔ STRUCTURED interconversion
// preserves it, a coded name included.
func TestREQ140CompositionNameInterconverts(t *testing.T) {
	for name, tc := range map[string]struct {
		flat map[string]any
		want any
	}{
		"plain": {map[string]any{rootNameKey: "Routine checkup"}, []any{"Routine checkup"}},
		"coded": {
			map[string]any{rootNameKey + "|code": "at0001", rootNameKey + "|value": "Routine checkup"},
			[]any{map[string]any{"|code": "at0001", "|value": "Routine checkup"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, doc := structuredDoc(t, tc.flat)
			got := doc[rmattrRoot].(map[string]any)["_name"]
			if !jsonEqual(t, got, tc.want) {
				t.Errorf("STRUCTURED root _name = %s, want %s", mustJSON(t, got), mustJSON(t, tc.want))
			}
			back, err := StructuredToFlat(mustJSON(t, doc))
			if err != nil {
				t.Fatalf("StructuredToFlat = %v", err)
			}
			var flat map[string]any
			if err := json.Unmarshal(back, &flat); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{}
			for k, v := range tc.flat {
				want[strings.Replace(k, "/_name", "/_name:0", 1)] = v
			}
			if !jsonEqual(t, flat, want) {
				t.Errorf("StructuredToFlat = %s, want %s", back, mustJSON(t, want))
			}
		})
	}
}

// jsonEqual compares two JSON-shaped values by their JSON text.
func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	return string(mustJSON(t, a)) == string(mustJSON(t, b))
}
