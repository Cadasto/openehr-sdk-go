package bmmgen

import "strings"

// Target describes one generation destination: which BMM root to
// load, which Go package name to emit, where on disk to write it,
// and how to handle cross-target class references.
//
// Phase 2 emitted a single hard-coded target (RM). Phase 4 introduces
// AOM 1.4 as a sibling target — see docs/plans/2026-05-15-bmm-codegen.md.
// The two targets share `openehr_base_1.3.0` via include, but the AOM
// target does NOT re-emit base classes; instead it references them
// as `rm.<GoName>` (Option C in the Phase 4 design — one-way dep
// `aom14 → rm`).
type Target struct {
	// RootID is the BMM include id to load (no .bmm.json suffix).
	// Example: "openehr_rm_1.2.0" or "openehr_am_1.4.0".
	RootID string
	// GoPackage is the Go package name written in the file header.
	// Example: "rm", "aom14".
	GoPackage string
	// OutSubDir is the sub-directory under OutDir where _gen.go files
	// land. Example: "openehr/rm", "openehr/aom/aom14".
	OutSubDir string
	// SourceLabel is rendered into the generated-file header after
	// "// Source: ". Example:
	// "openehr_rm_1.2.0.bmm.json + openehr_base_1.3.0.bmm.json".
	SourceLabel string
	// OwnPackagePrefixes is the list of BMM package dotted-name
	// prefixes whose classes this target *owns* (emits). Classes whose
	// enclosing BMM package starts with one of these prefixes are
	// emitted in this target; classes elsewhere are treated as
	// "external" and referenced via [ExternalQualifier] (if non-empty)
	// or `any` (if empty).
	//
	// For the RM target this is empty — RM owns every class that
	// survives the skip rules (including base types).
	//
	// For the AOM target this is ["org.openehr.am."] — only AOM-namespaced
	// classes are emitted. Everything else (base classes, RM types
	// transitively included by AOM properties) is rendered as
	// `rm.<GoName>`.
	OwnPackagePrefixes []string
	// ExternalQualifier is the Go package qualifier prepended to class
	// references that fall outside [OwnPackagePrefixes]. Example: "rm".
	// Empty means "no qualifier" — the class is assumed to live in the
	// same package (used for the RM target).
	ExternalQualifier string
	// ExternalImport is the import path corresponding to
	// [ExternalQualifier]. Empty when ExternalQualifier is empty.
	// Example: "github.com/cadasto/openehr-sdk-go/openehr/rm".
	ExternalImport string
	// RegistryImport is the import path for the typereg package used
	// by this target. Both RM and AOM share the same registry:
	// "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg". The
	// _type discriminator strings are disjoint between the two models.
	RegistryImport string
}

// TargetRM is the RM generation target.
var TargetRM = Target{
	RootID:             "openehr_rm_1.2.0",
	GoPackage:          "rm",
	OutSubDir:          "openehr/rm",
	SourceLabel:        "openehr_rm_1.2.0.bmm.json + openehr_base_1.3.0.bmm.json",
	OwnPackagePrefixes: nil, // RM owns all surviving classes.
	ExternalQualifier:  "",
	ExternalImport:     "",
	RegistryImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg",
}

// TargetAOM14 is the AOM 1.4 generation target. Emits to
// `openehr/aom/aom14/`. AOM-defined classes are emitted locally;
// base/RM classes referenced by AOM (e.g. AUTHORED_RESOURCE,
// ARCHETYPE_ID, HIER_OBJECT_ID, VALIDITY_KIND) are imported from
// the rm package as `rm.<GoName>`. See docs/plans/2026-05-15-bmm-codegen.md
// Phase 4 § Architectural decisions for rationale.
var TargetAOM14 = Target{
	RootID:             "openehr_am_1.4.0",
	GoPackage:          "aom14",
	OutSubDir:          "openehr/aom/aom14",
	SourceLabel:        "openehr_am_1.4.0.bmm.json + openehr_base_1.3.0.bmm.json",
	OwnPackagePrefixes: []string{"org.openehr.am."},
	ExternalQualifier:  "rm",
	ExternalImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm",
	RegistryImport:     "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg",
}

// DefaultTargets is the set of targets emitted when the CLI is invoked
// with no explicit target list.
func DefaultTargets() []Target {
	return []Target{TargetRM, TargetAOM14}
}

// owns reports whether t emits the given BMM package path. Empty
// OwnPackagePrefixes means "owns everything" (the RM target).
func (t Target) owns(pkgPath string) bool {
	if len(t.OwnPackagePrefixes) == 0 {
		return true
	}
	for _, p := range t.OwnPackagePrefixes {
		if strings.HasPrefix(pkgPath, p) {
			return true
		}
	}
	return false
}
