// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass, for COMPOSITION and for any other archetypeable root. That
// pass applies the Reference Model rules too, so a result that is OK
// is valid against both the template and the RM rules the validator
// checks. The Reference Model rules alone, [ValidateRM] and its typed
// wrappers, cover resources that bind to no operational template
// (FOLDER, EHR_STATUS, EHR_ACCESS, untemplated demographic PARTY).
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
// rules together.
//
//	r := validation.ValidateComposition(comp, compiled)
//	for _, issue := range r.Issues {
//	    log.Printf("%s: %s: %s", issue.Path, issue.Code, issue.Detail)
//	}
//	valid := r.OK
//
// A root that no template applies to is checked with [ValidateRM]
// instead, which applies the RM rules alone.
//
// The walk works over any value source: [Validate] runs it over any
// RM root the closed RM set recognises, and the typed wrappers
// [ValidateComposition], [ValidateDemographic] (PERSON / ORGANISATION /
// GROUP / AGENT / ROLE), [ValidateFolder], and [ValidateEHRStatus]
// delegate to it. PARTY sub-components (ADDRESS, CONTACT,
// PARTY_IDENTITY, PARTY_RELATIONSHIP, CAPABILITY) validate in place
// during a PARTY walk or as roots via [Validate].
//
// # Trust model
//
// The template entries ([ValidateComposition], [Validate], and the typed
// wrappers) visit the instance in one walk. The compiled OPT binds the
// instance's nodes to its own and decides which content the walk admits;
// the value and the Reference Model supply the rest of what it visits.
// At each node it knows the RM class of the value and, where the template
// reaches, the template node the value binds to. It reads the node's
// attributes through access the generator emits for every registered RM
// class: those the template declares, those the value holds, and those the
// Reference Model marks mandatory for the class. At each attribute it
// applies, in order, the template's admission (an attribute the template
// prohibits, or an item that binds to none of its nodes, is reported once
// and not entered), presence (existence), counts (cardinality and
// occurrences), and then, for each admitted item, its archetype-id identity
// and RM-type match, the Reference Model rules of its class, the template's
// value constraint at a leaf, and the same steps one level down. A list
// attribute that is nil is absent, and one that is set and holds no item is
// present and empty. An item of a list that holds no RM value, a nil
// element or a nil pointer, still counts toward its list; the Reference
// Model rules report it as `required` at its own path and do not walk it,
// unless the template refuses it, which is then the one finding.
//
// The Reference Model rules are the RM-only layer of that walk. With no
// template ([ValidateRM] and its typed wrappers) every attribute is
// unconstrained and the walk applies them alone, with the class metadata of
// the pinned Reference Model as the sole structural source: RM-mandatory
// attribute presence plus a small per-RM-type invariant catalogue:
//
//   - a non-empty CODE_PHRASE code_string (a DV_SCALE symbol, which the
//     RM lets carry no code, is exempt);
//   - DV_QUANTITY and DV_PROPORTION precision, and whole-number operands
//     on a DV_PROPORTION of precision 0;
//   - DV_INTERVAL numeric bounds in order, and no side both unbounded and
//     included;
//   - an ISO 8601 value on DV_DATE, DV_TIME, DV_DATE_TIME and DV_DURATION
//     (an empty value is a missing mandatory attribute, reported once, as
//     `required`);
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
// With a template the same rules run on every node the template admits,
// the nodes it does not constrain included, and on no content it refuses.
// Their findings come in the one [Result], in the order the walk found
// them. An attribute both layers find missing or empty is one finding: a
// node the template requires and the instance lacks is the template's
// `required` alone, and an empty list the template counts is the
// template's finding alone. A Reference Model rule's own finding at that
// path, such as the terminology check of a coded attribute, is reported
// beside it. A value held from two places is validated at each, so a
// defect in it is reported at each path. No option leaves the Reference
// Model rules out. Callers without a template run [ValidateRM], which
// applies them alone.
//
// The template-driven validators and [ValidateRM] write [Issue.Path] by
// one rule, so a node has one path whichever of them reports it: an
// item of a multi-valued attribute is written by its archetype node id
// when that id is unique among its siblings, and otherwise by the name
// the template gives its node or by its position. With no template the
// walk knows no template names and writes the position. An entry of a
// Hash attribute is written by its 1-based position in ascending key
// order, as in /details[2], never by its key. A composition missing an
// OPT-required attribute is flagged at the attribute's path, and one
// missing an OPT-required node at the path the template gives that node,
// rather than silently bypassed.
//
// # Collect-all
//
// Every failing clause emits one [Issue]; the walk does not stop at a
// finding, so UIs and CI runners get the full list. An attribute the
// template and the Reference Model rules both find missing or empty is
// one issue; a Reference Model rule's own finding at that path, such as a
// coded attribute's terminology check, is a second.
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
// [WithLimits] sets the two [Limits] of the walk: the nesting depth below
// which it does not descend (256 by default) and the
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
// classes the walk visited that the validator evaluates on no instance: most
// because it does not evaluate them yet, some because they need a record the
// instance does not carry, such as the versions stored for a versioned
// object. Every invariant of the pinned Reference Model and BASE schemas is
// classified in the file invariants.json beside this package's source, with
// the reason the SDK does not evaluate it, and the classes' rules are
// dispatched from that classification, so the list is what the validator
// really skipped. An invariant another check already decides, such as one
// that a mandatory attribute be not empty, counts as evaluated and is not
// listed. Complete does not read it.
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
