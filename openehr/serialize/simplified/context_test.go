package simplified_test

// REQ-053 — ctx/ context: composition-level metadata (language, territory,
// composer, time, setting) is carried under the ctx/ prefix (FLAT) / a ctx
// object (STRUCTURED). Language + territory are mandatory on decode.
import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// bodyWeightOPT is a fixture whose Web Template root id ("body_weight") sorts
// *before* the literal "ctx/" prefix, where minimalObsOPT ("minimal") and
// vitalSignsOPT ("encounter") sort after it. siphonContext walks a body in sorted
// key order, so the pair pins both directions of that walk — see
// TestMetadataCompositeValueRefusedInBothKeyOrders.
const bodyWeightOPT = "../../../testkit/cassettes/templates/body_weight.opt"

// corpusFlatOPT is the PROBE-086 corpus template — the one vendored OPT whose
// Web Template carries a context/setting node, so it is the only target on which
// the ctx/-owned-leaf shadowing is observable.
const corpusFlatOPT = "../../../testkit/cassettes/flat-conformance/templates/conformance_ehrbase.de.v0.opt"

func TestContextEncodeAndRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m1 map[string]any
	if err := json.Unmarshal(f1, &m1); err != nil {
		t.Fatal(err)
	}
	// Mandatory + common context fields must be emitted (instance.Generate sets
	// language=en, territory=NL, composer="Test Composer", start_time set).
	wantCtx := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Test Composer",
	}
	for k, want := range wantCtx {
		if m1[k] != want {
			t.Errorf("%s = %#v, want %#v", k, m1[k], want)
		}
	}
	if _, ok := m1["ctx/time"]; !ok {
		t.Error("ctx/time missing")
	}

	// Round-trip: decode rebuilds the context, re-encode reproduces the FLAT.
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ctx/language", "ctx/territory", "ctx/composer_name", "ctx/time"} {
		if m1[k] != m2[k] {
			t.Errorf("ctx round-trip %s: %#v -> %#v", k, m1[k], m2[k])
		}
	}
}

// TestComposerSelfRoundTrip pins the PARTY_SELF composer branch end-to-end:
// encode emits ctx/composer_self, decode rebuilds PARTY_SELF, and the FLAT
// survives a second round-trip. (The generated fixtures always use
// PARTY_IDENTIFIED, so without this test the branch has zero coverage and the
// WithTemplate default would mask its loss.)
func TestComposerSelfRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	comp.Composer = &rm.PartySelf{}

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	if m["ctx/composer_self"] != true {
		t.Fatalf("ctx/composer_self = %#v, want true (keys: %v)", m["ctx/composer_self"], m)
	}
	if _, ok := m["ctx/composer_name"]; ok {
		t.Error("ctx/composer_name emitted alongside composer_self")
	}
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if _, ok := comp2.Composer.(*rm.PartySelf); !ok {
		t.Errorf("decoded composer = %T, want *rm.PartySelf", comp2.Composer)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if m2["ctx/composer_self"] != true {
		t.Errorf("composer_self lost on round-trip: %#v", m2["ctx/composer_self"])
	}
}

func TestDecodeMissingContextErrors(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	// Strip context; the remaining content-only payload must be rejected.
	for k := range m {
		if strings.HasPrefix(k, "ctx/") {
			delete(m, k)
		}
	}
	stripped, _ := json.Marshal(m)
	if _, err := simplified.UnmarshalFlat(stripped, wt); !errors.Is(err, simplified.ErrMissingContext) {
		t.Fatalf("UnmarshalFlat(no ctx) err = %v, want ErrMissingContext", err)
	}
}

// TestContextStructuredShape checks ctx is grouped under a non-arrayified ctx
// object in STRUCTURED, and survives FLAT<->STRUCTURED interconversion.
func TestContextStructuredShape(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	s, err := simplified.FlatToStructured(f1)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var sm map[string]any
	if err := json.Unmarshal(s, &sm); err != nil {
		t.Fatal(err)
	}
	ctx, ok := sm["ctx"].(map[string]any)
	if !ok {
		t.Fatalf("STRUCTURED has no ctx object: %v", sm["ctx"])
	}
	if ctx["language"] != "en" { // direct value, not an array
		t.Errorf("ctx.language = %#v, want \"en\" (non-arrayified)", ctx["language"])
	}
	back, err := simplified.StructuredToFlat(s)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var bm map[string]any
	if err := json.Unmarshal(back, &bm); err != nil {
		t.Fatal(err)
	}
	if bm["ctx/language"] != "en" || bm["ctx/territory"] != "NL" {
		t.Errorf("ctx lost through interconversion: language=%#v territory=%#v", bm["ctx/language"], bm["ctx/territory"])
	}
}

