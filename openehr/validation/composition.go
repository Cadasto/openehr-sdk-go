package validation

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// ValidateComposition validates an in-memory RM Composition
// against a compiled OPT and returns every issue in one pass.
//
// The walk visits the composition (see package doc): the compiled OPT
// binds the nodes it reaches and decides which content it admits, and the
// composition and the Reference Model supply the rest of what the walk
// visits. Each issue's path is written as [Issue.Path] describes, by the
// same rule [ValidateRM] uses: a required attribute that is absent is
// reported at the attribute's own path, and a required node the
// composition lacks at the path the template gives it.
//
// The result includes the Reference Model rules ([ValidateRM]), applied in
// the same walk, as [Validate] describes: an attribute both layers find
// missing or empty is one finding, and a Reference Model rule's own finding
// at that path is reported beside it. A composition that passes is valid
// against the template and the RM rules the validator checks.
//
// Returns a [Result] whose Issues slice is never nil (zero-length
// allocation when no issues fire).
//
// ValidateComposition is the COMPOSITION-typed convenience over the
// generic [Validate]: it guards the nil composition (yielding
// nil_composition), then delegates. Other archetypeable RM roots are
// validated through [Validate] or its siblings [ValidateDemographic],
// [ValidateFolder], [ValidateEHRStatus].
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result {
	if comp == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_composition",
			Detail:   "ValidateComposition called with a nil *rm.Composition argument",
			Severity: Error,
		}})
	}
	return Validate(comp, c)
}

// rmTypeInfo is the single source of truth for "what RM class is
// this Go value and (when LOCATABLE) what is its archetype_node_id".
// The walk and the rules route through this function — describeRMType
// and locatableArchetypeNodeID are thin wrappers.
//
// Since ADR 0013 it decomposes onto the generated surface: the name
// half is rmbind.TypeName, the name the binding tiers compare with an
// OPT child's RMTypeName, so the binding and the RM type match read one
// name. It delegates to rm.RMTypeName (every registered concrete) and
// keeps the parameterised DV_INTERVAL names from rmnames: they decide
// the RM type match against an OPT child's RMTypeName, and
// rm_type_mismatch details carry the same names. The archetype_node_id
// half reads polymorphically through rm.Locatable.
//
// Returns ("", "", false) for nil, typed-nil, and non-RM Go values.
// No reflection.
func rmTypeInfo(v any) (rmType string, archetypeNodeID string, ok bool) {
	name, ok := rmbind.TypeName(v)
	if !ok {
		return "", "", false
	}
	if l, isLocatable := v.(rm.Locatable); isLocatable {
		// TypeName refuses typed-nils, so none reaches the getter.
		return name, l.GetArchetypeNodeID(), true
	}
	return name, "", true
}

// describeRMType returns a string suitable for inclusion in Issue
// Detail messages identifying the concrete Go type of an RM value.
// Pointer types are unwrapped to their elem type name.
func describeRMType(v any) string {
	if v == nil {
		return "<nil>"
	}
	if name, _, ok := rmTypeInfo(v); ok {
		return name
	}
	return fmt.Sprintf("%T", v)
}
