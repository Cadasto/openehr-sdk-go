// Package crossformat runs the PROBE-105 upstream cross-format parity
// harness. Each set of the vendored cross-format corpus gives one composition
// in two or more of canonical JSON, canonical XML, FLAT and STRUCTURED. For
// every pair of formats a set carries, the harness carries the composition
// from one format to the other through the SDK codecs and compares the result
// with the upstream sibling.
//
// # Legs
//
// A set runs every leg whose two formats it has:
//
//   - [LegJSONXML]: both canonical documents decode, and their canonical JSON
//     re-encodes have equal leaf sets. The JSON side is the reference.
//   - [LegCanonicalFlat]: the decoded canonical document, encoded as FLAT, is
//     compared key by key with the upstream FLAT, the reference.
//   - [LegFlatCanonical]: the upstream FLAT, decoded with its composition
//     metadata kept, is compared leaf by leaf in canonical JSON with the
//     decoded canonical document, the reference.
//   - [LegFlatStructured]: the upstream FLAT, restructured without a template,
//     is compared leaf by leaf with the upstream STRUCTURED, the reference.
//   - [LegStructuredFlat]: the upstream STRUCTURED, flattened, decoded and
//     encoded as FLAT, is compared key by key with the upstream FLAT taken
//     through the same decode and encode, the reference.
//
// Every FLAT comparison holds composition metadata out on both sides with the
// hold-out of the upstream FLAT parity harness (webtemplate.IsCompositionMeta).
// Every FLAT decode is that harness's reducing decode
// (webtemplate.DecodeReducing): a key family the codec refuses is removed, no
// wider than the refusal names, and the decode retried.
//
// # Outcomes and the ratchet
//
// A leg's [Outcome] is either a refusal, the codec error that ended it, or
// the counts compared, missing, extra and altered. [Recorded] holds the
// outcome expected for every set and leg, with the reason for every refusal
// and difference. [Verify] compares a measured result with its record, and a
// change in either direction fails until the record changes with it.
// CENSUS.md beside this file publishes the outcomes and reasons; the package
// tests regenerate it and fail when the committed copy differs.
package crossformat

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
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Leg names one format-to-format comparison.
type Leg string

// The five legs, in the order a set runs them.
const (
	// LegJSONXML compares the canonical JSON and canonical XML documents.
	LegJSONXML Leg = "json-xml"
	// LegCanonicalFlat encodes the canonical document as FLAT.
	LegCanonicalFlat Leg = "canonical-flat"
	// LegFlatCanonical decodes the FLAT document into canonical JSON.
	LegFlatCanonical Leg = "flat-canonical"
	// LegFlatStructured restructures the FLAT document as STRUCTURED.
	LegFlatStructured Leg = "flat-structured"
	// LegStructuredFlat flattens the STRUCTURED document and re-encodes it as
	// FLAT.
	LegStructuredFlat Leg = "structured-flat"
)

// Legs returns the legs set can run, in run order: each needs both of its
// formats, and a canonical leg takes canonical JSON or, without it,
// canonical XML.
func Legs(set fixtures.CrossFormatSet) []Leg {
	canonical := set.CanonicalJSON != "" || set.CanonicalXML != ""
	var legs []Leg
	if set.CanonicalJSON != "" && set.CanonicalXML != "" {
		legs = append(legs, LegJSONXML)
	}
	if canonical && set.FLAT != "" {
		legs = append(legs, LegCanonicalFlat, LegFlatCanonical)
	}
	if set.FLAT != "" && set.STRUCTURED != "" {
		legs = append(legs, LegFlatStructured, LegStructuredFlat)
	}
	return legs
}

// Outcome is what one leg measured: either a refusal or the counts of a
// comparison. Compared is the size of the reference side; Missing counts its
// keys or leaves absent from the side under test, Extra the reverse, and
// Altered those present on both with different values.
type Outcome struct {
	// Refused is the codec error that ended the leg, or "" when the leg ran
	// to a comparison. In a [Record] it is a stable substring of that error.
	Refused string
	// Compared is how many keys or leaves the reference side has.
	Compared int
	// Missing is how many reference keys or leaves the side under test
	// lacks.
	Missing int
	// Extra is how many keys or leaves only the side under test has.
	Extra int
	// Altered is how many keys or leaves both sides have with different
	// values.
	Altered int
}