// TestSettingEncodeEmitsPair — REQ-053 (amended 2026-08-05): a populated
// EVENT_CONTEXT.setting is emitted as the ctx/setting|code + ctx/setting|value
// pair — the sixth respelled metadata field (ADR 0015's left-open emission gap,
// closed) — and never as the real-path spelling.
func TestSettingEncodeEmitsPair(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	comp.Context.Setting = rm.DVCodedText{
		DVText:       rm.DVText{Value: "home"},
		DefiningCode: rm.CodePhrase{CodeString: "225", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	f, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f, &m); err != nil {
		t.Fatal(err)
	}
	if m["ctx/setting|code"] != "225" {
		t.Errorf("ctx/setting|code = %#v, want %q", m["ctx/setting|code"], "225")
	}
	if m["ctx/setting|value"] != "home" {
		t.Errorf("ctx/setting|value = %#v, want %q", m["ctx/setting|value"], "home")
	}
	for k := range m {
		if strings.HasPrefix(k, wt.Tree.ID+"/context/setting") {
			t.Errorf("encode emitted the real-path spelling %q; ctx/ must be the only output form", k)
		}
	}
}

// TestSettingEncodeAllZeroWritesNothing — REQ-053: the all-zero setting writes
// nothing. Setting is a non-pointer DV_CODED_TEXT on EVENT_CONTEXT, so "unset"
// and "zero" coincide (the CODE_PHRASE all-zero precedent) — an unconditional
// emit would put blank setting keys on every composition decoded through the
// ctx/ forms.
func TestSettingEncodeAllZeroWritesNothing(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	comp.Context.Setting = rm.DVCodedText{}
	f, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ctx/setting|code", "ctx/setting|value"} {
		if v, ok := m[k]; ok {
			t.Errorf("all-zero setting emitted %s = %#v; want nothing", k, v)
		}
	}
}

// TestSettingEncodeRefusals — REQ-053: a setting the ctx/setting pair cannot
// carry is a typed error naming ctx/setting, never an omission (mirrors the
// composer PARTY_RELATED rule — omitting it would let a WithTemplate decode
// substitute the 238|other care default silently): a non-openehr terminology
// (the pair implies openehr), extras beyond code+value (mappings, formatting, a
// preferred term, …), and a value without a code.
func TestSettingEncodeRefusals(t *testing.T) {
	okSetting := func() rm.DVCodedText {
		return rm.DVCodedText{
			DVText:       rm.DVText{Value: "other care"},
			DefiningCode: rm.CodePhrase{CodeString: "238", TerminologyID: rm.TerminologyID{Value: "openehr"}},
		}
	}
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		mut  func(*rm.DVCodedText)
	}{
		{"non-openehr terminology", func(s *rm.DVCodedText) { s.DefiningCode.TerminologyID.Value = "SNOMED-CT" }},
		{"empty terminology", func(s *rm.DVCodedText) { s.DefiningCode.TerminologyID.Value = "" }},
		{"mappings extra", func(s *rm.DVCodedText) {
			s.Mappings = []rm.TermMapping{{Match: '=', Target: rm.CodePhrase{CodeString: "x"}}}
		}},
		{"formatting extra", func(s *rm.DVCodedText) { s.Formatting = str("plain") }},
		{"language extra", func(s *rm.DVCodedText) { s.Language = &rm.CodePhrase{CodeString: "en"} }},
		{"preferred term extra", func(s *rm.DVCodedText) { s.DefiningCode.PreferredTerm = str("Other care") }},
		{"value without code", func(s *rm.DVCodedText) { s.DefiningCode = rm.CodePhrase{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp, wt := genComposition(t, minimalObsOPT)
			setting := okSetting()
			tc.mut(&setting)
			comp.Context.Setting = setting
			_, err := simplified.MarshalFlat(comp, wt)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if !strings.Contains(err.Error(), "ctx/setting") {
				t.Errorf("error %q does not name ctx/setting", err)
			}
		})
	}
}

// TestSettingDecodePair — REQ-053: ctx/setting|code + ctx/setting|value rebuild
// EVENT_CONTEXT.setting as a DV_CODED_TEXT in the implied openehr terminology.
func TestSettingDecodePair(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	body := `{"ctx/language":"en","ctx/territory":"NL","ctx/setting|code":"238","ctx/setting|value":"other care"}`
	comp, err := simplified.UnmarshalFlat([]byte(body), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if comp.Context == nil {
		t.Fatal("decoded composition has no context")
	}
	s := comp.Context.Setting
	if s.DefiningCode.CodeString != "238" {
		t.Errorf("setting code = %q, want 238", s.DefiningCode.CodeString)
	}
	if s.DefiningCode.TerminologyID.Value != "openehr" {
		t.Errorf("setting terminology = %q, want the implied openehr", s.DefiningCode.TerminologyID.Value)
	}
	if s.Value != "other care" {
		t.Errorf("setting value = %q, want %q", s.Value, "other care")
	}
}

// TestSettingDecodeHalfPairRejected — REQ-053: one of the ctx/setting pair
// without the other is a typed error naming the missing key, not a guessed
// completion (the rubric is not derivable from the code without a terminology
// service, and a code is not derivable from the rubric at all).
func TestSettingDecodeHalfPairRejected(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	tests := []struct {
		name, body, missing string
	}{
		{
			"code without value",
			`{"ctx/language":"en","ctx/territory":"NL","ctx/setting|code":"238"}`,
			"ctx/setting|value",
		},
		{
			"value without code",
			`{"ctx/language":"en","ctx/territory":"NL","ctx/setting|value":"other care"}`,
			"ctx/setting|code",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := simplified.UnmarshalFlat([]byte(tc.body), wt)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if !strings.Contains(err.Error(), tc.missing) {
				t.Errorf("error %q does not name the missing key %q", err, tc.missing)
			}
		})
	}
}

