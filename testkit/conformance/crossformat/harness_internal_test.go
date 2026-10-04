package crossformat

// Unit tests for the harness's comparison helpers (PROBE-105, REQ-080). The
// corpus run proves the pipeline end to end, but it cannot show that a leaf
// comparison is semantic rather than byte-wise, or that metadata is held out of
// both FLAT sides: these tests pin each of those on its own.

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestProbe105Leaves pins the leaf form REQ-080 asks for: a semantic
// comparison, insensitive to member order, exact on number spelling and
// array position, with empty containers as leaves of their own.
func TestProbe105Leaves(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want map[string]string
	}{
		{
			name: "member order does not matter",
			doc:  `{"b":1,"a":"x"}`,
			want: map[string]string{"/a": `"x"`, "/b": "1"},
		},
		{
			name: "numbers keep their literal text",
			doc:  `{"int":1,"real":1.0,"big":9007199254740993}`,
			want: map[string]string{"/int": "1", "/real": "1.0", "/big": "9007199254740993"},
		},
		{
			name: "array elements by position",
			doc:  `{"a":["x","y"]}`,
			want: map[string]string{"/a/0": `"x"`, "/a/1": `"y"`},
		},
		{
			name: "empty containers are leaves",
			doc:  `{"o":{},"a":[]}`,
			want: map[string]string{"/o": "{}", "/a": "[]"},
		},
		{
			name: "member names are escaped as JSON pointer tokens",
			doc:  `{"a/b":1,"c~d":2}`,
			want: map[string]string{"/a~1b": "1", "/c~0d": "2"},
		},
		{
			name: "null and booleans",
			doc:  `{"n":null,"t":true}`,
			want: map[string]string{"/n": "null", "/t": "true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := leaves([]byte(tt.doc))
			if err != nil {
				t.Fatalf("leaves(%s) error = %v", tt.doc, err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("leaves(%s) = %v, want %v", tt.doc, got, tt.want)
			}
		})
	}
	for _, bad := range []string{`{"a":`, `{"a":1}{"b":2}`, `{"a":1} x`} {
		if _, err := leaves([]byte(bad)); err == nil {
			t.Errorf("leaves(%q) error = nil, want an error", bad)
		}
	}
}

// TestProbe105CompareLeaves pins the three difference classes in both
// directions, the reference-side count, and sorted output.
func TestProbe105CompareLeaves(t *testing.T) {
	ref := map[string]string{"/a": "1", "/b": `"x"`, "/c": "2", "/z": "0"}
	ours := map[string]string{"/a": "1", "/b": `"y"`, "/d": "3", "/e": "4"}
	got := compareLeaves(ref, ours)

	want := Outcome{Compared: 4, Missing: 2, Extra: 2, Altered: 1}
	if got.Outcome != want {
		t.Errorf("compareLeaves().Outcome = %+v, want %+v", got.Outcome, want)
	}
	if want := []string{"/c", "/z"}; !slices.Equal(got.MissingKeys, want) {
		t.Errorf("MissingKeys = %v, want %v", got.MissingKeys, want)
	}
	if want := []string{"/d", "/e"}; !slices.Equal(got.ExtraKeys, want) {
		t.Errorf("ExtraKeys = %v, want %v", got.ExtraKeys, want)
	}
	if want := []Alteration{{Key: "/b", Reference: `"x"`, Ours: `"y"`}}; !slices.Equal(got.Alterations, want) {
		t.Errorf("Alterations = %v, want %v", got.Alterations, want)
	}
	if clean := compareLeaves(ref, ref); !clean.Outcome.Clean() || clean.Outcome.Compared != 4 {
		t.Errorf("compareLeaves(ref, ref).Outcome = %+v, want clean over 4 leaves", clean.Outcome)
	}
}

