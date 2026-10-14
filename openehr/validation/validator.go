package validation

import (
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// The limits a [Validator] built with no options applies. No decoded
// document comes near either: the largest corpus document needs about a
// thousand visits.
const (
	defaultDepth = 256
	defaultNodes = 100_000
)

// Limits bounds the walk over the Reference Model rules, so that a value
// built in Go that nests without end, or holds itself through a list that
// shares its own backing array, stops the walk rather than running it
// without end. A field that is zero or negative stands for its default, so
// no setting turns a limit off.
//
// The walk the template drives is bounded by the compiled template it
// follows, so the limits apply to the Reference Model walk alone.
type Limits struct {
	// Depth is the nesting depth at which the walk stops descending. On a
	// node that deep the walk still checks the invariants of the node's
	// class and its coded terms, but it reports none of the node's absent
	// mandatory attributes or empty mandatory lists, and walks none of its
	// attributes. The walk reports the first place it stops as a max_depth
	// issue. 256 by default.
	//
	// There is no upper cap. Raising it raises the cost of walking a deeply
	// nested value faster than in proportion to the depth, since the path
	// written for each node grows with its depth.
	Depth int

	// Nodes is the number of values the walk visits in one call before it
	// stops, reporting a max_nodes issue where it stopped. 100,000 by
	// default.
	Nodes int
}

// withDefaults returns l with each field that is zero or negative replaced
// by its default.
func (l Limits) withDefaults() Limits {
	if l.Depth <= 0 {
		l.Depth = defaultDepth
	}
	if l.Nodes <= 0 {
		l.Nodes = defaultNodes
	}
	return l
}

// config is what the options of [New] set.
type config struct {
	limits Limits
}

// Option configures a [Validator]; pass options to [New].
type Option func(*config)

// WithLimits sets the limits of the Reference Model walk. Each field that
// is zero or negative keeps its default. When WithLimits is passed more than
// once, the last one wins, its fields replacing all of those set before.
func WithLimits(l Limits) Option {
	return func(c *config) { c.limits = l }
}

// Validator validates in-memory Reference Model values, with the template
// rules of a compiled template when the caller gives one, and the Reference
// Model rules always. It holds its settings only: build it once with [New]
// and share it between goroutines; nothing a call does changes it. The zero
// Validator, and a nil *Validator, behave as one built by New with no
// options.
//
// The package-level functions, [Validate], [ValidateComposition],
// [ValidateRM] and the others, behave as a Validator built with no options.
// A caller that needs other limits builds one.
type Validator struct {
	// limits is held as the options gave it; a zero or negative field
	// stands for its default, which is what the zero Validator holds.
	limits Limits
}

// New returns a [Validator] configured by opts; a nil option is ignored.
// With no options it applies the default [Limits].
func New(opts ...Option) *Validator {
	var c config
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return &Validator{limits: c.limits}
}

// limitsOrDefault returns the limits v applies, with each default filled in.
// A nil v applies the defaults.
func (v *Validator) limitsOrDefault() Limits {
	if v == nil {
		return Limits{}.withDefaults()
	}
	return v.limits.withDefaults()
}

// Validate validates root, any Reference Model value the SDK registers.
//
// With a nil c it applies the Reference Model rules alone. With a compiled
// template it runs the template walk and then the Reference Model rules,
// whose issues follow the template walk's less those that repeat one, under
// the guards [Validate] describes. The limits of v bound the Reference Model
// walk. Built with no options, v returns what [ValidateRM] returns for a nil
// c, its guards included, and what [Validate] returns for a compiled
// template.
//
// [Result.Coverage] lists each path where a limit stopped the walk, with a
// template each node the template binds to a CONSTRAINT_REF, and the
// Reference Model invariants of the classes either walk visited that the
// validator does not evaluate.
func (v *Validator) Validate(root any, c *templatecompile.Compiled) Result {
	lim := v.limitsOrDefault()
	if c == nil {
		return validateRM(root, lim)
	}
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return nilRootResult()
	}
	if c.Root() == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "compiled template has no root node",
			Severity: Error,
		}})
	}

	w := newWalker(c)
	w.walkNode(c.Root(), root, "/")
	floor, stopped, visited := floorIssues(root, w.names, lim)
	r := resultFromIssues(withoutRepeats(w.issues, floor))
	r.Coverage = Coverage{Stopped: stopped, External: w.external, Unevaluated: unevaluatedIn(visited, w.visited)}
	return r
}