// TestSettingWithTemplateDefaultRoundTrip — REQ-053: the round-trip interaction
// of the synthesised default. A WithTemplate decode of a body carrying no
// setting completes EVENT_CONTEXT.setting to 238|other care so the result
// validates (deviations.md § RM-mandatory attributes); re-encoding that
// completed composition emits ctx/setting|code + |value — a faithful encoding
// of the completed composition, gaining exactly the two default keys over the
// input. An OPT-free decode synthesises nothing, so its re-encode stays
// byte-identical to the setting-less input.
func TestSettingWithTemplateDefaultRoundTrip(t *testing.T) {
	parsed, err := template.ParseFile(minimalObsOPT)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	compiled, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	comp, wt := genComposition(t, minimalObsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m1 map[string]any
	if err := json.Unmarshal(f1, &m1); err != nil {
		t.Fatal(err)
	}
	// The generated source carries the default setting, so its encoding pins the
	// exact two keys the WithTemplate completion re-creates below.
	if m1["ctx/setting|code"] != "238" || m1["ctx/setting|value"] != "other care" {
		t.Fatalf("generated encoding carries setting %#v|%#v, want the 238|other care default",
			m1["ctx/setting|code"], m1["ctx/setting|value"])
	}
	noSetting := make(map[string]any, len(m1))
	maps.Copy(noSetting, m1)
	delete(noSetting, "ctx/setting|code")
	delete(noSetting, "ctx/setting|value")
	body, err := json.Marshal(noSetting)
	if err != nil {
		t.Fatal(err)
	}

	// OPT-free: nothing synthesised, nothing emitted — byte-identical.
	bare, err := simplified.UnmarshalFlat(body, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (bare): %v", err)
	}
	f2, err := simplified.MarshalFlat(bare, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (bare re-encode): %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(noSetting, m2) {
		t.Errorf("OPT-free round-trip of a setting-less body is not byte-identical:\n in  (%d keys) = %v\n out (%d keys) = %v",
			len(noSetting), sortedKeys(noSetting), len(m2), sortedKeys(m2))
	}

	// WithTemplate: the completed composition re-encodes with exactly the two
	// default keys gained — nothing else moves.
	named, err := simplified.UnmarshalFlat(body, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	f3, err := simplified.MarshalFlat(named, wt)
	if err != nil {
		t.Fatalf("MarshalFlat (WithTemplate re-encode): %v", err)
	}
	var m3 map[string]any
	if err := json.Unmarshal(f3, &m3); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m1, m3) {
		t.Errorf("WithTemplate round-trip did not gain exactly the two default setting keys:\n want (%d keys) = %v\n got  (%d keys) = %v",
			len(m1), sortedKeys(m1), len(m3), sortedKeys(m3))
	}
}

// reflatten re-marshals comp's FLAT form as a mutable map, applies mut, and
// returns the JSON bytes — the shared setup for the metadata-spelling tests.
func reflatten(t *testing.T, comp *rm.Composition, wt *webtemplate.WebTemplate, mut func(root string, m map[string]any)) []byte {
	t.Helper()
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	mut(wt.Tree.ID, m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestMetadataRealPathSpellingAccepted — ADR 0015. The reference spells
// composition metadata as real paths under the template root where REQ-053 reads
// the ctx/ short forms. Both are accepted on input; before the alias table an
// EHRbase-authored body was mishandled over a pure respelling — first refused as
// ErrUnsupportedDatatype (the composition-level language / territory / composer
// leaves are CODE_PHRASE and PARTY_PROXY; only composer_self, which reaches no
// Web Template node, was ErrUnknownPath), then, once CODE_PHRASE became a mapped
// leaf type, decoded silently through ordinary leaf placement with ctx
// normalisation bypassed altogether.
func TestMetadataRealPathSpellingAccepted(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	tests := []struct {
		name  string
		mut   func(root string, m map[string]any)
		check func(t *testing.T, got *rm.Composition)
	}{
		{
			name: "language",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/language")
				m[root+"/language|code"] = "de"
				m[root+"/language|terminology"] = "ISO_639-1"
			},
			check: func(t *testing.T, got *rm.Composition) {
				if got.Language.CodeString != "de" {
					t.Errorf("language = %q, want de", got.Language.CodeString)
				}
			},
		},
		{
			name: "territory",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/territory")
				m[root+"/territory|code"] = "NL"
			},
			check: func(t *testing.T, got *rm.Composition) {
				if got.Territory.CodeString != "NL" {
					t.Errorf("territory = %q, want NL", got.Territory.CodeString)
				}
			},
		},
		{
			name: "composer name",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/composer_name")
				delete(m, "ctx/composer_self")
				m[root+"/composer|name"] = "Dr Real Path"
			},
			check: func(t *testing.T, got *rm.Composition) {
				p, ok := got.Composer.(*rm.PartyIdentified)
				if !ok {
					t.Fatalf("composer is %T, want *rm.PartyIdentified", got.Composer)
				}
				if p.Name == nil || *p.Name != "Dr Real Path" {
					t.Errorf("composer name = %v, want Dr Real Path", p.Name)
				}
			},
		},
		{
			name: "context start_time",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/time")
				m[root+"/context/start_time"] = "2021-06-01T09:30:00Z"
			},
			check: func(t *testing.T, got *rm.Composition) {
				if got.Context == nil || got.Context.StartTime.Value != "2021-06-01T09:30:00Z" {
					t.Errorf("start_time = %#v, want 2021-06-01T09:30:00Z", got.Context)
				}
			},
		},
		{
			// REQ-053 (amended 2026-08-05): context/setting is the sixth respelled
			// field — the real-path pair normalises onto ctx/setting|code + |value,
			// with |terminology as an openehr witness (checked, then discarded).
			name: "context setting",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/setting|code")
				delete(m, "ctx/setting|value")
				m[root+"/context/setting|code"] = "225"
				m[root+"/context/setting|value"] = "home"
				m[root+"/context/setting|terminology"] = "openehr"
			},
			check: func(t *testing.T, got *rm.Composition) {
				if got.Context == nil {
					t.Fatal("decoded composition has no context")
				}
				s := got.Context.Setting
				if s.DefiningCode.CodeString != "225" || s.Value != "home" ||
					s.DefiningCode.TerminologyID.Value != "openehr" {
					t.Errorf("setting = %#v, want 225|home in openehr", s)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, tc.mut), wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat(real path): %v", err)
			}
			tc.check(t, got)
		})
	}
}

