// Package terminology answers "is this code a member of that openEHR
// terminology group, what is its rubric, and which code carries this
// rubric?" — the closed, versioned value sets the RM's own invariants
// reference (EVENT_CONTEXT.Setting_valid, AUDIT_DETAILS.Change_type_valid,
// PARTICIPATION.Mode_valid, DV_ORDERED.Normal_status_validity, …).
//
// The questions are on [Group] (17 groups of coded concepts, each with a
// rubric) and on [CodeSet] (3 sets of bare codes with no rubric). Both are
// closed and source-ordered, both report absence as a false second return
// rather than a fabricated value, and a nil pointer of either is inert
// (REQ-025) — no method panics. [Groups], [CodeSets], [GroupByID] and
// [CodeSetByID] enumerate the pinned tables; [ID] is the TERMINOLOGY_ID
// value they are all defined in.
//
// A rubric belongs to a code *within a group*, which is why every lookup is
// per group. The pin says so itself, for code 532: "the rubric for this
// concept is 'completed' in the 'instruction states' group (known issue, see
// SPECPR-51)" — 532 is complete in version lifecycle state and completed in
// instruction states.
//
// The data is generated from the openEHR Foundation's pinned
// openehr_terminology.xml under resources/terminology/ (TERM Release-3.0.0)
// via cmd/termgen, and lives in one table the generator emits:
// openehr_gen.go, carrying one variable per group and per code set plus the
// release version and the pin's sha256. Regenerate with `make termgen`;
// `make termgen-verify` fails the build when the table drifts from the pin.
// The hand-written surface is the [Group] and [CodeSet] types, their
// nil-safe lookup methods, the [Concept] data type, the [ID] constant and
// the four registry accessors.
// No runtime terminology-service lookup — the generated tables are pure Go
// literals.
//
// Consumed by [github.com/cadasto/openehr-sdk-go/openehr/client/ehr] and
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution] for
// version-lifecycle-state and audit-change-type validity and rubrics, by
// [github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified] for
// participation modes and the ctx/ defaults, and by
// [github.com/cadasto/openehr-sdk-go/openehr/instance] for setting and
// category defaults.
// Any SDK surface that emits, defaults, validates or decodes an openehr
// coded value takes the code set, the rubric and the membership verdict from
// here; a hand-typed openEHR code table elsewhere is a defect (REQ-034).
//
// Building-block weight: stdlib-only, and deliberately below openehr/rm so
// RM-level consumers can adopt it without a cycle (REQ-034; it joins the
// REQ-013 building-block-independence set — imports_test.go is the
// tripwire). The tables are compiled-in literals; the only init-time work is
// the per-group code and rubric index each newGroup call builds, and the
// registry is a plain slice scanned linearly. Safe to import from any SDK
// sub-package.
package terminology
