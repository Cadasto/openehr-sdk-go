package webtemplate

// Tests for the exported reducing decode that PROBE-105 shares with PROBE-086
// (REQ-080): the two context modes, and the typed error a decode the loop
// cannot reduce comes back as.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestDecodeReducingContextModes pins the difference between the two modes:
// InjectContext supplies the mandatory context a held-out body lacks, and
// KeepContext adds nothing, so the same body fails with the codec's
// missing-context error, carried by an *IrreducibleError that names no key.
func TestDecodeReducingContextModes(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	_, body := modelledQuantityLeaf(t, target)

	comp, refusals, err := DecodeReducing(target, maps.Clone(body), InjectContext)
	if err != nil || comp == nil || len(refusals) != 0 {
		t.Fatalf("DecodeReducing(InjectContext) = %v, %v, %v; want a composition, no refusals and no error", comp, refusals, err)
	}

	_, _, err = DecodeReducing(target, maps.Clone(body), KeepContext)
	ie, ok := errors.AsType[*IrreducibleError](err)
	if !ok || ie == nil {
		t.Fatalf("DecodeReducing(KeepContext, no ctx/) error = %v, want an *IrreducibleError", err)
	}
	if !errors.Is(err, simplified.ErrMissingContext) || !errors.Is(ie.Err, simplified.ErrMissingContext) {
		t.Errorf("DecodeReducing(KeepContext, no ctx/) error = %v, want it to carry simplified.ErrMissingContext", err)
	}
	if ie.Key != "" || !strings.HasPrefix(err.Error(), "decode failed with no attributable key: ") {
		t.Errorf("IrreducibleError = {Key: %q, %q}, want no key and the no-attributable-key account", ie.Key, err)
	}

	// KeepContext decodes a body that carries its own context, as given.
	withCtx := maps.Clone(body)
	withCtx["ctx/language"], withCtx["ctx/territory"] = "en", "US"
	if _, _, err := DecodeReducing(target, withCtx, KeepContext); err != nil {
		t.Errorf("DecodeReducing(KeepContext, with ctx/) error = %v, want nil", err)
	}
}

// TestDecodeReducingNotAGap pins the shape PROBE-086 counts as a harness fault
// and PROBE-105 as a leg's refusal: a keyed codec error outside the two gap
// sentinels stops the loop with an *IrreducibleError carrying the key and the
// codec's own error, and nothing is removed from the body.
func TestDecodeReducingNotAGap(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	base, body := modelledQuantityLeaf(t, target)
	// |accuracy must be a number; a string that is not one is a payload defect,
	// refused naming the leaf with no gap sentinel.
	body[base+"|accuracy"] = "not a number"
	before := len(body)

	_, _, err = DecodeReducing(target, body, InjectContext)
	ie, ok := errors.AsType[*IrreducibleError](err)
	if !ok || ie == nil {
		t.Fatalf("DecodeReducing(malformed |accuracy) error = %v, want an *IrreducibleError", err)
	}
	if ie.Key != base {
		t.Errorf("IrreducibleError.Key = %q, want the leaf %q", ie.Key, base)
	}
	if ie.Err == nil || errors.Is(ie.Err, simplified.ErrUnknownPath) || errors.Is(ie.Err, simplified.ErrUnsupportedDatatype) {
		t.Errorf("IrreducibleError.Err = %v, want the codec's error, outside the gap sentinels", ie.Err)
	}
	if !strings.Contains(err.Error(), "harness fault rather than a codec gap") || !strings.HasSuffix(err.Error(), ie.Err.Error()) {
		t.Errorf("IrreducibleError.Error() = %q, want the harness's account followed by the codec's error", err)
	}
	if len(body) != before {
		t.Errorf("body has %d keys after the refusal, want the %d it had: an irreducible error removes nothing", len(body), before)
	}
}

// TestDecodeReducingKeepsRefusalsOnError pins that the key families the loop
// already removed come back with the error that stops it, an irreducible
// decode or a breach of the refusal budget, rather than being lost with it.
// PROBE-105 reports them for a refused leg.
func TestDecodeReducingKeepsRefusalsOnError(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}

	base, leaf := modelledQuantityLeaf(t, target)
	// Decode reads leaf groups in sorted key order and checks the context
	// after them, so the composer's external_ref, sorting first, is refused and
	// removed before the irreducible error stops the loop.
	refused := target.Root + "/composer|id"
	if refused >= base {
		t.Fatalf("refused key %q does not sort before the leaf %q; pick another", refused, base)
	}
	irreducible := []struct {
		name    string
		mode    ContextMode
		change  func(map[string]any)
		wantKey string
	}{
		{
			name:    "keyed error outside the gap sentinels",
			mode:    InjectContext,
			change:  func(b map[string]any) { b[base+"|accuracy"] = "not a number" },
			wantKey: base,
		},
		{
			name:   "error that names no key",
			mode:   KeepContext,
			change: func(map[string]any) {}, // no ctx/language or ctx/territory
		},
	}
	for _, tt := range irreducible {
		t.Run(tt.name, func(t *testing.T) {
			body := maps.Clone(leaf)
			body[refused] = "1234-5678"
			tt.change(body)

			_, refusals, err := DecodeReducing(target, body, tt.mode)
			if ie, ok := errors.AsType[*IrreducibleError](err); !ok || ie == nil || ie.Key != tt.wantKey {
				t.Fatalf("DecodeReducing() error = %v, want an *IrreducibleError with key %q", err, tt.wantKey)
			}
			want := []Refusal{{
				Key:     refused,
				Reason:  "unsupported datatype: PARTY_PROXY",
				Message: `simplified: decode "<key>": simplified: unsupported datatype: PARTY_PROXY`,
				Keys:    1,
			}}
			if !slices.Equal(refusals, want) {
				t.Errorf("DecodeReducing() refusals = %+v, want %+v", refusals, want)
			}
			if _, ok := body[refused]; ok {
				t.Errorf("body still carries %s; the refusal that removed it must have been applied", refused)
			}
		})
	}

	t.Run("refusal budget breached", func(t *testing.T) {
		// One refused key more than the budget allows, each its own family: an
		// index past the bound of the template root, which the codec refuses one
		// key at a time. (Unmatched keys would all be reported in one pass.)
		body := make(map[string]any, maxRefusals+1)
		for i := range maxRefusals + 1 {
			body[fmt.Sprintf("%s:1/no_such_node_%03d", target.Root, i)] = "x"
		}
		_, refusals, err := DecodeReducing(target, body, InjectContext)
		if err == nil || !strings.Contains(err.Error(), "did not converge") {
			t.Fatalf("DecodeReducing(%d refused keys) error = %v, want the budget breach", maxRefusals+1, err)
		}
		if _, ok := errors.AsType[*IrreducibleError](err); ok {
			t.Errorf("DecodeReducing(%d refused keys) error = %v, want a harness fault, not an *IrreducibleError", maxRefusals+1, err)
		}
		if len(refusals) != maxRefusals || len(body) != 1 {
			t.Errorf("DecodeReducing(%d refused keys) = %d refusals, %d keys left; want %d refusals and 1 key left",
				maxRefusals+1, len(refusals), len(body), maxRefusals)
		}
	})
}

