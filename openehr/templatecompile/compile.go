package templatecompile

import (
	impl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Compiled is the public, externally-constructable compiled template.
// Produce one with [Compile]; the zero value is not useful.
//
// It is a type alias of the engine's compiled form, so a *Compiled is
// accepted directly by openehr/composition, openehr/instance,
// openehr/validation, and openehr/aql/lint without conversion. Beyond
// the identity getters (TemplateID, Concept, UID, Language) it is the
// entry point to the introspection tree: [Compiled.Root] /
// [Compiled.NodeAt] / [Compiled.Resolve] return [CompiledNode], and
// [Compiled.AllByRMType] / [Compiled.AllByNodeID] /
// [Compiled.AllByArchetypeID] index it. [Compiled.Resolve] and
// [Compiled.ValidatePath] walk the tree along a path that
// [github.com/cadasto/openehr-sdk-go/openehr/template.ParsePath] reads;
// [Compiled.NodeAt] looks up the exact AQL path the compile wrote.
type Compiled = impl.Compiled

// CompiledNode is one node in the compiled OPT tree, the unit of
// template introspection (form generation, path discovery, custom
// mapping/validation). It reports its canonical AQL path (AQLPath), the
// RM type it constrains (RMTypeName), its at-code / archetype id (NodeID,
// ArchetypeID), occurrences, slot rules (IsSlot, SlotRules,
// AllowsArchetypeID), primitive value constraint (PrimitiveConstraint),
// the default value the template's constraints give it (DefaultValue),
// human label (Term), template-level pinned node name (NodeName),
// parent (Parent), and child attributes (Attributes, Attribute).
// Like [Compiled] it is a type alias of the engine form.
type CompiledNode = impl.CompiledNode

// CompiledAttribute is one attribute on a [CompiledNode] (e.g. "content",
// "data", "value"): its Name, Cardinality, Existence, ChildMultiplicity,
// RMTypeName, whether it is BMM-Required or an Implicit RM injection,
// whether the template declares it under a name no class in the node's
// class hierarchy (the class, its ancestors and its descendants) has as
// a property or a function (NotInRM), and its child nodes (Children).
type CompiledAttribute = impl.CompiledAttribute

// Option configures [Compile]. Functional options keep the engine's
// option struct out of the public surface and leave room for additive
// knobs without a breaking signature change.
type Option func(*config)

type config struct {
	lookup       rminfo.Lookup
	skipImplicit bool
	strict       bool
}

// WithRMInfo overrides the RM-info source used to inject the implicit
// attributes an OPT omits. When unset, the SDK's default RM info is used.
// The lookup shapes the compiled template only: the validators in
// openehr/validation read which attributes the Reference Model makes
// mandatory from the SDK's own RM info, whatever lookup compiled the
// template.
func WithRMInfo(l rminfo.Lookup) Option {
	return func(c *config) { c.lookup = l }
}

// WithoutImplicitAttributes disables RM-attribute injection, so compiled
// nodes carry only the attributes the OPT declared. Useful for round-trip
// serialisation that must preserve the OPT's explicit-only shape.
func WithoutImplicitAttributes() Option {
	return func(c *config) { c.skipImplicit = true }
}

// WithStrict makes [Compile] refuse, with an error wrapping
// [ErrInvalidInput], each value it otherwise reads in a weaker form, as
// [github.com/cadasto/openehr-sdk-go/openehr/template.ParseOPTStrict]
// refuses them: a constraint reference with no reference, a timezone
// validity that names no validity kind or is mandatory on a date, a temporal
// range bound that is not ISO 8601 text of its type, and a primitive object
// with no item. It also refuses a default value of the template's
// constraints that addresses no node, which Compile otherwise skips.
func WithStrict() Option {
	return func(c *config) { c.strict = true }
}

// Compile turns an operational template, read into the model with
// [aom14.ReadOperationalTemplate], into the compiled driver used by the
// composition builder and the validator. The input is read-only.
//
// Compile reads identifiers, codes, list entries and term texts without
// their leading and trailing white space, reads an ELEMENT attribute named
// null_flavor as null_flavour, and closes each occurrences, existence and
// cardinality interval into a
// [github.com/cadasto/openehr-sdk-go/openehr/template.Multiplicity]. An interval the model
// leaves at its zero value, as it holds one the template leaves out, is read
// as absent. A slot's includes and excludes are its archetype-id patterns,
// one per assertion, and a slot with no node id takes the first of its
// includes, without its regex escapes, as the predicate of its AQL path
// segment.
//
// Each default value of the template's constraints is placed on the node its
// differential path and attribute name address ([CompiledNode.DefaultValue]),
// the first child of that attribute of the constraint object's RM type, or
// of the default value's where the object names none. A differential path
// that opens with the definition's archetype id in brackets resolves from the
// rest, and one that opens with a slash from the definition's root. A segment's
// predicate may carry a name, at0004, 'Systolic' or at0004 and
// name/value='Systolic', which selects the node of that node id or archetype
// id whose template-level name ([CompiledNode.NodeName]) it is, each run of
// white space in the predicate read as one space. A default value that
// addresses no node is skipped.
//
// By default Compile reads a value the model holds in the weaker form the
// deprecated [github.com/cadasto/openehr-sdk-go/openehr/template.ParseOPT]
// reads it: a constraint reference with no reference, with an empty one; a
// timezone validity that names no validity kind, or is mandatory on a date,
// as optional; a temporal range bound that is not ISO 8601 text of its type
// as unbounded on its side; and a primitive object with no item as no
// constraint. [WithStrict] refuses each of these, and a default value that
// addresses no node.
//
// Returns an error wrapping [ErrInvalidInput] when opt is nil, when it has
// no template_id or no definition (a definition that names no RM type),
// when an interval is inverted or closing leaves it empty, when an ordinal
// symbol is neither a coded text nor a code phrase, when a constraint
// object or an attribute is missing, when a primitive item is no primitive
// class held by pointer, when a default value does not encode as canonical
// JSON, when
// constraint objects nest more than 128 levels below the definition, when two
// nodes under different attributes have the same AQL path, which is a defect
// in the template, and, with [WithStrict], for each value it refuses. Nodes
// under one attribute that share a path (alternatives, or siblings with the
// same node id) and the descendants of such nodes are admitted, and
// [Compiled.NodeAt] resolves the path to the first one walked. Returns an
// error that does not wrap ErrInvalidInput when the model holds a constraint
// object or an attribute of a kind Compile does not handle, which
// [aom14.ReadOperationalTemplate] does not produce.
func Compile(opt *aom14.OperationalTemplate, opts ...Option) (*Compiled, error) {
	var cfg config
	for _, fn := range opts {
		if fn != nil {
			fn(&cfg)
		}
	}
	return impl.Compile(opt, impl.Options{
		Lookup:                 cfg.lookup,
		SkipImplicitAttributes: cfg.skipImplicit,
		Strict:                 cfg.strict,
	})
}

// ErrInvalidInput is returned, wrapped, by [Compile] for a template it
// refuses, as [Compile] lists them. It is the same sentinel the engine
// returns, so external callers can match it with errors.Is.
var ErrInvalidInput = impl.ErrInvalidInput

// ErrPathNotFound is returned, wrapped, by [Compiled.NodeAt] when a path
// string does not resolve to a compiled node, and by [Compiled.Resolve] and
// [Compiled.ValidatePath] when a path reaches no node; an error of the
// latter two also matches
// [github.com/cadasto/openehr-sdk-go/openehr/template.ErrPathNotFound].
// Re-exported so external callers can match it with errors.Is without
// importing an internal package.
var ErrPathNotFound = impl.ErrPathNotFound

// ResolveOption tunes [Compiled.Resolve] and [Compiled.ValidatePath].
// Construct one with [WithStrictPaths]. Like [Compiled] it is a type alias
// of the engine form.
type ResolveOption = impl.ResolveOption

// WithStrictPaths makes [Compiled.Resolve] refuse a segment without a
// predicate on an attribute that holds more than one child, with an error
// wrapping
// [github.com/cadasto/openehr-sdk-go/openehr/template.ErrAmbiguousPath],
// where by default it takes the first child.
func WithStrictPaths() ResolveOption { return impl.WithStrictPaths() }
