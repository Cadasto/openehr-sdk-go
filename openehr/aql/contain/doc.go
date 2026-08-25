// Package contain answers, for two AQL class expressions, whether a
// containment route connects them under the pinned openEHR Reference Model —
// the shared RM-derived containment relation for the AQL semantic lint
// (REQ-161) and the builder verification (REQ-162). REQ-160.
//
// The relation is derived at runtime from the BMM-backed class graph in
// openehr/rm/rminfo (REQ-048): descendant-at-any-depth reachability over the
// RM's by-value composition graph, with abstract classes standing for their
// concrete kinds and reference-typed attributes (OBJECT_REF and its
// descendants) terminating structural reachability. Facts the BMM cannot
// express — the EHR root's reference-based containment, the VERSION /
// VERSIONED_* tier, FOLDER→COMPOSITION as a reference hop — are overlay data
// (see [Default]); consumers extend the relation with their own overlay edges
// via [TypeRelation.WithOverlay] without forking it.
//
// A verdict is route-existence, not a claim of RM truth: on the RM-faithful
// subgraph an [Admissible] verdict is genuine nesting, but where the default
// relation is deliberately loose (the family-agnostic VERSION hub) it may
// over-admit a pair that cannot actually nest. That is a documented missed
// defect, never a false Error — the conservative flagging policy the linter
// inherits (REQ-161 § Flagging policy).
//
// Building block (REQ-013): this package imports only openehr/rm/rminfo,
// openehr/rm (REQ-120's canonical ParseArchetypeID), and the standard library.
package contain
