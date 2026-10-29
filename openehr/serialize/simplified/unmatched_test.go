package simplified

// REQ-170 — unmatched content on FLAT and STRUCTURED decode: ignored by
// default, reported under WithUnmatched, refused under WithStrict, while the
// keys the decode reads but refuses fail in every mode.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// strictRefusal is the exact error WithStrict gives for an unmatched item at
// path: the sentinel, the path quoted, and nothing from the input's values.
func strictRefusal(path string) string {
	return "simplified: path not in web template: " + strconv.Quote(path) + " is unmatched content"
}

// mustJSON marshals v, failing the test on error. A map marshals with its keys
// sorted, which is the document order the report then follows.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// canonical renders comp as canonical JSON, the form two decodes are compared
// in.
func canonical(t *testing.T, comp *rm.Composition) []byte {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	return b
}

// unmatchedStrictErr runs the three REQ-170 legs over a FLAT body whose
// unmatched keys are exactly want:
//
//   - by default the decode succeeds, with the value the body without want
//     decodes to;
//   - WithUnmatched receives want, each once, in document order (sorted, the
//     order json.Marshal writes a map in), and the value is the same;
//   - WithStrict refuses with an error that wraps ErrUnknownPath, names the
//     first of want, quoted, and nothing else.
//
// It returns the strict error for the caller's own checks.
func unmatchedStrictErr(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, want ...string) error {
	t.Helper()
	if len(want) == 0 {
		t.Fatal("unmatchedStrictErr needs the unmatched keys")
	}
	without := maps.Clone(body)
	for _, k := range want {
		if _, ok := without[k]; !ok {
			t.Fatalf("fixture: unmatched key %q is not in the body", k)
		}
		delete(without, k)
	}
	ref, err := UnmarshalFlat(mustJSON(t, without), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat(body without %q): %v", want, err)
	}
	b := mustJSON(t, body)

	got, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (default) = %v, want the unmatched keys %q ignored", err, want)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
		t.Errorf("UnmarshalFlat (default) value differs from the body without %q:\n got %s\nwant %s",
			want, canonical(t, got), canonical(t, ref))
	}

	var reported []string
	got, err = UnmarshalFlat(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithUnmatched) = %v, want no error", err)
	}
	sorted := slices.Sorted(slices.Values(want))
	if !slices.Equal(reported, sorted) {
		t.Errorf("WithUnmatched reported %q, want %q", reported, sorted)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
		t.Errorf("UnmarshalFlat (WithUnmatched) value differs from the default")
	}

	_, err = UnmarshalFlat(b, wt, WithStrict())
	if err == nil {
		t.Fatalf("UnmarshalFlat (WithStrict) succeeded, want a refusal naming %q", sorted[0])
	}
	if !errors.Is(err, ErrUnknownPath) {
		t.Errorf("UnmarshalFlat (WithStrict) = %v, want ErrUnknownPath", err)
	}
	if err.Error() != strictRefusal(sorted[0]) {
		t.Errorf("UnmarshalFlat (WithStrict) = %q, want %q", err, strictRefusal(sorted[0]))
	}
	return err
}

// assertUnmatched runs the three REQ-170 legs of [unmatchedStrictErr] for a
// caller that needs nothing from the strict refusal beyond those checks.
func assertUnmatched(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, want ...string) {
	t.Helper()
	if err := unmatchedStrictErr(t, wt, body, want...); err == nil {
		t.Fatal("unmatchedStrictErr returned no refusal")
	}
}

// assertRefusedInEveryMode is [refusedInEveryMode] for a caller that checks
// no more than that.
func assertRefusedInEveryMode(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) {
	t.Helper()
	if err := refusedInEveryMode(t, wt, body); err == nil {
		t.Fatal("refusedInEveryMode returned no refusal")
	}
}

// assertRefusalLegs is [refusalLegs] for a caller that checks no more than
// that.
func assertRefusalLegs(t *testing.T, wt *webtemplate.WebTemplate, body, keys map[string]any, want error) {
	t.Helper()
	if err := refusalLegs(t, wt, body, keys, want); err == nil {
		t.Fatal("refusalLegs returned no refusal")
	}
}

// errUnmatched marks a table case whose keys are unmatched content: the case
// runs [unmatchedStrictErr] instead of expecting a refusal in every mode.
var errUnmatched = errors.New("test: unmatched content")

// refusalLegs runs one table case of a refusal test: for a case marked
// [errUnmatched] the three REQ-170 legs over keys, returning the strict
// refusal; for any other, the refusal in every mode, which must match want.
func refusalLegs(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, keys map[string]any, want error) error {
	t.Helper()
	if errors.Is(want, errUnmatched) {
		return unmatchedStrictErr(t, wt, body, slices.Sorted(maps.Keys(keys))...)
	}
	err := refusedInEveryMode(t, wt, body)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	return err
}

// refusedInEveryMode asserts that body fails to decode by default, under
// WithUnmatched and under WithStrict, with the same error each time, and
// returns it. The keys a decode reads but refuses are not unmatched content.
func refusedInEveryMode(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) error {
	t.Helper()
	b := mustJSON(t, body)
	_, def := UnmarshalFlat(b, wt)
	if def == nil {
		t.Fatal("UnmarshalFlat (default) succeeded, want a refusal")
	}
	var reported []string
	_, rep := UnmarshalFlat(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
	_, strict := UnmarshalFlat(b, wt, WithStrict())
	for name, err := range map[string]error{"WithUnmatched": rep, "WithStrict": strict} {
		if err == nil || err.Error() != def.Error() {
			t.Errorf("UnmarshalFlat (%s) = %v, want the default's refusal %v", name, err, def)
		}
	}
	if len(reported) != 0 {
		t.Errorf("WithUnmatched reported %q on a refused decode, want nothing", reported)
	}
	return def
}

// structuredRefusedInEveryMode is [refusedInEveryMode] for the STRUCTURED
// form of the FLAT body flat: UnmarshalStructured of FlatToStructured fails by
// default, under WithUnmatched and under WithStrict with the same error, and
// the report receives nothing.
func structuredRefusedInEveryMode(t *testing.T, wt *webtemplate.WebTemplate, flat map[string]any) error {
	t.Helper()
	b, err := FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	return structuredDocRefusedInEveryMode(t, wt, b)
}

// structuredDocRefusedInEveryMode is [structuredRefusedInEveryMode] for a
// STRUCTURED document as written.
func structuredDocRefusedInEveryMode(t *testing.T, wt *webtemplate.WebTemplate, b []byte) error {
	t.Helper()
	_, def := UnmarshalStructured(b, wt)
	if def == nil {
		t.Fatal("UnmarshalStructured (default) succeeded, want a refusal")
	}
	var reported []string
	_, rep := UnmarshalStructured(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
	_, strict := UnmarshalStructured(b, wt, WithStrict())
	for name, err := range map[string]error{"WithUnmatched": rep, "WithStrict": strict} {
		if err == nil || err.Error() != def.Error() {
			t.Errorf("UnmarshalStructured (%s) = %v, want the default's refusal %v", name, err, def)
		}
	}
	if len(reported) != 0 {
		t.Errorf("WithUnmatched reported %q on a refused decode, want nothing", reported)
	}
	return def
}

// req170Body is a decodable FLAT body over the corpus template, carrying the
// four kinds of unmatched key REQ-170 names for FLAT, each with a value that
// must not reach any error.
func req170Body() (body map[string]any, unmatched []string) {
	const secret = "secret-value-0451"
	unmatched = []string{
		rmattrObs + "/no_such_node|magnitude", // a key the Web Template does not resolve
		rmattrObs + "/_x_extension",           // an unknown `_`-segment
		rmattrElement + "|x_note",             // an unknown suffix on a modelled leaf
		"ctx/x_flag",                          // an unknown ctx/ field
	}
	extra := make(map[string]any, len(unmatched))
	for _, k := range unmatched {
		extra[k] = secret
	}
	return rmattrBody(extra), unmatched
}

// TestREQ170FlatUnmatchedIgnoredReportedRefused — REQ-170 acceptance for FLAT:
// a key the Web Template does not resolve, an unknown `_`-segment, an unknown
// suffix on a modelled leaf and an unknown ctx/ field decode by default to the
// value without them; the report option receives each once in document order;
// strict fails wrapping ErrUnknownPath, names the first and carries none of
// its value.
func TestREQ170FlatUnmatchedIgnoredReportedRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	body, unmatched := req170Body()
	err := unmatchedStrictErr(t, wt, body, unmatched...)
	if strings.Contains(err.Error(), "secret-value-0451") {
		t.Errorf("strict refusal %q carries the unmatched key's value", err)
	}
}

// TestREQ170FlatReportFollowsDocumentOrder — REQ-170. The report follows the
// order the keys are written in, not a sorted order: the FLAT body here is
// written by hand with its unmatched keys in reverse.
func TestREQ170FlatReportFollowsDocumentOrder(t *testing.T) {
	wt, _ := conformanceWT(t)
	body, unmatched := req170Body()
	written := slices.Clone(unmatched)
	slices.Reverse(written)
	b := orderedFlat(t, body, written...)

	var reported []string
	if _, err := UnmarshalFlat(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if !slices.Equal(reported, written) {
		t.Errorf("WithUnmatched reported %q, want the written order %q", reported, written)
	}
	_, err := UnmarshalFlat(b, wt, WithStrict())
	if err == nil || err.Error() != strictRefusal(written[0]) {
		t.Errorf("UnmarshalFlat (WithStrict) = %v, want %q", err, strictRefusal(written[0]))
	}
}

// structuredDoc returns the STRUCTURED form of the FLAT body flat, as written
// and as a map to modify.
func structuredDoc(t *testing.T, flat map[string]any) ([]byte, map[string]any) {
	t.Helper()
	b, err := FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return b, doc
}

// structuredObservation is the observation element of a [structuredDoc] made
// from [rmattrBody].
func structuredObservation(doc map[string]any) map[string]any {
	section := doc[rmattrRoot].(map[string]any)["conformance_section"].([]any)[0].(map[string]any)
	return section["conformance_observation"].([]any)[0].(map[string]any)
}

// assertStructuredUnmatched runs the three REQ-170 legs over a STRUCTURED
// document whose unmatched items are exactly want, in document order: by
// default it decodes to what ref decodes to; WithUnmatched receives want,
// each once, and the value is the same; WithStrict refuses with an error that
// wraps ErrUnknownPath, names want[0] and carries none of secret.
func assertStructuredUnmatched(t *testing.T, wt *webtemplate.WebTemplate, doc, ref []byte, secret string, want ...string) {
	t.Helper()
	refComp, err := UnmarshalStructured(ref, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured (reference) = %v", err)
	}
	got, err := UnmarshalStructured(doc, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured (default) = %v, want the unmatched members ignored", err)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, refComp)) {
		t.Errorf("UnmarshalStructured (default) value differs from the body without the unmatched members")
	}
	var reported []string
	got, err = UnmarshalStructured(doc, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
	if err != nil {
		t.Fatalf("UnmarshalStructured (WithUnmatched) = %v", err)
	}
	if !slices.Equal(reported, want) {
		t.Errorf("WithUnmatched reported %q, want %q", reported, want)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, refComp)) {
		t.Errorf("UnmarshalStructured (WithUnmatched) value differs from the default")
	}
	_, err = UnmarshalStructured(doc, wt, WithStrict())
	if !errors.Is(err, ErrUnknownPath) || err.Error() != strictRefusal(want[0]) {
		t.Errorf("UnmarshalStructured (WithStrict) = %v, want %q wrapping ErrUnknownPath", err, strictRefusal(want[0]))
	}
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("strict refusal %q carries the unmatched member's value", err)
	}
}

// TestREQ170StructuredUnmatchedIgnoredReportedRefused — REQ-170 acceptance for
// STRUCTURED: the same four kinds of unmatched member decode by default to the
// value without them, the report names each once by its slash-joined member
// path in document order, and strict refuses naming the first, wrapping
// ErrUnknownPath and carrying none of its value. A member whose name resolves
// nothing is reported as the member itself, whatever it holds.
func TestREQ170StructuredUnmatchedIgnoredReportedRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	clean, doc := structuredDoc(t, rmattrBody(nil))

	// The same body with the four unmatched members: the ctx field, then the
	// unknown suffix, the unknown node and the unknown `_` member in the
	// observation.
	doc["ctx"].(map[string]any)["x_flag"] = secret
	obs := structuredObservation(doc)
	obs["any_event"].([]any)[0].(map[string]any)["dv_quantity"].([]any)[0].(map[string]any)["|x_note"] = secret
	obs["no_such_node"] = []any{map[string]any{"|magnitude": secret}}
	obs["_x_extension"] = []any{secret}

	obsPath := rmattrRoot + "/conformance_section/0/conformance_observation/0"
	// json.Marshal writes each object's members sorted, so that is the document
	// order: `_x_extension` sorts before `any_event` and `no_such_node`, and
	// `conformance-ehrbase.de.v0` before `ctx`.
	assertStructuredUnmatched(t, wt, mustJSON(t, doc), clean, secret,
		obsPath+"/_x_extension",
		obsPath+"/any_event/0/dv_quantity/0/|x_note",
		obsPath+"/no_such_node",
		"ctx/x_flag",
	)
}

