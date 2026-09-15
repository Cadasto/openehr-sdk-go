package canjson_test

// corpus_parity_test.go: the differential decode net for the move of the
// canonical-JSON path to encoding/json/v2
// (docs/plans/archive/2026-09-14-json-v2-migration.md, phase 1.3). REQ-052 § Field
// order, REQ-013 § building-block independence.
//
// Every canonical-JSON RM document vendored under testkit/cassettes is decoded
// twice, once through encoding/json and once through encoding/json/v2 with no
// options, into two fresh instances of the same generated type, and the two
// results are compared. The net exists so that the generator change and the
// canjson change can each show they moved no value: a decoded tree that differs
// between the two packages, or a document one package accepts and the other
// refuses, is reported here by cassette path and by the first field that
// differs.
//
// How to read the result, measured on Go 1.27 and load-bearing. In Go 1.27
// encoding/json is itself implemented over json/v2, and both packages honour
// both custom-unmarshaler interfaces: the v1 `UnmarshalJSON([]byte) error` and
// the v2 `UnmarshalJSONFrom(*jsontext.Decoder) error`. Every type registered in
// typereg.Default carries one of the two, so for a cassette both sides of the
// comparison reach the same generated code, and they still will once ADR 0022
// swaps which of the two methods that is. What this
// net therefore pins is entry-point parity rather than a contest between two
// codec implementations: a consumer calling encoding/json.Unmarshal on an RM
// type must keep getting the value a consumer calling encoding/json/v2.Unmarshal
// gets, before and after the migration, and a document must stay acceptable to
// both or refused by both. [testParityRegistryCensus] records the delegation
// fact that makes this reading necessary, and turns red on the day it stops
// holding.