// Clean reports whether the leg ran to a comparison and found no difference.
func (o Outcome) Clean() bool {
	return o.Refused == "" && o.Missing == 0 && o.Extra == 0 && o.Altered == 0
}

// String renders the outcome on one line.
func (o Outcome) String() string {
	if o.Refused != "" {
		return "refused: " + o.Refused
	}
	return fmt.Sprintf("compared %d, missing %d, extra %d, altered %d", o.Compared, o.Missing, o.Extra, o.Altered)
}

// LegResult is one leg's measured outcome and the detail behind it.
type LegResult struct {
	Leg     Leg
	Outcome Outcome
	// MissingKeys and ExtraKeys list the keys or leaves behind the Missing
	// and Extra counts, sorted.
	MissingKeys, ExtraKeys []string
	// Alterations list the keys or leaves behind the Altered count, sorted by
	// key.
	Alterations []Alteration
	// Refusals are the key families the leg's reducing FLAT decodes removed,
	// in the order they surfaced. They are information, not part of the
	// outcome: a removed key shows up in the comparison as missing.
	Refusals []webtemplate.Refusal
}

// Alteration is one key or leaf both sides carry with different values, each
// value as its JSON text.
type Alteration struct {
	Key, Reference, Ours string
}

// Excluded is how many keys the leg's reducing decodes removed.
func (r LegResult) Excluded() int {
	n := 0
	for _, f := range r.Refusals {
		n += f.Keys
	}
	return n
}

// SetResult is every leg's result for one set, in run order.
type SetResult struct {
	Set  string
	Legs []LegResult
}

// Run builds the set's Web Template from its OPT and runs every leg the set
// carries. It returns an error only for a harness fault: a set with no name or
// no leg, an OPT that is missing or does not compile, an unreadable file, an
// upstream FLAT or STRUCTURED document that is not JSON, or a reducing decode
// that does not converge. A codec error is a leg's refusal, recorded in its
// outcome.
func Run(set fixtures.CrossFormatSet) (SetResult, error) {
	legs := Legs(set)
	switch {
	case set.Name == "":
		return SetResult{}, errors.New("cross-format set has no name")
	case len(legs) == 0:
		return SetResult{}, fmt.Errorf("set %s: carries fewer than two formats that share a leg", set.Name)
	}
	target, err := webtemplate.NewTargetFromOPT(set.OPT)
	if err != nil {
		return SetResult{}, fmt.Errorf("set %s: %w", set.Name, err)
	}
	in, err := readInputs(set)
	if err != nil {
		return SetResult{}, fmt.Errorf("set %s: %w", set.Name, err)
	}
	res := SetResult{Set: set.Name}
	for _, leg := range legs {
		lr, err := runLeg(leg, target, in)
		if err != nil {
			return SetResult{}, fmt.Errorf("set %s, leg %s: %w", set.Name, leg, err)
		}
		lr.Leg = leg
		res.Legs = append(res.Legs, lr)
	}
	return res, nil
}

// inputs are a set's documents, read once. A format the set does not carry is
// nil.
type inputs struct {
	canonicalJSON, canonicalXML []byte
	flatRaw, structured         []byte
	// flat is flatRaw parsed with numbers kept literal.
	flat map[string]any
}

func readInputs(set fixtures.CrossFormatSet) (inputs, error) {
	var in inputs
	for _, f := range []struct {
		path string
		dst  *[]byte
	}{
		{set.CanonicalJSON, &in.canonicalJSON},
		{set.CanonicalXML, &in.canonicalXML},
		{set.FLAT, &in.flatRaw},
		{set.STRUCTURED, &in.structured},
	} {
		if f.path == "" {
			continue
		}
		b, err := os.ReadFile(f.path)
		if err != nil {
			return inputs{}, err
		}
		*f.dst = b
	}
	if in.flatRaw != nil {
		m, err := webtemplate.ParseFlat(in.flatRaw)
		if err != nil {
			return inputs{}, fmt.Errorf("upstream FLAT is not a FLAT body: %w", err)
		}
		in.flat = m
	}
	if in.structured != nil {
		if _, err := leaves(in.structured); err != nil {
			return inputs{}, fmt.Errorf("upstream STRUCTURED is not JSON: %w", err)
		}
	}
	return in, nil
}