// TestREQ170CarrierlessKeysRefusedInEveryMode — REQ-170. A key that resolves
// but has no carrier is matched content: it fails by default, under the report
// option and under strict, and the report receives nothing for it.
func TestREQ170CarrierlessKeysRefusedInEveryMode(t *testing.T) {
	wt, _ := conformanceWT(t)
	const instruction = rmattrSection + "/conformance_instruction"
	for _, tc := range []struct {
		name string
		keys map[string]any
		want error
	}{
		{"PARTICIPATION time", map[string]any{
			rmattrContext + "/_participation:0|function": "requester",
			rmattrContext + "/_participation:0|time":     "2021-12-21T14:19:31+01:00",
		}, ErrUnsupportedDatatype},
		{"PARTICIPATION time as an interval bound", map[string]any{
			rmattrContext + "/_participation:0|function":   "requester",
			rmattrContext + "/_participation:0/time/lower": "2021-12-21T14:19:31+01:00",
		}, ErrUnsupportedDatatype},
		{"PARTICIPATION time as a sub-path", map[string]any{
			rmattrContext + "/_participation:0|function": "requester",
			rmattrContext + "/_participation:0/time":     "2021-12-21T14:19:31+01:00",
		}, ErrUnsupportedDatatype},
		{"PARTICIPATION time as a sub-path with a suffix", map[string]any{
			rmattrContext + "/_participation:0|function":   "requester",
			rmattrContext + "/_participation:0/time|lower": "2021-12-21T14:19:31+01:00",
		}, ErrUnsupportedDatatype},
		{"FEEDER_AUDIT_DETAILS other_details", map[string]any{
			rmattrObs + "/_feeder_audit/originating_system_audit|system_id":       "orig",
			rmattrObs + "/_feeder_audit/originating_system_audit/other_details|x": "boom",
		}, ErrUnsupportedDatatype},
		{"FEEDER_AUDIT_DETAILS other_details as a suffix", map[string]any{
			rmattrObs + "/_feeder_audit/originating_system_audit|system_id":     "orig",
			rmattrObs + "/_feeder_audit/originating_system_audit|other_details": "boom",
		}, ErrUnsupportedDatatype},
		{"_instruction_details", map[string]any{rmattrAction + "/_instruction_details|activity_id": "activities[at0001]"}, ErrUnknownPath},
		{"_wf_definition", map[string]any{instruction + "/_wf_definition|value": "wf"}, ErrUnknownPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refusedInEveryMode(t, wt, rmattrBody(tc.keys))
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "is unmatched content") {
				t.Errorf("err = %v reads as unmatched content, but the key is matched", err)
			}
		})
	}
}

// TestREQ170MalformedMatchedContentRefusedInEveryMode — REQ-170. Malformed
// matched content stays an error in every mode: a value of the wrong shape, a
// missing required suffix, two spellings of one field that disagree, an index
// past its bound.
func TestREQ170MalformedMatchedContentRefusedInEveryMode(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{"wrong shape", map[string]any{rmattrElement + "|accuracy": "not a number"}},
		{"missing required suffix", map[string]any{rmattrCount + "|magnitude_status": "="}},
		{"disagreeing spellings", map[string]any{rmattrRoot + "/language|code": "de"}},
		{"index past its bound", map[string]any{rmattrRoot + "/context:1/_location": "lab"}},
		{"composer reference without its namespace", map[string]any{rmattrRoot + "/composer|id": "1234"}},
		{"composer reference type without an id", map[string]any{rmattrRoot + "/composer|type": "PERSON"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusedInEveryMode(t, wt, rmattrBody(tc.keys))
		})
	}
}

// TestREQ170ComposerRealPathIsMatchedContent — REQ-170 / REQ-053, ADR 0029,
// replacing the composer rows the carrier-less table held while the composer's
// real path had no carrier. The real path now carries the whole composer, so
// its external_ref, identifier and relationship keys are matched content the
// decode reads: strict decodes them and reports nothing. A bare value is not
// in the party grammar, at the composer as at every other party position, so
// it is unmatched content.
func TestREQ170ComposerRealPathIsMatchedContent(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{"composer external_ref", map[string]any{
			rmattrRoot + "/composer|id": "1234", rmattrRoot + "/composer|id_namespace": "demographic",
			rmattrRoot + "/composer|type": "PERSON",
		}},
		{"composer identifier", map[string]any{rmattrRoot + "/composer/_identifier:0|id": "122"}},
		{"composer relationship", map[string]any{
			"ctx/composer_name":                         "Dr. Who",
			rmattrRoot + "/composer/relationship|code":  "10",
			rmattrRoot + "/composer/relationship|value": "mother",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reported []string
			comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(tc.keys)), wt, WithStrict(),
				WithUnmatched(func(p string) { reported = append(reported, p) }))
			if err != nil {
				t.Fatalf("UnmarshalFlat (strict) = %v, want the composer keys read", err)
			}
			if len(reported) != 0 {
				t.Errorf("reported %q, want nothing", reported)
			}
			if comp.Composer == nil {
				t.Error("composer not decoded")
			}
		})
	}
	t.Run("composer bare value", func(t *testing.T) {
		key := rmattrRoot + "/composer"
		assertUnmatched(t, wt, rmattrBody(map[string]any{key: "Dr. Who"}), key)
	})
}

// TestREQ170StrictAcceptsAbsentMandatory — REQ-170. Strict covers unmatched
// content only: a body that leaves out RM-mandatory content the format does
// not require (the ENTRY language and encoding, every boundary flag) decodes
// under strict exactly as by default.
func TestREQ170StrictAcceptsAbsentMandatory(t *testing.T) {
	wt, _ := conformanceWT(t)
	body := map[string]any{
		"ctx/language":                      "en",
		"ctx/territory":                     "US",
		rmattrElement + "|magnitude":        65.9,
		rmattrElement + "|unit":             "unit",
		rmattrInterval + "/lower|magnitude": 1.0,
		rmattrInterval + "/lower|unit":      "Unit",
		rmattrInterval + "/upper|magnitude": 2.0,
		rmattrInterval + "/upper|unit":      "Unit",
	}
	b := mustJSON(t, body)
	def, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (default): %v", err)
	}
	strict, err := UnmarshalFlat(b, wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithStrict) = %v, want an absent member never refused", err)
	}
	if !bytes.Equal(canonical(t, strict), canonical(t, def)) {
		t.Error("UnmarshalFlat (WithStrict) value differs from the default")
	}
}

// TestREQ170NilOptionsChangeNothing — REQ-170. A nil option and a nil report
// function are the same as leaving the option out.
func TestREQ170NilOptionsChangeNothing(t *testing.T) {
	wt, _ := conformanceWT(t)
	body, _ := req170Body()
	b := mustJSON(t, body)
	def, err := UnmarshalFlat(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (default): %v", err)
	}
	for name, opts := range map[string][]Option{
		"nil option":      {nil},
		"nil report func": {WithUnmatched(nil)},
	} {
		got, err := UnmarshalFlat(b, wt, opts...)
		if err != nil {
			t.Fatalf("UnmarshalFlat (%s) = %v, want the default's success", name, err)
		}
		if !bytes.Equal(canonical(t, got), canonical(t, def)) {
			t.Errorf("UnmarshalFlat (%s) value differs from the default", name)
		}
	}
}

