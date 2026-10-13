package validation

// rule.go: the names an Issue carries in Rule. Every rule an emit site sets
// is one of the constants or table entries below, or, at a constrained leaf,
// the code of the violation the leaf's constraint reported, or, for a coded
// invariant, the name its codedRule row gives (rmfloor_coded.go). Keeping
// them here lets a test check each CLASS.Invariant name against the pinned
// BMMs.

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
	ruleCodeStringValid       = "CODE_PHRASE.Code_string_valid"
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

// valueValidRules gives, for each ISO 8601 data value the floor checks, the
// Value_valid invariant of that class: each of the four declares its own.
var valueValidRules = map[string]string{
	"DV_DATE":      "DV_DATE.Value_valid",
	"DV_TIME":      "DV_TIME.Value_valid",
	"DV_DATE_TIME": "DV_DATE_TIME.Value_valid",
	"DV_DURATION":  "DV_DURATION.Value_valid",
}

// archetypeRootRules gives, for each class that is always an archetype root,
// the Is_archetype_root invariant it is bound by: the one the class itself
// declares, or the one its ancestor PARTY or ENTRY declares.
var archetypeRootRules = map[string]string{
	"COMPOSITION":  "COMPOSITION.Is_archetype_root",
	"EHR_STATUS":   "EHR_STATUS.Is_archetype_root",
	"EHR_ACCESS":   "EHR_ACCESS.Is_archetype_root",
	"PERSON":       "PARTY.Is_archetype_root",
	"ORGANISATION": "PARTY.Is_archetype_root",
	"GROUP":        "PARTY.Is_archetype_root",
	"AGENT":        "PARTY.Is_archetype_root",
	"ROLE":         "PARTY.Is_archetype_root",
	"OBSERVATION":  "ENTRY.Is_archetype_root",
	"EVALUATION":   "ENTRY.Is_archetype_root",
	"INSTRUCTION":  "ENTRY.Is_archetype_root",
	"ACTION":       "ENTRY.Is_archetype_root",
	"ADMIN_ENTRY":  "ENTRY.Is_archetype_root",
}
