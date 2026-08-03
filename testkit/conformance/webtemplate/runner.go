package webtemplate

// PROBE-086 — the round-trip engine: decode an upstream-authored FLAT body
// through the REQ-053 codec, re-encode it, and compare. See the package doc
// in case.go for what is asserted and SKIPPED.md for what is not.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	wt "github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// maxRefusals bounds the decode/drop/retry loop. The corpus's worst fixture
// needs well under this; the cap only stops a pathological codec change from
// spinning (each pass must remove at least one key, so it also terminates on
// its own — this is belt and braces, and a breach is reported, not ignored).
const maxRefusals = 400

// Target is the compiled corpus template plus its Web Template — built once
// and shared by every case, since the whole corpus instantiates one OPT.
type Target struct {
	Compiled *templatecompile.Compiled
	Web      *wt.WebTemplate
	// Root is the Web Template tree id, which is also the first segment of
	// every upstream FLAT key.
	Root string
}

// NewTarget parses, compiles, and exports the corpus OPT.
func NewTarget() (*Target, error) {
	opt, err := template.ParseFile(fixtures.FlatConformanceOpt())
	if err != nil {
		return nil, fmt.Errorf("parse corpus OPT: %w", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, fmt.Errorf("compile corpus OPT: %w", err)
	}
	w, err := wt.Build(c)
	if err != nil {
		// Until REQ-116 landed the name-derived id + ordinal fallback, this
		// was ErrIDCollision and the whole probe was blocked on it.
		return nil, fmt.Errorf("build corpus Web Template: %w", err)
	}
	return &Target{Compiled: c, Web: w, Root: w.Tree.ID}, nil
}

// Refusal is one upstream key family the codec declined, with the reason it
// gave. Reason is normalised (the key itself is stripped out) so refusals
// aggregate across fixtures.
type Refusal struct {
	// Key is the offending FLAT key as the codec named it.
	Key string
	// Reason is the normalised cause, e.g. "path not in web template" or
	// "unsupported datatype: DV_MULTIMEDIA".
	Reason string
	// Keys is how many concrete FLAT entries this refusal removed (a leaf
	// contributes one per |suffix).
	Keys int
}

// Report is the outcome of round-tripping one fixture.
type Report struct {
	Case string
	// Total is every key in the upstream body.
	Total int
	// Meta is upstream composition-level metadata keys, held out of the
	// comparison on both sides (see [IsCompositionMeta]).
	Meta int
	// Refusals are the unmodelled key families, discovered from the codec's
	// own decode errors, in the order they surfaced.
	Refusals []Refusal
	// Excluded is how many concrete upstream keys the refusals removed.
	Excluded int
	// Compared is how many upstream keys survived into the comparison.
	Compared int

	// Missing are surviving upstream keys the re-encode did not produce — a
	// leaf this SDK decoded and then dropped. Failure, not skip: there is no
	// tolerated-drop bucket, deliberately (see case.go).
	Missing []string
	// Extra are keys the re-encode produced that upstream does not have,
	// composition metadata aside (see [IsCompositionMeta]) — a path this SDK
	// spells differently. Failure, not skip.
	Extra []string
	// Mismatched are keys present on both sides with differing values,
	// formatted "key: upstream=… ours=…".
	Mismatched []string
}

// Clean reports whether the modelled subset round-tripped exactly.
func (r Report) Clean() bool {
	return len(r.Missing) == 0 && len(r.Extra) == 0 && len(r.Mismatched) == 0
}

// Run round-trips one case against the target and returns its report. It
// returns an error only for a harness fault (unreadable or malformed
// fixture, a decode that cannot be reduced, a re-encode failure) — codec
// gaps are data in the Report, not errors.
func Run(t *Target, c Case) (Report, error) {
	if t == nil {
		return Report{}, fmt.Errorf("%s: nil target", c.Name)
	}
	raw, err := os.ReadFile(c.Flat)
	if err != nil {
		return Report{}, fmt.Errorf("%s: %w", c.Name, err)
	}
	upstream, err := parseFlat(raw)
	if err != nil {
		return Report{}, fmt.Errorf("%s: upstream FLAT is not valid JSON: %w", c.Name, err)
	}

	rep := Report{Case: c.Name, Total: len(upstream)}

	// Candidate set: archetyped content only. Composition-level metadata is
	// held out — see [IsCompositionMeta] — because upstream and this codec
	// spell it differently, so comparing it would report noise on both sides.
	candidate := make(map[string]any, len(upstream))
	for k, v := range upstream {
		if IsCompositionMeta(k, t.Root) {
			rep.Meta++
			continue
		}
		candidate[k] = v
	}

	comp, refusals, err := decodeReducing(t, candidate)
	if err != nil {
		return Report{}, fmt.Errorf("%s: %w", c.Name, err)
	}
	rep.Refusals = refusals
	for _, r := range refusals {
		rep.Excluded += r.Keys
	}
	rep.Compared = len(candidate)

	got, err := simplified.MarshalFlat(comp, t.Web)
	if err != nil {
		return Report{}, fmt.Errorf("%s: re-encode: %w", c.Name, err)
	}
	emitted, err := parseFlat(got)
	if err != nil {
		return Report{}, fmt.Errorf("%s: re-encoded FLAT is not valid JSON: %w", c.Name, err)
	}

	rep.Missing, rep.Extra, rep.Mismatched = compare(candidate, emitted, t.Root)
	return rep, nil
}

// compare matches the surviving upstream subset against what the re-encode
// produced, in both directions, and returns the three verdict lists sorted for
// stable output.
//
// Both directions matter and they catch different faults: candidate→emitted
// finds a key this SDK decoded and then dropped (Missing) or changed
// (Mismatched); emitted→candidate finds a key this SDK writes that upstream
// does not have at all (Extra) — the same leaf under a different spelling,
// which a one-directional check would read as a clean round-trip minus one
// missing key. Composition metadata is skipped on the emitted side only,
// because the candidate set never contained any (see [IsCompositionMeta] for
// what that hold-out does and does not cover).
//
// Extracted from [Run] so these three classifications are unit-testable: the
// corpus is expected to produce none of them, so no corpus fixture exercises
// this loop at all.
func compare(candidate, emitted map[string]any, root string) (missing, extra, mismatched []string) {
	for k, want := range candidate {
		have, ok := emitted[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		if !sameValue(want, have) {
			mismatched = append(mismatched, fmt.Sprintf("%s: upstream=%v ours=%v", k, want, have))
		}
	}
	for k := range emitted {
		if IsCompositionMeta(k, root) {
			continue
		}
		if _, ok := candidate[k]; !ok {
			extra = append(extra, k)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	slices.Sort(mismatched)
	return missing, extra, mismatched
}

// decodeReducing decodes candidate, removing each key family the codec
// refuses until what remains decodes. It mutates candidate down to the
// modelled subset and returns the refusals in the order they surfaced.
//
// This is what keeps the skip inventory honest: the excluded set is whatever
// the codec itself declines, so closing a gap shrinks it automatically and no
// hand-kept list can drift from the code.
func decodeReducing(t *Target, candidate map[string]any) (*rm.Composition, []Refusal, error) {
	var refusals []Refusal
	for range maxRefusals {
		comp, err := decodeSubset(t, candidate)
		if err == nil {
			return comp, refusals, nil
		}
		key := offendingKey(err)
		if key == "" {
			return nil, nil, fmt.Errorf("decode failed with no attributable key: %w", err)
		}
		removed, gap := dropRefused(candidate, key, err)
		if !gap {
			return nil, nil, fmt.Errorf("decode error on key %q is neither an unmodelled path nor an "+
				"unmodelled datatype, so it is a harness fault rather than a codec gap — "+
				"do not let it be counted as excluded surface: %w", key, err)
		}
		if removed == 0 {
			return nil, nil, fmt.Errorf("decode named key %q but it is not in the body: %w", key, err)
		}
		refusals = append(refusals, Refusal{Key: key, Reason: reasonOf(err, key), Keys: removed})
	}
	return nil, nil, fmt.Errorf("decode did not converge after %d refusals", maxRefusals)
}

// decodeSubset decodes the given keys with the template attached.
//
// ctx/language and ctx/territory are injected because decode requires them
// and upstream carries the same information only as real paths
// (`<root>/language|code`), which the codec refuses — see [ctxPrefix]. They
// are synthetic scaffolding for the decode, never compared.
func decodeSubset(t *Target, keys map[string]any) (*rm.Composition, error) {
	body := make(map[string]any, len(keys)+2)
	maps.Copy(body, keys)
	body["ctx/language"] = "en"
	body["ctx/territory"] = "US"
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return simplified.UnmarshalFlat(b, t.Web, simplified.WithTemplate(t.Compiled))
}

// dropRefused removes the entries one refusal actually covers and reports how
// many went, plus whether the error was a modelled-gap refusal at all. The
// codec names a *base* path (flat_decode groups a leaf's suffixes before
// decoding it), so how much of the body that base stands for has to be read
// off the error.
//
// Only the two REQ-053 gap sentinels are refusals. Any other keyed error —
// a bug in the codec, a malformed fixture, a budget breach — returns
// gap = false so [decodeReducing] fails on it, because reclassifying an
// unrecognised error as unmodelled surface would let a genuine regression
// present as a bigger Excluded count instead of a red test.
//
// Precision is the whole game here: every key dropped wider than the refusal
// is a key the codec would have round-tripped, silently missing from the
// compared subset this probe exists to grow. Three refusal shapes, three
// scopes:
//
//   - A suffix the datatype does not map ("unexpected |precision for
//     DV_QUANTITY") — only that one entry goes. |magnitude and |unit on the
//     same leaf are modelled and stay in the comparison; the next pass names
//     the next unmodelled suffix, so the leaf is pared down rather than
//     discarded.
//   - A leaf whose datatype is not modelled at all, including a container
//     addressed as a value leaf ("unsupported datatype: EVENT" for
//     `any_event:0|sample_count`) — the leaf's own entries go, but *not* the
//     subtree beneath it. An EVENT this codec cannot read as a value still
//     has children it reads fine.
//   - A path that does not resolve at all (ErrUnknownPath) — leaf and subtree
//     both go, since no longer path through an unresolvable node can resolve
//     either. This is the only shape where dropping the subtree is sound, and
//     it is also the bulk of the corpus's exclusions.
func dropRefused(candidate map[string]any, key string, err error) (removed int, gap bool) {
	base := baseOf(key)
	if sfx, ok := refusedSuffix(err); ok {
		return dropMatching(candidate, func(k string) bool { return k == base+sfx }), true
	}
	if errors.Is(err, simplified.ErrUnknownPath) {
		return dropMatching(candidate, func(k string) bool {
			// base+"/" and not base alone: the boundary keeps a string-prefix
			// sibling (`…/_uid_based_id` beside `…/_uid`) out of the subtree.
			return baseOf(k) == base || strings.HasPrefix(k, base+"/")
		}), true
	}
	if errors.Is(err, simplified.ErrUnsupportedDatatype) {
		return dropMatching(candidate, func(k string) bool { return baseOf(k) == base }), true
	}
	return 0, false
}

// dropMatching deletes every entry the predicate selects.
func dropMatching(candidate map[string]any, match func(string) bool) int {
	var removed int
	for k := range candidate {
		if match(k) {
			delete(candidate, k)
			removed++
		}
	}
	return removed
}

// refusedSuffix reads the single offending suffix out of a suffix-allowlist
// refusal, as the FLAT spelling appended to the base ("|precision", or "" for
// the bare-value form). ok is false for any other refusal shape.
func refusedSuffix(err error) (string, bool) {
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		return "", false
	}
	_, rest, ok := strings.Cut(err.Error(), "unexpected ")
	if !ok {
		return "", false
	}
	label, _, ok := strings.Cut(rest, " for ")
	if !ok {
		return "", false
	}
	if label == "bare value" {
		return "", true
	}
	if !strings.HasPrefix(label, "|") {
		return "", false
	}
	return label, true
}

// offendingKey extracts the FLAT key a decode error names: the *first*
// double-quoted token in the message. The two refusal shapes the census is
// built on put the key there — `path not in web template: "K"` and
// `decode "K": …`.
//
// This is a phrasing seam, not a contract the codec ever gave. Decode sites
// exist that quote something else first — flat_decode's reused-sibling refusal
// quotes the failing *segment* id, and parseFlatKey's invalid-`:index` refusal
// quotes the bad index substring — so the token this returns is not always a
// key. What protects the harness is not the quoting order but the fail-loud
// guard in [decodeReducing]: a token that matches nothing in the body aborts
// the run instead of dropping something arbitrary, and an unrecognised error
// class is rejected by [dropRefused] before any drop happens. The residual
// risk is narrow and real: a first token that coincidentally equals some
// *other* key present in the body would drop that key undetected. Closing it
// properly needs a typed key on the codec's errors, not a better regexp.
//
// TestRealCodecRefusal pins this seam against silent drift by parsing a
// genuine codec refusal rather than a hand-written lookalike.
func offendingKey(err error) string {
	msg := err.Error()
	_, rest, ok := strings.Cut(msg, `"`)
	if !ok {
		return ""
	}
	key, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return ""
	}
	return key
}

// reasonOf normalises a decode error into a key-independent cause, so the
// same gap aggregates across fixtures.
func reasonOf(err error, key string) string {
	msg := strings.ReplaceAll(err.Error(), key, "<key>")
	switch {
	case errors.Is(err, simplified.ErrUnknownPath):
		return "path not in web template"
	case errors.Is(err, simplified.ErrUnsupportedDatatype):
		_, detail, ok := strings.Cut(msg, "unsupported datatype: ")
		if !ok {
			return "unsupported datatype"
		}
		// A leaf may carry several suffixes the codec does not model, and
		// which one it names first depends on map iteration order. Collapse
		// to the datatype so the census is deterministic — the suffixes are
		// dropped and refused one at a time (see [dropRefused]), so the
		// aggregate count is stable even though the order is not.
		if _, dt, cut := strings.Cut(detail, " for "); cut {
			switch {
			case strings.HasPrefix(detail, "unexpected |"):
				return "unsupported |suffix for " + dt
			case strings.HasPrefix(detail, "unexpected bare value"):
				return "unsupported bare value for " + dt
			}
		}
		return "unsupported datatype: " + detail
	}
	// Strip the decode prefix so the residue reads as a cause.
	if _, detail, ok := strings.Cut(msg, `decode "<key>": `); ok {
		return detail
	}
	return msg
}

// sameValue compares two FLAT leaf values by their canonical JSON bytes.
// Both sides come from [parseFlat], so scalars are string / json.Number /
// bool and the byte comparison is exact — json.Number keeps 1 vs 1.0 and
// integers beyond 2^53 apart. Composites (a |raw fragment, should the codec
// admit one) compare structurally; no interface == here, which would panic
// on identical uncomparable dynamic types before any fallback ran.
func sameValue(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

// parseFlat decodes a FLAT JSON body with numbers preserved verbatim as
// [json.Number]. Decoding into float64 would round both sides of the
// comparison identically — a >2^53 precision regression or a 1-vs-1.0
// respelling in the codec would pass unseen, exactly the masking
// PROBE-076's equality was fixed for after its first review.
//
// Two shapes json.Decoder accepts are rejected here, because both would make
// the harness compare *less* than it reports comparing:
//
//   - a JSON `null` body unmarshals into a nil map, not an error, and a nil
//     map round-trips through the whole pipeline as zero keys, zero refusals,
//     "clean" — an empty fixture asserting nothing;
//   - content after the first JSON value is ignored by a single Decode, so a
//     truncated-then-restarted or concatenated body would silently be read as
//     whichever half came first.
func parseFlat(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("body is JSON null, not an object")
	}
	// A second Decode rather than dec.More(): More() reports false for a
	// trailing `]` or `}`, which is malformed rather than absent.
	switch err := dec.Decode(new(json.RawMessage)); {
	case errors.Is(err, io.EOF):
		return m, nil
	case err == nil:
		return nil, errors.New("trailing JSON value after the body object")
	default:
		return nil, fmt.Errorf("trailing content after the body object: %w", err)
	}
}
