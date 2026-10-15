package validation

// rule.go: the names an Issue carries in Rule. Every rule an emit site sets
// is one of the constants below, or, at a constrained leaf, the code of the
// violation the leaf's constraint reported, or, for a coded invariant, the
// name its codedRule row gives (rmfloor_coded.go), or, for an evaluator that
// serves one invariant on several classes (Is_archetype_root, Value_valid),
// the key of the invariant inventory that dispatched it
// (inventory_dispatch.go). Keeping them here lets a test check each
// CLASS.Invariant name against the pinned BMMs.

// The template-layer rules: the clause of the operational template an issue
// evaluates.
const (
	// ruleExistence is an attribute the template requires and the instance
	// lacks, or one the template prohibits and the instance carries.
	ruleExistence = "existence"
	// ruleCardinality is the number of items a multi-valued attribute holds.
	// On the RM layer it is a mandatory list that is present and empty.
	ruleCardinality = "cardinality"
	// ruleOccurrences is the number of items bound to one child of an
	// attribute.
	ruleOccurrences = "occurrences"
	// ruleSlot is an item that fits no child and no slot.
	ruleSlot = "slot"
	// ruleType is type conformance, and the alternatives of an attribute.
	ruleType = "type"
	// ruleIdentity is an archetype id or a node id.
	ruleIdentity = "identity"
)

// The RM-layer rules the BMM states in an attribute's definition rather than
// as a named invariant. The third, a mandatory list present and empty, is
// ruleCardinality.
const (
	// ruleMandatory is an RM-mandatory attribute the instance lacks, a
	// mandatory string left empty included.
	ruleMandatory = "mandatory"
	// rulePrecisionRange is a precision below -1, which the BMM's
	// definition of precision rules out (-1 means no limit).
	rulePrecisionRange = "precision_range"
)

// The RM-layer rules the pinned RM and BASE BMMs name, written
// CLASS.Invariant: the class that declares the invariant and the
// invariant's name, both as the BMM spells them.
const (
	ruleProportionPrecision   = "DV_PROPORTION.Precision_validity"
	ruleNullFlavourIndicated  = "ELEMENT.Inv_null_flavour_indicated"
	ruleIntervalLimits        = "DV_INTERVAL.Limits_consistent"
	ruleLowerIncludedValid    = "Interval.Lower_included_valid"
	ruleUpperIncludedValid    = "Interval.Upper_included_valid"
	rulePartyBasicValidity    = "PARTY_IDENTIFIED.Basic_validity"
	rulePartyNameValid        = "PARTY_IDENTIFIED.Name_valid"
	rulePartyIdentifiersValid = "PARTY_IDENTIFIED.Identifiers_valid"
	ruleMappingsValid         = "DV_TEXT.Mappings_valid"
	ruleTermMappingMatch      = "TERM_MAPPING.Match_valid"
	ruleRMVersionValid        = "ARCHETYPED.Rm_version_valid"
)
