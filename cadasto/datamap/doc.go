// Package datamap is the client and builder for Datamap V2 — the
// Cadasto-specific JSON format for resource-free read and write of
// clinical and demographic data across the openEHR REST API surface
// (REQ-058).
//
// Datamap lets a consumer read and write openEHR data without knowing
// the Reference Model: load an operational template (OPT), get a
// flat-ish JSON structure (plus a JSON Schema describing it), and the
// codec converts that structure to and from a spec-compliant openEHR
// COMPOSITION. The three operations are:
//
//   - Schema(opt)              → JSON Schema derived from the OPT
//   - ToComposition(opt, dm)   → canonical RM COMPOSITION (write path)
//   - FromComposition(opt, c)  → datamap (read path)
//
// The normative format is described in SPEC.md (imported from the
// Cadasto dmv2 specification; the canonical home when REQ-058 lands is
// docs/specifications/datamap.md). Round-trip test vectors live under
// testdata/ (OPT + datamap + schema + composition fixtures).
//
// Building-block use: this package operates on openEHR-core types only
// (openehr/template, openehr/composition, openehr/rm, openehr/validation)
// and does NOT import transport/, auth/, openehr/client/*, or any other
// cadasto/<name> package. Per AGENTS.md the domain client (cadasto/care)
// consumes this codec through an interface contract, never a direct
// import — preserving the cadasto/ extraction cut line.
package datamap
