package composition

import (
	"errors"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Option configures NewSkeleton and NewBuilder. Constructed via the
// With* helpers; the unexported config struct keeps the contract
// closed.
type Option func(*config)

// config carries the resolved option values. Translated to
// instance.Options inside NewSkeleton / NewBuilder so this package
// stays the single source of composition-specific knobs.
type config struct {
	language  string
	territory string
	composer  rm.PartyProxy
	category  *rm.DVCodedText
	now       time.Time
}

// WithLanguage overrides the OPT-declared language for
// Composition.language. The supplied code is interpreted as an ISO
// 639-1 entry under terminology ISO_639-1 (set inside the
// instance.Generate post-walk defaults pass).
func WithLanguage(lang string) Option {
	return func(c *config) { c.language = lang }
}

// WithTerritory sets Composition.territory. Required when the OPT
// root is COMPOSITION — instance.Generate returns
// ErrTerritoryRequired otherwise.
func WithTerritory(code string) Option {
	return func(c *config) { c.territory = code }
}

// WithComposer sets Composition.composer. Required when the OPT root
// is COMPOSITION — instance.Generate returns ErrComposerRequired
// otherwise.
func WithComposer(p rm.PartyProxy) Option {
	return func(c *config) { c.composer = p }
}

// WithCategory overrides the default 433|event| Composition.category.
// The caller-supplied value is applied to the skeleton after
// instance.Generate has run, so the OPT-declared category (if any)
// is overwritten — same semantics as ehrbase's WebTemplateSkeletonBuilder.
func WithCategory(c rm.DVCodedText) Option {
	return func(cf *config) {
		v := c
		cf.category = &v
	}
}

// WithNow injects the clock used by instance.Generate for EVENT /
// EventContext start_time defaults. Zero value lets the default
// `time.Now().UTC()` apply; tests pin a fixed timestamp for
// determinism.
func WithNow(t time.Time) Option {
	return func(c *config) { c.now = t }
}

// ErrUnknownPath signals that the supplied path is not addressable in
// the compiled template. Returned by Builder.Set and aggregated at
// Build time. Comparable via errors.Is.
var ErrUnknownPath = errors.New("composition: path not in template")

// ErrTypeMismatch signals that the Go value supplied to Builder.Set
// does not match the compiled-node RM type at the path. Comparable
// via errors.Is.
var ErrTypeMismatch = errors.New("composition: value type does not match path RM type")

// ErrInvalidPath signals a structural problem navigating the path
// against the skeleton (missing parent, no addressable attribute on
// parent). Distinct from ErrUnknownPath: the path IS in the template
// but the live skeleton lacks the addressable parent.
var ErrInvalidPath = errors.New("composition: path not addressable on skeleton")
