package instance

import (
	"errors"
	mrand "math/rand/v2"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Policy controls how much of the OPT tree is materialised.
type Policy int

const (
	// Minimal materialises only attributes with existence lower ≥ 1
	// and BMM-mandatory implicits. Smallest valid tree.
	Minimal Policy = iota
	// Example materialises Minimal plus every primitive leaf
	// populated with its REQ-103 ExampleValue. Default for fixtures.
	Example
)

// String returns "minimal" / "example" / "unknown(N)" for diagnostics.
func (p Policy) String() string {
	switch p {
	case Minimal:
		return "minimal"
	case Example:
		return "example"
	}
	return "unknown"
}

// ValueFill controls how primitive leaves are valued, orthogonally to
// [Policy] (which controls *which* nodes are materialised). REQ-107.
type ValueFill int

const (
	// ExampleFill is the default: each primitive leaf gets its REQ-103
	// PrimitiveConstraint.ExampleValue — a single representative value,
	// byte-identical across calls for one OPT.
	ExampleFill ValueFill = iota
	// RandomFill draws each leaf from within its constraint (in-range
	// magnitudes, value-set-member codes, enumeration entries), valid by
	// construction and varying between calls. Use [Options.ValueSource]
	// to make a run reproducible.
	RandomFill
)

// Any ValueFill other than RandomFill behaves as ExampleFill (the
// RandomFill check in Generate is exact), so an out-of-range value
// degrades to the deterministic example fill rather than erroring.

// String returns "example" / "random" / "unknown" for diagnostics.
func (f ValueFill) String() string {
	switch f {
	case ExampleFill:
		return "example"
	case RandomFill:
		return "random"
	}
	return "unknown"
}

// Options configures one [Generate] invocation. Zero value is valid
// for non-COMPOSITION roots: Minimal policy, English language,
// time.Now().UTC() clock.
type Options struct {
	// Policy is Minimal or Example. Zero value = Minimal.
	Policy Policy

	// Language is the ISO 639-1 code for DV_TEXT / Composition.language /
	// Entry.language defaults. Empty falls back to the compiled
	// template's Language(), then "en".
	Language string

	// Territory is the ISO 3166-1 alpha-2 country code required on
	// COMPOSITION roots. Non-COMPOSITION roots ignore it. Empty on a
	// COMPOSITION root returns ErrTerritoryRequired.
	Territory string

	// Composer is the party responsible for the composition content.
	// Required when the root rm_type_name is COMPOSITION; nil returns
	// ErrComposerRequired. Non-COMPOSITION roots ignore it.
	Composer rm.PartyProxy

	// Now is the clock for EVENT.time / EventContext.start_time
	// defaults. Zero value falls back to time.Now().UTC() inside
	// Generate so callers that don't pin the clock get a sensible
	// default; tests pin it for determinism.
	Now time.Time

	// UIDSource is the optional generator for LOCATABLE.uid values.
	// Each LOCATABLE root that openEHR requires a uid on (Composition,
	// Observation, Evaluation, Instruction, Action, AdminEntry,
	// GenericEntry) calls UIDSource once during synthesis. Nil falls
	// back to a random RFC 4122 v4 UUID via crypto/rand. Tests pin a
	// counter or named-seed source for deterministic UIDs in golden
	// fixtures.
	UIDSource func() *rm.HierObjectID

	// ValueFill selects how primitive leaves are valued. Zero value =
	// ExampleFill (the REQ-103 representative value). RandomFill draws
	// in-constraint values that vary between calls. REQ-107.
	ValueFill ValueFill

	// ValueSource seeds the in-constraint sampler used when ValueFill is
	// RandomFill; ignored otherwise. A fixed source (e.g.
	// rand.NewPCG(seed, seed)) makes the leaf values byte-reproducible;
	// nil draws from the package-global, auto-seeded generator so
	// successive calls differ. Mirrors the UIDSource seam.
	//
	// A math/rand/v2.Source is not safe for concurrent use: do not share
	// one Source across concurrent Generate calls. Give each goroutine its
	// own source (or leave it nil to use the concurrency-safe global).
	ValueSource mrand.Source
}

// ErrTerritoryRequired signals that a COMPOSITION-root template
// invocation omitted the Territory option.
var ErrTerritoryRequired = errors.New("instance.Generate: Territory is required for COMPOSITION roots")

// ErrComposerRequired signals that a COMPOSITION-root template
// invocation omitted the Composer option.
var ErrComposerRequired = errors.New("instance.Generate: Composer is required for COMPOSITION roots")

// ErrTypeMismatch is returned by AsComposition / AsObservation /
// etc. when the produced root is not the requested concrete type.
var ErrTypeMismatch = errors.New("instance: root type does not match requested accessor")

// ErrNilCompiled signals a nil compiled-template argument.
var ErrNilCompiled = errors.New("instance.Generate: nil compiled template")

// ErrSlotFillUnsupported signals that a required ARCHETYPE_SLOT fill
// could not be synthesized safely from the parsed slot assertions.
var ErrSlotFillUnsupported = errors.New("instance.Generate: required slot fill cannot be synthesized")
