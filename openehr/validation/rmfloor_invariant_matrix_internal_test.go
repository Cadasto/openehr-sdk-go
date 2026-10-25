package validation

// rmfloor_invariant_matrix_internal_test.go: PROBE-077 — REQ-112, REQ-171.
// The RM-floor invariant matrix. For every invariant the inventory calls
// evaluated, and every class the registry decodes that is subject to it, a
// value of the class decoded from canonical JSON that breaks the invariant
// must draw the invariant's finding, and a value of the class that keeps it
// must draw none. For an invariant another evaluated check decides, the
// finding is the one that check reports, with that check's rule (REQ-171
// Finding identity). The generated inventory supplies the pairs: an
// invariant newly called evaluated that has no row fails the probe, and so
// does a class newly subject to an invariant whose row does not fit the
// class. DV_INTERVAL.Limits_consistent is ordered by the type of the bounds,
// so its row breaks it with bounds of every concrete DV_ORDERED the pinned
// RM declares, and a type newly declared fails the probe until its pair is
// added.

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// bogusPhrase and bogusCoded hold a code no group or code set of the
// openEHR terminology has.
const (
	bogusPhrase = `{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"no-such-code"}`
	bogusCoded  = `{"_type":"DV_CODED_TEXT","value":"x","defining_code":` + bogusPhrase + `}`
)

// archetypeDetailsMember holds a complete archetype_details. The floor reads
// neither the grammar of the archetype id nor the class it names, so one
// value serves every archetype root.
const archetypeDetailsMember = `"archetype_details":{"_type":"ARCHETYPED","archetype_id":{"_type":"ARCHETYPE_ID","value":"openEHR-EHR-OBSERVATION.x.v1"},"rm_version":"1.1.0"}`

// codePhraseJSON returns a CODE_PHRASE holding code in the terminology id.
func codePhraseJSON(id, code string) string {
	return `{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"` + id + `"},"code_string":"` + code + `"}`
}

// codedTextJSON returns a DV_CODED_TEXT whose defining code is code in the
// openEHR terminology.
func codedTextJSON(code string) string {
	return `{"_type":"DV_CODED_TEXT","value":"x","defining_code":` + codePhraseJSON("openehr", code) + `}`
}

// localSymbolJSON returns a DV_CODED_TEXT symbol holding code in the local
// terminology.
func localSymbolJSON(code string) string {
	return `{"_type":"DV_CODED_TEXT","value":"x","defining_code":` + codePhraseJSON("local", code) + `}`
}

// matrixRow is the row of one evaluated invariant. breaking holds the JSON
// members that break the invariant on a value of any class subject to it;
// empty, the class's zero value breaks it. conforming holds the members of a
// value of any such class that keeps it: the breaking members with what
// breaks the invariant changed, or with the member that keeps it added. For
// an invariant another evaluated check decides, rule and path name the issue
// that check reports for the break: its rule is the rule of the check
// (REQ-171 Finding identity), not the invariant's. more holds further pairs
// of breaking and conforming members, each checked like the row's own, for
// an invariant whose evaluator reads values of one class in more than one
// way: Limits_consistent orders the bounds of a DV_INTERVAL by their type,
// Fraction_validity holds on a fraction and on an integer fraction, and
// Events_valid reads a HISTORY's events absent and empty.
type matrixRow struct {
	breaking, conforming string
	rule, path           string
	more                 []matrixPair
}

// matrixPair is one further pair of a row: the members of a value that
// breaks the invariant and of one that keeps it.
type matrixPair struct {
	breaking, conforming string
}

// boundPair returns the pair of a DV_INTERVAL whose bounds are of class, from
// the members of a high and a low bound of that class: breaking holds high as
// the lower bound and low as the upper one, and conforming the two in order.
func boundPair(class, high, low string) matrixPair {
	bound := func(members string) string { return `{"_type":"` + class + `",` + members + `}` }
	return matrixPair{
		breaking:   `"lower":` + bound(high) + `,"upper":` + bound(low),
		conforming: `"lower":` + bound(low) + `,"upper":` + bound(high),
	}
}