// TestMetadataSpellingEmitsCtxOnly — ADR 0015 makes the codec asymmetric:
// either spelling in, ctx/ only out. A body that arrived in the real-path
// spelling must re-encode to the short form and nothing else, or the round-trip
// would carry two spellings for one value.
func TestMetadataSpellingEmitsCtxOnly(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		delete(m, "ctx/language")
		m[root+"/language|code"] = "de"
	})
	got, err := simplified.UnmarshalFlat(in, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	out, err := simplified.MarshalFlat(got, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["ctx/language"] != "de" {
		t.Errorf("ctx/language = %#v, want de", m["ctx/language"])
	}
	// Only the *composition-level* spelling is metadata. `language|code` also
	// occurs one level down as an ENTRY's own in-context leaf, which is real
	// clinical-envelope data the codec is expected to emit — hence the exact
	// root-relative match here rather than a suffix test.
	for _, k := range []string{
		wt.Tree.ID + "/language|code",
		wt.Tree.ID + "/language|terminology",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("encode emitted the real-path spelling %q; ctx/ must be the only output form", k)
		}
	}
}

// TestMetadataSpellingConflictRejected — two spellings of one field that
// disagree are a defect in the payload. Silently preferring either would
// corrupt composition metadata, so the codec refuses and names both.
func TestMetadataSpellingConflictRejected(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		m["ctx/language"] = "en"
		m[root+"/language|code"] = "de" // same field, different value
	})
	_, err := simplified.UnmarshalFlat(in, wt)
	if !errors.Is(err, simplified.ErrUnknownPath) {
		t.Fatalf("err = %v, want ErrUnknownPath", err)
	}
	for _, want := range []string{"ctx/language", "language|code"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestMetadataSpellingAgreementAccepted: the conflict rule keys on the value,
// not on the presence of both spellings — a body that says the same thing twice
// is redundant, not wrong.
func TestMetadataSpellingAgreementAccepted(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		m[root+"/language|code"] = m["ctx/language"]
	})
	if _, err := simplified.UnmarshalFlat(in, wt); err != nil {
		t.Fatalf("agreeing spellings rejected: %v", err)
	}
}

// TestMetadataTerminologyWitnessChecked: the ctx/ form carries only the code and
// applyContext rebuilds the CODE_PHRASE with a hardcoded terminology, so a real
// path naming a different terminology must fail rather than be silently
// rewritten to the implied one.
func TestMetadataTerminologyWitnessChecked(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		m[root+"/language|terminology"] = "some-other-terminology"
	})
	_, err := simplified.UnmarshalFlat(in, wt)
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
}

// TestMetadataNonRespellingsStillRefused — ADR 0015 admits respellings only.
// The composer's external_ref suffixes are not aliased onto ctx/ (the short
// forms structurally cannot carry them), so they are never quietly absorbed
// into the context — they surface as an error from whatever refuses them.
// (context/setting left this test on 2026-08-05: the amended REQ-053 aliases it
// onto ctx/setting|code + |value — see TestMetadataRealPathSpellingAccepted.)
func TestMetadataNonRespellingsStillRefused(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		m[root+"/composer|id"] = "12345"
		m[root+"/composer|id_scheme"] = "HOSPITAL-NS"
	})
	if _, err := simplified.UnmarshalFlat(in, wt); err == nil {
		t.Error("composer external_ref decoded without error; want a refusal (see ADR 0015)")
	}
}

// TestMetadataConflictOnCompositeValueDoesNotPanic — regression, PR #86 review.
// putCtx compared two candidate values with `!=` on `any`, which panics for a
// JSON object or array. A malformed payload reaches that path whenever both
// accepted spellings of one field are present, so the codec crashed on
// untrusted input instead of reporting a typed error (REQ-025 — no panics).
func TestMetadataConflictOnCompositeValueDoesNotPanic(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	root := wt.Tree.ID
	for _, body := range []string{
		`{"ctx/language":{"a":1},"` + root + `/language|code":{"b":2},"ctx/territory":"US"}`,
		`{"ctx/language":["a"],"` + root + `/language|code":["b"],"ctx/territory":"US"}`,
		`{"ctx/territory":{"a":1},"` + root + `/territory|code":{"a":1},"ctx/language":"en"}`,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on malformed input %s: %v", body, r)
				}
			}()
			if _, err := simplified.UnmarshalFlat([]byte(body), wt); err == nil {
				t.Errorf("composite ctx value accepted: %s", body)
			} else if !errors.Is(err, simplified.ErrUnsupportedDatatype) && !errors.Is(err, simplified.ErrUnknownPath) {
				t.Errorf("unexpected error class for %s: %v", body, err)
			}
		}()
	}
}

