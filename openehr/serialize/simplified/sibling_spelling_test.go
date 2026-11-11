package simplified_test

// REQ-053 — a duplicate sibling's earlier spelling. The Web Template spells the
// second and later of siblings that share an id `<id>_1`, `<id>_2`; the
// reference, and earlier releases, wrote `<id>2`, `<id>3`. Decode reads that
// spelling as the sibling it stands for where the parent has the bare id and
// that sibling and no child of the earlier id's own, and encode writes the Web
// Template's id.

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// test_all_types names three siblings of its composition alike, the
// OBSERVATION test_all_types, the EVALUATION test_all_types_1 and the SECTION
// test_all_types_2, and two leaves of the OBSERVATION, time and time_1. The
// reference keys the duplicates test_all_types2, test_all_types3 and time2.
const (
	evalURI        = "test_all_types/test_all_types_1:0/uri"
	evalURIEarlier = "test_all_types/test_all_types2:0/uri"
	obsTime        = shorthandObs + "/time_1"
	obsTimeEarlier = shorthandObs + "/time2"
	// sectionTail is the path below the SECTION to a DV_TEXT.
	sectionTail             = ":0/section_2/section_3/test_all_types:0/narrative"
	sectionNarrativeCurrent = "test_all_types/test_all_types_2" + sectionTail
	sectionNarrativeEarlier = "test_all_types/test_all_types3" + sectionTail
)

// unmatchedOf decodes body in format with WithUnmatched, failing the test on
// an error, and returns the composition and the paths reported.
func unmatchedOf(t *testing.T, format string, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) (*rm.Composition, []string) {
	t.Helper()
	var reported []string
	opts = append(opts, simplified.WithUnmatched(func(p string) { reported = append(reported, p) }))
	comp, err := decodeFormat(t, format, wt, body, opts...)
	if err != nil {
		t.Fatalf("%s decode of %v = %v", format, body, err)
	}
	return comp, reported
}

// decodeFormat decodes body, a FLAT body, as format: FLAT, or the STRUCTURED
// body FLAT → STRUCTURED conversion gives it.
func decodeFormat(t *testing.T, format string, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) (*rm.Composition, error) {
	t.Helper()
	for _, f := range formats {
		if f.name == format {
			return f.decode(t, wt, body, opts...)
		}
	}
	t.Fatalf("no format %q", format)
	return nil, nil
}

// TestREQ053DuplicateSiblingEarlierSpellingDecodes — REQ-053: a key that
// spells a duplicate sibling `<id>2` or `<id>3` decodes, in FLAT and in
// STRUCTURED, to the composition its `<id>_1` or `<id>_2` spelling decodes to,
// and reports nothing unmatched; without the key the composition differs, so
// the key is read.
func TestREQ053DuplicateSiblingEarlierSpellingDecodes(t *testing.T) {
	wt, _ := allTypesSet(t)
	tests := []struct {
		name, current, earlier string
		value                  any
	}{
		{name: "second composition sibling", current: evalURI, earlier: evalURIEarlier, value: "http://example.org/a"},
		{name: "third composition sibling", current: sectionNarrativeCurrent, earlier: sectionNarrativeEarlier, value: "narrative"},
		{name: "second leaf", current: obsTime, earlier: obsTimeEarlier, value: "2019-01-28T21:22:49.332Z"},
	}
	for _, f := range formats {
		for _, tc := range tests {
			t.Run(f.name+"/"+tc.name, func(t *testing.T) {
				want, reported := unmatchedOf(t, f.name, wt, completionBody(map[string]any{tc.current: tc.value}))
				if len(reported) > 0 {
					t.Fatalf("decode of %q reported %v unmatched", tc.current, reported)
				}
				got, reported := unmatchedOf(t, f.name, wt, completionBody(map[string]any{tc.earlier: tc.value}))
				if len(reported) > 0 {
					t.Errorf("decode of %q reported %v unmatched, want it read as %q", tc.earlier, reported, tc.current)
				}
				if g, w := canonicalComposition(t, got), canonicalComposition(t, want); string(g) != string(w) {
					t.Errorf("decode of %q =\n%s\nwant what %q gives:\n%s", tc.earlier, g, tc.current, w)
				}
				without, _ := unmatchedOf(t, f.name, wt, completionBody(nil))
				if string(canonicalComposition(t, without)) == string(canonicalComposition(t, want)) {
					t.Errorf("decode of %q gives the composition of a body without it", tc.current)
				}
			})
		}
	}
}