import (
	v1 "encoding/json"
	v2 "encoding/json/v2"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// minParityDocuments is a floor on how many cassettes the selection rule must
// admit and both codecs must accept. It guards the rule itself: a skip
// condition that quietly stopped matching would empty the net without failing
// anything. 103 cassettes clear both bars today, so the floor sits below that
// with room for vendoring churn.
const minParityDocuments = 90

// parityVerdict is what [compareCodecs] found for one document.
type parityVerdict struct {
	v1Err error
	v2Err error
	// diff names the first field at which the two decoded values differ. It is
	// empty when reflect.DeepEqual holds, and unread when either decode failed.
	diff string
}

// diverged reports whether the two packages disagreed: one accepted the
// document and the other refused it, or both accepted it and produced
// different values. A document both packages refuse is agreement.
func (v parityVerdict) diverged() bool {
	return (v.v1Err == nil) != (v.v2Err == nil) || v.diff != ""
}

// describe renders one divergence for a failure message, naming the document.
func (v parityVerdict) describe(name string) string {
	switch {
	case v.v1Err != nil && v.v2Err == nil:
		return fmt.Sprintf("  %s: encoding/json refused it (%v), encoding/json/v2 accepted it", name, v.v1Err)
	case v.v1Err == nil && v.v2Err != nil:
		return fmt.Sprintf("  %s: encoding/json/v2 refused it (%v), encoding/json accepted it", name, v.v2Err)
	default:
		return fmt.Sprintf("  %s: first differing field %s", name, v.diff)
	}
}

// compareCodecs decodes raw twice into fresh values built by ctor, once through
// encoding/json and once through encoding/json/v2 with no options, and reports
// how the two compare. A decode failure on either side is recorded rather than
// raised, so one malformed cassette reports as a line in a census instead of
// ending the run.
func compareCodecs(ctor func() any, raw []byte) parityVerdict {
	a, b := ctor(), ctor()
	verdict := parityVerdict{
		v1Err: v1.Unmarshal(raw, a),
		v2Err: v2.Unmarshal(raw, b),
	}
	if verdict.v1Err != nil || verdict.v2Err != nil {
		return verdict
	}
	if reflect.DeepEqual(a, b) {
		return verdict
	}
	if where, found := firstDiff(reflect.ValueOf(a), reflect.ValueOf(b), ""); found {
		verdict.diff = where
		return verdict
	}
	verdict.diff = "the two values are not deeply equal but the field walk found no differing field"
	return verdict
}

func TestCorpusParityV1V2(t *testing.T) {
	t.Run("corpus", testParityCorpus)
	t.Run("control_case_mismatch_is_reported", testParityControlCaseMismatch)
	t.Run("control_two_cassettes_report_a_field", testParityControlDistinctCassettes)
	t.Run("RegistryCensus", testParityRegistryCensus)
}

// testParityCorpus walks testkit/cassettes and compares the two packages on
// every canonical-JSON RM document it holds.
//
// Selection rule. A file is a canonical-JSON RM document for this net when its
// top-level JSON value is an object carrying a non-empty `_type` member whose
// value is registered in typereg.Default. Everything else is skipped and
// counted:
//
//   - No `_type`, or a top-level value that is not an object. This is the FLAT
//     conformance corpus, the ITS-REST error, discovery, query, system and
//     definition envelopes, the Web Template goldens, and the ECIS-shaped
//     EHR_STATUS sample. None of them is canonical JSON, and without the
//     discriminator there is no generated type to decode into.
//   - A `_type` that typereg.Default does not know. No generated type exists,
//     so there is nothing to decode into and nothing to compare. None today;
//     the class is counted and logged so a newly vendored cassette of an
//     unsupported type becomes visible rather than silently dropped.
//   - JSON that will not parse at all. Kept apart from the class above, which
//     is about well-formed documents of another shape: a cassette that stopped
//     parsing is a corrupt file rather than a fixture this net has no opinion
//     on. None today; counted and logged like the class above.
//
// HAR recordings live under testkit/recordings, not under testkit/cassettes,
// so none reaches this walk.
func testParityCorpus(t *testing.T) {
	root := fixtures.CassettesRoot()
	var scanned, compared, refusedByBoth, skippedNoType, skippedUnregistered, skippedMalformed int
	var malformed, unregistered, divergent []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		scanned++
		rel := relativeToRoot(root, path)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", rel, readErr)
		}
		var top any
		if err := v1.Unmarshal(raw, &top); err != nil {
			skippedMalformed++
			malformed = append(malformed, fmt.Sprintf("%s (%v)", rel, err))
			return nil
		}
		object, isObject := top.(map[string]any)
		if !isObject {
			skippedNoType++
			return nil
		}
		typeName, _ := object["_type"].(string)
		if typeName == "" {
			skippedNoType++
			return nil
		}
		ctor, known := typereg.Default.Lookup(typeName)
		if !known {
			skippedUnregistered++
			unregistered = append(unregistered, fmt.Sprintf("%s (_type %s)", rel, typeName))
			return nil
		}
		switch verdict := compareCodecs(ctor, raw); {
		case verdict.diverged():
			divergent = append(divergent, verdict.describe(rel))
		case verdict.v1Err != nil:
			refusedByBoth++
		default:
			compared++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	if len(divergent) > 0 {
		t.Errorf("encoding/json and encoding/json/v2 disagree on %d of the %d cassettes scanned:\n%s",
			len(divergent), scanned, strings.Join(divergent, "\n"))
	}
	if classified := skippedMalformed + skippedNoType + skippedUnregistered + refusedByBoth + compared + len(divergent); classified != scanned {
		t.Errorf("the census does not add up: %d cassettes classified, %d scanned", classified, scanned)
	}
	if compared < minParityDocuments {
		t.Errorf("only %d cassettes were decoded by both packages, want at least %d; the selection rule is skipping documents it should admit (no usable _type %d, unregistered _type %d, unparsable %d, refused by both %d)",
			compared, minParityDocuments, skippedNoType, skippedUnregistered, skippedMalformed, refusedByBoth)
	}
	t.Logf("cassettes scanned %d: compared %d, refused by both packages %d, skipped without a usable _type %d, skipped on an unregistered _type %d, skipped as unparsable JSON %d",
		scanned, compared, refusedByBoth, skippedNoType, skippedUnregistered, skippedMalformed)
	if len(unregistered) > 0 {
		t.Logf("cassettes carrying a _type the registry does not know:\n  %s", strings.Join(unregistered, "\n  "))
	}
	if len(malformed) > 0 {
		t.Logf("cassettes that would not parse as JSON:\n  %s", strings.Join(malformed, "\n  "))
	}
}

// parityControlValue is a canonical-JSON shape with no custom unmarshaler of
// its own, so each package applies its own member-name matching to it. That is
// what makes the case mismatch in [testParityControlCaseMismatch] observable,
// and it is why the control cannot be carried by a vendored cassette: every
// type in typereg.Default has a generated unmarshaler method, which both
// packages call in preference to their own field matching (see
// [testParityRegistryCensus]).
type parityControlValue struct {
	Type      string  `json:"_type"`
	Magnitude float64 `json:"magnitude"`
	Units     string  `json:"units"`
}

// testParityControlCaseMismatch is the can-fail control for the corpus sweep:
// it feeds the harness a document the two packages genuinely read differently
// and requires the harness to say so. The document spells one member name so it
// differs from its struct tag only by case. encoding/json matches such a name
// and fills the field; encoding/json/v2 requires an exact match and leaves the
// field zero, so the two decoded values differ at Magnitude.
//
// The mutation that turns this red is any harness that stops looking: decoding
// both sides with the same package, dropping the reflect.DeepEqual comparison
// in [compareCodecs], or a [firstDiff] that reports nothing. Without this
// control a corpus sweep reporting zero divergences would be indistinguishable
// from a sweep that compares nothing.
func testParityControlCaseMismatch(t *testing.T) {
	const typeName = "PARITY_CONTROL"
	registry := typereg.NewRegistry()
	registry.Register(typeName, func() any { return new(parityControlValue) })
	ctor, ok := registry.Lookup(typeName)
	if !ok {
		t.Fatalf("Lookup(%q) missed straight after Register", typeName)
	}

	faithful := []byte(`{"_type":"PARITY_CONTROL","magnitude":80.5,"units":"kg"}`)
	if verdict := compareCodecs(ctor, faithful); verdict.diverged() {
		t.Fatalf("the two packages already disagree on the unmutated control document:\n%s", verdict.describe("control"))
	}

	mutated := []byte(`{"_type":"PARITY_CONTROL","Magnitude":80.5,"units":"kg"}`)
	verdict := compareCodecs(ctor, mutated)
	if !verdict.diverged() {
		t.Fatalf("compareCodecs(%s) reported no divergence; encoding/json matches a member name that differs from its tag only by case and encoding/json/v2 does not, so this document must come back divergent", mutated)
	}
	if !strings.Contains(verdict.diff, "Magnitude") {
		t.Errorf("compareCodecs(%s) reported the divergence at %q, want the Magnitude field named", mutated, verdict.diff)
	}
}

// parityControlCassettes are two vendored COMPOSITION cassettes with different
// content, used as the second leg of the control.
var parityControlCassettes = [2]string{"body_weight", "BMI"}

// testParityControlDistinctCassettes is the second leg of the can-fail control.
// The case-mismatch leg proves the harness reports a divergence on a small
// hand-built shape; this leg proves the field walk behind that report also
// finds and names a difference inside two real decoded RM trees, with their
// polymorphic slots, slices and nested structures. Two different cassettes must
// produce a named differing field.
//
// The mutation that turns it red is a [firstDiff] that stops descending, for
// example one that skips struct fields or treats an interface slot as always
// equal: the corpus sweep would then keep reporting zero divergences whatever
// the codecs did below the top level.
func testParityControlDistinctCassettes(t *testing.T) {
	decode := func(name string) *rm.Composition {
		t.Helper()
		raw, err := os.ReadFile(fixtures.CompositionJSON(name))
		if err != nil {
			t.Fatalf("read cassette %s: %v", name, err)
		}
		var c rm.Composition
		if err := v1.Unmarshal(raw, &c); err != nil {
			t.Fatalf("decode cassette %s: %v", name, err)
		}
		return &c
	}
	a := decode(parityControlCassettes[0])
	b := decode(parityControlCassettes[1])

	where, found := firstDiff(reflect.ValueOf(a), reflect.ValueOf(b), "")
	if !found {
		t.Fatalf("firstDiff found no difference between cassettes %s and %s, which hold different content; the field walk is not descending",
			parityControlCassettes[0], parityControlCassettes[1])
	}
	if !strings.HasPrefix(where, ".") {
		t.Errorf("firstDiff reported %q, want a field path starting at the composition root", where)
	}
}

// testParityRegistryCensus pins the invariant that makes the corpus sweep
// readable: both packages reach the same generated code. Every type registered
// in typereg.Default carries a generated unmarshaler method, and on Go 1.27
// both encoding/json and encoding/json/v2 call either method in preference to
// their own struct field matching, so each cassette is compared across two
// entry points into one implementation rather than across two implementations.
//
// The assertion is the disjunction on purpose. Today the generated method is
// the v1 `UnmarshalJSON([]byte) error`; ADR 0022 replaces it with the streaming
// `UnmarshalJSONFrom(*jsontext.Decoder) error`, and the invariant holds either
// way. What turns this red is a registered type that has neither, which would
// leave the two packages doing their own field matching on it, at which point
// the sweep above starts comparing two implementations and its result has to be
// re-read rather than assumed.
func testParityRegistryCensus(t *testing.T) {
	names := typereg.Default.Names()
	if len(names) == 0 {
		t.Fatal("typereg.Default is empty; the rm package's init did not run")
	}
	var without []string
	for _, name := range names {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Errorf("Names() listed %q but Lookup(%q) missed it", name, name)
			continue
		}
		value := ctor()
		_, hasV1 := value.(v1.Unmarshaler)
		_, hasV2 := value.(v2.UnmarshalerFrom)
		if !hasV1 && !hasV2 {
			without = append(without, name)
		}
	}
	if len(without) > 0 {
		t.Errorf("%d of %d registered types carry neither a generated UnmarshalJSON nor a generated UnmarshalJSONFrom: %s\nFor those types the two packages do their own field matching, so the corpus sweep above is comparing two implementations rather than two entry points into one; re-read its result",
			len(without), len(names), strings.Join(without, ", "))
	}
}