// TestMetadataTerritoryTerminologyWitnessChecked mirrors the language case for
// territory: the ctx/ form carries only the code and applyContext rebuilds the
// CODE_PHRASE as ISO_3166-1, so a real path naming another terminology must fail
// rather than be silently re-terminologised.
func TestMetadataTerritoryTerminologyWitnessChecked(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		m[root+"/territory|terminology"] = "not-ISO-3166"
	})
	if _, err := simplified.UnmarshalFlat(in, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
}

// TestMetadataRealPathComposerSelf: composer_self is in metadataAliases, so the
// real-path spelling must decode to a PARTY_SELF composer just as
// ctx/composer_self does.
func TestMetadataRealPathComposerSelf(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	in := reflatten(t, comp, wt, func(root string, m map[string]any) {
		delete(m, "ctx/composer_self")
		delete(m, "ctx/composer_name")
		m[root+"/composer_self"] = true
	})
	got, err := simplified.UnmarshalFlat(in, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat(real-path composer_self): %v", err)
	}
	if _, ok := got.Composer.(*rm.PartySelf); !ok {
		t.Errorf("composer = %T, want *rm.PartySelf", got.Composer)
	}
}

// TestMetadataAliasAccessorsPinned pins the exported alias tables. The PROBE-086
// conformance harness derives its metadata hold-out from these accessors instead
// of restating the codec's table (conformance.md § PROBE-086), so their contents
// are public API: a spelling added or dropped here changes what a census excuses
// and must be a deliberate edit, not a side effect.
func TestMetadataAliasAccessorsPinned(t *testing.T) {
	wantAliases := []string{
		"composer_self", "composer|name",
		"context/setting|code", "context/setting|value", "context/start_time",
		"language|code", "territory|code",
	}
	if got := simplified.MetadataAliasSpellings(); !slices.Equal(got, wantAliases) {
		t.Errorf("MetadataAliasSpellings() = %q, want %q", got, wantAliases)
	}
	wantWitnesses := []string{"context/setting|terminology", "language|terminology", "territory|terminology"}
	if got := simplified.MetadataWitnessSpellings(); !slices.Equal(got, wantWitnesses) {
		t.Errorf("MetadataWitnessSpellings() = %q, want %q", got, wantWitnesses)
	}
	// Both are root-relative (the decoder strips "<root>/" before the lookup), so
	// a caller composes "<root>/" + spelling; a ctx/-prefixed or absolute entry
	// would silently match nothing.
	for _, a := range append(simplified.MetadataAliasSpellings(), simplified.MetadataWitnessSpellings()...) {
		if strings.HasPrefix(a, "ctx/") || strings.HasPrefix(a, "/") {
			t.Errorf("spelling %q is not root-relative", a)
		}
	}
	// Fresh slices: mutating the result must not reach the decoder's own table.
	mutated := simplified.MetadataAliasSpellings()
	mutated[0] = "clobbered"
	if again := simplified.MetadataAliasSpellings(); !slices.Equal(again, wantAliases) {
		t.Errorf("MetadataAliasSpellings() aliases internal state: after mutation = %q", again)
	}
}

// TestMetadataCompositeValueRefusedInBothKeyOrders — regression, PR #86 review
// round 3. siphonContext walks a body in sorted key order, so which spelling of a
// metadata field lands in the ctx map first depends on how the template root sorts
// against "ctx/". The conflict check used to type-switch on the value already
// stored and could not classify a JSON object or array, returning "not provably
// different" — so a composite followed by a scalar was silently *overwritten* and
// the body decoded clean, while the mirror order errored. Refusing the composite
// up front makes the check total: both orders now fail, and the error names the
// body key that carries the bad value.
func TestMetadataCompositeValueRefusedInBothKeyOrders(t *testing.T) {
	for _, fx := range []struct{ name, opt string }{
		{"root sorts before ctx/", bodyWeightOPT},
		{"root sorts after ctx/", minimalObsOPT},
	} {
		t.Run(fx.name, func(t *testing.T) {
			_, wt := genComposition(t, fx.opt)
			root := wt.Tree.ID
			for _, tc := range []struct {
				name      string
				body      string
				wantNamed []string
			}{
				{
					name:      "object under ctx/, scalar on the real path",
					body:      `{"ctx/language":{"a":1},"` + root + `/language|code":"de","ctx/territory":"US"}`,
					wantNamed: []string{"ctx/language"},
				},
				{
					name:      "scalar under ctx/, object on the real path",
					body:      `{"ctx/language":"en","` + root + `/language|code":{"b":2},"ctx/territory":"US"}`,
					wantNamed: []string{"ctx/language", root + "/language|code"},
				},
				{
					name:      "array under ctx/, scalar on the real path",
					body:      `{"ctx/territory":["NL"],"` + root + `/territory|code":"NL","ctx/language":"en"}`,
					wantNamed: []string{"ctx/territory"},
				},
				{
					name:      "scalar under ctx/, array on the real path",
					body:      `{"ctx/territory":"NL","` + root + `/territory|code":["NL"],"ctx/language":"en"}`,
					wantNamed: []string{"ctx/territory", root + "/territory|code"},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, err := simplified.UnmarshalFlat([]byte(tc.body), wt)
					if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
						t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
					}
					for _, want := range tc.wantNamed {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q does not name %q", err, want)
						}
					}
				})
			}
		})
	}
}