// intervalOrderPairs are the further pairs of DV_INTERVAL.Limits_consistent,
// one for each bound type but DV_COUNT, whose pair is the row's own. Each
// breaking interval holds two bounds the RM makes strictly comparable, lower
// above upper by the RM's less_than for their type (REQ-112).
var intervalOrderPairs = []matrixPair{
	boundPair("DV_QUANTITY", `"magnitude":5,"units":"mm"`, `"magnitude":1,"units":"mm"`),
	boundPair("DV_ORDINAL", `"value":3,"symbol":`+localSymbolJSON("at3"), `"value":1,"symbol":`+localSymbolJSON("at1")),
	boundPair("DV_SCALE", `"value":2.5,"symbol":`+localSymbolJSON("at3"), `"value":0.5,"symbol":`+localSymbolJSON("at1")),
	boundPair("DV_PROPORTION", `"numerator":3,"denominator":4,"type":0`, `"numerator":1,"denominator":4,"type":0`),
	boundPair("DV_DATE", `"value":"2025-01-01"`, `"value":"2024-01-01"`),
	boundPair("DV_TIME", `"value":"10:00:00"`, `"value":"08:00:00"`),
	boundPair("DV_DATE_TIME", `"value":"2025-01-01T00:00:00Z"`, `"value":"2024-01-01T00:00:00Z"`),
	boundPair("DV_DURATION", `"value":"P2D"`, `"value":"P1D"`),
}

// historyEventsPairs are the further pairs of HISTORY.Events_valid, whose
// own pair breaks it with neither events nor a summary and keeps it with a
// summary: events present and holding no item break it too, and an event
// keeps it.
var historyEventsPairs = []matrixPair{
	{breaking: `"events":[]`, conforming: `"events":[{"_type":"POINT_EVENT","archetype_node_id":"at0002","name":{"_type":"DV_TEXT","value":"x"},"time":{"_type":"DV_DATE_TIME","value":"2024-01-01T00:00:00Z"}}]`},
}

// pairs returns the row's own pair followed by its further pairs.
func (r matrixRow) pairs() []matrixPair {
	return append([]matrixPair{{breaking: r.breaking, conforming: r.conforming}}, r.more...)
}