// firstDiff walks two values of the same type and returns a dotted path to the
// first place they differ, together with what each side holds there. The second
// result is false when the walk found no difference. Unexported fields are
// skipped: JSON decoding never sets them, so they cannot carry a difference
// between two decodes of the same bytes.
//
// reflect.DeepEqual, not this walk, is the verdict in [compareCodecs]; this
// walk exists so the failure message names a field instead of printing two
// composition trees.
func firstDiff(a, b reflect.Value, path string) (string, bool) {
	at := pathOrRoot(path)
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() == b.IsValid() {
			return "", false
		}
		return fmt.Sprintf("%s: encoding/json valid=%t, encoding/json/v2 valid=%t", at, a.IsValid(), b.IsValid()), true
	}
	if a.Type() != b.Type() {
		return fmt.Sprintf("%s: encoding/json decoded %s, encoding/json/v2 decoded %s", at, a.Type(), b.Type()), true
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() != b.IsNil() {
			return fmt.Sprintf("%s: encoding/json nil=%t, encoding/json/v2 nil=%t", at, a.IsNil(), b.IsNil()), true
		}
		if a.IsNil() {
			return "", false
		}
		return firstDiff(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := range a.NumField() {
			field := a.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			if where, found := firstDiff(a.Field(i), b.Field(i), path+"."+field.Name); found {
				return where, true
			}
		}
		return "", false
	case reflect.Slice, reflect.Array:
		if a.Kind() == reflect.Slice && a.IsNil() != b.IsNil() {
			return fmt.Sprintf("%s: encoding/json nil slice=%t, encoding/json/v2 nil slice=%t", at, a.IsNil(), b.IsNil()), true
		}
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: encoding/json has %d elements, encoding/json/v2 has %d", at, a.Len(), b.Len()), true
		}
		for i := range a.Len() {
			if where, found := firstDiff(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); found {
				return where, true
			}
		}
		return "", false
	case reflect.Map:
		if a.IsNil() != b.IsNil() {
			return fmt.Sprintf("%s: encoding/json nil map=%t, encoding/json/v2 nil map=%t", at, a.IsNil(), b.IsNil()), true
		}
		return firstMapDiff(a, b, path)
	default:
		if !a.CanInterface() || !b.CanInterface() {
			return at + ": the value cannot be read through reflection, so it was not compared", true
		}
		if reflect.DeepEqual(a.Interface(), b.Interface()) {
			return "", false
		}
		return fmt.Sprintf("%s: encoding/json has %s, encoding/json/v2 has %s", at, render(a), render(b)), true
	}
}