// TestMetadataAliasFamilyConflictAndAgreement sweeps every aliased metadata
// family, and every terminology witness, through both halves of the ADR 0015
// decision-4 rule: two spellings that disagree are a payload defect and must be
// refused naming both, while two that agree are merely redundant and must decode.
// Table-driven because the rule is per-family — a family wired into the alias
// table but not into the conflict check would decode a contradiction silently, and
// one wired the other way round would reject an agreeing body.
func TestMetadataAliasFamilyConflictAndAgreement(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	root := wt.Tree.ID
	for _, tc := range []struct {
		name      string
		agree     func(root string, m map[string]any)
		disagree  func(root string, m map[string]any)
		wantErr   error
		wantNamed []string
	}{
		{
			name:  "language",
			agree: func(root string, m map[string]any) { m[root+"/language|code"] = m["ctx/language"] },
			disagree: func(root string, m map[string]any) {
				m["ctx/language"] = "en"
				m[root+"/language|code"] = "de"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/language", root + "/language|code"},
		},
		{
			name:  "territory",
			agree: func(root string, m map[string]any) { m[root+"/territory|code"] = m["ctx/territory"] },
			disagree: func(root string, m map[string]any) {
				m["ctx/territory"] = "NL"
				m[root+"/territory|code"] = "BE"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/territory", root + "/territory|code"},
		},
		{
			name:  "composer_name",
			agree: func(root string, m map[string]any) { m[root+"/composer|name"] = m["ctx/composer_name"] },
			disagree: func(root string, m map[string]any) {
				m[root+"/composer|name"] = "Dr Someone Else"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/composer_name", root + "/composer|name"},
		},
		{
			// A PARTY_SELF composer carries no name, so both bodies drop
			// ctx/composer_name — keeping it would be the *other* refusal
			// (TestComposerSelfWithComposerNameRejected), not a spelling conflict.
			name: "composer_self",
			agree: func(root string, m map[string]any) {
				delete(m, "ctx/composer_name")
				m["ctx/composer_self"] = true
				m[root+"/composer_self"] = true
			},
			disagree: func(root string, m map[string]any) {
				delete(m, "ctx/composer_name")
				m["ctx/composer_self"] = true
				m[root+"/composer_self"] = false
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/composer_self", root + "/composer_self"},
		},
		{
			name:  "time / context start_time",
			agree: func(root string, m map[string]any) { m[root+"/context/start_time"] = m["ctx/time"] },
			disagree: func(root string, m map[string]any) {
				m[root+"/context/start_time"] = "1999-12-31T23:59:00Z"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/time", root + "/context/start_time"},
		},
		{
			// The generated body carries the default ctx/setting pair, so the
			// real-path spelling agrees or disagrees against it (REQ-053).
			name:  "setting code",
			agree: func(root string, m map[string]any) { m[root+"/context/setting|code"] = m["ctx/setting|code"] },
			disagree: func(root string, m map[string]any) {
				m[root+"/context/setting|code"] = "225"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/setting|code", root + "/context/setting|code"},
		},
		{
			name:  "setting value",
			agree: func(root string, m map[string]any) { m[root+"/context/setting|value"] = m["ctx/setting|value"] },
			disagree: func(root string, m map[string]any) {
				m[root+"/context/setting|value"] = "home"
			},
			wantErr:   simplified.ErrUnknownPath,
			wantNamed: []string{"ctx/setting|value", root + "/context/setting|value"},
		},
		{
			// The setting witness mirrors the language one: the ctx/ pair implies
			// openehr, so a real path naming any other terminology is refused
			// rather than silently re-terminologised (REQ-053).
			name:  "setting terminology witness",
			agree: func(root string, m map[string]any) { m[root+"/context/setting|terminology"] = "openehr" },
			disagree: func(root string, m map[string]any) {
				m[root+"/context/setting|terminology"] = "SNOMED-CT"
			},
			wantErr:   simplified.ErrUnsupportedDatatype,
			wantNamed: []string{root + "/context/setting|terminology"},
		},
		{
			// A witness is checked and discarded, so "disagree" means naming a
			// terminology the ctx/ short form cannot carry — a different refusal
			// class (ErrUnsupportedDatatype) with only the witness key to name.
			name:  "language terminology witness",
			agree: func(root string, m map[string]any) { m[root+"/language|terminology"] = "ISO_639-1" },
			disagree: func(root string, m map[string]any) {
				m[root+"/language|terminology"] = "ISO_639-2"
			},
			wantErr:   simplified.ErrUnsupportedDatatype,
			wantNamed: []string{root + "/language|terminology"},
		},
		{
			name:  "territory terminology witness",
			agree: func(root string, m map[string]any) { m[root+"/territory|terminology"] = "ISO_3166-1" },
			disagree: func(root string, m map[string]any) {
				m[root+"/territory|terminology"] = "ISO_3166-2"
			},
			wantErr:   simplified.ErrUnsupportedDatatype,
			wantNamed: []string{root + "/territory|terminology"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("conflict refused", func(t *testing.T) {
				_, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, tc.disagree), wt)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				for _, want := range tc.wantNamed {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not name %q", err, want)
					}
				}
			})
			t.Run("agreement accepted", func(t *testing.T) {
				if _, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, tc.agree), wt); err != nil {
					t.Fatalf("agreeing spellings rejected: %v", err)
				}
			})
		})
	}
}

// TestComposerSelfWithComposerNameRejected — PR #86 review round 3. composer_self
// and a composer name are mutually exclusive representations of one RM attribute:
// applyContext's switch prefers PARTY_SELF, so the pair used to decode to a
// PARTY_SELF with the name silently dropped. That is the same class of defect as
// two disagreeing spellings of one field and gets the same refusal. `false` beside
// a name denies nothing the name asserts, and must still decode.
func TestComposerSelfWithComposerNameRejected(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	root := wt.Tree.ID
	for _, tc := range []struct {
		name      string
		mut       func(root string, m map[string]any)
		wantNamed []string
	}{
		{
			name: "ctx/composer_self with the real-path name",
			mut: func(root string, m map[string]any) {
				delete(m, "ctx/composer_name")
				m["ctx/composer_self"] = true
				m[root+"/composer|name"] = "Dr X"
			},
			wantNamed: []string{"ctx/composer_self", root + "/composer|name"},
		},
		{
			name: "both in the ctx/ spelling",
			mut: func(root string, m map[string]any) {
				m["ctx/composer_self"] = true
				m["ctx/composer_name"] = "Dr X"
			},
			wantNamed: []string{"ctx/composer_self", "ctx/composer_name"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, tc.mut), wt)
			if !errors.Is(err, simplified.ErrUnknownPath) {
				t.Fatalf("err = %v, want ErrUnknownPath", err)
			}
			for _, want := range tc.wantNamed {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
		})
	}
	// composer_self: false is not an assertion of PARTY_SELF, so the name stands.
	got, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, func(root string, m map[string]any) {
		delete(m, "ctx/composer_name")
		m["ctx/composer_self"] = false
		m[root+"/composer|name"] = "Dr X"
	}), wt)
	if err != nil {
		t.Fatalf("composer_self=false beside a name rejected: %v", err)
	}
	p, ok := got.Composer.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("composer = %T, want *rm.PartyIdentified", got.Composer)
	}
	if p.Name == nil || *p.Name != "Dr X" {
		t.Errorf("composer name = %v, want Dr X", p.Name)
	}
}

