package composition

import (
	"context"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// NewSkeleton produces a structurally-conformant *rm.Composition for
// c with no clinical data. It delegates to instance.Generate with the
// policy WithPolicy selects, instance.Medium, the instance package's
// default, when none is given: every required RM attribute is filled
// (BMM-mandatory implicits and OPT-declared ones), the optional content
// the policy builds is added (under instance.Medium, at most one optional
// archetype under an attribute; under instance.Large, a member of every
// optional archetype the template names; under instance.Required, none),
// and primitive leaves carry ExampleValue defaults, so the resulting tree
// is valid against `validation.ValidateComposition`.
// NewSkeleton accepts instance.Medium, instance.Required and
// instance.Large, and so their other names instance.Minimal and
// instance.Complete. It refuses any other Policy value, before generating
// anything, with an error wrapping both ErrUnsupportedPolicy and
// instance.ErrUnsupportedPolicy.
//
// WithComposer and WithTerritory are required for COMPOSITION roots
// (instance.Generate enforces); WithLanguage / WithCategory / WithNow
// are optional defaults documented per Option. WithValueFill
// (instance.RandomFill) with WithValueSource switches leaves from the
// fixed ExampleValue to seeded, sampled in-constraint values, for a
// varied but valid corpus.
func NewSkeleton(ctx context.Context, c *templatecompile.Compiled, opts ...Option) (*rm.Composition, error) {
	return newSkeleton(ctx, c, buildConfig(opts...))
}

// newSkeleton is NewSkeleton on the options cfg holds. NewSkeleton passes
// the zero Policy, the instance package's default, when no WithPolicy is
// given; NewBuilder passes Large in that case.
func newSkeleton(ctx context.Context, c *templatecompile.Compiled, cfg *config) (*rm.Composition, error) {
	if c == nil || c.Root() == nil {
		return nil, instance.ErrNilCompiled
	}
	if rt := c.Root().RMTypeName(); rt != "COMPOSITION" {
		return nil, fmt.Errorf("composition.NewSkeleton: OPT root %q is not COMPOSITION", rt)
	}
	switch cfg.policy {
	case instance.Medium, instance.Required, instance.Large:
		// The policies the instance package defines.
	default:
		return nil, errUndefinedPolicy("composition.NewSkeleton", cfg.policy)
	}
	v, err := instance.Generate(ctx, c, instance.Options{
		Policy:      cfg.policy,
		Language:    cfg.language,
		Territory:   cfg.territory,
		Composer:    cfg.composer,
		Now:         cfg.now,
		ValueFill:   cfg.valueFill,
		ValueSource: cfg.valueSource,
	})
	if err != nil {
		return nil, fmt.Errorf("composition.NewSkeleton: %w", err)
	}
	comp, err := instance.AsComposition(v)
	if err != nil {
		return nil, fmt.Errorf("composition.NewSkeleton: %w", err)
	}
	if cfg.category != nil {
		comp.Category = *cfg.category
	}
	return comp, nil
}

// errUndefinedPolicy returns the error the entry point op gives for p, a
// Policy value that is none of those the instance package defines. It
// wraps ErrUnsupportedPolicy, and instance.ErrUnsupportedPolicy as
// instance.Generate does for such a value, and prints p's number, since
// Policy's String names every such value "unknown".
func errUndefinedPolicy(op string, p instance.Policy) error {
	return fmt.Errorf("%s: %w: %w: %d", op, ErrUnsupportedPolicy, instance.ErrUnsupportedPolicy, p)
}

// buildConfig folds opts into a config struct.
func buildConfig(opts ...Option) *config {
	cfg := &config{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return cfg
}