// TestREQ170ConcurrentDecodesKeepTheirOwnOptions — REQ-170. The options belong
// to one decode: concurrent decodes of one body with different options do not
// see each other's policy. Run it with -race.
func TestREQ170ConcurrentDecodesKeepTheirOwnOptions(t *testing.T) {
	wt, _ := conformanceWT(t)
	body, unmatched := req170Body()
	b := mustJSON(t, body)
	want := slices.Sorted(slices.Values(unmatched))

	const rounds = 16
	var wg sync.WaitGroup
	errs := make(chan error, 3*rounds)
	for range rounds {
		wg.Go(func() {
			if _, err := UnmarshalFlat(b, wt); err != nil {
				errs <- fmt.Errorf("default decode: %w", err)
			}
		})
		wg.Go(func() {
			var reported []string
			if _, err := UnmarshalFlat(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
				errs <- fmt.Errorf("report decode: %w", err)
				return
			}
			if !slices.Equal(reported, want) {
				errs <- fmt.Errorf("report decode reported %q, want %q", reported, want)
			}
		})
		wg.Go(func() {
			_, err := UnmarshalFlat(b, wt, WithStrict())
			switch {
			case err == nil:
				errs <- fmt.Errorf("strict decode succeeded, want %q", strictRefusal(want[0]))
			case err.Error() != strictRefusal(want[0]):
				errs <- fmt.Errorf("strict decode = %w, want %q", err, strictRefusal(want[0]))
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestREQ170IgnoredKeysLeaveNoStructuralTrace — REQ-170. The default value is
// the decode of the body without its unmatched keys, structural checks
// included: an unmatched key at the only `:0` slot of a list leaves `:1`
// sparse and refused, exactly as the body without it is, and a family instance
// spelled only by an unmatched suffix is no instance at all.
func TestREQ170IgnoredKeysLeaveNoStructuralTrace(t *testing.T) {
	wt, _ := conformanceWT(t)
	link1 := map[string]any{
		rmattrObs + "/_link:1|meaning": "m", rmattrObs + "/_link:1|type": "t", rmattrObs + "/_link:1|target": "ehr:/x",
	}
	t.Run("sparse once the unmatched slot is gone", func(t *testing.T) {
		keys := maps.Clone(link1)
		const unmatched = rmattrObs + "/_link:0|x_note"
		keys[unmatched] = "n"
		b := mustJSON(t, rmattrBody(keys))
		var reported []string
		for name, opts := range map[string][]Option{
			"default":       nil,
			"WithUnmatched": {WithUnmatched(func(p string) { reported = append(reported, p) })},
		} {
			_, err := UnmarshalFlat(b, wt, opts...)
			if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "sparse") {
				t.Errorf("UnmarshalFlat (%s) = %v, want the sparse :index refusal the body without the unmatched key gets", name, err)
			}
		}
		if len(reported) != 0 {
			t.Errorf("WithUnmatched reported %q on a refused decode, want nothing", reported)
		}
		// Strict meets the unmatched key first and refuses it.
		if _, err := UnmarshalFlat(b, wt, WithStrict()); err == nil || err.Error() != strictRefusal(unmatched) {
			t.Errorf("UnmarshalFlat (WithStrict) = %v, want %q", err, strictRefusal(unmatched))
		}
	})
	t.Run("sparse once a nested slot of unmatched keys is gone", func(t *testing.T) {
		// A FEEDER_AUDIT item-id list judges its slots before it decodes them, so
		// only a decode of the body without the unmatched key sees slot :0 gone.
		fa := rmattrObs + "/_feeder_audit"
		const unmatched = rmattrObs + "/_feeder_audit/originating_system_item_id:0|x_note"
		b := mustJSON(t, rmattrBody(map[string]any{
			fa + "/originating_system_audit|system_id": "orig",
			fa + "/originating_system_item_id:1|id":    "id2",
			unmatched:                                  "n",
		}))
		for name, opts := range map[string][]Option{"default": nil, "WithUnmatched": {WithUnmatched(func(string) {})}} {
			_, err := UnmarshalFlat(b, wt, opts...)
			if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "sparse") {
				t.Errorf("UnmarshalFlat (%s) = %v, want the sparse :index refusal the body without the unmatched key gets", name, err)
			}
		}
	})
	t.Run("an instance of only unmatched keys is absent", func(t *testing.T) {
		assertUnmatched(t, wt, rmattrBody(map[string]any{rmattrObs + "/_uid|x_note": "n"}), rmattrObs+"/_uid|x_note")
	})
}

// TestREQ170IndexPastBoundSentinels — REQ-170 / REQ-053 / REQ-140. An index
// past its bound is malformed matched content, refused in every mode naming
// the key, with the sentinel of the grammar the segment belongs to: a segment
// of the Web Template path (a single-valued attribute, a party leaf, an
// interval leaf on a single-valued attribute, the last in
// [TestREQ170RepeatableIntervalElement]), a `_family:N` list and a
// FEEDER_AUDIT sub-path give ErrUnknownPath; a
// sub-path inside an underscore family's value or an interval value gives
// ErrUnsupportedDatatype. deviations.md states the same split. STRUCTURED
// judges an element's position the same way, whatever the element holds.
func TestREQ170IndexPastBoundSentinels(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		name string
		key  string
		val  any
		want error
		// names is the path up to the offending segment, which every refusal
		// names.
		names string
	}{
		{"single-valued Web Template segment", rmattrObs + "/language:1|code", "nl", ErrUnknownPath, rmattrObs + "/language:1"},
		{"root segment alone, with a suffix", rmattrRoot + ":1|x_note", "n", ErrUnknownPath, rmattrRoot + ":1"},
		{"root segment alone", rmattrRoot + ":1", "n", ErrUnknownPath, rmattrRoot + ":1"},
		// Inside a family or a composite leaf's grammar the key is judged from
		// the left too: a sub-path past its bound is refused even where what
		// follows it would leave the key unmatched.
		{"interval bound, an unread key below", rmattrInterval + "/lower:1/x_unknown", 1.0, ErrUnsupportedDatatype, rmattrInterval + "/lower:1"},
		{"range bound, an unread key below", rmattrCount + "/_normal_range/lower:1/x_unknown", 1, ErrUnsupportedDatatype, rmattrCount + "/_normal_range/lower:1"},
		{"range meaning, an unread key below", rmattrElement + "/_other_reference_ranges:0/meaning:1/x_unknown", "m", ErrUnsupportedDatatype, rmattrElement + "/_other_reference_ranges:0/meaning:1"},
		{"mapping target, an unread key below", rmattrText + "/_mapping:0/target:1/x_unknown", "t", ErrUnsupportedDatatype, rmattrText + "/_mapping:0/target:1"},
		{"party relationship, an unread key below", rmattrObs + "/subject/relationship:1/x_unknown|code", "x", ErrUnsupportedDatatype, rmattrObs + "/subject/relationship:1"},
		{"party identifier list, an unread key below", rmattrObs + "/subject/_identifier:10001/x_unknown|id", "x", ErrUnknownPath, rmattrObs + "/subject/_identifier:10001"},
		{"scalar underscore family", rmattrObs + "/_uid:1", "8f8f8f8f-0000-4000-8000-000000000000", ErrUnknownPath, rmattrObs + "/_uid:1"},
		{"FEEDER_AUDIT sub-path", rmattrObs + "/_feeder_audit/originating_system_audit:1|system_id", "orig", ErrUnknownPath, rmattrObs + "/_feeder_audit/originating_system_audit"},
		{"interval value sub-path", rmattrInterval + "/lower:1|magnitude", 1.0, ErrUnsupportedDatatype, rmattrInterval + "/lower:1"},
		{"underscore family value sub-path", rmattrCount + "/_normal_range/lower:1", 1, ErrUnsupportedDatatype, rmattrCount + "/_normal_range/lower:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refusalLegs(t, wt, rmattrBody(map[string]any{tc.key: tc.val}), nil, tc.want)
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("err = %v, want it to name %q", err, tc.names)
			}
		})
	}
	// STRUCTURED: an element past the bound of its member's position is
	// refused whatever it holds, even when every member it holds is unmatched
	// content, as the FLAT spelling of the same body is (REQ-170).
	for _, tc := range []struct {
		name   string
		extra  map[string]any
		at     string
		member string
		// want is the sentinel of the grammar the position belongs to.
		want error
	}{
		{"context", map[string]any{rmattrContext + "/_health_care_facility|name": "Hospital"}, rmattrRoot, "context", ErrUnknownPath},
		{"composer", map[string]any{rmattrRoot + "/composer|name": "Dr. Who"}, rmattrRoot, "composer", ErrUnknownPath},
		{"observation language", nil, rmattrRoot + "/conformance_section/0/conformance_observation/0", "language", ErrUnknownPath},
		{"single-valued family", map[string]any{rmattrObs + "/_uid": "8f8f8f8f-0000-4000-8000-000000000000"}, rmattrRoot + "/conformance_section/0/conformance_observation/0", "_uid", ErrUnknownPath},
		{
			"FEEDER_AUDIT sub-path",
			map[string]any{rmattrObs + "/_feeder_audit/originating_system_audit|system_id": "orig"},
			rmattrRoot + "/conformance_section/0/conformance_observation/0/_feeder_audit/0", "originating_system_audit", ErrUnknownPath,
		},
		{"interval bound", map[string]any{
			rmattrIntervalEvent + "/time":       "2022-01-12T09:00:11.7842493+01:00",
			rmattrInterval + "/lower|magnitude": 1.0, rmattrInterval + "/lower|unit": "Unit",
			rmattrInterval + "/upper|magnitude": 2.0, rmattrInterval + "/upper|unit": "Unit",
		}, rmattrRoot + "/conformance_section/0/conformance_interval/0/any_event/0/interval_dv_quantity/0", "lower", ErrUnsupportedDatatype},
		{"range bound", map[string]any{
			rmattrElement + "/_normal_range/lower|magnitude": 20.5, rmattrElement + "/_normal_range/lower|unit": "unit",
			rmattrElement + "/_normal_range/upper|magnitude": 66.6, rmattrElement + "/_normal_range/upper|unit": "unit",
		}, rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity/0/_normal_range/0", "lower", ErrUnsupportedDatatype},
		{"range meaning", map[string]any{
			rmattrElement + "/_other_reference_ranges:0/lower|magnitude":     70.5,
			rmattrElement + "/_other_reference_ranges:0/lower|unit":          "unit",
			rmattrElement + "/_other_reference_ranges:0/meaning|code":        "260360000",
			rmattrElement + "/_other_reference_ranges:0/meaning|terminology": "SNOMED-CT",
			rmattrElement + "/_other_reference_ranges:0/meaning|value":       "very high",
			rmattrElement + "/_other_reference_ranges:0|upper_unbounded":     true,
		}, rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity/0/_other_reference_ranges/0", "meaning", ErrUnsupportedDatatype},
		{"mapping target", map[string]any{
			rmattrText: "text value", rmattrText + "/_mapping:0|match": "=",
			rmattrText + "/_mapping:0/target|code": "Z.1", rmattrText + "/_mapping:0/target|terminology": "ICD10",
		}, rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_text/0/_mapping/0", "target", ErrUnsupportedDatatype},
		{"party relationship", map[string]any{
			rmattrObs + "/subject|name":                     "Silvia Blake",
			rmattrObs + "/subject/relationship|code":        "10",
			rmattrObs + "/subject/relationship|terminology": "openehr",
			rmattrObs + "/subject/relationship|value":       "mother",
		}, rmattrRoot + "/conformance_section/0/conformance_observation/0/subject/0", "relationship", ErrUnsupportedDatatype},
		{"party inside FEEDER_AUDIT", map[string]any{
			rmattrObs + "/_feeder_audit/originating_system_audit|system_id":                         "orig",
			rmattrObs + "/_feeder_audit/originating_system_audit/provider|name":                     "Per 1",
			rmattrObs + "/_feeder_audit/originating_system_audit/provider/relationship|code":        "10",
			rmattrObs + "/_feeder_audit/originating_system_audit/provider/relationship|value":       "mother",
			rmattrObs + "/_feeder_audit/originating_system_audit/provider/relationship|terminology": "openehr",
		}, rmattrRoot + "/conformance_section/0/conformance_observation/0/_feeder_audit/0/originating_system_audit/0/provider/0", "relationship", ErrUnsupportedDatatype},
	} {
		t.Run("STRUCTURED "+tc.name+"/1 holding only unmatched members", func(t *testing.T) {
			_, doc := structuredDoc(t, rmattrBody(tc.extra))
			obj := structuredObject(t, doc, tc.at)
			obj[tc.member] = append(obj[tc.member].([]any), map[string]any{"x_unknown": []any{"v"}})
			b := mustJSON(t, doc)
			err := structuredDocRefusedInEveryMode(t, wt, b)
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "single-valued") {
				t.Errorf("STRUCTURED err = %v, want %v for a single-valued position", err, tc.want)
			}
			flat, convErr := StructuredToFlat(b)
			if convErr != nil {
				t.Fatalf("StructuredToFlat: %v", convErr)
			}
			var body map[string]any
			if err := json.Unmarshal(flat, &body); err != nil {
				t.Fatal(err)
			}
			if err := refusedInEveryMode(t, wt, body); !errors.Is(err, tc.want) {
				t.Errorf("FLAT err = %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("party inside FEEDER_AUDIT, an unread key below", func(t *testing.T) {
		const audit = rmattrObs + "/_feeder_audit/originating_system_audit"
		body := rmattrBody(map[string]any{
			audit + "|system_id":                              "orig",
			audit + "/provider/relationship:1/x_unknown|code": "x",
		})
		err := refusedInEveryMode(t, wt, body)
		if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), audit+"/provider/relationship:1") {
			t.Errorf("err = %v, want ErrUnsupportedDatatype naming %q", err, audit+"/provider/relationship:1")
		}
	})
	t.Run("STRUCTURED party identifier list past its bound, holding only unmatched members", func(t *testing.T) {
		_, doc := structuredDoc(t, rmattrBody(map[string]any{
			rmattrObs + "/subject|name":             "Silvia Blake",
			rmattrObs + "/subject/_identifier:0|id": "122",
		}))
		subject := structuredObject(t, doc, rmattrRoot+"/conformance_section/0/conformance_observation/0/subject/0")
		ids := subject["_identifier"].([]any)
		for len(ids) <= maxRepeatIndex+1 {
			ids = append(ids, map[string]any{"x_unknown": []any{"v"}})
		}
		subject["_identifier"] = ids
		err := structuredDocRefusedInEveryMode(t, wt, mustJSON(t, doc))
		if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "exceeds bound") {
			t.Errorf("STRUCTURED err = %v, want ErrUnknownPath for an index past the list's bound", err)
		}
	})
}

// TestREQ170RepeatableIntervalElement — REQ-053 / REQ-170. A DV_INTERVAL
// leaf's :N belongs to the ELEMENT the Web Template folds away and is judged
// on the RM attribute that holds that ELEMENT, as any other leaf's is. Where
// the ELEMENT repeats, two intervals decode to two ELEMENTs and MarshalFlat →
// UnmarshalFlat round-trips them. Where the interval stands on a single-valued
// attribute (ELEMENT.value, under an ELEMENT the Web Template keeps as a
// node), an index past :0 is an index past its bound, refused in every mode.
func TestREQ170RepeatableIntervalElement(t *testing.T) {
	wt, _ := conformanceWT(t)
	interval := func(i int, lower, upper float64) map[string]any {
		leaf := rmattrIntervalEvent + "/interval_dv_quantity:" + strconv.Itoa(i)
		return map[string]any{
			leaf + "/lower|magnitude": lower, leaf + "/lower|unit": "Unit",
			leaf + "/upper|magnitude": upper, leaf + "/upper|unit": "Unit",
		}
	}

	t.Run("a repeating ELEMENT round-trips", func(t *testing.T) {
		repeat := editWTNode(t, wt, rmattrInterval, func(n *webtemplate.Node) { n.Max = -1 })
		body := rmattrBody(map[string]any{rmattrIntervalEvent + "/time": "2022-01-12T09:00:11.7842493+01:00"})
		maps.Copy(body, interval(0, 1, 2))
		maps.Copy(body, interval(1, 3, 4))
		comp, err := UnmarshalFlat(mustJSON(t, body), repeat, WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalFlat (two intervals) = %v", err)
		}
		flat, err := MarshalFlat(comp, repeat)
		if err != nil {
			t.Fatalf("MarshalFlat: %v", err)
		}
		for _, k := range []string{rmattrIntervalEvent + "/interval_dv_quantity:0/lower|magnitude", rmattrIntervalEvent + "/interval_dv_quantity:1/upper|magnitude"} {
			if !bytes.Contains(flat, mustJSON(t, k)) {
				t.Errorf("MarshalFlat = %s, want it to carry %q", flat, k)
			}
		}
		again, err := UnmarshalFlat(flat, repeat, WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalFlat (MarshalFlat output) = %v", err)
		}
		if !bytes.Equal(canonical(t, again), canonical(t, comp)) {
			t.Errorf("the round trip changed the value:\n got %s\nwant %s", canonical(t, again), canonical(t, comp))
		}
	})

	t.Run("a single-valued interval refuses :1", func(t *testing.T) {
		// The interval ELEMENT kept as a node, its value a child on
		// ELEMENT.value, the shape a Web Template gives an ELEMENT it does
		// not collapse.
		single := editWTNode(t, wt, rmattrInterval, func(n *webtemplate.Node) {
			valuePath, rmType := n.AQLPath, n.RMType
			n.AQLPath, n.RMType = strings.TrimSuffix(valuePath, "/value"), "ELEMENT"
			n.Children = []*webtemplate.Node{{ID: "interval_value", RMType: rmType, Max: 1, AQLPath: valuePath}}
		})
		leaf := rmattrInterval + "/interval_value"
		at := func(index string) map[string]any {
			return rmattrBody(map[string]any{
				rmattrIntervalEvent + "/time":     "2022-01-12T09:00:11.7842493+01:00",
				leaf + index + "/lower|magnitude": 1.0, leaf + index + "/lower|unit": "Unit",
				leaf + index + "/upper|magnitude": 2.0, leaf + index + "/upper|unit": "Unit",
			})
		}
		if _, err := UnmarshalFlat(mustJSON(t, at(":0")), single, WithStrict()); err != nil {
			t.Fatalf("fixture: UnmarshalFlat (:0) = %v, want the one value decoded", err)
		}
		err := refusalLegs(t, single, at(":1"), nil, ErrUnknownPath)
		if !strings.Contains(err.Error(), leaf+":1") || !strings.Contains(err.Error(), "single-valued") {
			t.Errorf("err = %v, want it to name %q as single-valued", err, leaf+":1")
		}
	})
}

// TestREQ170IsmTransitionReferenceKeysRefusedInEveryMode — REQ-170 / REQ-140.
// The reference's in-context spelling of an ACTION's ISM transition and time
// names RM data the Web Template this SDK builds does not model. It is refused
// by default, under the report option and under strict, naming the key and
// none of its value: a `current_state`, `transition` or `careflow_step` leaf or
// a `_reason` family under the reference's in-context `ism_transition` node; a
// `transition` leaf or `_reason` family under an archetyped
// `ism_transition[atNNNN]` node (FLAT id `transition`), which models
// `current_state` and `careflow_step` but neither of those; and the ACTION's own
// `time` leaf. Each refusal names the attribute it refuses and no
// requirement id. The STRUCTURED form of each body is refused the same way.
func TestREQ170IsmTransitionReferenceKeysRefusedInEveryMode(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	type refusal struct {
		name  string
		keys  []string
		names string
		attr  string
	}
	leaf := func(base, id string) refusal {
		return refusal{id + " leaf under " + base, []string{base + "/" + id + "|code", base + "/" + id + "|value", base + "/" + id + "|terminology"}, base + "/" + id + "|code", "an ACTION's ISM transition " + id}
	}
	reference, archetyped := rmattrAction+"/ism_transition", rmattrAction+"/transition"
	cases := []refusal{
		leaf(reference, "current_state"),
		leaf(reference, "transition"),
		leaf(reference, "careflow_step"),
		leaf(archetyped, "transition"),
		{"bare transition leaf under " + reference, []string{reference + "/transition"}, reference + "/transition", "an ACTION's ISM transition transition"},
		{"bare transition leaf under " + archetyped, []string{archetyped + "/transition"}, archetyped + "/transition", "an ACTION's ISM transition transition"},
		{"reason family under " + reference, []string{reference + "/_reason:0"}, reference + "/_reason:0", "an ACTION's ISM transition reason"},
		{"reason family under " + archetyped, []string{archetyped + "/_reason:0"}, archetyped + "/_reason:0", "an ACTION's ISM transition reason"},
		{"reason family with a suffix", []string{reference + "/_reason|code"}, reference + "/_reason", "an ACTION's ISM transition reason"},
		{"ACTION time leaf", []string{rmattrAction + "/time"}, rmattrAction + "/time", "an ACTION's time"},
		{"ACTION time leaf with a suffix", []string{rmattrAction + "/time|magnitude_status"}, rmattrAction + "/time|magnitude_status", "an ACTION's time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := make(map[string]any, len(tc.keys))
			for _, k := range tc.keys {
				keys[k] = secret
			}
			err := refusedInEveryMode(t, wt, rmattrBody(keys))
			if !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
			if !strings.Contains(err.Error(), strconv.Quote(tc.names)) {
				t.Errorf("err = %v, want it to name %q", err, tc.names)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %v carries the key's value", err)
			}
			if !strings.Contains(err.Error(), tc.attr) || strings.Contains(err.Error(), "REQ-") {
				t.Errorf("err = %v, want it to name %q and no requirement id", err, tc.attr)
			}
			// The STRUCTURED form of the same body reaches the same refusal.
			serr := structuredRefusedInEveryMode(t, wt, rmattrBody(keys))
			if !errors.Is(serr, ErrUnknownPath) || !strings.Contains(serr.Error(), "in-context spelling") ||
				!strings.Contains(serr.Error(), tc.attr) || strings.Contains(serr.Error(), "REQ-") ||
				strings.Contains(serr.Error(), secret) {
				t.Errorf("STRUCTURED err = %v, want the in-context refusal naming %q, no requirement id and no value", serr, tc.attr)
			}
		})
	}
	// Any other key the Web Template does not resolve under those nodes is
	// unmatched content, ignored by default; so is a leaf name that is not the
	// node's last segment.
	for _, key := range []string{
		reference + "/x_note|code",
		reference + "/transition/deeper|code",
		reference + "/current_state/deeper|code",
		archetyped + "/x_note",
		rmattrAction + "/time/deeper",
	} {
		t.Run("unmatched "+key, func(t *testing.T) {
			assertUnmatched(t, wt, rmattrBody(map[string]any{key: "x"}), key)
		})
	}
}

// TestREQ170ArchetypedIsmTransitionStillDecodes — REQ-170 / REQ-053. The
// archetyped `ism_transition[atNNNN]` node's modelled `current_state` and
// `careflow_step` resolve in the Web Template, so they decode, in every mode,
// into the ACTION's ISM transition: only the reference's in-context spelling is
// refused.
func TestREQ170ArchetypedIsmTransitionStillDecodes(t *testing.T) {
	wt, _ := conformanceWT(t)
	body := rmattrBody(ismTransitionKeys("transition", "524", "at0005"))
	for name, opts := range map[string][]Option{"default": nil, "WithStrict": {WithStrict()}} {
		comp, err := UnmarshalFlat(mustJSON(t, body), wt, opts...)
		if err != nil {
			t.Fatalf("UnmarshalFlat (%s): %v", name, err)
		}
		tr := firstAction(t, comp).IsmTransition
		if tr.CurrentState.DefiningCode.CodeString != "524" || tr.CareflowStep == nil || tr.CareflowStep.DefiningCode.CodeString != "at0005" {
			t.Errorf("UnmarshalFlat (%s) ISM transition = %+v, want current_state 524 and careflow_step at0005", name, tr)
		}
	}
}

// TestREQ170ActionTimeNotReplacedByDefault — REQ-170 / REQ-053. With
// WithTemplate the decode fills an absent RM-mandatory ACTION.time from
// ctx/time. A body that carries the ACTION's own time in the reference's
// in-context spelling must not have it replaced by that default: the key is
// refused, in every mode, before the default step runs.
func TestREQ170ActionTimeNotReplacedByDefault(t *testing.T) {
	wt, compiled := conformanceWT(t)
	key := rmattrAction + "/time"
	body := mustJSON(t, rmattrBody(map[string]any{
		rmattrAction + "/dv_text": "action text",
		key:                       "2021-12-21T15:00:00+01:00",
	}))
	for name, opts := range map[string][]Option{
		"default":       nil,
		"WithUnmatched": {WithUnmatched(func(string) {})},
		"WithStrict":    {WithStrict()},
	} {
		comp, err := UnmarshalFlat(body, wt, append(opts, WithTemplate(compiled))...)
		if err == nil {
			t.Fatalf("UnmarshalFlat (%s, WithTemplate) decoded, ACTION.time = %q — the body's own time is replaced by the ctx/time default", name, firstAction(t, comp).Time.Value)
		}
		if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), strconv.Quote(key)) {
			t.Errorf("UnmarshalFlat (%s, WithTemplate) = %v, want ErrUnknownPath naming %q", name, err, key)
		}
	}
}

