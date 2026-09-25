// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass, for COMPOSITION and for any other archetypeable root.
// A template-less floor, [ValidateRM] and its typed wrappers, covers
// resources that bind to no operational template (FOLDER,
// EHR_STATUS, EHR_ACCESS, untemplated demographic PARTY).
//
// The package also validates AQL query text:
// [ValidateAQL] and [ValidateAQLWithTypeRelation] run the
// openehr/aql/lint layers and report through the same [Issue] /
// [Result] model, so a caller already using [ValidateComposition] gets
// one uniform result shape for both kinds of artefact. Their doc
// comments describe which lint layers a nil template drops and how
// to supply the RM containment relation.
//
// Public entry:
//
//	r := validation.ValidateComposition(comp, compiled)
//	if !r.OK {
//	    for _, issue := range r.Issues {
//	        log.Printf("%s: %s: %s", issue.Path, issue.Code, issue.Detail)
//	    }
//	}
//
// The walker works over any value source: [Validate] runs it over any
// RM root the closed RM set recognises, and the typed wrappers
// [ValidateComposition], [ValidateDemographic] (PERSON / ORGANISATION /
// GROUP / AGENT / ROLE), [ValidateFolder], and [ValidateEHRStatus]
// delegate to it. PARTY sub-components (ADDRESS, CONTACT,
// PARTY_IDENTITY, PARTY_RELATIONSHIP, CAPABILITY) validate in place
// during a PARTY walk or as roots via [Validate].
//
// # Trust model
//
// The template-driven entries ([ValidateComposition], [Validate], and
// the typed wrappers) treat the compiled OPT as the authoritative driver
// and the composition as the value source. For each compiled OPT node
// the walker reads the corresponding RM property via
// [github.com/cadasto/openehr-sdk-go/openehr/validation/rmread],
// enforces existence / cardinality / alternatives / RM-type match /
// archetype-id identity, and recurses into matched RM children.
//
// The RM floor ([ValidateRM] and its typed wrappers) is the RM-only
// layer beneath that: a second driver that walks any RM root with
// rminfo as the sole structural source (no OPT) and enforces
// RM-mandatory attribute presence plus a small per-RM-type invariant
// catalogue (CODE_PHRASE, DV_QUANTITY precision, DV_PROPORTION
// precision, DV_INTERVAL numeric bounds, OBJECT_REF
// id/type/namespace). Template validity does not imply RM validity:
// the template layer covers RM-mandatory presence on the nodes the
// template models, not the per-type invariant catalogue, so a
// composition can pass [Validate] and still be RM-invalid. The two
// layers therefore compose but do not chain: callers with a template
// run [Validate], callers without one run [ValidateRM], callers
// wanting both run both.
//
// Path strings in [Issue.Path] are built by appending OPT-side
// attribute names and matched-child predicates to the parent OPT
// node's path as the walker descends. The OPT contributes the
// attribute names; the RM-side archetype_node_id of each matched
// child contributes the bracket predicate. Composition-supplied
// predicates therefore appear in the path only on RM nodes the
// walker has bound to an OPT child. A composition missing an
// OPT-required node is flagged at the parent attribute's path
// (no descent) rather than silently bypassed.
//
// # Collect-all
//
// Every failing clause emits one [Issue]; the walk never
// short-circuits, so UIs and CI runners get the full list.
//
// # Dependencies
//
// The package is importable without `transport/`, `auth/`,
// `openehr/client/*`, or `openehr/serialize/`.
//
// # Compiled templates
//
// The `c` argument is the compiled template. Construct it from a parsed
// OPT with
// [github.com/cadasto/openehr-sdk-go/openehr/templatecompile.Compile].
// The exported signatures here reference that public type, so external
// modules can call [Validate] / [ValidateComposition] / siblings without
// importing any internal/ package. The constructor lives in its own
// package rather than openehr/template to avoid an import cycle and to
// keep openehr/template stdlib-only.
package validation
