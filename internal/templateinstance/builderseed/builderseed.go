// Package builderseed marks a call to the instance generator as the one
// the composition builder seeds a Builder with.
//
// The builder routes each value into a node its skeleton already holds, so
// it seeds a Builder with one member of each optional archetype and of each
// use of one, and nothing more. The instance generator's Large policy
// builds more than that: a second member of each node that repeats, and
// optional RM attributes the template does not name. The builder asks for
// its narrower skeleton through the context it passes the generator, not
// through an exported option: the narrower skeleton serves the builder
// alone, so the generator's and the builder's public options do not carry
// it, and only packages of this module can set it.
//
// The package imports only the standard library, so the composition
// builder and the instance generator both import it without a cycle.
package builderseed

import "context"

// key is the context key [With] sets.
type key struct{}

// With returns a copy of ctx that marks the generator call made with it as
// the composition builder's seed.
func With(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, true)
}

// Asked reports whether ctx carries the mark [With] sets.
func Asked(ctx context.Context) bool {
	marked, _ := ctx.Value(key{}).(bool)
	return marked
}
