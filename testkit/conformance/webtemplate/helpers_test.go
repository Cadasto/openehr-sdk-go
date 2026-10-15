package webtemplate

// Unit tests for the harness's key bookkeeping. These are deliberately
// separate from the corpus run: the corpus proves the pipeline end to end but
// cannot show *why* a key left the comparison, and getting that wrong is how a
// harness quietly understates what the codec really round-trips.

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	wt "github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
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
	const root = corpusRoot
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

// fitErr reproduces the shape a value that does not fit its RM attribute
// takes (REQ-053): the refusal names the key it read the value from, quoted,
// and the decode-site wrapper names the leaf's *base* path first.
func fitErr(base, key string) error {
	inner := fmt.Errorf("%w: %q does not fit DV_QUANTITY.precision (Integer), which takes a whole number in the Integer range, bare or quoted; got a JSON number",
		simplified.ErrUnsupportedDatatype, key)
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

// TestDropRefusedScope pins the drop scopes. Every case here was a real
// over-drop at some point: the suffix cases used to take the whole leaf with
// them, the container case the whole subtree, and a refusal naming a whole key
// every suffix of its leaf (the composer's `|name` beside its `|id`, while the
// codec refused the `|id`).
func TestDropRefusedScope(t *testing.T) {
	const (
		qty   = "c/obs/quantity"
		event = "c/obs/data/any_event:0"
		count = "c/obs/count"
		rate  = "c/obs/rate"
	)
	body := func() map[string]any {
		return map[string]any{
			qty + "|magnitude":               12.0,
			qty + "|unit":                    "mg",
			qty + "|precision":               1.0,
			event + "|sample_count":          3.0,
			event + "/temperature|magnitude": 37.0,
			event + "/temperature|unit":      "Cel",
			count:                            1.5,
			count + "|magnitude_status":      "=",
			rate + "|":                       2.5,
			rate + "|magnitude_status":       "=",
			"c/obs/_uid":                     "x",
			"c/obs/_uid/whatever":            "y",
			// A string-prefix sibling, not a child: it must survive a subtree
			// drop of `c/obs/_uid`. strings.HasPrefix on the bare base — without
			// the "/" — would take it too.
			"c/obs/_uid_based_id": "z",
			// A metadata leaf: `|code` is the ctx/language alias the decode
			// reads, `|preferred_term` a suffix the short form cannot carry.
			"c/language|code":           "en",
			"c/language|preferred_term": "English",
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
			// A refusal naming a whole key covers that key alone, whichever
			// sentinel it carries: `language|preferred_term` is refused, the
			// `language|code` beside it is an accepted alias and stays
			// (PROBE-086: each refusal removes only what it names). The error
			// is the codec's own refusal of that suffix.
			name: "a whole key takes only that entry",
			key:  "c/language|preferred_term",
			err: fmt.Errorf("%w: %q is not a spelling the ctx/ short form can carry — ctx/language owns this leaf, so only its accepted suffixes travel (see deviations.md)",
				simplified.ErrUnsupportedDatatype, "c/language|preferred_term"),
			removed: 1,
			gone:    []string{"c/language|preferred_term"},
		},
		{
			name:    "a whole key under ErrUnknownPath takes only that entry",
			key:     qty + "|magnitude",
			err:     fmt.Errorf("%w: conflicting spellings: %q", simplified.ErrUnknownPath, qty+"|magnitude"),
			removed: 1,
			gone:    []string{qty + "|magnitude"},
		},
		{
			// A value that does not fit its attribute is refused naming its own
			// key after the leaf's base (REQ-053): that key alone goes, and the
			// modelled suffixes beside it stay (PROBE-086).
			name:    "a value that does not fit takes only its key",
			key:     qty,
			err:     fitErr(qty, qty+"|precision"),
			removed: 1,
			gone:    []string{qty + "|precision"},
		},
		{
			// A bare value that does not fit names the leaf's base as its own
			// key: the bare key alone goes, not the suffixes sharing the base.
			name:    "a bare value that does not fit takes only the bare key",
			key:     count,
			err:     fitErr(count, count),
			removed: 1,
			gone:    []string{count},
		},
		{
			// `x|` spells the bare value of `x`, and the refusal names `x`, the
			// key it spells: the spelling as written goes.
			name:    "a bare value spelled x| that does not fit takes only that key",
			key:     rate,
			err:     fitErr(rate, rate),
			removed: 1,
			gone:    []string{rate + "|"},
		},
		{
			// Nothing below a refused path decodes either, so the subtree goes
			// with it — but only the subtree: `_uid_based_id` is a different leaf
			// that merely starts with the same characters.
			name:    "refused path takes leaf and subtree but not a prefix sibling",
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

// corpusRoot is the Web Template tree id of the pinned corpus OPT, which is also
// the first segment of every upstream FLAT key. Hyphens, not underscores: the OPT
// *file* is `conformance_ehrbase.de.v0.opt`, the template id inside it is not, and
// a root literal that matches nothing would make every hold-out assertion below
// vacuously true. [TestHoldOutMatchesCodecAliases] pins the literal against the
// real target for exactly that reason.
const corpusRoot = "conformance-ehrbase.de.v0"

// TestHoldOutMatchesCodecAliases pins the hold-out against the codec's *own*
// tables, in both directions. Neither direction is symmetric noise-avoidance:
// they fail on opposite, differently dangerous drifts.
//
// Forward — every spelling the decoder accepts as composition metadata
// ([simplified.MetadataAliasSpellings] plus the terminology witnesses of
// [simplified.MetadataWitnessSpellings], ADR 0015) must be held out. One accepted
// there but compared here is reported as both missing and extra: cosmetic noise,
// and no corpus fixture need exercise the spelling for the inconsistency to be
// real. Derived from the exported accessors rather than a restated list, so a new
// alias cannot land in the codec without this test noticing.
//
// Reverse — every hold-out the harness actually applies over the corpus key
// universe must be one of those accepted spellings. **There is no waiver class**,
// and this direction is what keeps it that way: a hold-out with no codec spelling
// behind it silently absorbs a **refusal**, so a key the codec loses stops being
// counted as lost. It is what the base-matched `composer` entry did to
// `composer|id`/`|id_scheme`/`|id_namespace` — 12 corpus keys of the composer's
// `external_ref`, held out as if they were respellings, while SKIPPED.md,
// ADR 0015 and the codec's own doc comments all said they were refused and
// visible in the census (PR #86 review, round 3). Since ADR 0029 decode reads
// those keys and encode writes them at the same real path, so the census
// compares them; holding them out would hide a difference there instead. The suite carried exactly one
// named waiver — `context/setting`, a real encode-side drop while `ctx/setting`
// emission was deferred — until the amended REQ-053 closed that gap on
// 2026-08-05 and its keys became ordinary respellings; reintroducing a waiver
// is a spec decision (conformance.md § PROBE-086), not a harness edit, and this
// test fails on it.
func TestHoldOutMatchesCodecAliases(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	if target.Root != corpusRoot {
		t.Fatalf("corpus Web Template root is %q, but this file's tests assert against %q — "+
			"every hold-out assertion here would be vacuous; update corpusRoot", target.Root, corpusRoot)
	}

	accepted := map[string]bool{}
	for _, rel := range slices.Concat(simplified.MetadataAliasSpellings(), simplified.MetadataWitnessSpellings()) {
		accepted[rel] = true
	}
	if len(accepted) == 0 {
		t.Fatal("the codec reports no metadata spellings at all — the accessors, not the hold-out, are broken")
	}

	// notHeldOut records accepted spellings the harness deliberately compares
	// instead of holding out, each with its reason. Expected to stay empty: a
	// spelling decode normalises into ctx/ has no real-path counterpart on the
	// emitted side, so comparing it can only produce a false verdict. It exists so
	// that if one ever is justified, the justification is in the source rather
	// than in a weakened assertion.
	notHeldOut := map[string]string{}
	for _, rel := range slices.Sorted(maps.Keys(accepted)) {
		held := IsCompositionMeta(corpusRoot+"/"+rel, corpusRoot)
		why, waived := notHeldOut[rel]
		switch {
		case !held && !waived:
			t.Errorf("codec accepts %q as a composition-metadata spelling but the comparison does not "+
				"hold it out — it will be reported as both missing and extra; hold it out, or record "+
				"it in notHeldOut with the reason", rel)
		case held && waived:
			t.Errorf("%q is held out but still listed in notHeldOut (%s) — drop the stale entry", rel, why)
		}
		// REQ-053: the same spelling with `:0` on every segment is the same real
		// path, and STRUCTURED → FLAT conversion writes exactly that, so it is
		// held out too. Any other index is refused by the codec, so holding it
		// out would hide that refusal.
		indexed := indexEverySegment(rel, zeroIndex)
		if !IsCompositionMeta(corpusRoot+"/"+indexed, corpusRoot) && !waived {
			t.Errorf("codec reads %q as the accepted spelling %q, but the comparison does not hold it out "+
				"— a STRUCTURED-sourced body reaches decode with it beside the injected ctx/ context", indexed, rel)
		}
		if over := indexEverySegment(rel, ":1"); IsCompositionMeta(corpusRoot+"/"+over, corpusRoot) {
			t.Errorf("the comparison holds out %q, but the codec refuses an index other than :0 on a "+
				"metadata path — the hold-out would absorb that refusal", over)
		}
		// The template root's own segment takes `:0` as well, and nothing else.
		for _, spelling := range []string{rel, indexed} {
			if key := corpusRoot + zeroIndex + "/" + spelling; !IsCompositionMeta(key, corpusRoot) && !waived {
				t.Errorf("codec reads %q as the accepted spelling %q, but the comparison does not hold it out", key, rel)
			}
			if key := corpusRoot + ":1/" + spelling; IsCompositionMeta(key, corpusRoot) {
				t.Errorf("the comparison holds out %q, but the codec refuses an index other than :0 on the "+
					"template root — the hold-out would absorb that refusal", key)
			}
		}
	}

	// The reverse direction, over the real corpus rather than a hand-picked list:
	// whatever the harness holds out when it runs is what has to be justified.
	cases, err := Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}
	heldOut := map[string]int{}
	for _, c := range cases {
		raw, err := os.ReadFile(c.Flat)
		if err != nil {
			t.Fatalf("read %s: %v", c.Name, err)
		}
		body, err := parseFlat(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", c.Name, err)
		}
		for key := range body {
			if !IsCompositionMeta(key, corpusRoot) {
				continue
			}
			if strings.HasPrefix(key, ctxPrefix) {
				// The ctx/ side of the hold-out is unbounded by design — see
				// IsCompositionMeta, with PROBE-076's decode leg as its backstop.
				// One corpus key rides it (`ctx/composer_self`, in party_self:
				// upstream writes the short form there, not the real path), and it
				// needs no per-spelling justification because the ctx/ forms are
				// this codec's own surface.
				continue
			}
			rel, rooted := strings.CutPrefix(key, corpusRoot+"/")
			if !rooted {
				rel, rooted = strings.CutPrefix(key, corpusRoot+zeroIndex+"/")
			}
			if !rooted {
				t.Errorf("held out %q, which is neither ctx/-prefixed nor under the corpus root", key)
				continue
			}
			heldOut[rel]++
		}
	}
	if len(heldOut) == 0 {
		t.Fatal("the corpus produced no hold-outs at all — it writes real-path metadata, so the " +
			"matcher or the root is wrong and the Meta count is silently zero")
	}
	for _, rel := range slices.Sorted(maps.Keys(heldOut)) {
		if accepted[unindexedSpelling(rel)] {
			continue
		}
		t.Errorf("the harness holds out %d corpus keys spelled %q, but the codec accepts no such "+
			"metadata spelling — the hold-out is absorbing a refusal, so the loss stops being "+
			"counted. Either the codec must accept the spelling (simplified.metadataAliases) or "+
			"the key must reach decode and be counted in the census; the waiver class is empty "+
			"since ctx/setting emission landed (REQ-053) and reintroducing one is a spec "+
			"decision (conformance.md § PROBE-086), not a harness edit",
			heldOut[rel], rel)
	}
}

// indexEverySegment writes index after every segment of a root-relative FLAT
// spelling's path, keeping its |suffix: ("context/start_time", ":0") gives
// "context:0/start_time:0", the spelling STRUCTURED → FLAT conversion writes.
func indexEverySegment(rel, index string) string {
	path, suffix, piped := strings.Cut(rel, "|")
	out := strings.ReplaceAll(path, "/", index+"/") + index
	if piped {
		out += "|" + suffix
	}
	return out
}

// TestIsCompositionMeta guards the hold-out's edges. It is the one place the
// harness excuses a difference on *both* sides, so it has to stay narrow:
// `context/other_context` carries archetyped data and must never be swallowed,
// and the composer's other real-path keys, which carry the rest of the
// composer, must not ride in on its `|name` sibling.
func TestIsCompositionMeta(t *testing.T) {
	const root = corpusRoot
	tests := []struct {
		key  string
		want bool
	}{
		{"ctx/language", true},
		{root + "/language|code", true},
		// The terminology witness has no separate ctx/ counterpart at all — one
		// ctx/language stands for the whole CODE_PHRASE — so language and territory
		// hold out every suffix, not just the aliased one.
		{root + "/language|terminology", true},
		{root + "/territory|terminology", true},
		{root + "/composer|name", true},
		{root + "/context/start_time", true},
		{root + "/context/setting|code", true},
		// The composer hold-out is suffix-aware: `|name` is respelled as
		// `ctx/composer_name`, but the composer's external_ref and party
		// sub-structure are no respelling: decode reads them and encode writes
		// them at the same real path (ADR 0029, conformance.md § PROBE-086), so
		// the census compares them. Base-matching `composer` once absorbed the
		// external_ref and hid 12 corpus keys (PR #86 review).
		{root + "/composer|id", false},
		{root + "/composer|id_scheme", false},
		{root + "/composer|id_namespace", false},
		{root + "/composer|type", false},
		{root + "/composer/_identifier:0|id", false},
		{root + "/composer/_identifier:0|assigner", false},
		{root + "/composer/relationship|code", false},
		{root + "/composer:0/relationship:0|value", false},
		// composer_self is an accepted real-path alias (ADR 0015), so it must be
		// held out on both sides like the other respellings — the present corpus
		// writes only the ctx/ form, which is why its absence from the hold-out went
		// unnoticed until the PR #86 review.
		{root + "/composer_self", true},
		{"ctx/composer_self", true},
		// REQ-053: a metadata path whose segments carry `:0` is the same real
		// path, and OPT-free STRUCTURED → FLAT conversion writes it that way, so
		// it is held out exactly as the unindexed spelling is, suffix-aware
		// composer included.
		{root + "/language:0|code", true},
		{root + "/language:0|terminology", true},
		{root + "/territory:0|code", true},
		{root + "/composer:0|name", true},
		{root + "/composer_self:0", true},
		{root + "/context:0/start_time:0", true},
		{root + "/context:0/start_time", true},
		{root + "/context/start_time:0", true},
		{root + "/context:0/setting:0|code", true},
		{root + "/context:0/setting:0|terminology", true},
		{root + "/composer:0|id", false},
		{root + "/context:0/other_context:0/anything", false},
		// Any other index is refused by the codec, so it reaches decode and is
		// counted rather than held out.
		{root + "/language:1|code", false},
		{root + "/composer:1|name", false},
		{root + "/composer_self:1", false},
		{root + "/context:1/start_time:0", false},
		{root + "/context:0/start_time:1", false},
		{root + "/context:0/setting:1|code", false},
		// The template root's own segment takes `:0` too, and nothing else.
		{root + ":0/language|code", true},
		{root + ":0/composer:0|name", true},
		{root + ":0/context:0/start_time:0", true},
		{root + ":0/composer|id", false},
		{root + ":0/context/other_context/anything", false},
		{root + ":1/language|code", false},
		{root + ":1/composer|name", false},
		{root + ":00/language|code", false},
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

// TestRealCodecRefusal drives the *genuine* codec rather than a hand-written
// lookalike. Every other test above feeds the extraction helpers an error this
// file phrased itself (see [suffixErr]), so if flat_decode's wording drifted
// they would all still pass while the census silently lost its attribution —
// the refusal key, its scope, and the reason row are all read out of the
// codec's prose. This test is the seam's pin, for both ways a key leaves the
// comparison (REQ-170): reported as unmatched content, and refused.
func TestRealCodecRefusal(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	base, leaf := modelledQuantityLeaf(t, target)

	t.Run("unmatched suffix is reported and excluded alone", func(t *testing.T) {
		// |normal_range is a DV_QUANTITY attribute with no suffix mapping: it is a
		// DV_INTERVAL, so it can never reduce to a scalar suffix and is outside
		// the datatype's suffix set — unmatched content the codec ignores and
		// reports.
		body := maps.Clone(leaf)
		body[base+"|normal_range"] = json.Number("1")
		_, refusals, err := DecodeReducing(target, body, InjectContext)
		if err != nil {
			t.Fatalf("DecodeReducing: %v", err)
		}
		want := []Refusal{{Key: base + "|normal_range", Reason: UnmatchedReason, Message: UnmatchedMessage, Keys: 1}}
		if !slices.Equal(refusals, want) {
			t.Errorf("DecodeReducing refusals = %+v, want %+v", refusals, want)
		}
		if _, still := body[base+"|normal_range"]; still {
			t.Error("|normal_range survived its own report")
		}
		if _, ok := body[base+"|magnitude"]; !ok {
			t.Error("|magnitude was dropped by a |normal_range report — the leaf, not the entry, went")
		}
	})

	t.Run("refused suffix named by its leaf is removed alone", func(t *testing.T) {
		// A DV_TEXT `|value` without the `|code` that selects the coded subtype
		// is malformed matched content: the codec refuses it naming the leaf's
		// base with the allowlist's "unexpected |x for TYPE" wording, which
		// refusedSuffix parses so that only the one entry goes and the bare value
		// beside it stays.
		text := target.Root + "/conformance_section/conformance_observation/any_event:0/dv_text"
		body := maps.Clone(leaf)
		body[text] = "text value"
		body[text+"|value"] = "stray"
		_, err := decodeSubset(target, body, nil)
		if err == nil {
			t.Fatal("dv_text|value without |code decoded — the codec no longer refuses it, so this test needs another malformed suffix")
		}
		if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
			t.Fatalf("real refusal is not ErrUnsupportedDatatype: %v", err)
		}
		if got := offendingKey(err); got != text {
			t.Errorf("offendingKey(real refusal) = %q, want the leaf base %q\nrefusal: %v", got, text, err)
		}
		if got, ok := refusedSuffix(err); !ok || got != "|value" {
			t.Errorf("refusedSuffix(real refusal) = %q, %v; want %q, true\nrefusal: %v", got, ok, "|value", err)
		}
		if got, want := reasonOf(err, text), "unsupported |suffix for DV_TEXT"; got != want {
			t.Errorf("reasonOf(real refusal) = %q, want %q\nrefusal: %v", got, want, err)
		}
		removed, gap := dropRefused(body, offendingKey(err), err)
		if !gap || removed != 1 {
			t.Fatalf("dropRefused(real refusal) = %d, %v; want 1, true", removed, gap)
		}
		if _, still := body[text+"|value"]; still {
			t.Error("|value survived its own refusal")
		}
		if _, ok := body[text]; !ok {
			t.Error("the bare dv_text was dropped by a |value refusal — the leaf, not the entry, went")
		}
		if _, err := decodeSubset(target, body, nil); err != nil {
			t.Errorf("body decodes only with the refused suffix removed, but still fails: %v", err)
		}
	})

	t.Run("value that does not fit is removed alone", func(t *testing.T) {
		// REQ-053: a |precision with a fraction does not fit the Integer it
		// fills. The codec refuses it naming the leaf's base first and the key
		// itself in the refusal, and the harness removes that one key: the
		// modelled |magnitude and |unit beside it stay in the comparison
		// (PROBE-086).
		body := maps.Clone(leaf)
		body[base+"|precision"] = 1.5
		_, refusals, err := DecodeReducing(target, body, InjectContext)
		if err != nil {
			t.Fatalf("DecodeReducing: %v", err)
		}
		if len(refusals) != 1 || refusals[0].Key != base || refusals[0].Keys != 1 {
			t.Fatalf("DecodeReducing refusals = %+v, want one refusal of the leaf %q removing 1 key", refusals, base)
		}
		if !strings.Contains(refusals[0].Message, `"<key>|precision" does not fit`) {
			t.Errorf("refusal message = %q, want the fit refusal naming the |precision key", refusals[0].Message)
		}
		if !maps.Equal(body, leaf) {
			t.Errorf("DecodeReducing left %v, want exactly the modelled leaf %v", slices.Sorted(maps.Keys(body)), slices.Sorted(maps.Keys(leaf)))
		}
	})

	t.Run("refused key is removed alone", func(t *testing.T) {
		// A suffix of a leaf the ctx/ short form owns that its ctx/ form cannot
		// carry is read and refused (REQ-053): the codec names the key itself,
		// so the harness removes that key and not the accepted `language|code`
		// alias beside it. (The composer's external_ref served here until its
		// real path became its carrier, ADR 0029.)
		body := maps.Clone(leaf)
		key := target.Root + "/language|preferred_term"
		maps.Copy(body, map[string]any{
			"ctx/language": "en", "ctx/territory": "US",
			target.Root + "/language|code": "en",
			key:                            "English",
		})
		_, err := decodeAsGiven(target, body, nil)
		if err == nil {
			t.Fatal("language|preferred_term decoded — the codec no longer refuses it, so this test needs another refusal")
		}
		if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
			t.Fatalf("real refusal is not ErrUnsupportedDatatype: %v", err)
		}
		if got := offendingKey(err); got != key {
			t.Errorf("offendingKey(real refusal) = %q, want the key %q\nrefusal: %v", got, key, err)
		}
		const reason = `unsupported datatype: "<key>" is not a spelling the ctx/ short form can carry — ctx/language owns this leaf, so only its accepted suffixes travel (see deviations.md)`
		if got := reasonOf(err, key); got != reason {
			t.Errorf("reasonOf(real refusal) = %q, want %q\nrefusal: %v", got, reason, err)
		}
		removed, gap := dropRefused(body, offendingKey(err), err)
		if !gap || removed != 1 {
			t.Fatalf("dropRefused(real refusal) = %d, %v; want 1, true", removed, gap)
		}
		if _, ok := body[target.Root+"/language|code"]; !ok {
			t.Error("language|code was dropped by a language|preferred_term refusal — wider than the refusal names")
		}
		// What remains must decode: otherwise the assertions above passed on an
		// error raised for some other reason entirely.
		if _, err := decodeAsGiven(target, body, nil); err != nil {
			t.Errorf("body decodes only with the refused key removed, but still fails: %v", err)
		}
	})

	t.Run("refused bare value at an unmapped leaf is removed alone", func(t *testing.T) {
		// A leaf whose datatype has neither a suffix set nor a sub-path
		// grammar refuses its bare value (REQ-170), naming the leaf's base
		// with the allowlist's "unexpected bare value" wording: the harness
		// removes that one key, not the `|x_note` sibling sharing the base.
		// The corpus template has no such leaf since the composer gained the
		// party grammar (ADR 0029), so a DV_STATE leaf stands in.
		target := &Target{Root: "root", Web: &wt.WebTemplate{Tree: &wt.Node{
			ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
			Children: []*wt.Node{{
				ID: "state", RMType: "DV_STATE", NodeID: "at0002", Max: 1,
				AQLPath: "/context/other_context[at0001]/items[at0002]/value",
			}},
		}}}
		key := "root/state"
		body := map[string]any{
			"ctx/language": "en", "ctx/territory": "US",
			key:             "bare state",
			key + "|x_note": "kept",
		}
		_, err := decodeAsGiven(target, body, nil)
		if err == nil {
			t.Fatal("a bare DV_STATE decoded — the codec no longer refuses it, so this test needs another bare refusal")
		}
		if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
			t.Fatalf("real refusal is not ErrUnsupportedDatatype: %v", err)
		}
		if got := offendingKey(err); got != key {
			t.Errorf("offendingKey(real refusal) = %q, want the leaf base %q\nrefusal: %v", got, key, err)
		}
		if got, want := reasonOf(err, key), "unsupported bare value for DV_STATE"; got != want {
			t.Errorf("reasonOf(real refusal) = %q, want %q\nrefusal: %v", got, want, err)
		}
		removed, gap := dropRefused(body, offendingKey(err), err)
		if !gap || removed != 1 {
			t.Fatalf("dropRefused(real refusal) = %d, %v; want 1, true", removed, gap)
		}
		if _, ok := body[key+"|x_note"]; !ok {
			t.Error("the |x_note sibling was dropped by a bare-value refusal — wider than the refusal names")
		}
		if _, err := decodeAsGiven(target, body, nil); err != nil {
			t.Errorf("body decodes only with the refused key removed, but still fails: %v", err)
		}
	})
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
			// Keep to a plain leaf: a key under an `_`-prefixed family is
			// decoded by that family's grammar, not the DV_QUANTITY leaf's.
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
			if _, err := decodeSubset(target, probe, nil); err == nil {
				return b, probe
			}
		}
	}
	t.Fatal("no modelled DV_QUANTITY leaf in the corpus — this test can no longer " +
		"build a real refusal; pick another datatype")
	return "", nil
}
