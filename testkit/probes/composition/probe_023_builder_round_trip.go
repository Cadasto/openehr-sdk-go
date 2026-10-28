package compositionprobes

import (
	"bytes"
	"context"
	"errors"
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
// helper, passed as a closure so the probe stays generic across
// primitive types.
//
// WireFragments holds the byte sequences the marshalled output must
// contain for the assignment to be considered round-tripped.
type Assignment struct {
	// Path is purely diagnostic; the actual Set call is wrapped
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
// both the first marshal and the post-unmarshal re-marshal (the
// PROBE-023 round-trip). The probe needs no client or server. For
// openEHR conformance, another implementation of the builder against
// the same OPT and assignments must produce the same pass outcome.
func Probe023BuilderRoundTrip(ctx context.Context, c *templatecompile.Compiled, opts []composition.Option, assigns []Assignment) (Result, error) { // PROBE-023 (REQ-082, REQ-101)
	return probe023(ctx, c, opts, assigns, canjsonRoundTrip)
}

// canjsonRoundTrip decodes payload into a fresh *rm.Composition and
// marshals the result again: the second leg of PROBE-023. Decoding into a
// fresh value shows that the canjson dispatch on the nested data values
// works in both directions.
func canjsonRoundTrip(payload []byte) ([]byte, error) {
	var decoded rm.Composition
	if err := canjson.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("canjson.Unmarshal: %w", err)
	}
	out, err := canjson.Marshal(&decoded)
	if err != nil {
		return nil, fmt.Errorf("canjson.Marshal (after Unmarshal): %w", err)
	}
	return out, nil
}

// probe023 is Probe023BuilderRoundTrip with the second leg passed in, so a
// test can change what that leg returns and leave the first one alone.
func probe023(ctx context.Context, c *templatecompile.Compiled, opts []composition.Option, assigns []Assignment, roundTrip func(payload []byte) ([]byte, error)) (Result, error) {
	r := Result{Probe: "PROBE-023"}
	if c == nil || c.Root() == nil {
		return r, errors.New("PROBE-023: nil compiled template")
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
	// The second leg: decode the marshalled payload, re-marshal it, and
	// check the same fragment set survives.
	reMarshalled, err := roundTrip(payload)
	if err != nil {
		failures = append(failures, err.Error())
	} else {
		for _, a := range assigns {
			for _, frag := range a.WireFragments {
				if !bytes.Contains(reMarshalled, frag) {
					failures = append(failures, fmt.Sprintf("Set@%s: round-trip output missing fragment %q", a.Path, string(frag)))
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