// TestDecodeReducingExcludesWhatTheCodecReports pins the exclusion the report
// option drives (PROBE-086, PROBE-105; REQ-170): the keys the codec reports as
// unmatched content leave the body, each as one [Refusal] in document order,
// and nothing else does. Without the exclusion they would stay in the body
// and come back as Missing, since the decode ignored them.
func TestDecodeReducingExcludesWhatTheCodecReports(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	base, leaf := modelledQuantityLeaf(t, target)
	body := maps.Clone(leaf)
	unmatched := []string{base + "|x_note", target.Root + "/no_such_node"}
	for _, k := range unmatched {
		body[k] = "x"
	}
	_, refusals, err := DecodeReducing(target, body, InjectContext)
	if err != nil {
		t.Fatalf("DecodeReducing() error = %v", err)
	}
	var want []Refusal
	for _, k := range slices.Sorted(slices.Values(unmatched)) {
		want = append(want, Refusal{Key: k, Reason: UnmatchedReason, Message: UnmatchedMessage, Keys: 1})
	}
	if !slices.Equal(refusals, want) {
		t.Errorf("DecodeReducing() refusals = %+v, want %+v", refusals, want)
	}
	if !maps.Equal(body, leaf) {
		t.Errorf("DecodeReducing() left %v, want exactly the modelled leaf %v", slices.Sorted(maps.Keys(body)), slices.Sorted(maps.Keys(leaf)))
	}
	for _, r := range refusals {
		if !r.Unmatched() {
			t.Errorf("refusal %+v is not marked unmatched", r)
		}
	}
}

// TestExcludeUnmatchedRefusesAKeyTheBodyLacks pins the harness-fault guard: a
// key the codec reports must be one the body carries as written, so a report of
// any other key fails rather than being skipped.
func TestExcludeUnmatchedRefusesAKeyTheBodyLacks(t *testing.T) {
	body := map[string]any{"c/a": "1"}
	refusals, err := excludeUnmatched(body, []string{"c/a", "c/b"})
	if err == nil || !strings.Contains(err.Error(), `"c/b"`) {
		t.Fatalf("excludeUnmatched() error = %v, want one naming the key the body lacks", err)
	}
	if len(refusals) != 1 || len(body) != 0 {
		t.Errorf("excludeUnmatched() = %+v, body %v; want the carried key excluded before the fault", refusals, body)
	}
}

// TestDecodeReducingUnknownMode pins that a mode outside the two is a harness
// fault, not a decode refusal.
func TestDecodeReducingUnknownMode(t *testing.T) {
	target, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	_, _, err = DecodeReducing(target, map[string]any{}, ContextMode(99))
	if err == nil {
		t.Fatal("DecodeReducing(unknown mode) error = nil, want an error")
	}
	if _, ok := errors.AsType[*IrreducibleError](err); ok {
		t.Errorf("DecodeReducing(unknown mode) error = %v, want a harness fault, not an *IrreducibleError", err)
	}
}

// TestNewTargetFromOPT pins that NewTarget is NewTargetFromOPT over the corpus
// OPT, and that an unreadable OPT is an error.
func TestNewTargetFromOPT(t *testing.T) {
	want, err := NewTarget()
	if err != nil {
		t.Fatalf("NewTarget() error = %v", err)
	}
	got, err := NewTargetFromOPT(fixtures.FlatConformanceOpt())
	if err != nil {
		t.Fatalf("NewTargetFromOPT(corpus OPT) error = %v", err)
	}
	if got.Root != want.Root || got.Root != corpusRoot {
		t.Errorf("NewTargetFromOPT(corpus OPT).Root = %q, want %q", got.Root, corpusRoot)
	}
	if _, err := NewTargetFromOPT("does/not/exist.opt"); err == nil {
		t.Error("NewTargetFromOPT(missing file) error = nil, want an error")
	}
}