// renamedAllTypes is test_all_types's Web Template with its composition's
// child from renamed to.
func renamedAllTypes(t *testing.T, from, to string) *webtemplate.WebTemplate {
	t.Helper()
	wt, _ := allTypesSet(t)
	for _, ch := range wt.Tree.Children {
		if ch.ID == from {
			ch.ID = to
			return wt
		}
	}
	t.Fatalf("test_all_types has no composition child %q", from)
	return nil
}

// TestREQ053DuplicateSiblingOwnIDWins — REQ-053: where a sibling's own id is
// the earlier spelling, a key reaches that sibling, not the duplicate the
// spelling would stand for. With the SECTION renamed test_all_types2, the
// EVALUATION's uri spelled under test_all_types2 is a path the SECTION does
// not have, unmatched, and the SECTION's narrative is read under it.
func TestREQ053DuplicateSiblingOwnIDWins(t *testing.T) {
	wt := renamedAllTypes(t, "test_all_types_2", "test_all_types2")
	narrative := "test_all_types/test_all_types2" + sectionTail
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			_, reported := unmatchedOf(t, f.name, wt, completionBody(map[string]any{evalURIEarlier: "http://example.org/a"}))
			want := map[string]string{"FLAT": evalURIEarlier, "STRUCTURED": "test_all_types/test_all_types2/0/uri"}[f.name]
			if !slices.Equal(reported, []string{want}) {
				t.Errorf("unmatched of %q = %v, want [%s]: the SECTION named test_all_types2 has no uri", evalURIEarlier, reported, want)
			}
			if _, reported := unmatchedOf(t, f.name, wt, completionBody(map[string]any{narrative: "narrative"})); len(reported) > 0 {
				t.Errorf("unmatched of %q = %v, want it read at the SECTION", narrative, reported)
			}
		})
	}
}

// TestREQ053DuplicateSiblingEarlierSpellingNeedsItsSiblings — REQ-053, REQ-170:
// a number after an id is no earlier spelling where the parent lacks the
// sibling it would stand for, or the bare id. Such a key resolves nothing: it
// is unmatched content, reported as written and refused under WithStrict.
func TestREQ053DuplicateSiblingEarlierSpellingNeedsItsSiblings(t *testing.T) {
	allTypes, _ := allTypesSet(t)
	tests := []struct {
		name       string
		wt         *webtemplate.WebTemplate
		key        string
		structured string
	}{
		{
			name: "no sibling it stands for", wt: allTypes,
			key: "test_all_types/test_all_types4:0/uri", structured: "test_all_types/test_all_types4",
		},
		{
			name: "no bare id", wt: renamedAllTypes(t, "test_all_types", "observation"),
			key: evalURIEarlier, structured: "test_all_types/test_all_types2",
		},
	}
	for _, f := range formats {
		for _, tc := range tests {
			t.Run(f.name+"/"+tc.name, func(t *testing.T) {
				want := map[string]string{"FLAT": tc.key, "STRUCTURED": tc.structured}[f.name]
				body := completionBody(map[string]any{tc.key: "http://example.org/a"})
				if _, reported := unmatchedOf(t, f.name, tc.wt, body); !slices.Equal(reported, []string{want}) {
					t.Errorf("unmatched = %v, want [%s]", reported, want)
				}
				_, err := decodeFormat(t, f.name, tc.wt, body, simplified.WithStrict())
				if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), want) {
					t.Errorf("strict decode = %v, want ErrUnknownPath naming %q", err, want)
				}
			})
		}
	}
}

// TestREQ053DuplicateSiblingTwoSpellings — REQ-053, REQ-170: a key and its
// earlier spelling are two spellings of one field. Equal values, or one of
// them null, read once; disagreeing values are refused with ErrUnknownPath
// naming both keys and neither value.
func TestREQ053DuplicateSiblingTwoSpellings(t *testing.T) {
	wt, _ := allTypesSet(t)
	const a, b = "http://example.org/a", "http://example.org/b"
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			want, _ := unmatchedOf(t, f.name, wt, completionBody(map[string]any{evalURI: a}))
			for _, both := range []map[string]any{
				{evalURI: a, evalURIEarlier: a},
				{evalURI: a, evalURIEarlier: nil},
				{evalURI: nil, evalURIEarlier: a},
			} {
				got, reported := unmatchedOf(t, f.name, wt, completionBody(both))
				if len(reported) > 0 {
					t.Errorf("decode of %v reported %v unmatched", both, reported)
				}
				if g, w := canonicalComposition(t, got), canonicalComposition(t, want); string(g) != string(w) {
					t.Errorf("decode of %v =\n%s\nwant what %q alone gives:\n%s", both, g, evalURI, w)
				}
			}
			_, err := decodeFormat(t, f.name, wt, completionBody(map[string]any{evalURI: a, evalURIEarlier: b}))
			if !errors.Is(err, simplified.ErrUnknownPath) {
				t.Fatalf("decode of two spellings that disagree = %v, want ErrUnknownPath", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, evalURI) || !strings.Contains(msg, evalURIEarlier) || strings.Contains(msg, a) || strings.Contains(msg, b) {
				t.Errorf("refusal %q, want both keys named and neither value", msg)
			}
		})
	}
}

