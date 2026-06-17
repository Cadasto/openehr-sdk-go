// Package datamap is the client and builder for Datamap V2 — the
// Cadasto-specific JSON format for resource-free read and write of
// clinical and demographic data across the openEHR REST API surface
// (REQ-058).
//
// Datamap lets a consumer read and write openEHR data without knowing
// the Reference Model: load an operational template (OPT), get a
// flat-ish JSON structure (plus a JSON Schema describing it), and the
// codec converts that structure to and from spec-compliant openEHR
// canonical JSON. Two profiles share the same wire conventions:
//
//   - Composition profile — clinical COMPOSITION templates:
//     Schema, ToComposition, FromComposition, FromCompositionExpanded
//   - Party profile (Option B) — demographic PARTY templates
//     (PERSON, ORGANISATION, AGENT, GROUP, ROLE, …):
//     Schema, ToParty, FromParty, FromPartyExpanded
//
// Shared helpers: Validate, Empty.
//
// The normative format is described in SPEC.md (imported from the
// Cadasto dmv2 specification; the canonical home when REQ-058 lands is
// docs/specifications/datamap.md). Round-trip test vectors live under
// testdata/ (OPT + datamap + schema + composition fixtures).
//
// Building-block use: the codec operates on openEHR-core types only
// (openehr/template, openehr/composition, openehr/rm, openehr/
// validation) and does NOT import transport/, auth/, openehr/client/*,
// or any other cadasto/<name> package. Validate is the sole exception:
// it depends on a third-party JSON Schema engine (santhosh-tekuri/
// jsonschema) to check a payload against Schema(opt). Per AGENTS.md the
// domain client (cadasto/care) consumes this codec through an interface
// contract, never a direct import — preserving the cadasto/ extraction
// cut line.
package datamap