// TestREQ170TwoSpellingsOfABareValue — REQ-170 / REQ-053. `x|` spells the bare
// value of `x` (the empty suffix), so a body carrying both carries two
// spellings of one field: the same value is read once, and differing values are
// refused in every mode, naming both keys and neither value. The OPT-free
// interconversion reads them the same way, and STRUCTURED's `"|"` member is the
// same bare value as a bare scalar element.
func TestREQ170TwoSpellingsOfABareValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	bareOnly := rmattrBody(map[string]any{rmattrText: "text value"})
	ref, err := UnmarshalFlat(mustJSON(t, bareOnly), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (bare value alone): %v", err)
	}

	t.Run("equal values decode once", func(t *testing.T) {
		body := rmattrBody(map[string]any{rmattrText: "text value", rmattrText + "|": "text value"})
		for name, opts := range map[string][]Option{"default": nil, "WithStrict": {WithStrict()}} {
			got, err := UnmarshalFlat(mustJSON(t, body), wt, opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat (%s) = %v, want both spellings read as one", name, err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
				t.Errorf("UnmarshalFlat (%s) value differs from the bare value alone", name)
			}
		}
		s, err := FlatToStructured(mustJSON(t, body))
		if err != nil {
			t.Fatalf("FlatToStructured = %v, want both spellings read as one", err)
		}
		want, err := FlatToStructured(mustJSON(t, bareOnly))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(s, want) {
			t.Errorf("FlatToStructured = %s, want %s", s, want)
		}
	})

	t.Run("differing values are refused", func(t *testing.T) {
		const secret = "secret-value-0451"
		body := rmattrBody(map[string]any{rmattrText: "text value", rmattrText + "|": secret})
		err := refusedInEveryMode(t, wt, body)
		if !errors.Is(err, ErrUnknownPath) {
			t.Errorf("err = %v, want ErrUnknownPath", err)
		}
		for _, k := range []string{rmattrText, rmattrText + "|"} {
			if !strings.Contains(err.Error(), strconv.Quote(k)) {
				t.Errorf("err = %v, want it to name %q", err, k)
			}
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("err = %v carries a value", err)
		}
		if _, err := FlatToStructured(mustJSON(t, body)); !errors.Is(err, ErrUnknownPath) {
			t.Errorf("FlatToStructured = %v, want ErrUnknownPath", err)
		}
	})

	t.Run("STRUCTURED bare member and bare scalar agree", func(t *testing.T) {
		s, err := FlatToStructured(mustJSON(t, bareOnly))
		if err != nil {
			t.Fatal(err)
		}
		scalar, err := UnmarshalStructured(s, wt)
		if err != nil {
			t.Fatalf("UnmarshalStructured (scalar element): %v", err)
		}
		member := bytes.Replace(s, []byte(`"dv_text":["text value"]`), []byte(`"dv_text":[{"|":"text value"}]`), 1)
		if bytes.Equal(member, s) {
			t.Fatalf("fixture: no bare dv_text element in %s", s)
		}
		got, err := UnmarshalStructured(member, wt, WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalStructured (\"|\" member) = %v", err)
		}
		if !bytes.Equal(canonical(t, got), canonical(t, scalar)) || !bytes.Equal(canonical(t, got), canonical(t, ref)) {
			t.Error(`the "|" member decodes differently from the bare scalar element`)
		}
	})
}

