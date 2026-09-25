// Package instance synthesises an RM object graph from a compiled
// operational template. It is the inverse of openehr/validation:
// where validation walks an OPT and an existing RM tree in lockstep
// emitting issues, this package walks an OPT and constructs the RM
// tree, materialising primitive example values from the template's
// primitive constraints at leaf nodes.
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
//   - Minimal: only attributes with existence lower ≥ 1 (and
//     BMM-mandatory implicits). Smallest valid tree; primitive leaves
//     still receive [constraints.PrimitiveConstraint.ExampleValue]
//     so the result is structurally complete.
//   - Example: Minimal plus every primitive leaf populated with its
//     ExampleValue. Useful for fixtures and demos.
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