// firstMapDiff compares two maps of the same type entry by entry, in sorted key
// order so the field a failure names is the same on every run.
func firstMapDiff(a, b reflect.Value, path string) (string, bool) {
	at := pathOrRoot(path)
	if a.Len() != b.Len() {
		return fmt.Sprintf("%s: encoding/json has %d entries, encoding/json/v2 has %d", at, a.Len(), b.Len()), true
	}
	keys := a.MapKeys()
	slices.SortFunc(keys, func(x, y reflect.Value) int {
		return strings.Compare(fmt.Sprint(x.Interface()), fmt.Sprint(y.Interface()))
	})
	for _, key := range keys {
		label := fmt.Sprintf("%s[%v]", path, key.Interface())
		other := b.MapIndex(key)
		if !other.IsValid() {
			return label + ": the key is present under encoding/json and absent under encoding/json/v2", true
		}
		if where, found := firstDiff(a.MapIndex(key), other, label); found {
			return where, true
		}
	}
	return "", false
}

// pathOrRoot labels the top of the walk, where the path is still empty.
func pathOrRoot(path string) string {
	if path == "" {
		return "the decoded value"
	}
	return path
}

// render formats one side of a difference, short enough to read in a failure
// message.
func render(v reflect.Value) string {
	const limit = 120
	s := fmt.Sprintf("%#v", v.Interface())
	if runes := []rune(s); len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return s
}

// relativeToRoot names a cassette by its path under the cassettes root, in
// forward-slash form, falling back to the absolute path if it lies elsewhere.
func relativeToRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