// errRefused marks a codec error that ends a leg as its refusal.
type errRefused struct{ msg string }

func (e errRefused) Error() string { return e.msg }

// refuse wraps a codec error as the leg's refusal, naming the step.
func refuse(step string, err error) error {
	return errRefused{msg: step + ": " + err.Error()}
}

// runLeg runs one leg. A codec error becomes the leg's refusal; any other
// error is a harness fault.
func runLeg(leg Leg, t *webtemplate.Target, in inputs) (LegResult, error) {
	var (
		lr  LegResult
		err error
	)
	switch leg {
	case LegJSONXML:
		lr, err = legJSONXML(in)
	case LegCanonicalFlat:
		lr, err = legCanonicalFlat(t, in)
	case LegFlatCanonical:
		lr, err = legFlatCanonical(t, in)
	case LegFlatStructured:
		lr, err = legFlatStructured(in)
	case LegStructuredFlat:
		lr, err = legStructuredFlat(t, in)
	default:
		return LegResult{}, fmt.Errorf("unknown leg %q", leg)
	}
	if r, ok := errors.AsType[errRefused](err); ok {
		return LegResult{Outcome: Outcome{Refused: r.msg}, Refusals: lr.Refusals}, nil
	}
	return lr, err
}

// legJSONXML is leg (a): both canonical documents decode, and their canjson
// re-encodes are compared leaf by leaf, the JSON side as the reference.
func legJSONXML(in inputs) (LegResult, error) {
	fromJSON, err := canonicalJSONLeaves(in.canonicalJSON)
	if err != nil {
		return LegResult{}, err
	}
	comp, err := decodeXML(in.canonicalXML)
	if err != nil {
		return LegResult{}, err
	}
	fromXML, err := encodeLeaves(comp)
	if err != nil {
		return LegResult{}, err
	}
	return compareLeaves(fromJSON, fromXML), nil
}

// legCanonicalFlat is leg (b): the decoded canonical document, encoded as
// FLAT, against the upstream FLAT, metadata held out of both.
func legCanonicalFlat(t *webtemplate.Target, in inputs) (LegResult, error) {
	comp, err := decodeCanonical(in)
	if err != nil {
		return LegResult{}, err
	}
	out, err := simplified.MarshalFlat(comp, t.Web)
	if err != nil {
		return LegResult{}, refuse("FLAT encode", err)
	}
	ours, err := webtemplate.ParseFlat(out)
	if err != nil {
		return LegResult{}, fmt.Errorf("FLAT encode produced no FLAT body: %w", err)
	}
	return compareFlat(in.flat, ours, t.Root)
}

// legFlatCanonical is leg (c): the upstream FLAT, decoded with its metadata
// kept, against the decoded canonical document, both in canonical JSON.
func legFlatCanonical(t *webtemplate.Target, in inputs) (LegResult, error) {
	comp, err := decodeCanonical(in)
	if err != nil {
		return LegResult{}, err
	}
	ref, err := encodeLeaves(comp)
	if err != nil {
		return LegResult{}, err
	}
	decoded, refusals, err := decodeFlat(t, maps.Clone(in.flat), webtemplate.KeepContext)
	if err != nil {
		return LegResult{Refusals: refusals}, err
	}
	ours, err := encodeLeaves(decoded)
	if err != nil {
		return LegResult{Refusals: refusals}, err
	}
	lr := compareLeaves(ref, ours)
	lr.Refusals = refusals
	return lr, nil
}