// orderedFlat writes body as a FLAT document whose first keys are first, in
// that order, and whose other keys follow sorted, so a test controls the
// document order the report and strict follow.
func orderedFlat(t *testing.T, body map[string]any, first ...string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("{")
	rest := maps.Clone(body)
	for _, k := range first {
		delete(rest, k)
	}
	for i, k := range append(slices.Clone(first), slices.Sorted(maps.Keys(rest))...) {
		v, ok := body[k]
		if !ok {
			t.Fatalf("fixture: key %q is not in the body", k)
		}
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s:%s", mustJSON(t, k), mustJSON(t, v))
	}
	b.WriteString("}")
	return []byte(b.String())
}

// TestREQ170LoneEmptySuffixIsTheBareValue — REQ-170 / REQ-053. A lone `x|`
// spells the bare value of `x`, so the decode reads it wherever it reads `x`:
// `ctx/time|` sets the start time and `ctx/composer_name|` names the composer
// as their bare spellings do, in every mode, and the report receives nothing.
func TestREQ170LoneEmptySuffixIsTheBareValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		key   string
		value string
	}{
		{"ctx/time", "2020-02-03T04:05:06+01:00"},
		{"ctx/composer_name", "Dr Lone Pipe"},
		{rmattrText, "lone pipe text"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			bare := rmattrBody(map[string]any{tc.key: tc.value})
			ref := canonical(t, decodeRMAttr(t, wt, bare))
			if !bytes.Contains(ref, []byte(tc.value)) {
				t.Fatalf("fixture: %q does not reach the value %s", tc.key, ref)
			}
			piped := rmattrBody(nil)
			delete(piped, tc.key)
			piped[tc.key+"|"] = tc.value
			var reported []string
			for name, opts := range map[string][]Option{
				"default":       nil,
				"WithUnmatched": {WithUnmatched(func(p string) { reported = append(reported, p) })},
				"WithStrict":    {WithStrict()},
			} {
				got, err := UnmarshalFlat(mustJSON(t, piped), wt, opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat (%s) = %v, want %q read as %q", name, err, tc.key+"|", tc.key)
				}
				if !bytes.Equal(canonical(t, got), ref) {
					t.Errorf("UnmarshalFlat (%s) value differs from the bare spelling:\n got %s\nwant %s", name, canonical(t, got), ref)
				}
			}
			if len(reported) != 0 {
				t.Errorf("WithUnmatched reported %q, want nothing", reported)
			}
		})
	}
}

// TestREQ170SpellingPairsJudgedWhereRead — REQ-170. Two spellings of one key
// are two spellings of one field only where the decode reads that field, and
// only there must their values agree. Two spellings of a key the decode does
// not read are two unmatched keys, each reported as written, whatever their
// values: a key the Web Template does not resolve with and without its bare
// `|`, an unknown ctx/ field likewise, and a sub-path a party family does not
// read with and without its :0.
func TestREQ170SpellingPairsJudgedWhereRead(t *testing.T) {
	wt, _ := conformanceWT(t)
	const facility = rmattrContext + "/_health_care_facility"
	for _, tc := range []struct {
		name string
		read map[string]any
		keys map[string]any
	}{
		{"unresolved node", nil, map[string]any{
			rmattrObs + "/no_such_node":  "a",
			rmattrObs + "/no_such_node|": "b",
		}},
		{"unknown ctx field", nil, map[string]any{"ctx/x_flag": "a", "ctx/x_flag|": "b"}},
		{"party sub-path suffix", map[string]any{facility + "|name": "Hospital"}, map[string]any{
			facility + "/relationship|x_ext":   "a",
			facility + "/relationship:0|x_ext": "b",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := rmattrBody(tc.read)
			maps.Copy(body, tc.keys)
			assertUnmatched(t, wt, body, slices.Sorted(maps.Keys(tc.keys))...)
		})
	}

	// Where the family reads the sub-path, the same value is read once and
	// differing values are refused in every mode, naming both keys.
	read := func(second string) map[string]any {
		return rmattrBody(map[string]any{
			facility + "|name":                     "Hospital",
			facility + "/relationship|code":        "10",
			facility + "/relationship|terminology": "openehr",
			facility + "/relationship|value":       "mother",
			facility + "/relationship:0|code":      second,
		})
	}
	one := rmattrBody(map[string]any{
		facility + "|name":                     "Hospital",
		facility + "/relationship|code":        "10",
		facility + "/relationship|terminology": "openehr",
		facility + "/relationship|value":       "mother",
	})
	got := decodeRMAttr(t, wt, read("10"))
	if !bytes.Equal(canonical(t, got), canonical(t, decodeRMAttr(t, wt, one))) {
		t.Error("two equal spellings of a read sub-path decode differently from one")
	}
	err := refusedInEveryMode(t, wt, read("11"))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
	}
	for _, k := range []string{facility + "/relationship|code", facility + "/relationship:0|code"} {
		if !strings.Contains(err.Error(), strconv.Quote(k)) {
			t.Errorf("err = %v, want it to name %q", err, k)
		}
	}
}

// TestREQ170StrictNamesTheFirstSpellingWritten — REQ-170: strict names the
// first unmatched item in document order. Where `x|` and `x` carry the same
// value and the decode does not read `x`, both are unmatched, so a body that
// writes `x|` first is refused naming `x|`, and the report receives both in the
// order written.
func TestREQ170StrictNamesTheFirstSpellingWritten(t *testing.T) {
	wt, _ := conformanceWT(t)
	bare := rmattrObs + "/no_such_node"
	body := rmattrBody(map[string]any{bare: "same", bare + "|": "same"})
	b := orderedFlat(t, body, bare+"|", bare)

	_, err := UnmarshalFlat(b, wt, WithStrict())
	if !errors.Is(err, ErrUnknownPath) || err.Error() != strictRefusal(bare+"|") {
		t.Errorf("UnmarshalFlat (WithStrict) = %v, want %q", err, strictRefusal(bare+"|"))
	}
	var reported []string
	if _, err := UnmarshalFlat(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("UnmarshalFlat (WithUnmatched) = %v", err)
	}
	if want := []string{bare + "|", bare}; !slices.Equal(reported, want) {
		t.Errorf("WithUnmatched reported %q, want %q", reported, want)
	}
}

// TestREQ170StrictWithReportPassesTheFirstPath — REQ-170. With both options,
// strict refuses at the first unmatched item: the function receives that one
// path and no other, and the error is the strict refusal naming it.
func TestREQ170StrictWithReportPassesTheFirstPath(t *testing.T) {
	wt, _ := conformanceWT(t)
	body, unmatched := req170Body()
	first := slices.Sorted(slices.Values(unmatched))[0]
	var reported []string
	_, err := UnmarshalFlat(mustJSON(t, body), wt, WithStrict(), WithUnmatched(func(p string) { reported = append(reported, p) }))
	if !errors.Is(err, ErrUnknownPath) || err.Error() != strictRefusal(first) {
		t.Errorf("UnmarshalFlat (WithStrict, WithUnmatched) = %v, want %q", err, strictRefusal(first))
	}
	if !slices.Equal(reported, []string{first}) {
		t.Errorf("WithUnmatched under WithStrict received %q, want exactly [%q]", reported, first)
	}
}

// TestREQ170LoneBareValueIsAMissingSuffix — REQ-170 refuses in every mode
// "matched content that is malformed: … a missing required suffix". A bare
// value alone at a leaf whose datatype has no bare spelling is that datatype
// missing every suffix it requires, and a bare ctx/setting is the setting
// missing its |code and |value: both fail by default, under the report option
// and under strict, naming no value. Beside a suffix the datatype reads,
// DV_PROPORTION's derived bare magnitude stays unmatched content.
func TestREQ170LoneBareValueIsAMissingSuffix(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	quantity := rmattrBody(map[string]any{rmattrElement: secret})
	delete(quantity, rmattrElement+"|magnitude")
	delete(quantity, rmattrElement+"|unit")
	for name, body := range map[string]map[string]any{
		"DV_CODED_TEXT": rmattrBody(map[string]any{rmattrCodedText: secret}),
		"DV_QUANTITY":   quantity,
		"DV_IDENTIFIER": rmattrBody(map[string]any{rmattrEvent + "/dv_identifier": secret}),
		"ctx/setting":   rmattrBody(map[string]any{"ctx/setting": secret}),
	} {
		t.Run(name, func(t *testing.T) {
			err := refusedInEveryMode(t, wt, body)
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %v carries the value", err)
			}
		})
	}
	t.Run("DV_PROPORTION bare beside its numerator", func(t *testing.T) {
		assertUnmatched(t, wt, rmattrBody(map[string]any{
			rmattrProportion + "|numerator":   1,
			rmattrProportion + "|denominator": 2,
			rmattrProportion + "|type":        0,
			rmattrProportion:                  0.5,
		}), rmattrProportion)
	})
}

// TestREQ053CtxOwnedMetadataSuffixRefused — REQ-053 / REQ-170. COMPOSITION
// `language` and `territory` are owned by their ctx/ short forms, which carry
// the code alone: a suffix their CODE_PHRASE has but the ctx/ form cannot carry
// is refused in every mode with ErrUnsupportedDatatype, naming the key and the
// owning ctx/ form and no value — not as a CODE_PHRASE missing its `|code`. A
// suffix outside CODE_PHRASE's set is unmatched content.
func TestREQ053CtxOwnedMetadataSuffixRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	for _, field := range []string{"language", "territory"} {
		t.Run(field, func(t *testing.T) {
			key := rmattrRoot + "/" + field + "|preferred_term"
			code := "en"
			if field == "territory" {
				code = "US"
			}
			err := refusedInEveryMode(t, wt, rmattrBody(map[string]any{
				rmattrRoot + "/" + field + "|code": code,
				key:                                secret,
			}))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
			for _, want := range []string{strconv.Quote(key), "ctx/" + field} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to name %s", err, want)
				}
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "missing required") {
				t.Errorf("err = %v, want the ctx/ ownership refusal and no value", err)
			}
			unmatched := rmattrRoot + "/" + field + "|x_note"
			assertUnmatched(t, wt, rmattrBody(map[string]any{unmatched: "x"}), unmatched)
		})
	}
}

// TestREQ170RawDecorationIgnoresUnmatchedKey — REQ-170. A `|raw` fragment is
// placed by reference and a value decoration is then written into it, so a
// second decode pass must not see the first pass's writes: a body carrying a
// `|raw` value, a decoration on it and one unrelated unmatched key decodes, by
// default and under the report option, to the value of the body without that
// key, and strict refuses only the key.
func TestREQ170RawDecorationIgnoresUnmatchedKey(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{"dv_count raw with a normal range", map[string]any{
			rmattrCount + "|raw":                 map[string]any{"_type": "DV_COUNT", "magnitude": 3},
			rmattrCount + "/_normal_range/lower": 1,
			rmattrCount + "/_normal_range/upper": 5,
		}},
		{"dv_quantity raw with a normal range", map[string]any{
			rmattrElement + "|raw":                           map[string]any{"_type": "DV_QUANTITY", "magnitude": 65.9, "units": "unit"},
			rmattrElement + "/_normal_range/lower|magnitude": 1.0,
			rmattrElement + "/_normal_range/lower|unit":      "unit",
			rmattrElement + "/_normal_range|upper_unbounded": true,
		}},
		{"dv_text raw with a mapping", map[string]any{
			rmattrText + "|raw":                           map[string]any{"_type": "DV_TEXT", "value": "text value"},
			rmattrText + "/_mapping:0|match":              "=",
			rmattrText + "/_mapping:0/target|code":        "Z.1",
			rmattrText + "/_mapping:0/target|terminology": "ICD10",
		}},
		{"dv_date_time raw with an accuracy", map[string]any{
			rmattrDateTime + "|raw":       map[string]any{"_type": "DV_DATE_TIME", "value": "2022-01-12T13:22:34+01:00"},
			rmattrDateTime + "/_accuracy": "P1D",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := rmattrBody(tc.keys)
			if _, raw := tc.keys[rmattrElement+"|raw"]; raw {
				delete(body, rmattrElement+"|magnitude")
				delete(body, rmattrElement+"|unit")
			}
			body["ctx/x_flag"] = "x"
			assertUnmatched(t, wt, body, "ctx/x_flag")
		})
	}
}

