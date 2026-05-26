package compositionprobes

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// Assignment is one (path, expected wire-fragment) pair used by
// Probe023BuilderRoundTrip. Apply is the per-assignment hook that
// invokes the appropriate typed SetQuantity / SetText / SetCodedText
// helper — passed as a closure so the probe stays generic across
// primitive types.
//
// WireFragments holds the byte sequences the marshalled output MUST
// contain for the assignment to be considered round-tripped.
type Assignment struct {
	// Path is purely diagnostic — the actual Set call is wrapped
	// inside Apply. Kept on the struct so the probe Detail field
	// can name the failing assignment.
	Path string
	// Apply runs the Set call against the supplied Builder.
	Apply func(b *composition.Builder) error
	// WireFragments enumerates the byte sequences expected in the
	// marshalled composition (one match per fragment).
	WireFragments [][]byte
}

// Probe023BuilderRoundTrip exercises the canonical authoring round-
// trip: NewBuilder over c, apply each Assignment, Build,
// canjson.Marshal, canjson.Unmarshal back into a fresh *rm.Composition,
// re-marshal, and verify every fragment in every Assignment appears in
// BOTH the first marshal AND the post-unmarshal re-marshal — the
// REQ-101 + PROBE-023 normative round-trip (REQ-107 UID emission
// landed via the archived
// [`docs/plans/archive/2026-05-26-c-primitive-object-wire-parser.md`]).
// The probe is sandbox-only (no transport dependency); cross-SDK
// parity means another implementation of REQ-101 against the same
// OPT + assignments MUST produce the same pass outcome.
func Probe023BuilderRoundTrip(ctx context.Context, c *templatecompile.Compiled, opts []composition.Option, assigns []Assignment) (Result, error) {
	r := Result{Probe: "PROBE-023"}
	if c == nil || c.Root() == nil {
		return r, fmt.Errorf("PROBE-023: nil compiled template")
	}
	if rt := c.Root().RMTypeName(); rt != "COMPOSITION" {
		r.Status = "skip"
		r.Detail = fmt.Sprintf("OPT root %q not COMPOSITION; v1 probe scope is composition-only", rt)
		return r, nil
	}

	b, err := composition.NewBuilder(ctx, c, opts...)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("NewBuilder: %v", err)
		return r, nil
	}
	var failures []string
	for _, a := range assigns {
		if err := a.Apply(b); err != nil {
			failures = append(failures, fmt.Sprintf("Set@%s: %v", a.Path, err))
		}
	}
	comp, err := b.Build()
	if err != nil {
		failures = append(failures, fmt.Sprintf("Build: %v", err))
	}
	payload, err := canjson.Marshal(comp)
	if err != nil {
		failures = append(failures, fmt.Sprintf("canjson.Marshal: %v", err))
	}
	for _, a := range assigns {
		for _, frag := range a.WireFragments {
			if !bytes.Contains(payload, frag) {
				failures = append(failures, fmt.Sprintf("Set@%s: marshalled output missing fragment %q", a.Path, string(frag)))
			}
		}
	}
	// Unmarshal round-trip — the full PROBE-023 promise that
	// REQ-107 Phase 2 unblocked. Decode the marshalled payload back
	// into a fresh *rm.Composition (proving the canjson polymorphic
	// dispatch on Composition.uid + nested DataValues works
	// symmetrically), then re-marshal and assert the same fragment
	// set survives the round-trip.
	var decoded rm.Composition
	if err := canjson.Unmarshal(payload, &decoded); err != nil {
		failures = append(failures, fmt.Sprintf("canjson.Unmarshal: %v", err))
	} else {
		reMarshalled, err := canjson.Marshal(&decoded)
		if err != nil {
			failures = append(failures, fmt.Sprintf("canjson.Marshal (after Unmarshal): %v", err))
		} else {
			for _, a := range assigns {
				for _, frag := range a.WireFragments {
					if !bytes.Contains(reMarshalled, frag) {
						failures = append(failures, fmt.Sprintf("Set@%s: round-trip output missing fragment %q", a.Path, string(frag)))
					}
				}
			}
		}
	}

	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}
