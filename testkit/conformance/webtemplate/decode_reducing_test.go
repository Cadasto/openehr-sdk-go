package webtemplate

// Tests for the exported reducing decode that PROBE-105 shares with PROBE-086
// (REQ-080): the two context modes, and the typed error a decode the loop
// cannot reduce comes back as.

import (
	"errors"
	"maps"
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
