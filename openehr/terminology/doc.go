// Package terminology answers "is this code a member of that openEHR
// terminology group, what is its rubric, and which code carries this
// rubric?" for the closed, versioned value sets the RM's own invariants
// reference (EVENT_CONTEXT.Setting_valid, AUDIT_DETAILS.Change_type_valid,
// PARTICIPATION.Mode_valid, DV_ORDERED.Normal_status_validity, …).
//
// The questions are on [Group] (17 groups of coded concepts, each with a
// rubric) and on [CodeSet] (3 sets of bare codes with no rubric). Both are
// closed and source-ordered, and a nil pointer of either is inert: no method
// panics. A [Group]'s rubric and reverse-rubric lookups report a miss as a
// false second return rather than a fabricated value; a [CodeSet] carries no
// rubric, so its [CodeSet.Has] answers membership with a plain bool.
// [Groups], [CodeSets], [GroupByID] and [CodeSetByID] enumerate the pinned
// tables; [ID] is the TERMINOLOGY_ID value they are all defined in.
//
// A rubric belongs to a code within a group, which is why every lookup is
// per group. The terminology file says so itself, for code 532: "the rubric
// for this concept is 'completed' in the 'instruction states' group (known
// issue, see SPECPR-51)". Code 532 is "complete" in version lifecycle state
// and "completed" in instruction states.
//
// The data is generated from the openEHR Foundation's
// openehr_terminology.xml, pinned under resources/terminology/ (TERM
// Release-3.0.0), by cmd/termgen. It lives in one generated table,
// openehr_gen.go, with one variable per group and per code set plus the
// release version and the file's sha256. Those exported table variables are
// read-only: reassigning one changes validity across the whole SDK, so treat
// them as constants. Regenerate with `make termgen`; `make termgen-verify`
// fails the build when the table drifts from the pinned file. The
// hand-written surface is the [Group] and [CodeSet] types, their nil-safe
// lookup methods, the [Concept] data type, the [ID] constant and the four
// registry accessors. There is no runtime terminology-service lookup: the
// generated tables are plain Go literals.
//
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr] and
// [github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution] use it
// for version-lifecycle-state and audit-change-type validity and rubrics,
// [github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified] for
// participation modes and the ctx/ defaults, and
// [github.com/cadasto/openehr-sdk-go/openehr/instance] for setting and
// category defaults. SDK code that mints, defaults, validates or
// reconstructs an openEHR coded value from a bare code or rubric takes the
// code set, the rubric and the membership verdict from here. A codec that
// transports a caller-supplied code and rubric pair whole passes it through
// unchanged; this package does not enforce RM instance invariants on
// decoded values.
//
// The package uses only the standard library and sits below openehr/rm, so
// RM-level code can use it without an import cycle. The tables are
// compiled-in literals; the only init-time work is the per-group code and
// rubric index each group builds, and the registry is a plain slice scanned
// linearly. It is safe to import from any SDK sub-package.
package terminology