// TestProbe105LegJSONXMLComparesDecodedLeaves pins leg (a) on the vendored
// family_history set: the canonical XML is decoded and compared with the
// canonical JSON leaf by leaf, never by bytes and never with itself. The
// unchanged set agrees over every leaf, and one XML value changed in memory
// is reported as one altered leaf at its JSON pointer.
func TestProbe105LegJSONXMLComparesDecodedLeaves(t *testing.T) {
	sets, err := fixtures.ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error = %v", err)
	}
	i := slices.IndexFunc(sets, func(s fixtures.CrossFormatSet) bool { return s.Name == "family_history" })
	if i < 0 {
		t.Fatal("the cross-format corpus has no family_history set")
	}
	in, err := readInputs(sets[i])
	if err != nil {
		t.Fatalf("readInputs(family_history) error = %v", err)
	}

	got, err := legJSONXML(in)
	if err != nil {
		t.Fatalf("legJSONXML(family_history) error = %v", err)
	}
	if want := (Outcome{Compared: 95}); got.Outcome != want {
		t.Errorf("legJSONXML(family_history).Outcome = %+v, want %+v (missing %v, extra %v, altered %v)",
			got.Outcome, want, got.MissingKeys, got.ExtraKeys, got.Alterations)
	}

	// "Mother" is the value of exactly one leaf in each canonical document.
	const from, to = "<value>Mother</value>", "<value>Father</value>"
	if n := bytes.Count(in.canonicalXML, []byte(from)); n != 1 {
		t.Fatalf("family_history canonical.xml carries %q %d times, want once", from, n)
	}
	changed := in
	changed.canonicalXML = bytes.Replace(in.canonicalXML, []byte(from), []byte(to), 1)
	got, err = legJSONXML(changed)
	if err != nil {
		t.Fatalf("legJSONXML(family_history, XML %s) error = %v", to, err)
	}
	if want := (Outcome{Compared: 95, Altered: 1}); got.Outcome != want {
		t.Errorf("legJSONXML(family_history, XML %s).Outcome = %+v, want %+v", to, got.Outcome, want)
	}
	want := []Alteration{{Key: motherPointer, Reference: `"Mother"`, Ours: `"Father"`}}
	if !slices.Equal(got.Alterations, want) {
		t.Errorf("legJSONXML(family_history, XML %s).Alterations = %v, want %v", to, got.Alterations, want)
	}
}

// motherPointer is the JSON pointer of the family_history leaf whose value is
// "Mother", the Relationship element of the family member: the same pointer
// in the upstream canonical JSON and in the canjson re-encode of either
// canonical document.
const motherPointer = "/content/0/data/items/0/items/1/value/value"

// TestProbe105CompareFlatHoldsMetadataOutOnBothSides pins the PROBE-086
// hold-out on both FLAT sides: a ctx/ short form on one side and the real-path
// spelling on the other are not a difference, while the composer's
// external_ref, which no ctx/ form carries, still is. Values compare by their
// JSON text, so 1 and 1.0 differ.
func TestProbe105CompareFlatHoldsMetadataOutOnBothSides(t *testing.T) {
	const root = "r"
	ref := map[string]any{
		"r/language|code":      "en",
		"r/composer|name":      "x",
		"r/context/start_time": "2020-01-01T00:00:00Z",
		"r/composer|id":        "c1",
		"r/obs/q|magnitude":    json.Number("1.0"),
		"r/obs/text":           "a",
	}
	ours := map[string]any{
		"ctx/language":      "en",
		"ctx/composer_name": "x",
		"ctx/time":          "2020-01-01T00:00:00Z",
		"r/obs/q|magnitude": json.Number("1"),
		"r/obs/text":        "a",
	}
	got, err := compareFlat(ref, ours, root)
	if err != nil {
		t.Fatalf("compareFlat() error = %v", err)
	}
	want := Outcome{Compared: 3, Missing: 1, Altered: 1}
	if got.Outcome != want {
		t.Errorf("compareFlat().Outcome = %+v, want %+v (missing %v, extra %v, altered %v)",
			got.Outcome, want, got.MissingKeys, got.ExtraKeys, got.Alterations)
	}
	if !slices.Equal(got.MissingKeys, []string{"r/composer|id"}) {
		t.Errorf("MissingKeys = %v, want only the composer external_ref, which the hold-out must not cover", got.MissingKeys)
	}

	// The reverse: metadata only on our side is not extra either.
	rev, err := compareFlat(ours, ref, root)
	if err != nil {
		t.Fatalf("compareFlat() error = %v", err)
	}
	if !slices.Equal(rev.ExtraKeys, []string{"r/composer|id"}) {
		t.Errorf("reversed ExtraKeys = %v, want only the composer external_ref", rev.ExtraKeys)
	}
}

