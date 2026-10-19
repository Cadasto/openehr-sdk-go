package composition

import (
	"errors"
	mrand "math/rand/v2"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
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
	language    string
	territory   string
	composer    rm.PartyProxy
	category    *rm.DVCodedText
	now         time.Time
	policy      instance.Policy
	policySet   bool // WithPolicy was given; its policy can be the zero value
	valueFill   instance.ValueFill
	valueSource mrand.Source
}

// WithLanguage overrides the OPT-declared language for
// Composition.language. The supplied code is interpreted as an ISO
// 639-1 entry under terminology ISO_639-1 (set inside the
// instance.Generate post-walk defaults pass).
func WithLanguage(lang string) Option {
	return func(c *config) { c.language = lang }
}

// WithTerritory sets Composition.territory. Required when the OPT
// root is COMPOSITION; instance.Generate returns
// ErrTerritoryRequired otherwise.
func WithTerritory(code string) Option {
	return func(c *config) { c.territory = code }
}

// WithComposer sets Composition.composer. Required when the OPT root
// is COMPOSITION; instance.Generate returns ErrComposerRequired
// otherwise.
func WithComposer(p rm.PartyProxy) Option {
	return func(c *config) { c.composer = p }
}

// WithCategory overrides the default 433|event| Composition.category.
// The caller-supplied value is applied to the skeleton after
// instance.Generate has run, so the OPT-declared category (if any)
// is overwritten. This matches EHRbase's WebTemplateSkeletonBuilder.
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

// WithPolicy selects the instance.Policy NewSkeleton passes to
// instance.Generate, which decides which nodes the skeleton holds.
// NewSkeleton's default is the instance package's, instance.Medium, which
// builds at most one optional archetype under an attribute.
// instance.Large builds a member of every optional archetype the template
// names, the nodes Set needs to route a value into, and instance.Required
// builds only what the template makes mandatory and a composition's
// context; instance.Complete and instance.Minimal are other names for
// Large and Required. NewSkeleton accepts the three. NewBuilder builds on
// Large when no WithPolicy is given and accepts only Large, since Set does
// not build a node the skeleton lacks: it refuses an explicit
// WithPolicy(instance.Medium), though Medium is the zero value. Each
// returns ErrUnsupportedPolicy for a Policy value it does not accept.
func WithPolicy(p instance.Policy) Option {
	return func(c *config) {
		c.policy = p
		c.policySet = true
	}
}

// WithValueFill selects how primitive leaves are valued. The default
// (instance.ExampleFill) emits each constraint's representative example
// value; instance.RandomFill draws in-constraint values that vary per
// call. Seed it with WithValueSource for reproducibility.
func WithValueFill(f instance.ValueFill) Option {
	return func(c *config) { c.valueFill = f }
}

// WithValueSource seeds the in-constraint sampler used under
// instance.RandomFill. A fixed source (e.g. rand.NewPCG(seed, seed))
// makes the generated leaf values byte-reproducible; nil draws from the
// auto-seeded global generator so successive calls differ. Ignored
// under ExampleFill.
//
// A math/rand/v2.Source is not safe for concurrent use: do not share one
// Source across concurrent NewSkeleton / Build calls. Give each its own,
// or leave it nil to use the concurrency-safe global.
func WithValueSource(src mrand.Source) Option {
	return func(c *config) { c.valueSource = src }
}

// ErrUnknownPath signals that the supplied path is not addressable in
// the compiled template. Returned by Builder.Set and aggregated at
// Build time. Comparable via errors.Is.
var ErrUnknownPath = errors.New("composition: path not in template")

// ErrTypeMismatch signals that the value supplied to Builder.Set is
// not one the template allows at the path: it binds none of the nodes
// the template allows there, by the rule the validator binds a value
// with. A value binds a node whose RM type it has or descends from, a
// node typed with an AOM 1.4 primitive short name it carries (a
// DV_DURATION at DURATION), an untyped node, and a typed interval node
// DV_INTERVAL<T> when each of its bounds is a T; Builder.Set lists the
// cases. Every alternative the template lists at the path counts, except
// one whose occurrences are 0..0. A value that binds a node there but
// that the RM attribute at the path cannot hold is refused too: a
// DV_DURATION at the DURATION node under a DV_DURATION's value, which
// holds a String, or a DV_TEXT at a bound of a DV_INTERVAL the template
// does not parameterise, which holds a DV_ORDERED. So is any value at a
// path that writes no RM attribute, such as a template's constraint on
// DV_PROPORTION.is_integral, which the model defines as a function. A nil
// value, a nil pointer and a value that is not an RM value at all are
// refused the same way. Comparable via errors.Is.
var ErrTypeMismatch = errors.New("composition: value type does not match path RM type")

// ErrInvalidPath signals a structural problem navigating the path
// against the skeleton (missing parent, no addressable attribute on
// parent). Distinct from ErrUnknownPath: the path IS in the template
// but the live skeleton lacks the addressable parent.
var ErrInvalidPath = errors.New("composition: path not addressable on skeleton")

// ErrUnsupportedPolicy signals that an entry point refuses the policy
// WithPolicy selects, before it generates anything. NewBuilder returns it
// for instance.Medium and instance.Required (and so instance.Minimal),
// naming the policy, since Set routes a value only into a node the
// skeleton already holds and only an instance.Large skeleton holds a
// member of every optional archetype. NewBuilder also returns it for any
// Policy value the instance package does not define. NewSkeleton accepts
// instance.Medium, instance.Required and instance.Large, and returns it
// for any other Policy value. Comparable via errors.Is.
var ErrUnsupportedPolicy = errors.New("composition: unsupported skeleton policy")
