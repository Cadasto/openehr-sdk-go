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
	var b strings.Builder
	b.WriteString("{")
	written := slices.Clone(unmatched)
	slices.Reverse(written)
	order := append(slices.Clone(written), slices.Sorted(maps.Keys(rmattrBody(nil)))...)
	for i, k := range order {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s:%s", mustJSON(t, k), mustJSON(t, body[k]))
	}
	b.WriteString("}")

	var reported []string
	if _, err := UnmarshalFlat([]byte(b.String()), wt, WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if !slices.Equal(reported, written) {
		t.Errorf("WithUnmatched reported %q, want the written order %q", reported, written)
	}
	_, err := UnmarshalFlat([]byte(b.String()), wt, WithStrict())
	if err == nil || err.Error() != strictRefusal(written[0]) {
		t.Errorf("UnmarshalFlat (WithStrict) = %v, want %q", err, strictRefusal(written[0]))
	}
}

// TestREQ170StructuredUnmatchedIgnoredReportedRefused — REQ-170 acceptance for
// STRUCTURED: the same four kinds of unmatched member decode by default to the
// value without them, the report names each once by its slash-joined member
// path in document order, and strict refuses naming the first, wrapping
// ErrUnknownPath and carrying none of its value.
func TestREQ170StructuredUnmatchedIgnoredReportedRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	clean := rmattrBody(nil)
	cleanStructured, err := FlatToStructured(mustJSON(t, clean))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	ref, err := UnmarshalStructured(cleanStructured, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured (clean): %v", err)
	}

	// The same body with the four unmatched members, written by hand so their
	// document order is known: the ctx field, then the unknown suffix, the
	// unknown node and the unknown `_` member in the observation, in this order.
	var doc map[string]any
	if err := json.Unmarshal(cleanStructured, &doc); err != nil {
		t.Fatal(err)
	}
	doc["ctx"].(map[string]any)["x_flag"] = secret
	obs := doc[rmattrRoot].(map[string]any)["conformance_section"].([]any)[0].(map[string]any)["conformance_observation"].([]any)[0].(map[string]any)
	obs["any_event"].([]any)[0].(map[string]any)["dv_quantity"].([]any)[0].(map[string]any)["|x_note"] = secret
	obs["no_such_node"] = []any{map[string]any{"|magnitude": secret}}
	obs["_x_extension"] = []any{secret}
	b := mustJSON(t, doc)

	obsPath := rmattrRoot + "/conformance_section/0/conformance_observation/0"
	// json.Marshal writes each object's members sorted, so that is the document
	// order: `_x_extension` sorts before `any_event` and `no_such_node`, and
	// `conformance-ehrbase.de.v0` before `ctx`.
	want := []string{
		obsPath + "/_x_extension/0",
		obsPath + "/any_event/0/dv_quantity/0/|x_note",
		obsPath + "/no_such_node/0/|magnitude",
		"ctx/x_flag",
	}

	got, err := UnmarshalStructured(b, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured (default) = %v, want the unmatched members ignored", err)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
		t.Errorf("UnmarshalStructured (default) value differs from the body without the unmatched members")
	}
	var reported []string
	got, err = UnmarshalStructured(b, wt, WithUnmatched(func(p string) { reported = append(reported, p) }))
	if err != nil {
		t.Fatalf("UnmarshalStructured (WithUnmatched) = %v", err)
	}
	if !slices.Equal(reported, want) {
		t.Errorf("WithUnmatched reported %q, want %q", reported, want)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
		t.Errorf("UnmarshalStructured (WithUnmatched) value differs from the default")
	}
	_, err = UnmarshalStructured(b, wt, WithStrict())
	if !errors.Is(err, ErrUnknownPath) || err.Error() != strictRefusal(want[0]) {
		t.Errorf("UnmarshalStructured (WithStrict) = %v, want %q wrapping ErrUnknownPath", err, strictRefusal(want[0]))
	}
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("strict refusal %q carries the unmatched member's value", err)
	}
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
		{"composer external_ref", map[string]any{rmattrRoot + "/composer|id": "1234"}, ErrUnsupportedDatatype},
		{"composer identifier", map[string]any{rmattrRoot + "/composer/_identifier:0|id": "122"}, ErrUnsupportedDatatype},
		{"composer relationship", map[string]any{rmattrRoot + "/composer/relationship|code": "10"}, ErrUnsupportedDatatype},
		{"PARTICIPATION time", map[string]any{
			rmattrContext + "/_participation:0|function": "requester",
			rmattrContext + "/_participation:0|time":     "2021-12-21T14:19:31+01:00",
		}, ErrUnsupportedDatatype},
		{"FEEDER_AUDIT_DETAILS other_details", map[string]any{
			rmattrObs + "/_feeder_audit/originating_system_audit|system_id":       "orig",
			rmattrObs + "/_feeder_audit/originating_system_audit/other_details|x": "boom",
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusedInEveryMode(t, wt, rmattrBody(tc.keys))
		})
	}
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
// of the Web Template path (a single-valued attribute, a party or interval
// leaf), a `_family:N` list and a FEEDER_AUDIT sub-path give ErrUnknownPath; a
// sub-path inside an underscore family's value or an interval value gives
// ErrUnsupportedDatatype. deviations.md states the same split.
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
		{"interval leaf segment", rmattrIntervalEvent + "/interval_dv_quantity:1/lower|magnitude", 1.0, ErrUnknownPath, rmattrIntervalEvent + "/interval_dv_quantity:1"},
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
// `time` leaf.
func TestREQ170IsmTransitionReferenceKeysRefusedInEveryMode(t *testing.T) {
	wt, _ := conformanceWT(t)
	const secret = "secret-value-0451"
	type refusal struct {
		name  string
		keys  []string
		names string
	}
	leaf := func(base, id string) refusal {
		return refusal{id + " leaf under " + base, []string{base + "/" + id + "|code", base + "/" + id + "|value", base + "/" + id + "|terminology"}, base + "/" + id + "|code"}
	}
	reference, archetyped := rmattrAction+"/ism_transition", rmattrAction+"/transition"
	cases := []refusal{
		leaf(reference, "current_state"),
		leaf(reference, "transition"),
		leaf(reference, "careflow_step"),
		leaf(archetyped, "transition"),
		{"bare transition leaf under " + reference, []string{reference + "/transition"}, reference + "/transition"},
		{"bare transition leaf under " + archetyped, []string{archetyped + "/transition"}, archetyped + "/transition"},
		{"reason family under " + reference, []string{reference + "/_reason:0"}, reference + "/_reason:0"},
		{"reason family under " + archetyped, []string{archetyped + "/_reason:0"}, archetyped + "/_reason:0"},
		{"reason family with a suffix", []string{reference + "/_reason|code"}, reference + "/_reason"},
		{"ACTION time leaf", []string{rmattrAction + "/time"}, rmattrAction + "/time"},
		{"ACTION time leaf with a suffix", []string{rmattrAction + "/time|magnitude_status"}, rmattrAction + "/time|magnitude_status"},
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
