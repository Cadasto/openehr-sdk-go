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
	// after them, so this unknown path, sorting first, is refused and removed
	// before the irreducible error stops the loop.
	unknown := target.Root + "/0_no_such_node"
	if unknown >= base {
		t.Fatalf("unknown path %q does not sort before the leaf %q; pick another", unknown, base)
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
			body[unknown+"|code"], body[unknown+"|value"] = "x", "y"
			tt.change(body)

			_, refusals, err := DecodeReducing(target, body, tt.mode)
			if ie, ok := errors.AsType[*IrreducibleError](err); !ok || ie == nil || ie.Key != tt.wantKey {
				t.Fatalf("DecodeReducing() error = %v, want an *IrreducibleError with key %q", err, tt.wantKey)
			}
			want := []Refusal{{
				Key:     unknown,
				Reason:  "path not in web template",
				Message: `simplified: path not in web template: "<key>"`,
				Keys:    2,
			}}
			if !slices.Equal(refusals, want) {
				t.Errorf("DecodeReducing() refusals = %+v, want %+v", refusals, want)
			}
			if _, ok := body[unknown+"|code"]; ok {
				t.Errorf("body still carries %s|code; the refusal that removed it must have been applied", unknown)
			}
		})
	}

	t.Run("refusal budget breached", func(t *testing.T) {
		// One unknown path more than the budget allows, each its own family.
		body := make(map[string]any, maxRefusals+1)
		for i := range maxRefusals + 1 {
			body[fmt.Sprintf("%s/no_such_node_%03d", target.Root, i)] = "x"
		}
		_, refusals, err := DecodeReducing(target, body, InjectContext)
		if err == nil || !strings.Contains(err.Error(), "did not converge") {
			t.Fatalf("DecodeReducing(%d unknown paths) error = %v, want the budget breach", maxRefusals+1, err)
		}
		if _, ok := errors.AsType[*IrreducibleError](err); ok {
			t.Errorf("DecodeReducing(%d unknown paths) error = %v, want a harness fault, not an *IrreducibleError", maxRefusals+1, err)
		}
		if len(refusals) != maxRefusals || len(body) != 1 {
			t.Errorf("DecodeReducing(%d unknown paths) = %d refusals, %d keys left; want %d refusals and 1 key left",
				maxRefusals+1, len(refusals), len(body), maxRefusals)
		}
	})
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
