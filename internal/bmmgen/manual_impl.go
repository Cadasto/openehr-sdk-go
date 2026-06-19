package bmmgen

// manuallyImplemented lists the BMM behavioural functions whose stub
// emission renderFunctions MUST skip, because they are hand-written in
// a non-generated `*_funcs.go` file in the target package (the
// generator's documented "implement in a non-generated file"
// extension point). Without this skip the hand-written method and the
// generated panic-stub would collide ("method redeclared").
//
// Keys are `OWNER.function` where OWNER is the BMM class the function
// is *declared* on (ALL_CAPS) and function is the BMM snake_case name —
// the same pair that appears in the generated panic message. For a
// function declared on an abstract class and propagated to every
// concrete descendant (e.g. UID_BASED_ID.root, PATHABLE.item_at_path),
// the single declaring-owner key suppresses the stub on all
// descendants.
//
// Scope: only the pure / derived RM behavioural functions realised by
// REQ-120..123 (see docs/specifications/rm-functions.md and ADR 0011).
// Deferred functions (temporal arithmetic, PATHABLE.parent /
// path_of_item, VERSIONED_OBJECT container ops, commit_*) are NOT
// listed — they remain generated fail-loud panic stubs until a
// follow-up REQ realises them.
//
// Generator structural rationale: ADR 0002 § D7 (manual-implementation
// skip).
var manuallyImplemented = map[string]bool{
	// REQ-120 — identifier parsing & derivation.
	// Hand-written: openehr/rm/identification_funcs.go.
	"UID_BASED_ID.root":                    true, // → HierObjectID, ObjectVersionID
	"UID_BASED_ID.extension":               true,
	"UID_BASED_ID.has_extension":           true,
	"OBJECT_VERSION_ID.object_id":          true,
	"OBJECT_VERSION_ID.creating_system_id": true,
	"OBJECT_VERSION_ID.version_tree_id":    true,
	"OBJECT_VERSION_ID.is_branch":          true,
	"VERSION_TREE_ID.trunk_version":        true,
	"VERSION_TREE_ID.branch_number":        true,
	"VERSION_TREE_ID.branch_version":       true,
	"VERSION_TREE_ID.is_branch":            true,
	"VERSION_TREE_ID.is_first":             true,
	"ARCHETYPE_ID.rm_originator":           true,
	"ARCHETYPE_ID.rm_name":                 true,
	"ARCHETYPE_ID.rm_entity":               true,
	"ARCHETYPE_ID.qualified_rm_entity":     true,
	"ARCHETYPE_ID.domain_concept":          true,
	"ARCHETYPE_ID.specialisation":          true,
	"ARCHETYPE_ID.version_id":              true,
	"TERMINOLOGY_ID.name":                  true,
	"TERMINOLOGY_ID.version_id":            true,
	"LOCATABLE_REF.as_uri":                 true,

	// REQ-121 — locatable path read access. The path-method stubs are
	// suppressed (not filled): openehr/rm/rmpath provides the read API
	// as package functions, because a delegating rm method would create
	// an rm ↔ rmpath import cycle (ADR 0011 refinement). PATHABLE.parent
	// and PATHABLE.path_of_item stay generated stubs (out of scope).
	"PATHABLE.item_at_path":  true,
	"PATHABLE.items_at_path": true,
	"PATHABLE.path_exists":   true,
	"PATHABLE.path_unique":   true,

	// REQ-122 — version-control derived helper.
	// Hand-written: openehr/rm/changecontrol_funcs.go (on the concrete
	// OriginalVersion / ImportedVersion, which carry the uid).
	"VERSION.is_branch": true,

	// REQ-123 — temporal data-value helpers (magnitude + ordering).
	// Hand-written: openehr/rm/temporal_funcs.go. Arithmetic (add,
	// subtract, diff, multiply, negative) and is_equal stay deferred
	// stubs.
	"DV_DATE.magnitude":                      true,
	"DV_DATE.less_than":                      true,
	"DV_DATE.is_strictly_comparable_to":      true,
	"DV_TIME.magnitude":                      true,
	"DV_TIME.less_than":                      true,
	"DV_TIME.is_strictly_comparable_to":      true,
	"DV_DATE_TIME.magnitude":                 true,
	"DV_DATE_TIME.less_than":                 true,
	"DV_DATE_TIME.is_strictly_comparable_to": true,
	"DV_DURATION.magnitude":                  true,
	"DV_DURATION.less_than":                  true,
	"DV_DURATION.is_strictly_comparable_to":  true,
}
