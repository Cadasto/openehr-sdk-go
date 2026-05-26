// Package instance synthesises an RM object graph from a compiled
// operational template (REQ-107). It is the inverse of
// openehr/validation: where validation walks an OPT and an existing
// RM tree in lockstep emitting issues, this package walks an OPT
// and CONSTRUCTS the RM tree, materialising primitive example
// values from REQ-103 constraints at leaf nodes.
//
// The public entry point is [Generate]:
//
//	out, err := instance.Generate(ctx, compiled, instance.Options{
//	    Policy:    instance.Minimal,
//	    Territory: "NL",
//	    Composer:  &rm.PartyIdentified{Name: "Test"},
//	})
//
// Typed accessors ([AsComposition], [AsObservation], …) cast the
// returned `any` into a concrete RM root for downstream code that
// knows the template's root type.
//
// # Policies
//
//   - Minimal — only attributes with existence lower ≥ 1 (and
//     BMM-mandatory implicits). Smallest valid tree; primitive leaves
//     still receive [constraints.PrimitiveConstraint.ExampleValue]
//     so the result is structurally complete.
//   - Example — Minimal plus every primitive leaf populated with its
//     ExampleValue. Default for fixtures / demos.
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
// # REQ-013 building-block independence
//
// This package imports openehr/rm, openehr/rm/typereg,
// openehr/rm/rminfo, openehr/template, openehr/template/constraints,
// internal/templatecompile, internal/templatecompile/walk, and
// internal/templateinstance/rmwrite — same building-block universe
// as openehr/validation. It does NOT import openehr/serialize,
// openehr/client, transport, auth, openehr/composition (REQ-101
// consumes this engine, not the reverse), or openehr/validation
// (validation depends on instance only via cross-package probes).
package instance
