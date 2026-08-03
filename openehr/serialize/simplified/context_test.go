package simplified_test

// REQ-053 — ctx/ context: composition-level metadata (language, territory,
// composer, time) is carried under the ctx/ prefix (FLAT) / a ctx object
// (STRUCTURED). Language + territory are mandatory on decode.
import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

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
// the ctx/ short forms. Both are accepted on input; before this, an
// EHRbase-authored body failed with ErrUnknownPath over a pure respelling.
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
// context/setting is unsupported on the ctx/ side too (an unimplemented field,
// not a spelling gap) and the composer's external_ref cannot be carried by the
// short forms at all; both must stay loud rather than be quietly absorbed.
func TestMetadataNonRespellingsStillRefused(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	for _, tc := range []struct {
		name string
		mut  func(root string, m map[string]any)
	}{
		{"context/setting", func(root string, m map[string]any) {
			m[root+"/context/setting|code"] = "238"
			m[root+"/context/setting|value"] = "other care"
		}},
		{"composer external_ref", func(root string, m map[string]any) {
			m[root+"/composer|id"] = "12345"
			m[root+"/composer|id_scheme"] = "HOSPITAL-NS"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := simplified.UnmarshalFlat(reflatten(t, comp, wt, tc.mut), wt); err == nil {
				t.Error("decoded without error; want a refusal (see ADR 0015)")
			}
		})
	}
}
