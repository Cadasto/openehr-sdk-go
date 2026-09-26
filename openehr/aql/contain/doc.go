// Package contain answers, for two AQL class expressions, whether a
// containment route connects them under the pinned openEHR Reference Model.
// It is the shared RM-derived containment relation behind the AQL semantic
// lint, the builder's containment verification, and the path-shape lint's
// redundant-step check (via [TypeRelation.Unavoidable]).
//
// The relation is derived at runtime from the BMM-backed class graph in
// openehr/rm/rminfo: descendant-at-any-depth reachability over the RM's
// by-value composition graph, with abstract classes standing for their
// concrete kinds and reference-typed attributes (OBJECT_REF and its
// descendants) ending structural reachability. Facts the BMM cannot express
// (the EHR root's reference-based containment, the VERSION / VERSIONED_*
// tier, FOLDER→COMPOSITION as a reference hop) are overlay data (see
// [Default]); consumers extend the relation with their own overlay edges via
// [TypeRelation.WithOverlay] without forking it.
//
// A verdict reports that a route exists; it is not a claim of RM truth. On the
// RM-faithful subgraph an [Admissible] verdict is genuine nesting, but where
// the default relation is deliberately loose (the family-agnostic VERSION hub)
// it may admit a pair that cannot actually nest. That can hide a defect but
// never produces a false error, which is the conservative flagging policy the
// linter inherits.
//
// This package imports only openehr/rm/rminfo, openehr/rm (for
// ParseArchetypeID), and the standard library.
package contain