// invariantMatrix holds the row of every invariant the inventory calls
// evaluated, by its key. The first group holds the invariants the floor
// dispatches an evaluator of their own for, and the second the coded ones,
// which the coded pass evaluates. The last three hold those another evaluated
// check decides: the mandatory-presence rule, for an empty mandatory string
// or list; the class's Value_valid, at the value itself, for an ISO 8601
// value the REQ-123 parse refuses; and DV_PROPORTION's Precision_validity, at
// the value itself, for a numerator or denominator that is not whole under
// the precision of 0 the RM reads is_integral as.
var invariantMatrix = map[string]matrixRow{
	"ARCHETYPED.Rm_version_valid":           {breaking: `"archetype_id":{"_type":"ARCHETYPE_ID","value":"openEHR-EHR-OBSERVATION.x.v1"}`, conforming: `"archetype_id":{"_type":"ARCHETYPE_ID","value":"openEHR-EHR-OBSERVATION.x.v1"},"rm_version":"1.1.0"`},
	"ATTESTATION.Items_valid":               {breaking: `"items":[]`, conforming: `"items":[{"_type":"DV_EHR_URI","value":"ehr://example.org/x"}]`},
	"COMPOSITION.Content_valid":             {breaking: `"content":[]`, conforming: `"content":[{"_type":"SECTION","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"}}]`},
	"COMPOSITION.Is_archetype_root":         {breaking: ``, conforming: archetypeDetailsMember},
	"DV_DATE.Value_valid":                   {breaking: `"value":"example"`, conforming: `"value":"2024-01-01"`},
	"DV_DATE_TIME.Value_valid":              {breaking: `"value":"example"`, conforming: `"value":"2024-01-01T10:30:00Z"`},
	"DV_DURATION.Value_valid":               {breaking: `"value":"example"`, conforming: `"value":"P1D"`},
	"DV_INTERVAL.Limits_consistent":         {breaking: `"lower":{"_type":"DV_COUNT","magnitude":5},"upper":{"_type":"DV_COUNT","magnitude":1}`, conforming: `"lower":{"_type":"DV_COUNT","magnitude":5},"upper":{"_type":"DV_COUNT","magnitude":5}`, more: intervalOrderPairs},
	"DV_PROPORTION.Fraction_validity":       {breaking: `"numerator":1.5,"denominator":2,"type":3`, conforming: `"numerator":1,"denominator":2,"type":3`, more: []matrixPair{{breaking: `"numerator":3,"denominator":2.5,"type":4`, conforming: `"numerator":3,"denominator":2,"type":4`}}},
	"DV_PROPORTION.Percent_validity":        {breaking: `"numerator":1,"denominator":1,"type":2`, conforming: `"numerator":1,"denominator":100,"type":2`},
	"DV_PROPORTION.Precision_validity":      {breaking: `"numerator":1.5,"denominator":2,"type":0,"precision":0`, conforming: `"numerator":1,"denominator":2,"type":0,"precision":0`},
	"DV_PROPORTION.Type_validity":           {breaking: `"numerator":1,"denominator":2,"type":5`, conforming: `"numerator":1,"denominator":2,"type":0`},
	"DV_PROPORTION.Unitary_validity":        {breaking: `"numerator":1,"denominator":2,"type":1`, conforming: `"numerator":1,"denominator":1,"type":1`},
	"DV_PROPORTION.Valid_denominator":       {breaking: `"numerator":1,"denominator":0,"type":0`, conforming: `"numerator":1,"denominator":2,"type":0`},
	"DV_TEXT.Mappings_valid":                {breaking: `"value":"x","mappings":[]`, conforming: `"value":"x","mappings":[{"_type":"TERM_MAPPING","match":"="}]`},
	"DV_TIME.Value_valid":                   {breaking: `"value":"example"`, conforming: `"value":"10:30:00"`},
	"EHR_ACCESS.Is_archetype_root":          {breaking: ``, conforming: archetypeDetailsMember},
	"EHR_STATUS.Is_archetype_root":          {breaking: ``, conforming: archetypeDetailsMember},
	"ELEMENT.Inv_null_flavour_indicated":    {breaking: ``, conforming: `"value":{"_type":"DV_TEXT","value":"x"}`},
	"ENTRY.Is_archetype_root":               {breaking: ``, conforming: archetypeDetailsMember},
	"EVENT_CONTEXT.Participations_validity": {breaking: `"participations":[]`, conforming: `"participations":[{"_type":"PARTICIPATION","function":{"_type":"DV_TEXT","value":"x"},"performer":{"_type":"PARTY_SELF"}}]`},
	"HISTORY.Events_valid":                  {breaking: ``, conforming: `"summary":{"_type":"ITEM_TREE","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"}}`, more: historyEventsPairs},
	"INSTRUCTION.Activities_valid":          {breaking: `"activities":[]`, conforming: `"activities":[{"_type":"ACTIVITY","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"}}]`},
	"Interval.Lower_included_valid":         {breaking: `"lower_unbounded":true,"lower_included":true`, conforming: `"lower_unbounded":true,"lower_included":false`},
	"Interval.Upper_included_valid":         {breaking: `"upper_unbounded":true,"upper_included":true`, conforming: `"upper_unbounded":true,"upper_included":false`},
	"PARTY.Is_archetype_root":               {breaking: ``, conforming: archetypeDetailsMember},
	"PARTY_IDENTIFIED.Basic_validity":       {breaking: ``, conforming: `"name":"x"`},
	"PARTY_IDENTIFIED.Identifiers_valid":    {breaking: `"identifiers":[]`, conforming: `"identifiers":[{"_type":"DV_IDENTIFIER","id":"x"}]`},
	"PARTY_IDENTIFIED.Name_valid":           {breaking: `"name":""`, conforming: `"name":"x"`},
	"SECTION.Items_valid":                   {breaking: `"items":[]`, conforming: `"items":[{"_type":"SECTION","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"x"}}]`},
	"TERM_MAPPING.Match_valid":              {breaking: `"match":"x"`, conforming: `"match":"="`},

	"COMPOSITION.Category_validity":                    {breaking: `"category":` + bogusCoded, conforming: `"category":` + codedTextJSON("433")},
	"COMPOSITION.Language_valid":                       {breaking: `"language":` + bogusPhrase, conforming: `"language":` + codePhraseJSON("ISO_639-1", "en")},
	"COMPOSITION.Territory_valid":                      {breaking: `"territory":` + bogusPhrase, conforming: `"territory":` + codePhraseJSON("ISO_3166-1", "NL")},
	"DV_ENCAPSULATED.Charset_valid":                    {breaking: `"charset":` + bogusPhrase, conforming: `"charset":` + codePhraseJSON("IANA_character-sets", "UTF-8")},
	"DV_ENCAPSULATED.Language_valid":                   {breaking: `"language":` + bogusPhrase, conforming: `"language":` + codePhraseJSON("ISO_639-1", "en")},
	"DV_MULTIMEDIA.Compression_algorithm_validity":     {breaking: `"compression_algorithm":` + bogusPhrase, conforming: `"compression_algorithm":` + codePhraseJSON("openehr_compression_algorithms", "gzip")},
	"DV_MULTIMEDIA.Integrity_check_algorithm_validity": {breaking: `"integrity_check_algorithm":` + bogusPhrase, conforming: `"integrity_check_algorithm":` + codePhraseJSON("openehr_integrity_check_algorithms", "SHA-256")},
	"DV_MULTIMEDIA.Media_type_valid":                   {breaking: `"media_type":` + bogusPhrase, conforming: `"media_type":` + codePhraseJSON("IANA_media-types", "text/plain")},
	"DV_ORDERED.Normal_status_validity":                {breaking: `"normal_status":` + bogusPhrase, conforming: `"normal_status":` + codePhraseJSON("openehr_normal_statuses", "N")},
	"DV_TEXT.Encoding_valid":                           {breaking: `"value":"x","encoding":` + bogusPhrase, conforming: `"value":"x","encoding":` + codePhraseJSON("IANA_character-sets", "UTF-8")},
	"DV_TEXT.Language_valid":                           {breaking: `"value":"x","language":` + bogusPhrase, conforming: `"value":"x","language":` + codePhraseJSON("ISO_639-1", "en")},
	"ELEMENT.Inv_null_flavour_valid":                   {breaking: `"null_flavour":` + bogusCoded, conforming: `"null_flavour":` + codedTextJSON("253")},
	"ENTRY.Encoding_valid":                             {breaking: `"encoding":` + bogusPhrase, conforming: `"encoding":` + codePhraseJSON("IANA_character-sets", "UTF-8")},
	"ENTRY.Language_valid":                             {breaking: `"language":` + bogusPhrase, conforming: `"language":` + codePhraseJSON("ISO_639-1", "en")},
	"EVENT_CONTEXT.Setting_valid":                      {breaking: `"setting":` + bogusCoded, conforming: `"setting":` + codedTextJSON("225")},
	"INTERVAL_EVENT.Math_function_validity":            {breaking: `"math_function":` + bogusCoded, conforming: `"math_function":` + codedTextJSON("146")},
	"ISM_TRANSITION.Current_state_valid":               {breaking: `"current_state":` + bogusCoded, conforming: `"current_state":` + codedTextJSON("526")},
	"ISM_TRANSITION.Transition_valid":                  {breaking: `"transition":` + bogusCoded, conforming: `"transition":` + codedTextJSON("535")},
	"PARTICIPATION.Function_valid":                     {breaking: `"function":` + bogusCoded, conforming: `"function":` + codedTextJSON("253")},
	"PARTICIPATION.Mode_valid":                         {breaking: `"mode":` + bogusCoded, conforming: `"mode":` + codedTextJSON("216")},
	"PARTY_RELATED.Relationship_valid":                 {breaking: `"relationship":` + bogusCoded, conforming: `"relationship":` + codedTextJSON("10")},
	"TERM_MAPPING.Purpose_valid":                       {breaking: `"purpose":` + bogusCoded, conforming: `"purpose":` + codedTextJSON("669")},

	"ACTIVITY.Action_archetype_id_valid":         {breaking: `"action_archetype_id":""`, conforming: `"action_archetype_id":"openEHR-EHR-ACTION.x.v1"`, rule: ruleMandatory, path: "/action_archetype_id"},
	"AUDIT_DETAILS.System_id_valid":              {breaking: `"system_id":""`, conforming: `"system_id":"x"`, rule: ruleMandatory, path: "/system_id"},
	"CODE_PHRASE.Code_string_valid":              {breaking: `"terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":""`, conforming: `"terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"`, rule: ruleMandatory, path: "/code_string"},
	"DV_IDENTIFIER.Id_valid":                     {breaking: `"id":""`, conforming: `"id":"x"`, rule: ruleMandatory, path: "/id"},
	"DV_PARAGRAPH.Items_valid":                   {breaking: `"items":[]`, conforming: `"items":[{"_type":"DV_TEXT","value":"x"}]`, rule: ruleCardinality, path: "/items"},
	"DV_PARSABLE.Formalism_valid":                {breaking: `"formalism":""`, conforming: `"formalism":"x"`, rule: ruleMandatory, path: "/formalism"},
	"DV_TEXT.Valid_value":                        {breaking: `"value":""`, conforming: `"value":"x"`, rule: ruleMandatory, path: "/value"},
	"DV_URI.Value_valid":                         {breaking: `"value":""`, conforming: `"value":"ehr://example.org/x"`, rule: ruleMandatory, path: "/value"},
	"FEEDER_AUDIT_DETAILS.System_id_valid":       {breaking: `"system_id":""`, conforming: `"system_id":"x"`, rule: ruleMandatory, path: "/system_id"},
	"INSTRUCTION_DETAILS.Activity_path_valid":    {breaking: `"activity_id":""`, conforming: `"activity_id":"activities[at0001]"`, rule: ruleMandatory, path: "/activity_id"},
	"LOCATABLE.Archetype_node_id_valid":          {breaking: `"archetype_node_id":""`, conforming: `"archetype_node_id":"at0001"`, rule: ruleMandatory, path: "/archetype_node_id"},
	"PARTY.Identities_valid":                     {breaking: `"identities":[]`, conforming: `"identities":[{"_type":"PARTY_IDENTITY"}]`, rule: ruleCardinality, path: "/identities"},
	"RESOURCE_DESCRIPTION.Lifecycle_state_valid": {breaking: `"lifecycle_state":""`, conforming: `"lifecycle_state":"x"`, rule: ruleMandatory, path: "/lifecycle_state"},
	"RESOURCE_DESCRIPTION_ITEM.Purpose_valid":    {breaking: `"purpose":""`, conforming: `"purpose":"x"`, rule: ruleMandatory, path: "/purpose"},
	"REVISION_HISTORY_ITEM.Audit_valid":          {breaking: `"audits":[]`, conforming: `"audits":[{"_type":"AUDIT_DETAILS"}]`, rule: ruleCardinality, path: "/audits"},
	"UID.Value_valid":                            {breaking: `"value":""`, conforming: `"value":"x"`, rule: ruleMandatory, path: "/value"},
	"VERSION_TREE_ID.Value_valid":                {breaking: `"value":""`, conforming: `"value":"1"`, rule: ruleMandatory, path: "/value"},

	"Iso8601_date.Day_valid":                    {breaking: `"value":"2023-02-29"`, conforming: `"value":"2024-02-29"`, rule: "DV_DATE.Value_valid", path: "/"},
	"Iso8601_date.Month_valid":                  {breaking: `"value":"2024-13-01"`, conforming: `"value":"2024-12-01"`, rule: "DV_DATE.Value_valid", path: "/"},
	"Iso8601_date.Partial_validity":             {breaking: `"value":"2024--15"`, conforming: `"value":"2024"`, rule: "DV_DATE.Value_valid", path: "/"},
	"Iso8601_date.Year_valid":                   {breaking: `"value":"-2024-01-01"`, conforming: `"value":"2024-01-01"`, rule: "DV_DATE.Value_valid", path: "/"},
	"Iso8601_date_time.Fractional_second_valid": {breaking: `"value":"2024-01-01T10:30.5"`, conforming: `"value":"2024-01-01T10:30:00.5"`, rule: "DV_DATE_TIME.Value_valid", path: "/"},
	"Iso8601_date_time.Hour_valid":              {breaking: `"value":"2024-01-01T25:00:00"`, conforming: `"value":"2024-01-01T23:00:00"`, rule: "DV_DATE_TIME.Value_valid", path: "/"},
	"Iso8601_date_time.Minute_valid":            {breaking: `"value":"2024-01-01T10:60:00"`, conforming: `"value":"2024-01-01T10:59:00"`, rule: "DV_DATE_TIME.Value_valid", path: "/"},
	"Iso8601_date_time.Partial_validity_minute": {breaking: `"value":"2024-01-01T10::30"`, conforming: `"value":"2024-01-01T10"`, rule: "DV_DATE_TIME.Value_valid", path: "/"},
	"Iso8601_date_time.Year_valid":              {breaking: `"value":"-2024-01-01T10:00:00"`, conforming: `"value":"2024-01-01T10:00:00"`, rule: "DV_DATE_TIME.Value_valid", path: "/"},
	"Iso8601_duration.Days_valid":               {breaking: `"value":"P-1D"`, conforming: `"value":"P1D"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Fractional_second_valid":  {breaking: `"value":"PT-0.5S"`, conforming: `"value":"PT0.5S"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Hours_valid":              {breaking: `"value":"PT-1H"`, conforming: `"value":"PT1H"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Minutes_valid":            {breaking: `"value":"PT-1M"`, conforming: `"value":"PT1M"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Months_valid":             {breaking: `"value":"P-1M"`, conforming: `"value":"P1M"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Seconds_valid":            {breaking: `"value":"PT-1S"`, conforming: `"value":"PT1S"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Weeks_valid":              {breaking: `"value":"P-1W"`, conforming: `"value":"P1W"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_duration.Years_valid":              {breaking: `"value":"P-1Y"`, conforming: `"value":"P1Y"`, rule: "DV_DURATION.Value_valid", path: "/"},
	"Iso8601_time.Fractional_second_valid":      {breaking: `"value":"10:30.5"`, conforming: `"value":"10:30:00.5"`, rule: "DV_TIME.Value_valid", path: "/"},
	"Iso8601_time.Hour_valid":                   {breaking: `"value":"25:00:00"`, conforming: `"value":"23:00:00"`, rule: "DV_TIME.Value_valid", path: "/"},
	"Iso8601_time.Minute_valid":                 {breaking: `"value":"10:60:00"`, conforming: `"value":"10:59:00"`, rule: "DV_TIME.Value_valid", path: "/"},
	"Iso8601_time.Partial_validity":             {breaking: `"value":"10::30"`, conforming: `"value":"10"`, rule: "DV_TIME.Value_valid", path: "/"},

	"DV_PROPORTION.Is_integral_validity": {breaking: `"numerator":1.5,"denominator":2,"type":0,"precision":0`, conforming: `"numerator":1,"denominator":2,"type":0,"precision":0`, rule: ruleProportionPrecision, path: "/"},
}

// validateMembers decodes, through the type registry, the canonical JSON
// object of class that carries members, and validates it with ValidateRM.
// Empty members give the class's zero value. It returns the body it decoded,
// for a failure to name.
func validateMembers(class, members string) (Result, string, error) {
	body := `{"_type":"` + class + `"`
	if members != "" {
		body += "," + members
	}
	body += "}"
	v, err := typereg.Default.Decode([]byte(body))
	if err != nil {
		return Result{}, body, err
	}
	return ValidateRM(v), body, nil
}

// TestPROBE077_REQ112_REQ171_InvariantMatrix is PROBE-077. Each subtest is
// one pair of an invariant the inventory calls evaluated and a registered
// class subject to it. Each breaking value of the row must draw the
// invariant's finding: an issue whose rule is the invariant, or, for an
// invariant another evaluated check decides, the issue that check reports, at
// its path and with its rule, and the invariant must not be listed as
// unevaluated. Each conforming value of the row must draw no such issue. The
// probe fails for an evaluated invariant without a row, for a row that names
// no conforming value, holds a further pair short of a value, or misstates
// which check decides its invariant, for a row of an invariant that is not
// evaluated, and for an evaluated invariant no class is subject to. It also
// fails for an invariant bound but never dispatched, dispatched to an
// evaluator that cannot read the class, or decided by a check that does not
// run on the class, and when the row of DV_INTERVAL.Limits_consistent breaks
// it with no bounds of a concrete DV_ORDERED.
func TestPROBE077_REQ112_REQ171_InvariantMatrix(t *testing.T) {
	t.Parallel()
	subject := map[string]int{}
	for _, class := range slices.Sorted(maps.Keys(classInvariants)) {
		for _, key := range classInvariants[class] {
			if invariantInventory[key].kind != invariantEvaluated {
				continue
			}
			subject[key]++
			t.Run(key+"/"+class, func(t *testing.T) {
				fn, covered := invariantInventory[key].function, invariantChecks[key].covered
				row, ok := invariantMatrix[key]
				switch {
				case !ok && covered:
					t.Fatalf("invariantMatrix holds no row for %s, which %s decides on %s; add one", key, fn, class)
				case !ok:
					t.Fatalf("invariantMatrix holds no row for %s, which %s evaluates on %s; add one", key, fn, class)
				case covered && row.rule == "":
					t.Fatalf("the row for %s names no issue, but %s decides it on %s; name the issue %s reports", key, fn, class, fn)
				case !covered && row.rule != "":
					t.Fatalf("the row for %s names the issue %s at %s, but %s evaluates %s itself and no other check decides it", key, row.rule, row.path, fn, key)
				case row.conforming == "":
					t.Fatalf("the row for %s names no value that keeps it; add one", key)
				case slices.ContainsFunc(row.more, func(p matrixPair) bool { return p.breaking == "" || p.conforming == "" }):
					t.Fatalf("the row for %s holds a further pair without a value that breaks it and one that keeps it; complete it", key)
				}
				finding := func(i Issue) bool { return i.Rule == key }
				want := "whose rule is " + key
				if covered {
					finding = func(i Issue) bool { return i.Rule == row.rule && i.Path == row.path }
					want = "whose rule is " + row.rule + " at " + row.path
				}
				for _, pair := range row.pairs() {
					if r, body, err := validateMembers(class, pair.breaking); err != nil {
						t.Errorf("typereg.Decode(%s): %v", body, err)
					} else {
						if !slices.ContainsFunc(r.Issues, finding) {
							t.Errorf("ValidateRM(%s) issues %q, want one %s: %s does not decide %s on %s", body, keys(r.Issues), want, fn, key, class)
						}
						if covered && slices.Contains(r.Coverage.Unevaluated, key) {
							t.Errorf("ValidateRM(%s) Coverage.Unevaluated lists %s: the dispatch does not count %s as run on %s", body, key, fn, class)
						}
					}
					r, body, err := validateMembers(class, pair.conforming)
					if err != nil {
						t.Fatalf("typereg.Decode(%s): %v", body, err)
					}
					var found []Issue
					for _, i := range r.Issues {
						if finding(i) {
							found = append(found, i)
						}
					}
					if len(found) > 0 {
						t.Errorf("ValidateRM(%s) issues %q, want none %s: %s reports %s on a value of %s that keeps it", body, keys(found), want, fn, key, class)
					}
				}
			})
		}
	}
	pairs := 0
	for _, n := range subject {
		pairs += n
	}
	t.Logf("checked %d evaluated invariants on %d class-invariant pairs, each with a breaking and a conforming value", len(subject), pairs)
	for _, key := range slices.Sorted(maps.Keys(invariantInventory)) {
		if invariantInventory[key].kind == invariantEvaluated && subject[key] == 0 {
			t.Errorf("%s is evaluated, but no class the registry decodes is subject to it", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(invariantMatrix)) {
		if invariantInventory[key].kind != invariantEvaluated {
			t.Errorf("invariantMatrix holds a row for %s, which the inventory does not call evaluated", key)
		}
	}
	for _, class := range uncoveredBoundTypes(t) {
		t.Errorf("the row for %s breaks it with no %s bounds; add a pair whose bounds are two strictly comparable %s", ruleIntervalLimits, class, class)
	}
}

// uncoveredBoundTypes returns the concrete DV_ORDERED classes of the pinned
// RM that no breaking value of the DV_INTERVAL.Limits_consistent row holds as
// its lower bound. Any of them can bound a DV_INTERVAL, and the floor orders
// two strictly comparable bounds of each (REQ-112).
func uncoveredBoundTypes(t *testing.T) []string {
	t.Helper()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatalf("rminfo.Default is %T, which answers no class-graph question", rminfo.Default)
	}
	ordered, known := h.ConcreteDescendants("DV_ORDERED")
	if !known || len(ordered) == 0 {
		t.Fatalf("rminfo.Default.ConcreteDescendants(DV_ORDERED) = %v, %v; want the classes the pinned RM declares", ordered, known)
	}
	held := map[string]bool{}
	for _, pair := range invariantMatrix[ruleIntervalLimits].pairs() {
		body := `{"_type":"DV_INTERVAL",` + pair.breaking + `}`
		v, err := typereg.Default.Decode([]byte(body))
		if err != nil {
			t.Fatalf("typereg.Decode(%s): %v", body, err)
		}
		iv, ok := v.(*rm.DVInterval[rm.DVOrdered])
		if !ok {
			t.Fatalf("typereg.Decode(%s) = %T, want *rm.DVInterval[rm.DVOrdered]", body, v)
		}
		class, _ := rm.RMTypeName(iv.Lower)
		held[class] = true
	}
	var missing []string
	for _, class := range ordered {
		if !held[class] {
			missing = append(missing, class)
		}
	}
	return missing
}