// legFlatStructured is leg (d): the upstream FLAT, restructured without a
// template, against the upstream STRUCTURED.
func legFlatStructured(in inputs) (LegResult, error) {
	conv, err := simplified.FlatToStructured(in.flatRaw)
	if err != nil {
		return LegResult{}, refuse("FLAT to STRUCTURED", err)
	}
	ours, err := leaves(conv)
	if err != nil {
		return LegResult{}, fmt.Errorf("FLAT to STRUCTURED produced no JSON: %w", err)
	}
	ref, err := leaves(in.structured)
	if err != nil {
		return LegResult{}, err
	}
	return compareLeaves(ref, ours), nil
}

// legStructuredFlat is leg (e): the upstream STRUCTURED, flattened, decoded
// and encoded as FLAT, against the upstream FLAT taken through the same
// decode and encode, metadata held out of both.
func legStructuredFlat(t *webtemplate.Target, in inputs) (LegResult, error) {
	ref, refRefusals, err := reencodeFlat(t, in.flat)
	if err != nil {
		return LegResult{Refusals: refRefusals}, err
	}
	flat, err := simplified.StructuredToFlat(in.structured)
	if err != nil {
		return LegResult{Refusals: refRefusals}, refuse("STRUCTURED to FLAT", err)
	}
	body, err := webtemplate.ParseFlat(flat)
	if err != nil {
		return LegResult{Refusals: refRefusals}, fmt.Errorf("STRUCTURED to FLAT produced no FLAT body: %w", err)
	}
	ours, oursRefusals, err := reencodeFlat(t, body)
	refusals := slices.Concat(refRefusals, oursRefusals)
	if err != nil {
		return LegResult{Refusals: refusals}, err
	}
	lr, err := compareFlat(ref, ours, t.Root)
	lr.Refusals = refusals
	return lr, err
}

// reencodeFlat holds a FLAT body's metadata out, decodes the rest with the
// reducing decode and encodes the result as FLAT again.
func reencodeFlat(t *webtemplate.Target, body map[string]any) (map[string]any, []webtemplate.Refusal, error) {
	comp, refusals, err := decodeFlat(t, holdOut(body, t.Root), webtemplate.InjectContext)
	if err != nil {
		return nil, refusals, err
	}
	out, err := simplified.MarshalFlat(comp, t.Web)
	if err != nil {
		return nil, refusals, refuse("FLAT encode", err)
	}
	m, err := webtemplate.ParseFlat(out)
	if err != nil {
		return nil, refusals, fmt.Errorf("FLAT encode produced no FLAT body: %w", err)
	}
	return m, refusals, nil
}

// decodeFlat runs the reducing decode over body, which it consumes. A decode
// error the loop cannot reduce is the leg's refusal, as the codec's own
// error; any other error is a harness fault.
func decodeFlat(t *webtemplate.Target, body map[string]any, mode webtemplate.ContextMode) (*rm.Composition, []webtemplate.Refusal, error) {
	comp, refusals, err := webtemplate.DecodeReducing(t, body, mode)
	if e, ok := errors.AsType[*webtemplate.IrreducibleError](err); ok {
		return nil, refusals, refuse("FLAT decode", e.Err)
	}
	return comp, refusals, err
}

// holdOut copies body without its composition metadata.
func holdOut(body map[string]any, root string) map[string]any {
	out := make(map[string]any, len(body))
	for k, v := range body {
		if !webtemplate.IsCompositionMeta(k, root) {
			out[k] = v
		}
	}
	return out
}

// decodeCanonical decodes the set's canonical JSON or, without it, its
// canonical XML.
func decodeCanonical(in inputs) (*rm.Composition, error) {
	if in.canonicalJSON != nil {
		return decodeJSON(in.canonicalJSON)
	}
	return decodeXML(in.canonicalXML)
}

func decodeJSON(b []byte) (*rm.Composition, error) {
	var comp rm.Composition
	if err := canjson.Unmarshal(b, &comp); err != nil {
		return nil, refuse("canonical JSON decode", err)
	}
	return &comp, nil
}