// TestProbe105DecodeFlatRefusalIsTheCodecError pins how a decode the reducing
// loop cannot reduce becomes the leg's outcome: a refusal carrying the codec's
// own error, not the harness's account of it, while a harness fault stays an
// error.
func TestProbe105DecodeFlatRefusalIsTheCodecError(t *testing.T) {
	target, err := webtemplate.NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	// No ctx/language or ctx/territory, and nothing injects them: decode fails
	// with the codec's missing-context error, which names no key.
	_, _, err = decodeFlat(target, map[string]any{}, webtemplate.KeepContext)
	refused, ok := errors.AsType[errRefused](err)
	if !ok {
		t.Fatalf("decodeFlat(no context) error = %v, want a leg refusal", err)
	}
	if !strings.HasPrefix(refused.msg, "FLAT decode: ") || !strings.Contains(refused.msg, "missing mandatory context") {
		t.Errorf("refusal = %q, want the codec's missing-context error after %q", refused.msg, "FLAT decode: ")
	}
	if strings.Contains(refused.msg, "attributable") {
		t.Errorf("refusal = %q, want the codec's error without the harness's account of it", refused.msg)
	}

	_, _, err = decodeFlat(target, map[string]any{}, webtemplate.ContextMode(99))
	if err == nil {
		t.Fatal("decodeFlat(unknown mode) error = nil, want a harness fault")
	}
	if _, ok := errors.AsType[errRefused](err); ok {
		t.Errorf("decodeFlat(unknown mode) error = %v, want a harness fault, not a leg refusal", err)
	}
}

// TestProbe105RunLegRecordsRefusals pins runLeg's split: a codec error ends the
// leg with a refusal outcome and no error, and an unknown leg is a harness
// fault.
func TestProbe105RunLegRecordsRefusals(t *testing.T) {
	in := inputs{flatRaw: []byte(`{`)}
	if _, err := simplified.FlatToStructured(in.flatRaw); err == nil {
		t.Fatal("FlatToStructured accepts a truncated body; pick another malformed one")
	}
	lr, err := runLeg(LegFlatStructured, nil, in)
	if err != nil {
		t.Fatalf("runLeg(FLAT that does not restructure) error = %v, want a refusal outcome", err)
	}
	if !strings.HasPrefix(lr.Outcome.Refused, "FLAT to STRUCTURED: ") || lr.Outcome.Compared != 0 {
		t.Errorf("Outcome = %+v, want a refusal carrying the restructure error and no counts", lr.Outcome)
	}
	if _, err := runLeg(Leg("no-such-leg"), nil, in); err == nil {
		t.Error("runLeg(unknown leg) error = nil, want a harness fault")
	}
}

// TestProbe105CodeSpan pins the census code spans: the fence outgrows any
// backtick run inside, and a span starting or ending with a backtick is
// padded.
func TestProbe105CodeSpan(t *testing.T) {
	for in, want := range map[string]string{
		"a|b":   "`a|b`",
		"a`b":   "``a`b``",
		"`a":    "`` `a ``",
		"a``b":  "```a``b```",
		"plain": "`plain`",
	} {
		if got := code(in); got != want {
			t.Errorf("code(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProbe105Abbreviate pins the shortening of long payloads in reports: a
// long token is cut, short tokens and the spaces between them are kept.
func TestProbe105Abbreviate(t *testing.T) {
	long := strings.Repeat("A", 150)
	got := Abbreviate(`parsing "` + long + `": invalid syntax`)
	want := `parsing "` + strings.Repeat("A", 39) + `... invalid syntax`
	if got != want {
		t.Errorf("Abbreviate(long payload) = %q, want %q", got, want)
	}
	if in := "short words stay as they are"; Abbreviate(in) != in {
		t.Errorf("Abbreviate(%q) = %q, want it unchanged", in, Abbreviate(in))
	}
	if got := abbreviateValue(strings.Repeat("é", 70)); got != strings.Repeat("é", 50)+"..." {
		t.Errorf("abbreviateValue(70 runes) = %q, want the first 50 runes and ...", got)
	}
}
