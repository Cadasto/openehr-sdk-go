// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass, for COMPOSITION and for any other archetypeable root. That
// pass includes the Reference Model floor, so a result that is OK is
// valid against both the template and the RM rules the floor checks.
// The floor on its own, [ValidateRM] and its typed wrappers, covers
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
// Public entry: one call checks the template constraints and the RM
// floor together.
//
//	r := validation.ValidateComposition(comp, compiled)
//	for _, issue := range r.Issues {
//	    log.Printf("%s: %s: %s", issue.Path, issue.Code, issue.Detail)
//	}
//	valid := r.OK
//
// A root that no template applies to is checked with [ValidateRM]
// instead, which runs the floor alone.
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
// the walker reads the corresponding RM property through access the
// generator emits for every registered RM class, enforces existence (an
// attribute the template requires, and one it prohibits) / cardinality /
// alternatives / RM-type match / archetype-id identity, and recurses into
// matched RM children. The template walk reports an attribute the template
// prohibits, when it is present, once, and does not enter it; the Reference
// Model rules still apply to what it holds. A list attribute that is nil is
// absent, and one that is set and holds no item is present and empty.
//
// The RM floor ([ValidateRM] and its typed wrappers) is the RM-only
// layer beneath that: a second driver that walks any RM root with
// rminfo as the sole structural source (no OPT) and enforces
// RM-mandatory attribute presence plus a small per-RM-type invariant
// catalogue:
//
//   - a non-empty CODE_PHRASE code_string (a DV_SCALE symbol, which the
//     RM lets carry no code, is exempt);
//   - DV_QUANTITY and DV_PROPORTION precision, and whole-number operands
//     on a DV_PROPORTION of precision 0;
//   - DV_INTERVAL numeric bounds in order, and no side both unbounded and
//     included;
//   - an ISO 8601 value on DV_DATE, DV_TIME, DV_DATE_TIME and DV_DURATION;
//   - exactly one of value and null_flavour on an ELEMENT;
//   - OBJECT_REF id/type/namespace, and the PARTY_IDENTIFIED rules (at
//     least one of name, identifiers and external_ref; neither of the first
//     two present but empty);
//   - a DV_TEXT mappings list that is not empty when present, and a
//     TERM_MAPPING match from its closed set;
//   - archetype_details on every archetype root (COMPOSITION, EHR_STATUS,
//     EHR_ACCESS, and every PARTY and ENTRY), and a non-empty archetype_id
//     and rm_version on every ARCHETYPED;
//   - a member of its group or code set (reported as
//     `code_not_in_value_set`; see [ValidateRM]) in each attribute the RM
//     codes from the openEHR terminology, apart from those of the
//     change-control and resource-description classes and
//     EXTRACT_PARTICIPATION.
//
// The template walk covers RM-mandatory presence only on the nodes the
// template models, and none of the floor's per-type rules. The
// template-driven entries therefore run the floor after the template walk,
// over the whole root, the nodes the template does not model included, and
// return both sets of findings in one [Result]: the template walk's issues
// first, then the floor's, each in the order its walk found them. Where the
// floor reports the same code at the same path as the template walk, the
// result keeps the template walk's issue and drops the floor's copy, so a
// defect both layers find is reported once. Issues under different codes,
// or at different paths, are all kept. No option leaves the floor out.
// Callers without a template run [ValidateRM], which is the floor alone.
//
// The template-driven validators and [ValidateRM] write [Issue.Path] by
// one rule, so a node has one path whichever of them reports it: an
// item of a multi-valued attribute is written by its archetype node id
// when that id is unique among its siblings, and otherwise by the name
// the template gives its node or by its position. Inside a
// template-driven validation the floor takes those names from the
// template walk, so it writes the same path for the same node; on its
// own it knows no template names and writes the position. An entry of a
// Hash attribute is written by its 1-based position in ascending key
// order, as in /details[2], never by its key. A composition missing an
// OPT-required attribute is flagged at the attribute's path, and one
// missing an OPT-required node at the path the template gives that node,
// rather than silently bypassed.
//
// # Collect-all
//
// Every failing clause emits one [Issue]; neither walk stops at a
// finding, so UIs and CI runners get the full list. A defect the
// template walk and the floor both find, under one code at one path, is
// one issue.
//
// # Finding identity
//
// Every issue says where it comes from. [Issue.Layer] is [LayerTemplate]
// for a clause of the operational template and [LayerRM] for a rule of the
// Reference Model, and [Issue.Rule] names the clause: a word such as
// "existence", "occurrences" or "type" for a template clause, and, for a
// Reference Model rule, the class invariant it evaluates, as in
// "DV_INTERVAL.Limits_consistent", or a word such as "mandatory". Issues
// that share a code, such as the rm_invariant issues, are told apart by
// Rule. A guard, a walk condition and an AQL lint issue carry [LayerNone]
// and an empty Rule.
//
// # Validator and limits
//
// A [Validator] is the value behind the entry points: [New] builds one
// from options, and its [Validator.Validate] takes any Reference Model value
// with a compiled template, or with nil to apply the Reference Model rules
// alone. It holds no state between calls, so one value can serve every
// goroutine of a server. The package-level functions behave as a Validator
// built with no options.
//
// [WithLimits] sets the two [Limits] of the Reference Model walk: the
// nesting depth below which it does not descend (256 by default) and the
// number of values it visits in one call (100,000 by default). No decoded
// document comes near either; they stop a value built in Go that nests
// without end or holds itself. A walk that reaches the depth limit reports
// one max_depth issue, at the first place it stopped, and one that reaches
// the visit limit reports max_nodes and ends.
//
//	v := validation.New(validation.WithLimits(validation.Limits{Depth: 64}))
//	r := v.Validate(comp, compiled)
//
// # Coverage
//
// [Result.OK] reports that no error was found in what the validator
// checked; [Result.Coverage] says what it did not check. [Coverage.Stopped]
// lists each path where a limit stopped the walk, and [Coverage.External]
// each node the template binds to a CONSTRAINT_REF, whose value set is bound
// outside the template and which the validator therefore cannot consult.
// [Coverage.Complete] is true when both lists are empty.
//
//	if r.OK && !r.Coverage.Complete() {
//	    // valid as far as checked; see r.Coverage for what was not
//	}
//
// [Coverage.Unevaluated] lists, as CLASS.Invariant, the invariants of the
// classes the walk visited that the validator evaluates on no instance, such
// as a rule that needs a terminology service or stored history. Every
// invariant of the pinned Reference Model and BASE schemas is classified in
// the file invariants.json beside this package's source, with the reason the
// SDK does not evaluate it, and the classes' rules are dispatched from that
// classification, so the list is what the validator really skipped.
// Complete does not read it.
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
