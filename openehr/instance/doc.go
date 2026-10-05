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
//	    Policy:    instance.Minimal,
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
// Under either policy the generator skips an attribute the template
// prohibits (an existence of 0..0), unless an RM rule needs it, such as
// an attribute the BMM marks mandatory, which it then writes as if the
// template allowed it; and it never visits one the RM computes rather
// than stores (offset on an event, is_integral on DV_QUANTITY and
// DV_PROPORTION), nor a locatable's uid, which the identity rule
// decides.
//
//   - Minimal: the required attributes, and those with an allowed
//     template child (one whose occurrences upper bound is not 0). An
//     RM rule can need more, such as an ELEMENT's value or null
//     flavour. Smallest valid tree.
//   - Example: every attribute the visit rule allows. Useful for
//     fixtures and demos.
//
// Under both, every primitive leaf the walk reaches is valued as
// [Options.ValueFill] says: the constraint's example value under
// [ExampleFill], an in-constraint draw under [RandomFill]. Where the
// template gives an RM attribute no value, the generator writes an
// RM-valid default, unless the template's own constraint on that
// attribute rejects it.
//
// # Trust model
//
// The OPT walk is authoritative. RM children are created via
// [internal/templateinstance/rmwrite.NewRM] (typereg.Default.Lookup),
// attached via rmwrite EnsureSingle / AppendMultiple, and decorated
// with LOCATABLE bookkeeping (archetype_node_id, name, uid,
// archetype_details) here in the instance package. rmwrite stays
// focused on the inverse-of-rmread attribute setter contract.
//
// # Dependencies
//
// The exported Generate signature takes the public compiled template
// openehr/templatecompile.Compiled; this package also imports
// openehr/rm, openehr/rm/typereg, openehr/rm/rminfo, openehr/template,
// openehr/template/constraints, internal/templatecompile (engine node
// types), internal/templatecompile/walk, and
// internal/templateinstance/rmwrite, the same set as
// openehr/validation. It does not import openehr/serialize,
// openehr/client, transport, auth, openehr/composition (which uses
// this package, not the reverse), or openehr/validation.
package instance
