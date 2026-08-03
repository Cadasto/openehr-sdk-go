package webtemplate

// Unit tests for the harness's key bookkeeping. These are deliberately
// separate from the corpus run: the corpus proves the pipeline end to end but
// cannot show *why* a key left the comparison, and getting that wrong is how a
// harness quietly understates what the codec really round-trips.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// TestSameValue pins the comparison's two contracts: scalars compare by their
// JSON bytes (so json.Number keeps 1 vs 1.0 and >2^53 integers apart), and
// composites — the |raw shape a codec change might admit — compare
// structurally instead of panicking on interface ==.
func TestSameValue(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		want bool
	}{
		{"equal strings", "x", "x", true},
		{"unequal strings", "x", "y", false},
		{"equal numbers", json.Number("42"), json.Number("42"), true},
		{"int vs float spelling differs", json.Number("1"), json.Number("1.0"), false},
		{"big int exact", json.Number("9007199254740993"), json.Number("9007199254740993"), true},
		{"big int off by one", json.Number("9007199254740993"), json.Number("9007199254740992"), false},
		{"equal maps", map[string]any{"a": "1"}, map[string]any{"a": "1"}, true},
		{"unequal maps", map[string]any{"a": "1"}, map[string]any{"a": "2"}, false},
		{"equal slices", []any{"a", "b"}, []any{"a", "b"}, true},
		{"unequal slices", []any{"a"}, []any{"b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameValue(tt.a, tt.b); got != tt.want {
				t.Errorf("sameValue(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestParseFlatPreservesNumbers — the harness must carry upstream numbers
// verbatim: rounding 9007199254740993 to float64 on both sides would let a
// >2^53 codec regression pass, and folding 1.0 into 1 would hide a
// formatting divergence. Exactly the masking PROBE-076's equality was
// already fixed for.
func TestParseFlatPreservesNumbers(t *testing.T) {
	m, err := parseFlat([]byte(`{"big": 9007199254740993, "frac": 1.0, "int": 1}`))
	if err != nil {
		t.Fatalf("parseFlat: %v", err)
	}
	for k, want := range map[string]json.Number{
		"big": "9007199254740993", "frac": "1.0", "int": "1",
	} {
		got, ok := m[k].(json.Number)
		if !ok || got != want {
			t.Errorf("parseFlat()[%q] = %v (%T), want json.Number(%q)", k, m[k], m[k], want)
		}
	}
}

// TestParseFlatRejectsNonBodies — parseFlat is the only gate between a fixture
// file and the comparison, and two shapes json.Decoder accepts would make the
// harness compare less than it reports: `null` (a nil map that round-trips as
// zero keys, zero refusals, "clean") and content after the first JSON value
// (silently read as whichever half came first).
func TestParseFlatRejectsNonBodies(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"object", `{"a":1}`, false},
		{"empty object", `{}`, false},
		{"trailing whitespace is fine", "{\"a\":1}\n", false},
		{"malformed", `{"a":`, true},
		{"null", `null`, true},
		{"array", `["a"]`, true},
		{"scalar", `42`, true},
		{"empty input", ``, true},
		{"trailing garbage", `{"a":1}garbage`, true},
		{"two objects", `{"a":1}{"b":2}`, true},
		// dec.More() reports false for a trailing `]`, which is why the check is
		// a second Decode against io.EOF rather than More().
		{"trailing bracket", `{"a":1}]`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseFlat([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFlat(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && m == nil {
				t.Errorf("parseFlat(%q) returned a nil map with a nil error", tt.in)
			}
		})
	}
}

// TestRunNilTarget — Run has an error-returning contract; a nil target must
// come back as an error, not a panic (no-library-panics convention).
func TestRunNilTarget(t *testing.T) {
	if _, err := Run(nil, Case{Name: "x"}); err == nil {
		t.Fatal("Run(nil, c) returned nil error, want error")
	}
}

// TestCompare exercises every verdict [compare] can reach. The corpus cannot:
// it is expected to produce none of them, so the classification loop is never
// observed to fire by the corpus run — and the excluded/compared pins cannot
// cover it either, since both are counted *before* the comparison. Without
// this, a compare that returned nothing at all would leave the suite green.
func TestCompare(t *testing.T) {
	const root = "conformance_ehrbase.de.v0"
	const key = "c/obs/quantity|magnitude"

	tests := []struct {
		name               string
		candidate, emitted map[string]any
		missing, extra     []string
		mismatched         []string
	}{
		{
			name:      "clean round-trip",
			candidate: map[string]any{key: json.Number("65.9"), "c/obs/quantity|unit": "mg"},
			emitted:   map[string]any{key: json.Number("65.9"), "c/obs/quantity|unit": "mg"},
		},
		{
			name:      "equal composite values are clean",
			candidate: map[string]any{key: map[string]any{"_type": "DV_PARSABLE", "value": "x"}},
			emitted:   map[string]any{key: map[string]any{"_type": "DV_PARSABLE", "value": "x"}},
		},
		{
			name:      "decoded but not re-emitted is Missing",
			candidate: map[string]any{key: json.Number("1"), "c/obs/quantity|unit": "mg"},
			emitted:   map[string]any{"c/obs/quantity|unit": "mg"},
			missing:   []string{key},
		},
		{
			name:      "emitted key upstream does not have is Extra",
			candidate: map[string]any{key: json.Number("1")},
			emitted:   map[string]any{key: json.Number("1"), "c/obs/quantity|units_system": "UCUM"},
			extra:     []string{"c/obs/quantity|units_system"},
		},
		{
			// The hold-out is one-sided by construction: the candidate set never
			// contains metadata, so without the skip every ctx/ key the encoder
			// writes would be reported as Extra.
			name:      "emitted ctx/ key is held out, not Extra",
			candidate: map[string]any{key: json.Number("1")},
			emitted: map[string]any{
				key:               json.Number("1"),
				"ctx/language":    "en",
				"ctx/time":        "2026-08-01T00:00:00Z",
				"ctx/composer_id": "irrelevant",
			},
		},
		{
			name:      "emitted root metadata path is held out, not Extra",
			candidate: map[string]any{key: json.Number("1")},
			emitted: map[string]any{
				key:                       json.Number("1"),
				root + "/language|code":   "en",
				root + "/context/setting": "other care",
			},
		},
		{
			name:       "differing value is Mismatched",
			candidate:  map[string]any{key: json.Number("65.9")},
			emitted:    map[string]any{key: json.Number("66.6")},
			mismatched: []string{key + ": upstream=65.9 ours=66.6"},
		},
		{
			// Byte-level, not numeric: 1 and 1.0 are the same number and a
			// different FLAT spelling, and the codec must not respell.
			name:       "int vs float spelling is Mismatched",
			candidate:  map[string]any{key: json.Number("1")},
			emitted:    map[string]any{key: json.Number("1.0")},
			mismatched: []string{key + ": upstream=1 ours=1.0"},
		},
		{
			name:      "unequal composites are Mismatched",
			candidate: map[string]any{key: []any{"a", "b"}},
			emitted:   map[string]any{key: []any{"a", "c"}},
			// %v of a []any — the exact rendering is the report's, not a contract.
			mismatched: []string{key + ": upstream=[a b] ours=[a c]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, extra, mismatched := compare(tt.candidate, tt.emitted, root)
			if !slices.Equal(missing, tt.missing) {
				t.Errorf("missing = %q, want %q", missing, tt.missing)
			}
			if !slices.Equal(extra, tt.extra) {
				t.Errorf("extra = %q, want %q", extra, tt.extra)
			}
			if !slices.Equal(mismatched, tt.mismatched) {
				t.Errorf("mismatched = %q, want %q", mismatched, tt.mismatched)
			}
		})
	}
}

// TestCompareSortsVerdicts — the reports are diffed across commits, so the
// three lists must not inherit map iteration order.
func TestCompareSortsVerdicts(t *testing.T) {
	candidate := map[string]any{"c/z": "1", "c/a": "1", "c/m": "1"}
	missing, _, _ := compare(candidate, map[string]any{}, "root")
	if want := []string{"c/a", "c/m", "c/z"}; !slices.Equal(missing, want) {
		t.Errorf("missing = %q, want %q", missing, want)
	}
}

// suffixErr reproduces the shape flat_decode produces for a suffix the
// datatype does not map: the allowlist error, wrapped with the *base* path
// (not the suffixed key) by the decode-site wrapper.
func suffixErr(base, label, rmType string) error {
	inner := fmt.Errorf("%w: unexpected %s for %s", simplified.ErrUnsupportedDatatype, label, rmType)
	return fmt.Errorf("simplified: decode %q: %w", base, inner)
}

func TestRefusedSuffix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
		ok   bool
	}{
		{"suffix", suffixErr("c/p", "|precision", "DV_QUANTITY"), "|precision", true},
		{"bare value", suffixErr("c/p", "bare value", "DV_PROPORTION"), "", true},
		{
			"whole datatype unmodelled",
			fmt.Errorf("simplified: decode %q: %w: DV_MULTIMEDIA", "c/p", simplified.ErrUnsupportedDatatype),
			"", false,
		},
		{
			"unknown path",
			fmt.Errorf("%w: %q", simplified.ErrUnknownPath, "c/p"),
			"", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := refusedSuffix(tt.err)
			if got != tt.want || ok != tt.ok {
				t.Errorf("refusedSuffix() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestDropRefusedScope pins the three drop scopes. Every case here was a real
// over-drop at some point: the suffix cases used to take the whole leaf with
// them, and the container case used to take the whole subtree.
func TestDropRefusedScope(t *testing.T) {
	const (
		qty   = "c/obs/quantity"
		event = "c/obs/data/any_event:0"
	)
	body := func() map[string]any {
		return map[string]any{
			qty + "|magnitude":               12.0,
			qty + "|unit":                    "mg",
			qty + "|precision":               1.0,
			event + "|sample_count":          3.0,
			event + "/temperature|magnitude": 37.0,
			event + "/temperature|unit":      "Cel",
			"c/obs/_uid":                     "x",
			"c/obs/_uid/whatever":            "y",
			// A string-prefix sibling, not a child: it must survive a subtree
			// drop of `c/obs/_uid`. strings.HasPrefix on the bare base — without
			// the "/" — would take it too.
			"c/obs/_uid_based_id": "z",
		}
	}

	tests := []struct {
		name    string
		key     string
		err     error
		removed int
		gone    []string
	}{
		{
			// The refusal is about |precision alone; |magnitude and |unit are
			// modelled and must survive into the comparison.
			name:    "unmapped suffix takes only that entry",
			key:     qty,
			err:     suffixErr(qty, "|precision", "DV_QUANTITY"),
			removed: 1,
			gone:    []string{qty + "|precision"},
		},
		{
			// An EVENT the codec cannot read *as a value* still has children
			// it reads fine — dropping the subtree here zeroed a whole fixture.
			name:    "container addressed as a leaf keeps its subtree",
			key:     event,
			err:     fmt.Errorf("simplified: decode %q: %w: EVENT", event, simplified.ErrUnsupportedDatatype),
			removed: 1,
			gone:    []string{event + "|sample_count"},
		},
		{
			// Nothing below an unresolvable node can resolve either, so the
			// subtree goes with it — but only the subtree: `_uid_based_id` is a
			// different leaf that merely starts with the same characters.
			name:    "unresolvable path takes leaf and subtree but not a prefix sibling",
			key:     "c/obs/_uid",
			err:     fmt.Errorf("%w: %q", simplified.ErrUnknownPath, "c/obs/_uid"),
			removed: 2,
			gone:    []string{"c/obs/_uid", "c/obs/_uid/whatever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := body()
			before := len(candidate)
			got, gap := dropRefused(candidate, tt.key, tt.err)
			if !gap {
				t.Fatalf("dropRefused classified a %v refusal as a harness fault", tt.err)
			}
			if got != tt.removed {
				t.Errorf("dropRefused removed %d keys, want %d", got, tt.removed)
			}
			for _, k := range tt.gone {
				if _, still := candidate[k]; still {
					t.Errorf("key %q should have been dropped", k)
				}
			}
			for k := range body() {
				if slices.Contains(tt.gone, k) {
					continue
				}
				if _, ok := candidate[k]; !ok {
					t.Errorf("key %q was dropped but is not part of this refusal", k)
				}
			}
			if len(candidate) != before-tt.removed {
				t.Errorf("body went from %d to %d keys, want %d removed", before, len(candidate), tt.removed)
			}
		})
	}
}

// TestDropRefusedRejectsUnclassifiedError — an error that names a key but is
// neither REQ-053 gap sentinel is a *harness fault*, not unmodelled surface.
// The fallback scope used to swallow it as a datatype refusal, which would let
// a codec bug present as a larger Excluded count with the suite still green.
func TestDropRefusedRejectsUnclassifiedError(t *testing.T) {
	const key = "c/obs/quantity"
	candidate := map[string]any{key + "|magnitude": 1.0}
	err := fmt.Errorf("simplified: decode %q: internal invariant broken", key)

	removed, gap := dropRefused(candidate, key, err)
	if gap {
		t.Error("dropRefused accepted a non-sentinel error as a codec gap")
	}
	if removed != 0 {
		t.Errorf("dropRefused removed %d keys for an unclassified error, want 0", removed)
	}
	if len(candidate) != 1 {
		t.Errorf("candidate was modified by an unclassified error: %v", candidate)
	}
}

func TestOffendingKey(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"unknown path", fmt.Errorf("%w: %q", simplified.ErrUnknownPath, "c/x"), "c/x"},
		{"decode wrapper", suffixErr("c/x", "|precision", "DV_QUANTITY"), "c/x"},
		// No quoted token at all: decodeReducing must see "" and fail loudly
		// rather than drop something arbitrary.
		{"unattributable", errors.New("simplified: budget exceeded"), ""},
		// Two quoted tokens: the *first* wins, whatever it is. Real codec sites
		// quote something other than the key first — this is parseFlatKey's
		// invalid-:index refusal, which quotes the bad index substring — so this
		// records what the harness does with them rather than pretending the
		// codec guarantees key-first. The token here matches nothing in the
		// body, which is what makes decodeReducing abort loudly.
		{
			"first of two quoted tokens wins",
			fmt.Errorf("%w: invalid :index %q in %q", simplified.ErrUnknownPath, "01", "c/obs/any_event:01"),
			"01",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := offendingKey(tt.err); got != tt.want {
				t.Errorf("offendingKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReasonOfIsKeyIndependent — the census aggregates refusals by reason, so
// the reason must not carry the key (or the row would be per-key) and must not
// carry the particular suffix the codec happened to name first (map order).
func TestReasonOfIsKeyIndependent(t *testing.T) {
	a := suffixErr("c/one", "|precision", "DV_QUANTITY")
	b := suffixErr("c/two", "|magnitude_status", "DV_QUANTITY")
	if got, want := reasonOf(a, "c/one"), "unsupported |suffix for DV_QUANTITY"; got != want {
		t.Errorf("reasonOf() = %q, want %q", got, want)
	}
	if reasonOf(a, "c/one") != reasonOf(b, "c/two") {
		t.Errorf("two suffixes of the same datatype gave different reasons: %q vs %q",
			reasonOf(a, "c/one"), reasonOf(b, "c/two"))
	}
	bare := suffixErr("c/p", "bare value", "DV_PROPORTION")
	if got, want := reasonOf(bare, "c/p"), "unsupported bare value for DV_PROPORTION"; got != want {
		t.Errorf("reasonOf(bare) = %q, want %q", got, want)
	}
}

// TestMetaLeavesMatchCodecAliases pins the invariant the two lists share: every
// composition-level real-path spelling the codec accepts on decode
// (simplified.metadataAliases, ADR 0015) must also be held out here. One accepted
// there but missing here is reported as both missing and extra — the exact noise
// the hold-out exists to remove — and no corpus fixture need exercise the
// spelling for the inconsistency to be real.
func TestMetaLeavesMatchCodecAliases(t *testing.T) {
	const root = "conformance_ehrbase.de.v0"
	// Mirrors simplified.metadataAliases; that table is unexported, so the
	// spellings are restated rather than imported. Adding one there without
	// adding it here fails this test.
	for _, rel := range []string{
		"language|code", "territory|code", "composer|name",
		"composer_self", "context/start_time",
	} {
		if !IsCompositionMeta(root+"/"+rel, root) {
			t.Errorf("codec accepts %q as a metadata alias but the comparison does not hold it out", rel)
		}
	}
}

// TestIsCompositionMeta guards the hold-out's edges. It is the one place the
// harness excuses a difference on *both* sides, so it has to stay narrow:
// `context/other_context` carries archetyped data and must never be swallowed.
func TestIsCompositionMeta(t *testing.T) {
	const root = "conformance_ehrbase.de.v0"
	tests := []struct {
		key  string
		want bool
	}{
		{"ctx/language", true},
		{root + "/language|code", true},
		{root + "/composer|name", true},
		{root + "/context/start_time", true},
		{root + "/context/setting|code", true},
		// composer_self is an accepted real-path alias (ADR 0015), so it must be
		// held out on both sides like the other respellings — the present corpus
		// writes only the ctx/ form, which is why its absence from metaLeaves went
		// unnoticed until the PR #86 review.
		{root + "/composer_self", true},
		{"ctx/composer_self", true},
		// category is *not* a ctx/ field: it is a template-constrained Web
		// Template leaf, spelled identically on both sides, so it is compared
		// like any other content key rather than held out.
		{root + "/category|code", false},
		{root + "/category|value", false},
		{root + "/context/other_context/anything", false},
		{root + "/context/health_care_facility|name", false},
		{root + "/content/observation/language", false},
		{"someothertemplate/language", false},
		{root, false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsCompositionMeta(tt.key, root); got != tt.want {
				t.Errorf("IsCompositionMeta(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestRealCodecRefusal parses a *genuine* codec refusal rather than a
// hand-written lookalike. Every other test above feeds the extraction helpers
// an error this file phrased itself (see [suffixErr]), so if flat_decode's
// wrapper were reworded they would all still pass while the census silently
// lost its attribution — the refusal key, the suffix scope, and the reason row
// are all read out of the codec's prose. This test is the seam's pin: it drives
// the real decoder against the real corpus template and asserts the harness
// still understands what came back.
func TestRealCodecRefusal(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	base, body := modelledQuantityLeaf(t, target)

	// |normal_range is a DV_QUANTITY attribute with no suffix mapping: it is a
	// DV_INTERVAL, so it can never reduce to a scalar suffix and rides |raw
	// instead — which makes it a durable choice here. (|precision served until
	// the optional-suffix set landed and made it modelled.) Appending it to a
	// leaf that otherwise decodes makes the codec produce exactly the shape
	// refusedSuffix has to read.
	body[base+"|normal_range"] = json.Number("1")
	_, err = decodeSubset(target, body)
	if err == nil {
		t.Fatalf("%q|normal_range decoded — the codec now models this suffix, so this test "+
			"needs a different unmodelled one (and SKIPPED.md needs regenerating)", base)
	}
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("real refusal is not ErrUnsupportedDatatype: %v", err)
	}
	if got := offendingKey(err); got != base {
		t.Errorf("offendingKey(real refusal) = %q, want the leaf base %q\nrefusal: %v", got, base, err)
	}
	if got, ok := refusedSuffix(err); !ok || got != "|normal_range" {
		t.Errorf("refusedSuffix(real refusal) = %q, %v; want %q, true\nrefusal: %v",
			got, ok, "|normal_range", err)
	}
	if got, want := reasonOf(err, base), "unsupported |suffix for DV_QUANTITY"; got != want {
		t.Errorf("reasonOf(real refusal) = %q, want %q\nrefusal: %v", got, want, err)
	}

	// And the scope decision the coverage number depends on: the offending
	// entry goes, its modelled siblings on the same leaf stay.
	removed, gap := dropRefused(body, offendingKey(err), err)
	if !gap || removed != 1 {
		t.Fatalf("dropRefused(real refusal) = %d, %v; want 1, true", removed, gap)
	}
	if _, still := body[base+"|normal_range"]; still {
		t.Error("|normal_range survived its own refusal")
	}
	if _, ok := body[base+"|magnitude"]; !ok {
		t.Error("|magnitude was dropped by a |normal_range refusal — the leaf, not the entry, went")
	}

	// What remains must decode: otherwise the assertions above passed on an
	// error raised for some other reason entirely.
	if _, err := decodeSubset(target, body); err != nil {
		t.Errorf("body decodes only with the refused suffix removed, but still fails: %v", err)
	}
}

// modelledQuantityLeaf returns a DV_QUANTITY leaf the codec *does* model, as
// its base path plus the upstream entries that decode cleanly. Derived from the
// corpus rather than hardcoded, so a corpus refresh cannot leave the test
// asserting against a path that no longer exists.
func modelledQuantityLeaf(t *testing.T, target *Target) (string, map[string]any) {
	t.Helper()
	cases, err := Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}
	for _, c := range cases {
		raw, err := os.ReadFile(c.Flat)
		if err != nil {
			t.Fatalf("read %s: %v", c.Name, err)
		}
		body, err := parseFlat(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", c.Name, err)
		}
		bases := make([]string, 0, len(body))
		for k := range body {
			b, sfx, ok := strings.Cut(k, "|")
			// The `_`-prefixed RM attribute family is not modelled at all and
			// would refuse with ErrUnknownPath before any suffix check.
			if !ok || sfx != "magnitude" || strings.Contains(b, "/_") {
				continue
			}
			if _, hasUnit := body[b+"|unit"]; !hasUnit {
				continue
			}
			bases = append(bases, b)
		}
		slices.Sort(bases)
		for _, b := range bases {
			probe := map[string]any{
				b + "|magnitude": body[b+"|magnitude"],
				b + "|unit":      body[b+"|unit"],
			}
			if _, err := decodeSubset(target, probe); err == nil {
				return b, probe
			}
		}
	}
	t.Fatal("no modelled DV_QUANTITY leaf in the corpus — this test can no longer " +
		"build a real refusal; pick another datatype")
	return "", nil
}