// TestREQ053DuplicateSiblingTwoUnreadSpellings — REQ-170: two spellings of a
// key the decode does not read are two unmatched keys, whatever their values,
// each reported as written; the key with no earlier spelling beside them is
// still read.
func TestREQ053DuplicateSiblingTwoUnreadSpellings(t *testing.T) {
	wt, _ := allTypesSet(t)
	body := completionBody(map[string]any{
		evalURI + "|x-note":        "one",
		evalURIEarlier + "|x-note": "two",
		evalURI:                    "http://example.org/a",
	})
	want, _ := unmatchedOf(t, "FLAT", wt, completionBody(map[string]any{evalURI: "http://example.org/a"}))
	got, reported := unmatchedOf(t, "FLAT", wt, body)
	slices.Sort(reported)
	if wantReported := []string{evalURIEarlier + "|x-note", evalURI + "|x-note"}; !slices.Equal(reported, wantReported) {
		t.Errorf("unmatched = %v, want %v", reported, wantReported)
	}
	if g, w := canonicalComposition(t, got), canonicalComposition(t, want); string(g) != string(w) {
		t.Errorf("decode =\n%s\nwant what the read key alone gives:\n%s", g, w)
	}
}

// TestREQ053DuplicateSiblingEarlierSpellingWithEmptySuffix — REQ-053: an
// earlier spelling ending in a bare `|` reads as the key without it in the Web
// Template's spelling, and an unmatched one is reported as written, its `|`
// and its earlier id kept.
func TestREQ053DuplicateSiblingEarlierSpellingWithEmptySuffix(t *testing.T) {
	wt, _ := allTypesSet(t)
	want, _ := unmatchedOf(t, "FLAT", wt, completionBody(map[string]any{evalURI: "http://example.org/a"}))
	got, reported := unmatchedOf(t, "FLAT", wt, completionBody(map[string]any{evalURIEarlier + "|": "http://example.org/a"}))
	if len(reported) > 0 {
		t.Errorf("decode of %q reported %v unmatched", evalURIEarlier+"|", reported)
	}
	if g, w := canonicalComposition(t, got), canonicalComposition(t, want); string(g) != string(w) {
		t.Errorf("decode of %q =\n%s\nwant what %q gives:\n%s", evalURIEarlier+"|", g, evalURI, w)
	}
	note := evalURIEarlier + "|x-note|"
	if _, reported := unmatchedOf(t, "FLAT", wt, completionBody(map[string]any{note: "n"})); !slices.Equal(reported, []string{note}) {
		t.Errorf("unmatched = %v, want [%s]", reported, note)
	}
}

// TestREQ053DuplicateSiblingEncodesCurrentSpelling — REQ-053: a composition
// decoded from the earlier spelling encodes under the Web Template's id, in
// FLAT and in STRUCTURED, and no key or member carries the earlier one.
func TestREQ053DuplicateSiblingEncodesCurrentSpelling(t *testing.T) {
	wt, _ := allTypesSet(t)
	comp, _ := unmatchedOf(t, "FLAT", wt, completionBody(map[string]any{evalURIEarlier: "http://example.org/a", obsTimeEarlier: "2019-01-28T21:22:49.332Z"}))
	got := flatKeys(t, comp, wt)
	for _, key := range []string{evalURI, obsTime} {
		if _, ok := got[key]; !ok {
			t.Errorf("MarshalFlat wrote no %q", key)
		}
	}
	for key := range got {
		if strings.Contains(key, "test_all_types2") || strings.Contains(key, "/time2") {
			t.Errorf("MarshalFlat wrote %q, the earlier spelling", key)
		}
	}
	out, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("MarshalStructured wrote no JSON object: %v", err)
	}
	root, _ := doc["test_all_types"].(map[string]any)
	if _, ok := root["test_all_types_1"]; !ok {
		t.Errorf("MarshalStructured wrote no member test_all_types_1 under the root: %s", out)
	}
	if _, ok := root["test_all_types2"]; ok {
		t.Errorf("MarshalStructured wrote the member test_all_types2, the earlier spelling: %s", out)
	}
}
