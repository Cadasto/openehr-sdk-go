// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass — REQ-102 (COMPOSITION) and REQ-110 (any archetypeable root).
// REQ-112 adds a template-less floor — [ValidateRM] and its typed
// sugars — for resources that bind to no operational template (FOLDER,
// EHR_STATUS, EHR_ACCESS, untemplated demographic PARTY).
//
// Public entry:
//
//	r := validation.ValidateComposition(comp, compiled)
//	if !r.OK {
//	    for _, issue := range r.Issues {
//	        log.Printf("%s: %s — %s", issue.Path, issue.Code, issue.Detail)
//	    }
//	}
//
// The walker is value-source-generic: [Validate] runs it over any RM
// root the closed RM set recognises, and the typed wrappers
// [ValidateComposition], [ValidateDemographic] (PERSON / ORGANISATION /
// GROUP / AGENT / ROLE), [ValidateFolder], and [ValidateEHRStatus]
// delegate to it (REQ-110). PARTY sub-components (ADDRESS, CONTACT,
// PARTY_IDENTITY, PARTY_RELATIONSHIP, CAPABILITY) validate in place
// during a PARTY walk or as roots via [Validate].
//
// # Trust model
//
// The template-driven entries ([ValidateComposition], [Validate], the
// REQ-110 typed wrappers) are **template-driven**: the compiled OPT is
// the authoritative driver and the composition is the value source.
// For each compiled OPT node the walker reads the corresponding RM
// property via [github.com/cadasto/openehr-sdk-go/openehr/validation/rmread],
// enforces existence / cardinality / alternatives / RM-type match /
// archetype-id identity, and recurses into matched RM children.
//
// The REQ-112 floor ([ValidateRM] + typed sugars) is the **RM-only**
// layer beneath that: a second driver that walks any RM root with
// rminfo as the sole structural source — no OPT — and enforces
// RM-mandatory attribute presence plus a small per-RM-type invariant
// catalogue (CODE_PHRASE, DV_QUANTITY precision, DV_INTERVAL numeric
// bounds, OBJECT_REF id/type/namespace). Template validity implies RM
// validity,
// so the two compose: callers with a template run [Validate], callers
// without one run [ValidateRM], callers wanting both run both.
//
// Path strings in [Issue.Path] are built by appending OPT-side
// attribute names and matched-child predicates to the parent OPT
// node's path as the walker descends. The OPT contributes the
// attribute names; the RM-side archetype_node_id of each matched
// child contributes the bracket predicate. Composition-supplied
// predicates therefore appear in the path only on RM nodes the
// walker has bound to an OPT child — a composition missing an
// OPT-required node is flagged at the parent attribute's path
// (no descent), rather than silently bypassed.
//
// See [docs/plans/archive/2026-05-24-composition-validation-template-driven.md]
// for the migration's phase split.
//
// # Collect-all
//
// Every failing clause emits one [Issue]; the walk never
// short-circuits. UIs and CI runners need the full list.
//
// # REQ-013 building-block independence
//
// The validator MUST be importable without `transport/`, `auth/`,
// `openehr/client/*`, or `openehr/serialize/`. The forbidden
// forbidden-import set is enforced by `TestValidationForbiddenImports`.
//
// # External callability (REQ-111)
//
// The `c` argument is the compiled template. Construct it from a parsed
// OPT with the public bridge
// [github.com/cadasto/openehr-sdk-go/openehr/templatecompile.Compile];
// the exported signatures here reference that public type, so external
// modules can call [Validate] / [ValidateComposition] / siblings without
// importing any internal/ package. The bridge lives in a sibling package
// rather than openehr/template to avoid an import cycle and REQ-100's
// stdlib-only contract — see docs/adr/0010-public-compiled-template-bridge.md.
package validation