// TestContextOwnedLeafUnacceptedSpellingRefused — REQ-053. The ctx/ short forms
// own EVENT_CONTEXT setting and start_time outright, and [applyContext] writes
// them last. A spelling of those leaves that the alias table does not accept
// used to resolve as an ordinary Web Template leaf and then be overwritten with
// no error — a silent drop of clinical metadata (PR #88 review). Every such
// spelling is now refused naming the key.
//
// The corpus OPT is the target on purpose: its Web Template carries a
// context/setting node, which is what made the drop reachable at all.
func TestContextOwnedLeafUnacceptedSpellingRefused(t *testing.T) {
	parsed, err := template.ParseFile(corpusFlatOPT)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	compiled, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	root := wt.Tree.ID

	for _, tc := range []struct {
		name string
		key  string
		val  any
	}{
		{"setting raw beside the ctx pair", root + "/context/setting|raw", map[string]any{
			"_type": "DV_CODED_TEXT", "value": "emergency care",
			"defining_code": map[string]any{
				"_type": "CODE_PHRASE", "code_string": "227",
				"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "openehr"},
			},
		}},
		{"setting formatting", root + "/context/setting|formatting", "plain"},
		{"setting bare", root + "/context/setting", "other care"},
		{"start_time raw beside ctx/time", root + "/context/start_time|raw", map[string]any{
			"_type": "DV_DATE_TIME", "value": "2026-08-05T10:00:00Z",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{
				"ctx/language": "en", "ctx/territory": "NL",
				"ctx/setting|code": "238", "ctx/setting|value": "other care",
				"ctx/time": "2026-08-05T09:00:00Z",
				tc.key:     tc.val,
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			_, err = simplified.UnmarshalFlat(raw, wt, simplified.WithTemplate(compiled))
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype (the value must not be silently dropped)", err)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name the offending key %q", err, tc.key)
			}
		})
	}
}

// TestSettingDecodeEmptyHalfRefused — REQ-053. An empty code or rubric is not a
// setting: the code would rebuild a CODE_PHRASE with no code_string (RM-invalid
// and refused by emitContextSetting, so decode would mint a composition this
// codec cannot re-encode), and an all-empty pair is not read as "absent" —
// absent is the keys not being there, and treating an explicit empty pair as
// absent would hand it the WithTemplate 238|other care default instead.
func TestSettingDecodeEmptyHalfRefused(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	for _, tc := range []struct{ name, body, named string }{
		{"empty code", `{"ctx/language":"en","ctx/territory":"NL","ctx/setting|code":"","ctx/setting|value":"other care"}`, "ctx/setting|code"},
		{"empty value", `{"ctx/language":"en","ctx/territory":"NL","ctx/setting|code":"238","ctx/setting|value":""}`, "ctx/setting|value"},
		{"both empty", `{"ctx/language":"en","ctx/territory":"NL","ctx/setting|code":"","ctx/setting|value":""}`, "ctx/setting|code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := simplified.UnmarshalFlat([]byte(tc.body), wt)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if !strings.Contains(err.Error(), tc.named) {
				t.Errorf("error %q does not name %q", err, tc.named)
			}
		})
	}
}

