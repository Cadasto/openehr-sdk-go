// Package instance synthesises an RM object graph from a compiled
// operational template. It is the inverse of openehr/validation:
// where validation walks an OPT and an existing RM tree in lockstep
// emitting issues, this package walks an OPT and constructs the RM
// tree, valuing each primitive leaf from the template's primitive
// constraint the way [Options.ValueFill] says.
//
// The public entry point is [Generate]:
//
//	name := "Test"
//	out, err := instance.Generate(ctx, compiled, instance.Options{
//	    Policy:    instance.Large,
//	    Territory: "NL",
//	    Composer:  &rm.PartyIdentified{Name: &name},
//	})
//
// Typed accessors ([AsComposition], [AsObservation], …) cast the
// returned `any` into a concrete RM root for downstream code that
// knows the template's root type.
//
// # Policies
//
// Under every policy the generator skips an attribute the template
// prohibits (an existence of 0..0), unless an RM rule needs it, such as
// an attribute the BMM marks mandatory, which it then writes as if the
// template allowed it; and it never visits one the RM computes rather
// than stores (offset on an event, is_integral on DV_QUANTITY and
// DV_PROPORTION), nor a locatable's uid, which the identity rule
// decides. Under every policy an RM rule can need more than the policy
// visits, such as an ELEMENT's value or null flavour, which the generator
// then builds as a visit does, from the attribute's first allowed
// template child where it has one; and a visited multi-valued attribute
// is topped up to its lower bound.
//
// The three policies follow the levels of the ITS-REST template
// example's detail_level:
//
//   - Medium: the required attributes (the BMM marks them mandatory, or
//     their existence or cardinality lower bound is at least 1), and those
//     with an allowed template child (one whose occurrences upper bound is
//     not 0). A visited multi-valued attribute gets its members as under
//     Large, except that an optional archetype after the first optional
//     archetype of the attribute gets none, whatever its archetype. So
//     Medium builds at most one optional archetype under an attribute, the
//     medium level. It is the zero value, and so the default.
//   - Required: the required attributes only, not one merely because it
//     has an allowed template child, except a composition's context,
//     which it visits as Medium does, so the context every policy writes
//     is built under the template's constraints. A visited multi-valued
//     attribute gets each allowed child that is not a slot as many times
//     as its occurrences lower bound says, so an optional child gets none.
//     It builds the data points the template makes mandatory and a
//     composition's context, the required level. An RM rule can still add
//     a node, such as the null flavour of an element whose value the
//     template leaves optional, so its tree is not always smaller than
//     Medium's.
//   - Large: every attribute the visit rule allows, the optional ones the
//     template names included. A visited multi-valued attribute gets at
//     least one member of each allowed child that is not a slot, except
//     an optional child whose identity a required or earlier sibling
//     carries. So Large builds a member of every optional archetype the
//     template names. It stops short of the complete level, every data
//     point the template allows, which no level of its own builds yet.
//
// [Minimal] is another name for Required, and [Complete] for Large until
// the complete level lands. [ParsePolicy] reads a policy from the names
// required, minimal, medium, large and complete, in any case. Generate
// refuses any other Policy value with [ErrUnsupportedPolicy], before it
// builds anything.
//
// Under every policy, every primitive leaf the walk reaches is valued as
// [Options.ValueFill] says: the constraint's example value under
// [ExampleFill], an in-constraint draw under [RandomFill]. Where the
// template gives an RM attribute no value, the generator writes an
// RM-valid default, unless the template's own constraint on that
// attribute rejects it. A DV_PROPORTION is then settled to the rules of
// its kind, inside the template's constraints on its parts: a percent
// over 100, a fraction over whole numbers, a ratio over a denominator that
// is not 0. Where the drawn type cannot meet its rule there, another type
// the template allows is taken, and where none can, the value stays as
// the walk wrote it.
//
// # Trust model
//
// The OPT walk is authoritative. RM children are created from the
// openehr/rm/typereg registry (typereg.Default.Lookup), directly or
// through [internal/templateinstance/rmwrite.NewRM], attached via
// rmwrite EnsureSingle / AppendMultiple, and decorated
// with LOCATABLE bookkeeping (archetype_node_id, name, uid,
// archetype_details) here in the instance package. rmwrite stays
// focused on the attribute setter contract, the inverse of the
// generated attribute access the validators read through.
//
// # Dependencies
//
// The exported Generate signature takes the public compiled template
// openehr/templatecompile.Compiled; this package also imports
// openehr/rm, openehr/rm/typereg, openehr/rm/rminfo, openehr/template,
// openehr/template/constraints, openehr/terminology,
// internal/templatecompile (engine node types),
// internal/templateinstance/rmwrite, internal/bmmtype and
// internal/rmroots. It does not import openehr/serialize,
// openehr/client, transport, auth, openehr/composition (which uses
// this package, not the reverse), or openehr/validation.
package instance
