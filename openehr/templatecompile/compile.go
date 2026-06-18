package templatecompile

import (
	impl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Compiled is the public, externally-constructable compiled template.
// Produce one with [Compile]; the zero value is not useful.
//
// It is a type alias of the engine's compiled form, so a *Compiled is
// accepted directly by openehr/composition, openehr/instance,
// openehr/validation, and openehr/aql/lint without conversion. Beyond
// the identity getters (TemplateID, Concept, UID, Language) it is the
// entry point to the introspection tree: [Compiled.Root] /
// [Compiled.NodeAt] return [CompiledNode], and [Compiled.AllByRMType] /
// [Compiled.AllByNodeID] / [Compiled.AllByArchetypeID] index it.
type Compiled = impl.Compiled

// CompiledNode is one node in the compiled OPT tree — the unit of
// template introspection (form generation, path discovery, custom
// mapping/validation). It reports its canonical AQL path (AQLPath), the
// RM type it constrains (RMTypeName), its at-code / archetype id (NodeID,
// ArchetypeID), occurrences, slot rules (IsSlot, SlotRules,
// AllowsArchetypeID), primitive value constraint (PrimitiveConstraint),
// human label (Term), parent (Parent), and child attributes (Attributes,
// Attribute). Like [Compiled] it is a type alias of the engine form.
type CompiledNode = impl.CompiledNode

// CompiledAttribute is one attribute on a [CompiledNode] (e.g. "content",
// "data", "value"): its Name, Cardinality, Existence, ChildMultiplicity,
// RMTypeName, whether it is BMM-Required or an Implicit RM injection, and
// its child nodes (Children).
type CompiledAttribute = impl.CompiledAttribute

// Option configures [Compile]. Functional options keep the engine's
// option struct out of the public surface and leave room for additive
// knobs without a breaking signature change.
type Option func(*config)

type config struct {
	lookup       rminfo.Lookup
	skipImplicit bool
}

// WithRMInfo overrides the RM-info source used to inject the implicit
// attributes an OPT omits. When unset, the SDK's default RM info is used.
func WithRMInfo(l rminfo.Lookup) Option {
	return func(c *config) { c.lookup = l }
}

// WithoutImplicitAttributes disables RM-attribute injection, so compiled
// nodes carry only the attributes the OPT declared. Useful for round-trip
// serialisation that must preserve the OPT's explicit-only shape.
func WithoutImplicitAttributes() Option {
	return func(c *config) { c.skipImplicit = true }
}

// Compile turns a parsed OPT (the output of [template.ParseFile] /
// [template.ParseOPT]) into the compiled driver used by the composition
// builder and the validator. The input is read-only.
//
// Returns [ErrInvalidInput] when opt is nil or has no root.
func Compile(opt *template.OperationalTemplate, opts ...Option) (*Compiled, error) {
	var cfg config
	for _, fn := range opts {
		if fn != nil {
			fn(&cfg)
		}
	}
	return impl.Compile(opt, impl.Options{
		Lookup:                 cfg.lookup,
		SkipImplicitAttributes: cfg.skipImplicit,
	})
}

// ErrInvalidInput is returned by [Compile] for a nil template or one
// whose root could not be resolved. It is the same sentinel the engine
// returns, so external callers can match it with errors.Is.
var ErrInvalidInput = impl.ErrInvalidInput

// ErrPathNotFound is returned by [Compiled.NodeAt] when a path string
// does not resolve to a compiled node. Re-exported so external callers
// can match it with errors.Is without importing an internal package.
var ErrPathNotFound = impl.ErrPathNotFound
