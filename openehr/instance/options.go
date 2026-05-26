package instance

import (
	"errors"
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
