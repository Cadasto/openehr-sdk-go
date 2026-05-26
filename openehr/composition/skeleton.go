package composition

import (
	"context"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// NewSkeleton produces a structurally-conformant *rm.Composition for
// c with no clinical data. Delegates to instance.Generate with
// Policy: Minimal — every required RM attribute is filled (BMM-
// mandatory implicits + OPT-declared) and primitive leaves carry
// REQ-103 ExampleValue defaults so the resulting tree is valid
// against `validation.ValidateComposition`.
//
// WithComposer and WithTerritory are required for COMPOSITION roots
// (instance.Generate enforces); WithLanguage / WithCategory / WithNow
// are optional defaults documented per Option.
func NewSkeleton(ctx context.Context, c *templatecompile.Compiled, opts ...Option) (*rm.Composition, error) {
	if c == nil || c.Root() == nil {
		return nil, instance.ErrNilCompiled
	}
	if rt := c.Root().RMTypeName(); rt != "COMPOSITION" {
		return nil, fmt.Errorf("composition.NewSkeleton: OPT root %q is not COMPOSITION", rt)
	}
	cfg := buildConfig(opts...)
	v, err := instance.Generate(ctx, c, instance.Options{
		Policy:    instance.Minimal,
		Language:  cfg.language,
		Territory: cfg.territory,
		Composer:  cfg.composer,
		Now:       cfg.now,
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
