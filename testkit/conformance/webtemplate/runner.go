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
	"strconv"
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

// Target is the compiled corpus template plus its Web Template, built once
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
	return NewTargetFromOPT(fixtures.FlatConformanceOpt())
}

// NewTargetFromOPT parses, compiles, and exports the OPT at path, for a
// harness whose bodies instantiate a template other than the FLAT corpus's.
func NewTargetFromOPT(path string) (*Target, error) {
	opt, err := template.ParseFile(path)
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

// Refusal is one upstream key family the decode left out, with the reason:
// a key the codec reported as unmatched content (see simplified.WithUnmatched;
// Reason [UnmatchedReason]), or a key family the codec refused, with the
// reason it gave. Reason is normalised (the key itself is stripped out) so refusals
// aggregate across fixtures.
type Refusal struct {
	// Key is the offending FLAT key as the codec named it.
	Key string
	// Reason is the normalised cause, e.g. "path not in web template" or
	// "unsupported datatype: DV_MULTIMEDIA".
	Reason string
	// Message is the codec's own error text with Key replaced by "<key>", so
	// refusals of one kind read the same whatever key they name.
	Message string
	// Keys is how many concrete FLAT entries this refusal removed (a leaf
	// contributes one per |suffix).
	Keys int
}

// UnmatchedReason is the [Refusal.Reason] of a key the codec reported as
// unmatched content (see simplified.WithUnmatched): the decode ignored it, so
// it cannot come back on re-encode and is excluded from the comparison.
// UnmatchedMessage is its [Refusal.Message].
const (
	UnmatchedReason  = "unmatched content"
	UnmatchedMessage = "reported as unmatched content"
)

// Unmatched reports whether r is a key the codec reported as unmatched
// content rather than a key family it refused.
func (r Refusal) Unmatched() bool { return r.Reason == UnmatchedReason }

// Report is the outcome of round-tripping one fixture.
type Report struct {
	Case string
	// Total is every key in the upstream body.
	Total int
	// Meta is upstream composition-level metadata keys, held out of the
	// comparison on both sides (see [IsCompositionMeta]).
	Meta int
	// Refusals are the key families left out of the comparison: those the
	// codec refused on decode, in the order they surfaced, then the keys it
	// reported as unmatched content, in document order.
	Refusals []Refusal
	// Excluded is how many concrete upstream keys the refusals removed.
	Excluded int
	// Compared is how many upstream keys survived into the comparison.
	Compared int

	// Missing are surviving upstream keys the re-encode did not produce: a
	// leaf this SDK decoded and then dropped. Failure, not skip: there is no
	// tolerated-drop bucket, deliberately (see case.go).
	Missing []string
	// Extra are keys the re-encode produced that upstream does not have,
	// composition metadata aside (see [IsCompositionMeta]): a path this SDK
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
// fixture, a decode that cannot be reduced, a re-encode failure). Codec
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

	comp, refusals, err := DecodeReducing(t, candidate, InjectContext)
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

// ParseFlat decodes a FLAT JSON body with every number kept as its literal
// text ([json.Number]). It refuses a `null` body and any content after the
// body object, so a comparison never checks less of the body than it reports.
func ParseFlat(raw []byte) (map[string]any, error) {
	return parseFlat(raw)
}

// ContextMode says how [DecodeReducing] supplies the composition context that
// FLAT decode requires.
type ContextMode int

const (
	// InjectContext adds a synthetic ctx/language and ctx/territory to every
	// decode, for a body whose composition metadata was held out before the
	// call ([IsCompositionMeta]). PROBE-086 decodes this way.
	InjectContext ContextMode = iota
	// KeepContext decodes the body as given, its own metadata spellings
	// included, and adds nothing. A body without its mandatory context then
	// fails as the codec says.
	KeepContext
)

// IrreducibleError is a decode failure [DecodeReducing] cannot reduce: the
// codec's error names no key, names a key the body does not carry, or is not
// one of the codec's two gap sentinels (simplified.ErrUnknownPath and
// simplified.ErrUnsupportedDatatype).
type IrreducibleError struct {
	// Key is the token the codec's error quoted first, or "" when it quoted
	// none.
	Key string
	// Err is the codec's decode error.
	Err error
	// why is the harness's account of the failure, without Err.
	why string
}

// Error gives the harness's account of the failure, then the codec's error.
func (e *IrreducibleError) Error() string { return e.why + ": " + e.Err.Error() }

// Unwrap returns the codec's error.
func (e *IrreducibleError) Unwrap() error { return e.Err }

// DecodeReducing decodes candidate with the target's template, excluding what
// the codec does not decode: the keys it reports as unmatched content (see
// simplified.WithUnmatched), which the decode ignores, and each key family it
// refuses, removed no wider than the refusal names before the decode is
// retried. It mutates
// candidate down to the modelled subset and returns the refusals in the order
// they surfaced, then one [Refusal] per unmatched key, in document order. mode
// says whether to inject the mandatory context.
//
// The excluded set is whatever the codec itself reports or refuses. Closing a
// gap shrinks it automatically, and there is no hand-kept list to drift from
// the code.
//
// A decode error the loop cannot reduce comes back as an [*IrreducibleError]
// carrying the codec's error. Any other error is a harness fault: an unknown
// mode, a decode that does not converge within the refusal budget, or an
// unmatched key the body does not carry. With either error the refusals made
// before it come back too, and the keys they removed are already gone from
// candidate.
func DecodeReducing(t *Target, candidate map[string]any, mode ContextMode) (*rm.Composition, []Refusal, error) {
	var decode func(*Target, map[string]any, simplified.Option) (*rm.Composition, error)
	switch mode {
	case InjectContext:
		decode = decodeSubset
	case KeepContext:
		decode = decodeAsGiven
	default:
		return nil, nil, fmt.Errorf("unknown context mode %d", mode)
	}
	var refusals []Refusal
	for range maxRefusals {
		var unmatched []string
		comp, err := decode(t, candidate, simplified.WithUnmatched(func(key string) { unmatched = append(unmatched, key) }))
		if err == nil {
			excluded, err := excludeUnmatched(candidate, unmatched)
			return comp, append(refusals, excluded...), err
		}
		key := offendingKey(err)
		if key == "" {
			return nil, refusals, &IrreducibleError{Err: err, why: "decode failed with no attributable key"}
		}
		removed, gap := dropRefused(candidate, key, err)
		if !gap {
			return nil, refusals, &IrreducibleError{Key: key, Err: err, why: fmt.Sprintf(
				"decode error on key %q is neither an unmodelled path nor an "+
					"unmodelled datatype, so it is a harness fault rather than a codec gap — "+
					"do not let it be counted as excluded surface", key)}
		}
		if removed == 0 {
			return nil, refusals, &IrreducibleError{Key: key, Err: err, why: fmt.Sprintf(
				"decode named key %q but it is not in the body", key)}
		}
		refusals = append(refusals, Refusal{
			Key:     key,
			Reason:  reasonOf(err, key),
			Message: strings.ReplaceAll(err.Error(), key, "<key>"),
			Keys:    removed,
		})
	}
	return nil, refusals, fmt.Errorf("decode did not converge after %d refusals", maxRefusals)
}

// excludeUnmatched removes from candidate every key the codec reported as
// unmatched content and returns one [Refusal] per key, in the order reported.
// The codec reports a FLAT key as written, so a reported key the body does not
// carry is a harness fault, not a key to skip.
func excludeUnmatched(candidate map[string]any, unmatched []string) ([]Refusal, error) {
	out := make([]Refusal, 0, len(unmatched))
	for _, key := range unmatched {
		if _, ok := candidate[key]; !ok {
			return out, fmt.Errorf("the codec reported %q as unmatched content, but the body does not carry it", key)
		}
		delete(candidate, key)
		out = append(out, Refusal{Key: key, Reason: UnmatchedReason, Message: UnmatchedMessage, Keys: 1})
	}
	return out, nil
}

// decodeAsGiven decodes the given keys with the template attached and nothing
// added, for [KeepContext].
func decodeAsGiven(t *Target, keys map[string]any, opt simplified.Option) (*rm.Composition, error) {
	b, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	return simplified.UnmarshalFlat(b, t.Web, simplified.WithTemplate(t.Compiled), opt)
}

// decodeSubset decodes the given keys with the template attached.
//
// ctx/language and ctx/territory are injected because decode requires them and
// the real-path spellings upstream uses (`<root>/language|code`) are held out of
// the candidate set before it gets here — see [IsCompositionMeta]. (Decode does
// accept those spellings since ADR 0015; the hold-out, not a refusal, is why
// they are absent.) They are synthetic scaffolding for the decode, never
// compared.
func decodeSubset(t *Target, keys map[string]any, opt simplified.Option) (*rm.Composition, error) {
	body := make(map[string]any, len(keys)+2)
	maps.Copy(body, keys)
	body["ctx/language"] = "en"
	body["ctx/territory"] = "US"
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return simplified.UnmarshalFlat(b, t.Web, simplified.WithTemplate(t.Compiled), opt)
}

// dropRefused removes the entries one refusal actually covers and reports how
// many went, plus whether the error was a modelled-gap refusal at all. What a
// refusal covers has to be read off the error: the codec names a whole key, a
// leaf's *base* path (flat_decode groups a leaf's suffixes before decoding it)
// or a family prefix.
//
// Only the two REQ-053 gap sentinels are refusals. Any other keyed error —
// a bug in the codec, an RM invariant a fixture's value breaks, a budget
// breach — returns gap = false so [DecodeReducing] returns an error, because
// reclassifying an unrecognised error as unmodelled surface would let a
// genuine regression present as a bigger Excluded count instead of a red
// test. A value that does not fit its RM attribute is not such an error: the
// codec refuses it with ErrUnsupportedDatatype (REQ-053), so it is a refusal
// like the others.
//
// Precision is the whole game here: every key dropped wider than the refusal
// names is a key the codec would have round-tripped, silently missing from
// the compared subset this probe exists to grow (PROBE-086: each refusal
// MUST remove only what it names). Since REQ-170 the codec reports what it
// does not read instead of refusing it, so a refusal is always of something
// it reads. Five shapes, five scopes:
//
//   - A value that does not fit its RM attribute (REQ-053), refused naming
//     its own key after the leaf's base (`"<base>|precision" does not fit
//     …`, or `"<base>" does not fit …` for the bare value): that one key
//     goes, and the leaf's other suffixes stay. `x|` spells the bare value
//     of `x` and the codec names it `x`, so the spelling as written goes.
//   - A whole key, suffix and all (`<root>/composer|id`, the composer's
//     `external_ref`): that one entry goes. `composer|name` beside it is an
//     accepted alias and stays.
//   - A suffix of a leaf named by its base ("unexpected |value for DV_TEXT",
//     a DV_TEXT `|value` without the `|code` that selects the coded subtype),
//     or its bare value ("unexpected bare value for PARTY_PROXY", the
//     composer's bare value): only that one entry goes, and the rest of the
//     leaf stays.
//   - Any other ErrUnsupportedDatatype refusal naming a leaf's base (a
//     missing required suffix): the leaf's own entries go, but *not* the
//     subtree beneath it, which belongs to other nodes or families.
//   - Any other ErrUnknownPath refusal naming a path or a family prefix (an
//     index past its bound, a deferred family): the entries under it go,
//     leaf and subtree, since everything under that segment is refused with
//     it.
func dropRefused(candidate map[string]any, key string, err error) (removed int, gap bool) {
	unknownPath, unsupported := errors.Is(err, simplified.ErrUnknownPath), errors.Is(err, simplified.ErrUnsupportedDatatype)
	if !unknownPath && !unsupported {
		return 0, false
	}
	if named, ok := misfitKey(err); ok {
		return dropMatching(candidate, func(k string) bool { return k == named || k == named+"|" }), true
	}
	if strings.Contains(key, "|") {
		return dropMatching(candidate, func(k string) bool { return k == key }), true
	}
	if sfx, ok := refusedSuffix(err); ok {
		return dropMatching(candidate, func(k string) bool { return k == key+sfx }), true
	}
	if unknownPath {
		return dropMatching(candidate, func(k string) bool {
			// key+"/" and not key alone: the boundary keeps a string-prefix
			// sibling (`…/_uid_based_id` beside `…/_uid`) out of the subtree.
			return baseOf(k) == key || strings.HasPrefix(k, key+"/")
		}), true
	}
	return dropMatching(candidate, func(k string) bool { return baseOf(k) == key }), true
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

// misfitPhrase is what follows the quoted key in the refusal of a value that
// does not fit its RM attribute (REQ-053).
const misfitPhrase = `" does not fit `

// misfitKey reads the key out of the refusal of a value that does not fit its
// RM attribute, the one key whose value it is: the quoted token that
// [misfitPhrase] follows, which the codec writes after `: `. ok is false for
// any other refusal shape.
func misfitKey(err error) (string, bool) {
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		return "", false
	}
	msg := err.Error()
	end := strings.Index(msg, misfitPhrase)
	if end < 0 {
		return "", false
	}
	start := strings.LastIndex(msg[:end], `: "`)
	if start < 0 {
		return "", false
	}
	key, uerr := strconv.Unquote(msg[start+len(": ") : end+1])
	if uerr != nil {
		return "", false
	}
	return key, true
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
// double-quoted token in the message. The refusal shapes the census is built
// on put the key there — `path not in web template: "K"` and `decode "K": …`.
//
// This is a phrasing seam, not a contract the codec ever gave. Decode sites
// exist that quote something else first — flat_decode's reused-sibling refusal
// quotes the failing *segment* id, and parseFlatKey's invalid-`:index` refusal
// quotes the bad index substring — so the token this returns is not always a
// key. What protects the harness is not the quoting order but the fail-loud
// guard in [DecodeReducing]: a token that matches nothing in the body makes
// DecodeReducing return an error instead of dropping something arbitrary.
// PROBE-105 records that error as the leg's refusal rather than failing the
// process. An unrecognised error class is rejected by [dropRefused] before
// any drop happens. The residual
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
		// the codec names one per refusal, the first in sorted order.
		// Collapse to the datatype so the refusals of one datatype share one
		// reason whichever suffix they name — the suffixes are dropped and
		// refused one at a time (see [dropRefused]).
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