// TestREQ170UnmappedLeafSplitsRefusedFromUnmatched — REQ-170, which refuses in
// every mode, "at a leaf whose datatype has neither a suffix set nor a sub-path
// grammar, its bare value and a suffix that names an attribute of its RM
// class". Such a leaf carries nothing but `|raw`; any other suffix or sub-path
// is unmatched content. An attribute of its RM class is refused with
// ErrUnsupportedDatatype as the other carrier-less keys at a leaf are. The
// composer is not such a leaf: its real path has the party grammar (ADR 0029),
// under which a suffix or a sub-path outside the grammar is unmatched content
// too. An unparameterised `DV_INTERVAL` leaf has a sub-path grammar, so the
// rule does not reach it.
func TestREQ170UnmappedLeafSplitsRefusedFromUnmatched(t *testing.T) {
	wt, _ := conformanceWT(t)
	t.Run("composer suffix the party grammar lacks, beside a name", func(t *testing.T) {
		key := rmattrRoot + "/composer|x_note"
		assertUnmatched(t, wt, rmattrBody(map[string]any{rmattrRoot + "/composer|name": "Dr. Who", key: "x"}), key)
	})
	t.Run("composer sub-path outside the party sub-structure", func(t *testing.T) {
		key := rmattrRoot + "/composer/x_foo|bar"
		assertUnmatched(t, wt, rmattrBody(map[string]any{key: "x"}), key)
	})

	state := &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "state", RMType: "DV_STATE", NodeID: "at0002", Max: 1,
			AQLPath: "/context/other_context[at0001]/items[at0002]/value",
		}},
	}}
	ctx := map[string]any{"ctx/language": "en", "ctx/territory": "NL"}
	t.Run("an attribute of the unmapped datatype", func(t *testing.T) {
		body := maps.Clone(ctx)
		body["root/state|is_terminal"] = true
		err := refusedInEveryMode(t, state, body)
		if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"root/state|is_terminal"`) {
			t.Errorf("err = %v, want ErrUnsupportedDatatype naming root/state|is_terminal", err)
		}
	})
	t.Run("the unmapped datatype's bare value", func(t *testing.T) {
		body := maps.Clone(ctx)
		body["root/state"] = "x"
		if err := refusedInEveryMode(t, state, body); !errors.Is(err, ErrUnsupportedDatatype) {
			t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
		}
	})
	t.Run("a suffix the unmapped datatype lacks", func(t *testing.T) {
		body := maps.Clone(ctx)
		body["root/state|x_note"] = "x"
		assertUnmatched(t, state, body, "root/state|x_note")
	})

	interval := &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{{
			ID: "range", RMType: "DV_INTERVAL", NodeID: "at0002", Max: 1,
			AQLPath: "/context/other_context[at0001]/items[at0002]/value",
		}},
	}}
	t.Run("an unparameterised DV_INTERVAL leaf has a sub-path grammar", func(t *testing.T) {
		body := maps.Clone(ctx)
		body["root/range|lower_unbounded"] = true
		body["root/range"] = "x"
		assertUnmatched(t, interval, body, "root/range", "root/range|lower_unbounded")
	})
}

// TestREQ170FamilyIndexJudgedBeforeItsTails — REQ-170. A key is judged from the
// left: a `_family:N` index past the family's bound makes the key malformed
// whatever follows it, so `_uid:1|x_note` is refused in every mode as
// `language:1|x_note` is. A list family's index within its bound is no
// refusal, so `_link:3|x_note` is unmatched content like any unknown suffix.
func TestREQ170FamilyIndexJudgedBeforeItsTails(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{rmattrObs + "/_uid:1|x_note", rmattrObs + "/_link:10001|x_note", rmattrObs + "/language:1|x_note"} {
		t.Run("refused "+key, func(t *testing.T) {
			if err := refusedInEveryMode(t, wt, rmattrBody(map[string]any{key: "x"})); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
	t.Run("unmatched _link:3|x_note", func(t *testing.T) {
		key := rmattrObs + "/_link:3|x_note"
		assertUnmatched(t, wt, rmattrBody(map[string]any{key: "x"}), key)
	})
}

// TestREQ170IntervalEventAndObservationAttrsRefused — REQ-170 / REQ-140. An
// INTERVAL_EVENT's `math_function`, `width` and `sample_count` and an
// OBSERVATION's `history_origin`, in the reference's spelling — a leaf under
// the node or a suffix on it — name RM data the Web Template this SDK builds
// does not model. Each is refused by default, under the report option and
// under strict, naming the key and the attribute, and none of its value and no
// requirement id, in FLAT and in its STRUCTURED form. Where the Web Template
// does resolve one, it decodes as before.
func TestREQ170IntervalEventAndObservationAttrsRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	for _, tc := range []struct {
		keys        []string
		names, attr string
	}{
		{[]string{rmattrEvent + "/math_function|code", rmattrEvent + "/math_function|value", rmattrEvent + "/math_function|terminology"}, rmattrEvent + "/math_function|code", "an INTERVAL_EVENT's math_function"},
		{[]string{rmattrEvent + "/width"}, rmattrEvent + "/width", "an INTERVAL_EVENT's width"},
		{[]string{rmattrEvent + "/sample_count"}, rmattrEvent + "/sample_count", "an INTERVAL_EVENT's sample_count"},
		{[]string{rmattrEvent + "|sample_count"}, rmattrEvent + "|sample_count", "an INTERVAL_EVENT's sample_count"},
		{[]string{rmattrEvent + "|math_function"}, rmattrEvent + "|math_function", "an INTERVAL_EVENT's math_function"},
		{[]string{rmattrEvent + "|width"}, rmattrEvent + "|width", "an INTERVAL_EVENT's width"},
		{[]string{rmattrObs + "/history_origin"}, rmattrObs + "/history_origin", "an OBSERVATION's history_origin"},
		{[]string{rmattrObs + "|history_origin"}, rmattrObs + "|history_origin", "an OBSERVATION's history_origin"},
	} {
		t.Run(tc.names, func(t *testing.T) {
			keys := make(map[string]any, len(tc.keys))
			for _, k := range tc.keys {
				keys[k] = secret
			}
			err := refusedInEveryMode(t, wt, rmattrBody(keys))
			if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), strconv.Quote(tc.names)) {
				t.Errorf("err = %v, want ErrUnknownPath naming %q", err, tc.names)
			}
			// The refusal names the attribute, and no requirement id.
			if !strings.Contains(err.Error(), tc.attr) || strings.Contains(err.Error(), "REQ-") {
				t.Errorf("err = %v, want it to name %q and no requirement id", err, tc.attr)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("err = %v carries the key's value", err)
			}
			// The STRUCTURED form of the same body reaches the same refusal.
			serr := structuredRefusedInEveryMode(t, wt, rmattrBody(keys))
			if !errors.Is(serr, ErrUnknownPath) || !strings.Contains(serr.Error(), "in-context spelling") ||
				!strings.Contains(serr.Error(), tc.attr) || strings.Contains(serr.Error(), "REQ-") ||
				strings.Contains(serr.Error(), secret) {
				t.Errorf("STRUCTURED err = %v, want the in-context refusal naming %q, no requirement id and no value", serr, tc.attr)
			}
		})
	}
	// Any other unresolved key under those nodes stays unmatched content.
	for _, key := range []string{rmattrEvent + "/x_note", rmattrEvent + "|x_note", rmattrObs + "/width"} {
		t.Run("unmatched "+key, func(t *testing.T) {
			assertUnmatched(t, wt, rmattrBody(map[string]any{key: "x"}), key)
		})
	}
	// A key that walks through one of several reused siblings is left to the
	// refusal that path gets, before any in-context rule could name it.
	t.Run("keys under a reused sibling", func(t *testing.T) {
		reused := twoReusedObservationWT()
		for _, key := range []string{"root/a/history_origin", "root/a/any_event:0/width"} {
			err := refusedInEveryMode(t, reused, map[string]any{"ctx/language": "en", "ctx/territory": "NL", key: "x"})
			if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "reused siblings") {
				t.Errorf("%s: err = %v, want the reused-siblings refusal", key, err)
			}
		}
	})
	t.Run("width decodes where the Web Template models it", func(t *testing.T) {
		interval := withIntervalEventWidth(t, wt)
		body := rmattrBody(map[string]any{rmattrEvent + "/width": "PT1H"})
		comp, err := UnmarshalFlat(mustJSON(t, body), interval, WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		ev, ok := firstObservation(t, comp).Data.Events[0].(*rm.IntervalEvent[rm.ItemStructure])
		if !ok || ev.Width.Value != "PT1H" {
			t.Errorf("event = %#v, want an INTERVAL_EVENT of width PT1H", firstObservation(t, comp).Data.Events[0])
		}
	})
}

// withIntervalEventWidth returns a copy of the corpus Web Template whose
// conformance_observation event is an INTERVAL_EVENT with a `width` leaf, the
// nodes on that path copied so the shared template is left as it is.
func withIntervalEventWidth(t *testing.T, wt *webtemplate.WebTemplate) *webtemplate.WebTemplate {
	t.Helper()
	return editWTNode(t, wt, rmattrEvent, func(node *webtemplate.Node) {
		node.RMType = "INTERVAL_EVENT"
		node.Children = append(slices.Clone(node.Children), &webtemplate.Node{
			ID: "width", RMType: "DV_DURATION", Max: 1, AQLPath: node.AQLPath + "/width",
		})
	})
}

// editWTNode returns a copy of wt in which edit has changed the node at the
// FLAT path flat (indexes ignored), copying every node on the way down so wt
// itself is untouched.
func editWTNode(t *testing.T, wt *webtemplate.WebTemplate, flat string, edit func(*webtemplate.Node)) *webtemplate.WebTemplate {
	t.Helper()
	ids := strings.Split(strings.TrimPrefix(flat, rmattrRoot+"/"), "/")
	out := *wt
	tree := *wt.Tree
	out.Tree = &tree
	node := out.Tree
	for _, id := range ids {
		id, _, _ = strings.Cut(id, ":")
		kids := slices.Clone(node.Children)
		i := slices.IndexFunc(kids, func(n *webtemplate.Node) bool { return n.ID == id })
		if i < 0 {
			t.Fatalf("corpus template has no %q under %q", id, node.ID)
		}
		cp := *kids[i]
		kids[i] = &cp
		node.Children = kids
		node = &cp
	}
	edit(node)
	return &out
}

// twoReusedObservationWT is a minimal Web Template with two reused OBSERVATION
// siblings — one archetype id, distinct pinned names 'A'/'B' — each holding an
// EVENT, the shape [twoSiblingWT] gives EVALUATIONs.
func twoReusedObservationWT() *webtemplate.WebTemplate {
	obs := func(id, pin string) *webtemplate.Node {
		base := "/content[openEHR-EHR-OBSERVATION.test.v1,'" + pin + "']"
		return &webtemplate.Node{
			ID: id, RMType: "OBSERVATION", NodeID: "openEHR-EHR-OBSERVATION.test.v1", Max: 1, AQLPath: base,
			Children: []*webtemplate.Node{{
				ID: "any_event", RMType: "EVENT", NodeID: "at0002", Max: -1,
				AQLPath: base + "/data[at0001]/events[at0002]",
			}},
		}
	}
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{obs("a", "A"), obs("b", "B")},
	}}
}

// TestREQ170FeederAuditSlotSpellingsMerge — REQ-170 / REQ-053. Inside a
// `_feeder_audit`, the index-less spelling of a sub-path segment and its `:0`
// name one slot, so their keys merge into one instance and each key keeps the
// spelling it was written in. A key under both spellings that the decode does
// not read is two unmatched keys, reported as written; two spellings of one
// field it reads must agree, judged after the decode: the same value is read
// once, and differing values are refused in every mode, naming both keys.
func TestREQ170FeederAuditSlotSpellingsMerge(t *testing.T) {
	wt, _ := conformanceWT(t)
	const audit = rmattrObs + "/_feeder_audit/originating_system_audit"

	t.Run("the two spellings of one slot merge", func(t *testing.T) {
		merged := decodeRMAttr(t, wt, rmattrBody(map[string]any{
			audit + "|system_id":    "orig",
			audit + ":0|version_id": "v1",
		}))
		one := decodeRMAttr(t, wt, rmattrBody(map[string]any{
			audit + "|system_id":  "orig",
			audit + "|version_id": "v1",
		}))
		if !bytes.Equal(canonical(t, merged), canonical(t, one)) {
			t.Errorf("the two slot spellings decode differently from one:\n got %s\nwant %s", canonical(t, merged), canonical(t, one))
		}
	})

	t.Run("an unread key under both spellings is two unmatched keys", func(t *testing.T) {
		body := rmattrBody(map[string]any{
			audit + "|system_id":     "orig",
			audit + "/x_unknown|a":   "1",
			audit + ":0/x_unknown|a": "2",
		})
		assertUnmatched(t, wt, body, audit+"/x_unknown|a", audit+":0/x_unknown|a")
	})

	t.Run("a read field under both spellings must agree", func(t *testing.T) {
		same := decodeRMAttr(t, wt, rmattrBody(map[string]any{audit + "|system_id": "orig", audit + ":0|system_id": "orig"}))
		one := decodeRMAttr(t, wt, rmattrBody(map[string]any{audit + "|system_id": "orig"}))
		if !bytes.Equal(canonical(t, same), canonical(t, one)) {
			t.Error("two equal spellings of system_id decode differently from one")
		}
		const secret = "secret-value-0451"
		err := refusedInEveryMode(t, wt, rmattrBody(map[string]any{audit + "|system_id": "orig", audit + ":0|system_id": secret}))
		if !errors.Is(err, ErrUnsupportedDatatype) {
			t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
		}
		for _, k := range []string{audit + "|system_id", audit + ":0|system_id"} {
			if !strings.Contains(err.Error(), strconv.Quote(k)) {
				t.Errorf("err = %v, want it to name %q", err, k)
			}
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("err = %v carries a value", err)
		}
	})
}

// structuredObject returns the object at member path p of a STRUCTURED
// document: "" is the document itself, "ctx" and "ctx/<field>" the ctx object
// and a field's object, and any other path alternates member names and array
// positions from the template root.
func structuredObject(t *testing.T, doc map[string]any, p string) map[string]any {
	t.Helper()
	if p == "" {
		return doc
	}
	if field, inCtx := strings.CutPrefix(p, "ctx"); inCtx {
		obj := doc["ctx"].(map[string]any)
		if field != "" {
			obj = obj[strings.TrimPrefix(field, "/")].(map[string]any)
		}
		return obj
	}
	cur := doc[rmattrRoot].(map[string]any)
	segs := strings.Split(strings.TrimPrefix(strings.TrimPrefix(p, rmattrRoot), "/"), "/")
	if segs[0] == "" {
		return cur
	}
	for i := 0; i+1 < len(segs); i += 2 {
		n, err := strconv.Atoi(segs[i+1])
		if err != nil {
			t.Fatalf("fixture: member path %q", p)
		}
		cur = cur[segs[i]].([]any)[n].(map[string]any)
	}
	return cur
}

// structuredLevels are one object at each level of a STRUCTURED body the
// REQ-170 rule judges, each with the FLAT keys that make the object exist.
var structuredLevels = func() []struct {
	name  string
	extra map[string]any
	path  string
} {
	section := rmattrRoot + "/conformance_section/0"
	obs := section + "/conformance_observation/0"
	quantity := obs + "/any_event/0/dv_quantity/0"
	interval := map[string]any{
		rmattrIntervalEvent + "/time":       "2022-01-12T09:00:11.7842493+01:00",
		rmattrInterval + "/lower|magnitude": 1.0, rmattrInterval + "/lower|unit": "Unit",
		rmattrInterval + "/upper|magnitude": 2.0, rmattrInterval + "/upper|unit": "Unit",
	}
	party := map[string]any{
		rmattrObs + "/subject|name":                     "Silvia Blake",
		rmattrObs + "/subject/relationship|code":        "10",
		rmattrObs + "/subject/relationship|terminology": "openehr",
		rmattrObs + "/subject/relationship|value":       "mother",
	}
	identifier := map[string]any{
		rmattrObs + "/subject|name":                   "Silvia Blake",
		rmattrObs + "/subject/_identifier:0|id":       "122",
		rmattrObs + "/subject/_identifier:0|issuer":   "issuer",
		rmattrObs + "/subject/_identifier:0|assigner": "assigner",
		rmattrObs + "/subject/_identifier:0|type":     "type",
	}
	normalRange := map[string]any{
		rmattrElement + "/_normal_range/lower|magnitude": 20.5,
		rmattrElement + "/_normal_range/lower|unit":      "unit",
		rmattrElement + "/_normal_range/upper|magnitude": 66.6,
		rmattrElement + "/_normal_range/upper|unit":      "unit",
	}
	feeder := map[string]any{
		rmattrObs + "/_feeder_audit/originating_system_audit|system_id":     "orig",
		rmattrObs + "/_feeder_audit/originating_system_audit/provider|name": "Per 1",
	}
	return []struct {
		name  string
		extra map[string]any
		path  string
	}{
		{"top level", nil, ""},
		{"template root", nil, rmattrRoot},
		{"ctx object", nil, "ctx"},
		{"ctx field", map[string]any{"ctx/setting|code": "238", "ctx/setting|value": "other care"}, "ctx/setting"},
		{"node with children", nil, section},
		{"leaf", nil, quantity},
		{"interval leaf", interval, section + "/conformance_interval/0/any_event/0/interval_dv_quantity/0"},
		{"interval leaf's lower", interval, section + "/conformance_interval/0/any_event/0/interval_dv_quantity/0/lower/0"},
		{"party leaf", party, obs + "/subject/0"},
		{"party leaf's relationship", party, obs + "/subject/0/relationship/0"},
		{"party leaf's _identifier", identifier, obs + "/subject/0/_identifier/0"},
		{"underscore family of a leaf", normalRange, quantity + "/_normal_range/0"},
		{"sub-path inside a family", normalRange, quantity + "/_normal_range/0/lower/0"},
		{"FEEDER_AUDIT_DETAILS inside a family", feeder, obs + "/_feeder_audit/0/originating_system_audit/0"},
		{"party inside a family", feeder, obs + "/_feeder_audit/0/originating_system_audit/0/provider/0"},
	}
}()

// TestREQ170StructuredOneRuleAtEveryLevel — REQ-170. At every level of a
// STRUCTURED body one rule holds: a member whose name the FLAT grammar does
// not read at that level is unmatched content "whatever the JSON shape of a
// STRUCTURED member's value". A scalar, an object and an array under such a
// name are each skipped by default, reported by the member's path and refused
// under strict, wrapping ErrUnknownPath and carrying no value. A name that
// spells more than one FLAT piece (a `:`, `|` or `/`) is judged by its base.
func TestREQ170StructuredOneRuleAtEveryLevel(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	shapes := []struct {
		name  string
		value func() any
	}{
		{"scalar", func() any { return secret }},
		{"object", func() any { return map[string]any{"y": secret} }},
		{"array", func() any { return []any{secret} }},
	}
	for _, level := range structuredLevels {
		// An unknown name, and an unknown underscore name, which names no
		// family the level carries.
		for _, name := range []string{"x_member", "_x_member"} {
			for _, shape := range shapes {
				t.Run(level.name+"/"+name+"/"+shape.name, func(t *testing.T) {
					ref, doc := structuredDoc(t, rmattrBody(level.extra))
					structuredObject(t, doc, level.path)[name] = shape.value()
					want := strings.TrimPrefix(level.path+"/"+name, "/")
					assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, want)
				})
			}
		}
	}
	quantity := rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity/0"
	// A member the level would read elsewhere: a family the leaf's datatype
	// does not carry (DV_TEXT has no normal_range).
	t.Run("_normal_range under a DV_TEXT leaf", func(t *testing.T) {
		text := map[string]any{rmattrText: "text value", rmattrText + "|formatting": "plain"}
		ref, doc := structuredDoc(t, rmattrBody(text))
		at := rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_text/0"
		structuredObject(t, doc, at)["_normal_range"] = secret
		assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, at+"/_normal_range")
	})
	// A suffix member the decode does not read is reported down to the
	// member: the bare "|" beside the suffixes DV_QUANTITY reads, and a
	// suffix of a ctx field the decode has no key for.
	t.Run("bare value member beside read suffixes", func(t *testing.T) {
		ref, doc := structuredDoc(t, rmattrBody(nil))
		structuredObject(t, doc, quantity)["|"] = 5
		assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, quantity+"/|")
	})
	t.Run("suffix member on a ctx field", func(t *testing.T) {
		setting := map[string]any{"ctx/setting|code": "238", "ctx/setting|value": "other care"}
		ref, doc := structuredDoc(t, rmattrBody(setting))
		structuredObject(t, doc, "ctx/setting")["|x"] = secret
		assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, "ctx/setting/|x")
	})
	// A name of several FLAT pieces whose base reads nothing.
	for _, name := range []string{"x_ext:0", "x|y", "x/y"} {
		for _, at := range []string{rmattrRoot, rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity/0"} {
			for _, shape := range shapes {
				t.Run(name+" under "+at+"/"+shape.name, func(t *testing.T) {
					ref, doc := structuredDoc(t, rmattrBody(nil))
					structuredObject(t, doc, at)[name] = shape.value()
					assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, at+"/"+name)
				})
			}
		}
	}
	// What a level holds only unmatched members of is absent, not empty.
	t.Run("element holding only unmatched members", func(t *testing.T) {
		ref, doc := structuredDoc(t, rmattrBody(nil))
		root := doc[rmattrRoot].(map[string]any)
		root["conformance_section"] = append(root["conformance_section"].([]any), map[string]any{"x_only": secret})
		assertStructuredUnmatched(t, wt, mustJSON(t, doc), ref, secret, rmattrRoot+"/conformance_section/1/x_only")
	})
}

// TestREQ170StructuredReadNamesKeepShapeRefusals — REQ-170. A STRUCTURED member
// whose name the FLAT grammar reads is matched content, so a value of the
// wrong shape under it is refused in every mode, the report receiving
// nothing: at the template root and at a node with children (a Web Template
// child), at a leaf (an owned underscore family), at a leaf's sub-path, inside
// a family, and a name of several FLAT pieces whose base reads something.
func TestREQ170StructuredReadNamesKeepShapeRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	extra := make(map[string]map[string]any)
	for _, l := range structuredLevels {
		extra[l.name] = l.extra
	}
	obs := rmattrRoot + "/conformance_section/0/conformance_observation/0"
	quantity := obs + "/any_event/0/dv_quantity/0"
	for _, tc := range []struct{ level, at, name string }{
		{"template root", rmattrRoot, "conformance_section"},
		{"template root", rmattrRoot, "conformance_section:0"},
		{"node with children", rmattrRoot + "/conformance_section/0", "conformance_observation"},
		{"node with children", obs, "_uid"},
		{"leaf", quantity, "_normal_range"},
		{"interval leaf's lower", rmattrRoot + "/conformance_section/0/conformance_interval/0/any_event/0/interval_dv_quantity/0", "lower"},
		{"party leaf's relationship", obs + "/subject/0", "relationship"},
		{"party leaf's _identifier", obs + "/subject/0", "_identifier"},
		{"underscore family of a leaf", quantity + "/_normal_range/0", "lower"},
		{"FEEDER_AUDIT_DETAILS inside a family", obs + "/_feeder_audit/0", "originating_system_audit"},
		{"FEEDER_AUDIT_DETAILS inside a family", obs + "/_feeder_audit/0/originating_system_audit/0", "provider"},
	} {
		for _, shape := range []any{secret, map[string]any{"y": secret}} {
			t.Run(tc.at+"/"+tc.name+fmt.Sprintf("/%T", shape), func(t *testing.T) {
				_, doc := structuredDoc(t, rmattrBody(extra[tc.level]))
				at, name := tc.at, tc.name
				structuredObject(t, doc, at)[name] = shape
				b := mustJSON(t, doc)
				_, def := UnmarshalStructured(b, wt)
				if !errors.Is(def, ErrUnknownPath) || !strings.Contains(def.Error(), "expected an array") {
					t.Fatalf("UnmarshalStructured (default) = %v, want the array-shape refusal", def)
				}
				var reported []string
				_, rep := UnmarshalStructured(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
				_, strict := UnmarshalStructured(b, wt, WithStrict())
				for mode, err := range map[string]error{"WithUnmatched": rep, "WithStrict": strict} {
					if err == nil || err.Error() != def.Error() {
						t.Errorf("UnmarshalStructured (%s) = %v, want %v", mode, err, def)
					}
				}
				if len(reported) != 0 {
					t.Errorf("WithUnmatched reported %q on a refused decode", reported)
				}
			})
		}
	}
}

// TestREQ053SettingTerminologyWitness — REQ-053: "the implied terminology is
// `openehr`, and a setting coded in any other terminology is a typed error,
// not a silent rewrite". A `|terminology` witness of the setting, in the ctx/
// spelling and in the real-path one, is read: naming openehr it is dropped and
// the value is the setting's without it, in every mode; naming any other
// terminology it is refused with ErrUnsupportedDatatype in every mode.
func TestREQ053SettingTerminologyWitness(t *testing.T) {
	wt, _ := conformanceWT(t)
	setting := map[string]any{"ctx/setting|code": "228", "ctx/setting|value": "primary medical care"}
	ref := canonical(t, decodeRMAttr(t, wt, rmattrBody(setting)))
	for _, key := range []string{"ctx/setting|terminology", rmattrContext + "/setting|terminology"} {
		t.Run(key, func(t *testing.T) {
			body := rmattrBody(setting)
			body[key] = "openehr"
			var reported []string
			for name, opts := range map[string][]Option{
				"default":       nil,
				"WithUnmatched": {WithUnmatched(func(p string) { reported = append(reported, p) })},
				"WithStrict":    {WithStrict()},
			} {
				got, err := UnmarshalFlat(mustJSON(t, body), wt, opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat (%s, openehr) = %v, want the witness read", name, err)
				}
				if !bytes.Equal(canonical(t, got), ref) {
					t.Errorf("UnmarshalFlat (%s, openehr) value differs from the setting without the witness", name)
				}
			}
			if len(reported) != 0 {
				t.Errorf("WithUnmatched reported %q, want nothing", reported)
			}
			body[key] = "local"
			err := refusedInEveryMode(t, wt, body)
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), key) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype naming %s", err, key)
			}
		})
	}
}

// TestREQ170BareValueAtANodeIsRefused — REQ-170: "Input is matched when it
// names something the decoder reads, even where the decoder then refuses it."
// A bare value at a key that resolves to a node with children names that node:
// matched content of the wrong shape, refused in every mode, in FLAT and in
// STRUCTURED, where an element of the node's array that is a scalar is the
// same value. A suffix the node does not read stays unmatched content.
func TestREQ170BareValueAtANodeIsRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	body := rmattrBody(map[string]any{rmattrSection: secret})
	err := refusedInEveryMode(t, wt, body)
	if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "SECTION") || strings.Contains(err.Error(), secret) {
		t.Errorf("err = %v, want ErrUnsupportedDatatype naming SECTION and no value", err)
	}
	_, doc := structuredDoc(t, rmattrBody(nil))
	root := doc[rmattrRoot].(map[string]any)
	root["conformance_section"] = append(root["conformance_section"].([]any), secret)
	b := mustJSON(t, doc)
	_, def := UnmarshalStructured(b, wt)
	if !errors.Is(def, ErrUnsupportedDatatype) || !strings.Contains(def.Error(), "SECTION") {
		t.Fatalf("UnmarshalStructured (default) = %v, want ErrUnsupportedDatatype naming SECTION", def)
	}
	for name, opts := range map[string][]Option{"WithUnmatched": {WithUnmatched(func(string) {})}, "WithStrict": {WithStrict()}} {
		if _, err := UnmarshalStructured(b, wt, opts...); err == nil || err.Error() != def.Error() {
			t.Errorf("UnmarshalStructured (%s) = %v, want %v", name, err, def)
		}
	}
	assertUnmatched(t, wt, rmattrBody(map[string]any{rmattrSection + "|x_note": "n"}), rmattrSection+"|x_note")
}

// TestREQ170StructuredMetadataWithoutANode — REQ-170 / REQ-053. A Web Template
// need not carry a `context` node: the decode still reads the metadata
// spellings below the root and resolves `<root>/context` as the EVENT_CONTEXT
// without it, so the STRUCTURED rule judges a `context` member by those
// spellings. Under it the decode reads a longer metadata spelling
// (`start_time`) and the underscore families an EVENT_CONTEXT carries
// (`_health_care_facility`); any other member, an unknown family or one
// EVENT_CONTEXT does not carry among them, is set aside whatever its value
// holds, reported by its member path and refused under strict. A name it reads
// keeps the shape refusal in every mode.
//
// The composer's party sub-structure (`_identifier`, `relationship`) is read
// whether or not the Web Template carries a composer node: its real path
// carries the whole composer (ADR 0029), so FLAT and STRUCTURED alike decode it
// to the same composer, and a member below it that the party grammar does not
// read is set aside and reported by its member path.
func TestREQ170StructuredMetadataWithoutANode(t *testing.T) {
	full, _ := conformanceWT(t)
	without := func(id string) *webtemplate.WebTemplate {
		t.Helper()
		out := *full
		root := *full.Tree
		root.Children = slices.DeleteFunc(slices.Clone(root.Children), func(n *webtemplate.Node) bool { return n.ID == id })
		if len(root.Children) == len(full.Tree.Children) {
			t.Fatalf("fixture: the corpus template has no %s node to remove", id)
		}
		out.Tree = &root
		return &out
	}
	wt := *without("context")
	const secret = "secret-value-0451"
	body := rmattrBody(map[string]any{
		rmattrContext + "/start_time":                 "2020-02-03T04:05:06+01:00",
		rmattrContext + "/_health_care_facility|name": "Hospital",
	})
	delete(body, "ctx/time")
	ref, _ := structuredDoc(t, body)
	refComp, err := UnmarshalStructured(ref, &wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalStructured (reference) = %v", err)
	}
	if c := canonical(t, refComp); !bytes.Contains(c, []byte("2020-02-03T04:05:06")) || !bytes.Contains(c, []byte("Hospital")) {
		t.Fatalf("fixture: the context's start time and facility do not reach the value: %s", c)
	}
	contextPath := rmattrRoot + "/context/0"
	for _, name := range []string{"x_member", "_x_unknown", "_normal_range"} {
		for _, value := range []any{secret, map[string]any{"y": secret}, []any{secret}} {
			t.Run(fmt.Sprintf("%s/%T", name, value), func(t *testing.T) {
				_, doc := structuredDoc(t, body)
				structuredObject(t, doc, contextPath)[name] = value
				assertStructuredUnmatched(t, &wt, mustJSON(t, doc), ref, secret, contextPath+"/"+name)
			})
		}
	}
	for _, name := range []string{"start_time", "_health_care_facility"} {
		t.Run("refused: "+name+" holding a scalar", func(t *testing.T) {
			_, doc := structuredDoc(t, body)
			structuredObject(t, doc, contextPath)[name] = secret
			b := mustJSON(t, doc)
			_, def := UnmarshalStructured(b, &wt)
			if !errors.Is(def, ErrUnknownPath) || !strings.Contains(def.Error(), "expected an array") {
				t.Fatalf("UnmarshalStructured (default) = %v, want the array-shape refusal", def)
			}
			var reported []string
			_, rep := UnmarshalStructured(b, &wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
			_, strict := UnmarshalStructured(b, &wt, WithStrict())
			for mode, err := range map[string]error{"WithUnmatched": rep, "WithStrict": strict} {
				if err == nil || err.Error() != def.Error() {
					t.Errorf("UnmarshalStructured (%s) = %v, want %v", mode, err, def)
				}
			}
			if len(reported) != 0 {
				t.Errorf("WithUnmatched reported %q on a refused decode", reported)
			}
		})
	}
	for _, tc := range []struct {
		name string
		wt   *webtemplate.WebTemplate
	}{{"with a composer node", full}, {"without a composer node", without("composer")}} {
		t.Run("composer sub-structure "+tc.name, func(t *testing.T) {
			body := rmattrBody(map[string]any{
				"ctx/composer_name":                           "Dr. Who",
				rmattrRoot + "/composer/_identifier:0|id":     "122",
				rmattrRoot + "/composer/relationship|code":    "10",
				rmattrRoot + "/composer/relationship|value":   "mother",
				rmattrRoot + "/composer/relationship|x_note":  secret,
				rmattrRoot + "/composer/_identifier:0/x|id":   secret,
				rmattrRoot + "/composer/relationship/x|code":  secret,
				rmattrRoot + "/composer/_identifier:0|x_note": secret,
			})
			flat, err := UnmarshalFlat(mustJSON(t, body), tc.wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat = %v, want the composer read", err)
			}
			related, ok := flat.Composer.(*rm.PartyRelated)
			if !ok || len(related.Identifiers) != 1 || related.Identifiers[0].ID != "122" || related.Relationship.Value != "mother" {
				t.Fatalf("composer = %#v, want a PARTY_RELATED carrying identifier 122 and relationship mother", flat.Composer)
			}
			_, doc := structuredDoc(t, body)
			var reported []string
			structured, err := UnmarshalStructured(mustJSON(t, doc), tc.wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
			if err != nil {
				t.Fatalf("UnmarshalStructured = %v, want the composer read", err)
			}
			if !bytes.Equal(canonical(t, structured), canonical(t, flat)) {
				t.Errorf("STRUCTURED decoded %s,\nFLAT decoded %s", canonical(t, structured), canonical(t, flat))
			}
			composerPath := rmattrRoot + "/composer/0"
			want := []string{
				composerPath + "/_identifier/0/x",
				composerPath + "/_identifier/0/|x_note",
				composerPath + "/relationship/0/x",
				composerPath + "/relationship/0/|x_note",
			}
			slices.Sort(reported)
			if !slices.Equal(reported, want) {
				t.Errorf("STRUCTURED reported %q,\nwant %q", reported, want)
			}
		})
	}
	// A metadata segment is single-valued with or without its node, so an
	// element past :0 is refused whatever it holds, in STRUCTURED and FLAT.
	for _, tc := range []struct{ name, at, member, flatKey string }{
		{"context/1", rmattrRoot, "context", rmattrRoot + "/context:1/x_unknown|y"},
		{"context/0/start_time/1", contextPath, "start_time", rmattrRoot + "/context/start_time:1/x_unknown|y"},
	} {
		t.Run(tc.name+" holding only unmatched members", func(t *testing.T) {
			_, doc := structuredDoc(t, body)
			obj := structuredObject(t, doc, tc.at)
			obj[tc.member] = append(obj[tc.member].([]any), map[string]any{"x_unknown": []any{"v"}})
			if err := structuredDocRefusedInEveryMode(t, &wt, mustJSON(t, doc)); !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "single-valued") {
				t.Errorf("STRUCTURED err = %v, want ErrUnknownPath for a single-valued position", err)
			}
			flat := maps.Clone(body)
			flat[tc.flatKey] = "v"
			if err := refusedInEveryMode(t, &wt, flat); !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "single-valued") {
				t.Errorf("FLAT err = %v, want ErrUnknownPath for a single-valued position", err)
			}
		})
	}
}