func decodeXML(b []byte) (*rm.Composition, error) {
	var comp rm.Composition
	if err := canxml.Unmarshal(b, &comp); err != nil {
		return nil, refuse("canonical XML decode", err)
	}
	return &comp, nil
}

// canonicalJSONLeaves decodes canonical JSON and returns the leaves of its
// canjson re-encode.
func canonicalJSONLeaves(b []byte) (map[string]string, error) {
	comp, err := decodeJSON(b)
	if err != nil {
		return nil, err
	}
	return encodeLeaves(comp)
}

// encodeLeaves encodes comp as canonical JSON and returns its leaves.
func encodeLeaves(comp *rm.Composition) (map[string]string, error) {
	b, err := canjson.Marshal(comp)
	if err != nil {
		return nil, refuse("canonical JSON encode", err)
	}
	l, err := leaves(b)
	if err != nil {
		return nil, fmt.Errorf("canonical JSON encode produced no JSON: %w", err)
	}
	return l, nil
}

// compareFlat compares two FLAT bodies key by key, composition metadata held
// out of both with the PROBE-086 hold-out. Values compare by their JSON text,
// as PROBE-086 compares them, so a number keeps its literal spelling.
func compareFlat(ref, ours map[string]any, root string) (LegResult, error) {
	r, err := flatLeaves(holdOut(ref, root))
	if err != nil {
		return LegResult{}, err
	}
	o, err := flatLeaves(holdOut(ours, root))
	if err != nil {
		return LegResult{}, err
	}
	return compareLeaves(r, o), nil
}

// flatLeaves maps each FLAT key to its value's JSON text.
func flatLeaves(body map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(body))
	for k, v := range body {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("FLAT value at %s: %w", k, err)
		}
		out[k] = string(b)
	}
	return out, nil
}

// compareLeaves compares two leaf maps in both directions, ref being the
// reference side.
func compareLeaves(ref, ours map[string]string) LegResult {
	lr := LegResult{Outcome: Outcome{Compared: len(ref)}}
	for k, want := range ref {
		have, ok := ours[k]
		switch {
		case !ok:
			lr.MissingKeys = append(lr.MissingKeys, k)
		case have != want:
			lr.Alterations = append(lr.Alterations, Alteration{Key: k, Reference: want, Ours: have})
		}
	}
	for k := range ours {
		if _, ok := ref[k]; !ok {
			lr.ExtraKeys = append(lr.ExtraKeys, k)
		}
	}
	slices.Sort(lr.MissingKeys)
	slices.Sort(lr.ExtraKeys)
	slices.SortFunc(lr.Alterations, func(a, b Alteration) int { return strings.Compare(a.Key, b.Key) })
	lr.Outcome.Missing, lr.Outcome.Extra, lr.Outcome.Altered = len(lr.MissingKeys), len(lr.ExtraKeys), len(lr.Alterations)
	return lr
}

// leaves flattens a JSON document into its leaves: a JSON pointer (RFC 6901)
// to each scalar, empty object and empty array, mapped to that value's JSON
// text. Numbers keep their literal text, so 1 and 1.0 differ and an integer
// past 2^53 compares exactly. Member order does not matter; array element
// order does. This is the semantic comparison REQ-080 asks for, never a byte
// comparison of the documents.
func leaves(doc []byte) (map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return nil, errors.New("content after the JSON document")
	}
	out := map[string]string{}
	if err := walk("", v, out); err != nil {
		return nil, err
	}
	return out, nil
}

// pointerEscaper escapes a member name as a JSON pointer reference token.
var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

func walk(ptr string, v any, out map[string]string) error {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			out[ptr] = "{}"
		}
		for k, c := range t {
			if err := walk(ptr+"/"+pointerEscaper.Replace(k), c, out); err != nil {
				return err
			}
		}
	case []any:
		if len(t) == 0 {
			out[ptr] = "[]"
		}
		for i, c := range t {
			if err := walk(ptr+"/"+strconv.Itoa(i), c, out); err != nil {
				return err
			}
		}
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("leaf %s: %w", ptr, err)
		}
		out[ptr] = string(b)
	}
	return nil
}
