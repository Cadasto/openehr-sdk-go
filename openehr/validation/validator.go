package validation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// The limits a [Validator] built with no options applies. The largest
// corpus document needs about a thousand visits, but a decoded document can
// reach either limit ([Limits]).
const (
	defaultDepth = 256
	defaultNodes = 100_000
)

// Limits bounds the walk, with a template as without one, so that a value
// built in Go that nests without end, or holds itself through a list that
// shares its own backing array, stops the walk rather than running it
// without end. A decoded document can reach either limit too: a large one
// the visit limit, a deeply nested one the depth limit. A field that is
// zero or negative stands for its default, so no setting turns a limit off.
type Limits struct {
	// Depth is the nesting depth at which the walk stops descending. On a
	// node that deep the walk still checks the node itself: its identity
	// and type against the template node it binds to, the invariants of
	// its class and its coded terms, and the template's value constraint.
	// But it reports none of the node's absent attributes or empty lists,
	// and walks none of its attributes. The walk reports the first place it
	// stops as a max_depth issue. Where it binds a value by trying it under
	// several template nodes, a trial the limit stops counts as one the
	// value fails, and a stop inside a trial the walk withdraws is not
	// reported. 256 by default.
	//
	// There is no upper cap. Raising it raises the cost of walking a deeply
	// nested value faster than in proportion to the depth, since the path
	// written for each node grows with its depth.
	Depth int

	// Nodes is the number of visits the walk makes in one call before it
	// stops, reporting a max_nodes issue where it stopped. An item of a list
	// that holds no value, a nil element or a nil pointer, counts as one
	// visit too, though the walk does not enter it. Where the walk binds a
	// value by trying it under several template nodes, each trial counts its
	// visits, so a value can count more than once, though one held by
	// pointer is tried under a node only once, and a node reached only
	// inside a trial the walk withdraws reports nothing. 100,000 by default.
	//
	// The stop ends the template check as well as the Reference Model
	// rules, since both run in the one walk. A node reached before the stop
	// has been checked itself, as for a node at the depth limit above, and
	// those checks can report at the path of an attribute the walk never
	// entered. What the stop leaves unchecked is the node where it stops,
	// every node after it, and the presence and counts of the attributes not
	// yet visited, so a defect only those checks would find is not reported.
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

// WithLimits sets the limits of the walk. Each field that is zero or
// negative keeps its default. When WithLimits is passed more than
// once, the last one wins, its fields replacing all of those set before.
func WithLimits(l Limits) Option {
	return func(c *config) { c.limits = l }
}

// Validator validates in-memory Reference Model values, with the template
// rules of a compiled template when the caller gives one, and the Reference
// Model rules always. It holds its settings only, never what belongs to one
// call: a decode's report of the members a document leaves out travels with
// the call, as [WithMissing]. Build it once with [New] and share it between
// goroutines; nothing a call does changes it. The zero Validator, and a nil
// *Validator, behave as one built by New with no options.
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
// template it walks root against the template and applies the Reference
// Model rules in the same walk, as [Validate] describes, under the guards
// [Validate] describes. The limits of v bound the walk. Built with no
// options, v returns what [ValidateRM] returns for a nil c, its guards
// included, and what [Validate] returns for a compiled template, given the
// same opts.
//
// A value built in Go cannot say which of its members a document left out:
// a number, a boolean or a value struct holds its zero value either way. A
// caller that decoded root with a codec's report of missing members passes
// it as [WithMissing], so that each member it names is read as absent;
// [Validator.ValidateJSON] and [Validator.ValidateXML] decode and validate
// in one call.
//
// [Result.Coverage] lists each path where a limit stopped the walk, with a
// template each node the template binds to a CONSTRAINT_REF, and the
// Reference Model invariants of the classes the walk visited that the
// validator does not evaluate.
func (v *Validator) Validate(root any, c *templatecompile.Compiled, opts ...ValidateOption) Result {
	return runEngine(root, c, v.limitsOrDefault(), opts...)
}