// TestSettingHalfPairNamesTheAuthorsSpelling — REQ-053. Both halves are reported
// in the spelling the payload used; telling a real-path author to add a
// ctx/-spelled key they never wrote points them at the wrong place.
func TestSettingHalfPairNamesTheAuthorsSpelling(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	root := wt.Tree.ID
	body := reflatten(t, comp, wt, func(root string, m map[string]any) {
		delete(m, "ctx/setting|code")
		delete(m, "ctx/setting|value")
		m[root+"/context/setting|code"] = "238"
		m[root+"/context/setting|terminology"] = "openehr"
	})
	_, err := simplified.UnmarshalFlat(body, wt)
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if want := root + "/context/setting|code"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name the present key in the author's spelling %q", err, want)
	}
}

// TestContextStructuredSuffixNesting — REQ-053. ctx/setting is the first ctx
// field to carry a |suffix, and a suffixed ctx field nests its members under the
// field name the way a clinical leaf does — ctx.setting["|code"], not a literal
// "setting|code" member. A flat piped member would not round-trip through a
// reference-shaped STRUCTURED body (PR #88 review).
func TestContextStructuredSuffixNesting(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	b, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	ctxObj, ok := m["ctx"].(map[string]any)
	if !ok {
		t.Fatalf("structured body has no ctx object: %v", m)
	}
	if _, flat := ctxObj["setting|code"]; flat {
		t.Errorf("ctx carries a literal piped member %q; suffixes must nest under setting", "setting|code")
	}
	setting, nested := ctxObj["setting"].(map[string]any)
	if !nested {
		t.Fatalf("ctx.setting = %#v, want an object nesting |code and |value", ctxObj["setting"])
	}
	if setting["|code"] != "238" || setting["|value"] != "other care" {
		t.Errorf("ctx.setting = %#v, want |code 238 and |value \"other care\"", setting)
	}

	// And the nesting is an exact inverse: STRUCTURED -> FLAT -> STRUCTURED.
	back, err := simplified.StructuredToFlat(b)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var fm map[string]any
	if err := json.Unmarshal(back, &fm); err != nil {
		t.Fatal(err)
	}
	if fm["ctx/setting|code"] != "238" || fm["ctx/setting|value"] != "other care" {
		t.Errorf("flattened ctx/setting = %#v / %#v, want 238 / other care",
			fm["ctx/setting|code"], fm["ctx/setting|value"])
	}
}

// TestSettingEncodeDecodeSymmetry — REQ-053. Whatever emitContextSetting emits,
// parseCtx must accept, and whatever parseCtx refuses, emitContextSetting must
// refuse too. The pair is the codec's only setting surface, so an asymmetry
// means MarshalFlat can produce a body UnmarshalFlat rejects (PR #88 re-review:
// a populated code with an empty rubric was emitted as |value:"" and then
// refused on the way back in).
func TestSettingEncodeDecodeSymmetry(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	for _, tc := range []struct {
		name    string
		setting rm.DVCodedText
		encodes bool
	}{
		{"code and rubric", rm.DVCodedText{
			DVText:       rm.DVText{Value: "home"},
			DefiningCode: rm.CodePhrase{CodeString: "225", TerminologyID: rm.TerminologyID{Value: "openehr"}},
		}, true},
		{"code with empty rubric", rm.DVCodedText{
			DefiningCode: rm.CodePhrase{CodeString: "238", TerminologyID: rm.TerminologyID{Value: "openehr"}},
		}, false},
		{"all zero writes nothing", rm.DVCodedText{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp.Context.Setting = tc.setting
			f, err := simplified.MarshalFlat(comp, wt)
			if !tc.encodes {
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
					t.Fatalf("MarshalFlat err = %v, want ErrUnsupportedDatatype (decode refuses this shape)", err)
				}
				if !strings.Contains(err.Error(), "ctx/setting") {
					t.Errorf("error %q does not name ctx/setting", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			// The symmetry assertion: our own output must survive our own decode.
			if _, err := simplified.UnmarshalFlat(f, wt); err != nil {
				t.Fatalf("encode produced a body decode refuses: %v", err)
			}
		})
	}
}

// TestStructuredEmptyCtxLeafRefused — REQ-053. An explicit empty ctx leaf object
// is refused rather than flattened to nothing: silently dropping it turns
// "explicitly empty" into "absent", which a WithTemplate decode then completes
// with the 238|other care default — the same class parseCtx closed for the FLAT
// pair (PR #88 re-review).
func TestStructuredEmptyCtxLeafRefused(t *testing.T) {
	body := []byte(`{"minimal":{},"ctx":{"language":"en","territory":"NL","setting":{}}}`)
	_, err := simplified.StructuredToFlat(body)
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("StructuredToFlat err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "ctx/setting") {
		t.Errorf("error %q does not name ctx/setting", err)
	}
}
