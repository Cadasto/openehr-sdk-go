package crossformat

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Record is the outcome expected for one set and leg, and why it is what it
// is.
//
// Four rules hold for every record, and [CheckRecords] enforces them: a
// refusal or a difference states its Reason; a clean record carries none, so
// a closed gap cannot leave a stale reason behind; a record that is not a
// refusal compares at least one key or leaf; and a refusal carries no counts.
type Record struct {
	// Outcome is the expected outcome. For a refusal, Refused is a stable
	// substring of the codec's error rather than the whole message.
	Outcome Outcome
	// Reason names the cause of every refusal and difference.
	Reason string
}

// Matches reports whether got is the outcome r records: a refusal whose
// error contains the recorded substring, or exactly the recorded counts. Both
// errors are read through [StableError] first.
func (r Record) Matches(got Outcome) bool {
	if r.Outcome.Refused != "" {
		return got.Refused != "" && strings.Contains(StableError(got.Refused), StableError(r.Outcome.Refused))
	}
	return got == r.Outcome
}

// StableError rewrites the one part of a codec error that changes from run to
// run: encoding/json/v2 words a failure "cannot …" or "unable to …", picking
// one at random once per process, so StableError always says "cannot".
func StableError(msg string) string {
	return strings.ReplaceAll(msg, "unable to ", "cannot ")
}

// check reports how r breaks the record rules.
func (r Record) check() error {
	o := r.Outcome
	var errs []error
	if o.Compared < 0 || o.Missing < 0 || o.Extra < 0 || o.Altered < 0 {
		errs = append(errs, errors.New("a count is negative"))
	}
	if o.Refused != "" {
		if o.Compared != 0 || o.Missing != 0 || o.Extra != 0 || o.Altered != 0 {
			errs = append(errs, errors.New("a refusal carries no counts"))
		}
	} else {
		if o.Compared < 1 {
			errs = append(errs, errors.New("a leg that is not refused must compare at least one key or leaf, "+
				"since agreement over an empty set is vacuous"))
		}
		if o.Missing+o.Altered > o.Compared {
			errs = append(errs, fmt.Errorf("missing %d plus altered %d exceed the %d compared", o.Missing, o.Altered, o.Compared))
		}
	}
	switch reason := strings.TrimSpace(r.Reason); {
	case !o.Clean() && reason == "":
		errs = append(errs, errors.New("a refusal or difference must state why it exists"))
	case o.Clean() && reason != "":
		errs = append(errs, errors.New("a clean record carries no reason; remove the stale one"))
	}
	return errors.Join(errs...)
}

// CheckRecords reports every way table breaks the record rules for sets: a
// set without records or a record for a set the corpus lacks, a leg the set
// runs without a record or a record for a leg it cannot run, and any record
// that breaks the rules of [Record].
func CheckRecords(sets []fixtures.CrossFormatSet, table map[string]map[Leg]Record) error {
	var errs []error
	known := make(map[string]bool, len(sets))
	for _, set := range sets {
		known[set.Name] = true
		recs, ok := table[set.Name]
		if !ok {
			errs = append(errs, fmt.Errorf("set %s: the corpus carries it but it has no records", set.Name))
			continue
		}
		errs = append(errs, CheckSetRecords(set, recs))
	}
	for _, name := range slices.Sorted(maps.Keys(table)) {
		if !known[name] {
			errs = append(errs, fmt.Errorf("set %s: recorded, but the corpus has no such set", name))
		}
	}
	return errors.Join(errs...)
}

// CheckSetRecords reports every way recs breaks the record rules for set: a
// leg the set runs without a record, a record for a leg the set cannot run,
// and any record that breaks the rules of [Record].
func CheckSetRecords(set fixtures.CrossFormatSet, recs map[Leg]Record) error {
	var errs []error
	legs := Legs(set)
	for _, leg := range legs {
		if _, ok := recs[leg]; !ok {
			errs = append(errs, fmt.Errorf("set %s, leg %s: the set runs this leg but it has no record", set.Name, leg))
		}
	}
	for _, leg := range slices.Sorted(maps.Keys(recs)) {
		if !slices.Contains(legs, leg) {
			errs = append(errs, fmt.Errorf("set %s, leg %s: recorded, but the set does not carry both of its formats", set.Name, leg))
			continue
		}
		if err := recs[leg].check(); err != nil {
			errs = append(errs, fmt.Errorf("set %s, leg %s: %w", set.Name, leg, err))
		}
	}
	return errors.Join(errs...)
}

// Verify compares every leg of res with its record and returns one line per
// disagreement, naming the set, the leg, the recorded outcome and the
// measured one. It returns nil when every leg matches its record and every
// record matches a leg.
func Verify(res SetResult, recs map[Leg]Record) []string {
	var out []string
	ran := make(map[Leg]bool, len(res.Legs))
	for _, lr := range res.Legs {
		ran[lr.Leg] = true
		rec, ok := recs[lr.Leg]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s %s: no record, measured %s", res.Set, lr.Leg, Abbreviate(lr.Outcome.String())))
		case !rec.Matches(lr.Outcome):
			out = append(out, fmt.Sprintf("%s %s: recorded %s, measured %s",
				res.Set, lr.Leg, rec.Outcome, Abbreviate(lr.Outcome.String())))
		}
	}
	for _, leg := range slices.Sorted(maps.Keys(recs)) {
		if !ran[leg] {
			out = append(out, fmt.Sprintf("%s %s: recorded %s, but the leg did not run", res.Set, leg, recs[leg].Outcome))
		}
	}
	return out
}

// Recorded is the outcome expected for every set and leg of the cross-format
// corpus, with the reason for every refusal and difference. A leg whose
// measured outcome changes fails PROBE-105 until its record here changes in
// the same commit, so a gap opens or closes only deliberately; CENSUS.md
// publishes this table beside the harness that regenerates it.
//
// Reasons name their causes with these labels:
//
//   - GAP-A canxml Array<Octet>: canonical XML reads DV_MULTIMEDIA.data and
//     integrity_check as one element per byte, where ITS-XML types them
//     xs:base64Binary.
//   - GAP-B body-form composer: FLAT decode refuses the composer's real-path
//     keys (composer|name and the external_ref composer|id, |id_scheme,
//     |id_namespace) as an unsupported PARTY_PROXY, and FLAT encode writes the
//     composer as ctx/composer_name alone, without its external_ref.
//   - GAP-C INTERVAL_EVENT attributes: the Web Template has no math_function
//     or width node for the event, so FLAT encode writes neither and decode
//     refuses both.
//   - GAP-D reused archetype siblings: several siblings reuse one archetype
//     under one attribute, which FLAT encode refuses ("path resolves to
//     multiple items") and FLAT decode refuses ("not yet decodable").
//   - RM-FLOOR archetype_details: decode adds archetype_details on archetype
//     roots and stamps the SDK's RM release in rm_version; upstream canonical
//     documents lack some of them, say 1.0.4 or 1.0.2, or carry a template_id
//     below the root that decode does not add.
var Recorded = map[string]map[Leg]Record{
	"alternative_events": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 21, Missing: 4},
			Reason: "GAP-C INTERVAL_EVENT attributes: the interval event any_event_en:1 has no Web Template node for " +
				"math_function or width, so FLAT encode writes none of their four keys.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 115, Missing: 8, Extra: 4, Altered: 13},
			Reason: "GAP-C INTERVAL_EVENT attributes: decode refuses math_function and width (4 keys), so the interval " +
				"event loses both and decodes as a POINT_EVENT. Sibling order: FLAT carries no order between the " +
				"birth_en and any_event_en events; decode lists them in Web Template order, the canonical lists " +
				"Birth first, so the leaves of all three events compare at shifted positions. HISTORY.origin: the " +
				"FLAT has no origin key and decode fills it from the context start time (deviations.md), where the " +
				"canonical sets it to the Birth time. RM-FLOOR archetype_details: the canonical OBSERVATION has none, " +
				"decode adds it (4 extra leaves), and rm_version is 1.2.0 against 1.0.4.",
		},
	},
	"consult_record": {
		LegJSONXML: {
			Outcome: Outcome{Refused: `(_type="DV_MULTIMEDIA"): canxml: strconv.ParseUint`},
			Reason: "GAP-A canxml Array<Octet>: the canonical XML carries the media file's DV_MULTIMEDIA data as " +
				"base64 text (ITS-XML xs:base64Binary), and canxml reads that attribute as one element per byte, " +
				"so it fails parsing the base64 text as a byte.",
		},
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 26, Missing: 5, Extra: 6},
			Reason: "GAP-B body-form composer: FLAT encode writes the composer as ctx/composer_name alone, so " +
				"composer|id, |id_scheme and |id_namespace are missing. LOCATABLE name: the upstream FLAT writes the " +
				"composition's own name as _name, which the codec does not carry (deviations.md, LOCATABLE.name). " +
				"Choice element: the template lets media_file/created hold a DV_DATE_TIME or a DV_INTERVAL; upstream " +
				"spells the value created/date_time_value, the SDK's Web Template keeps one value leaf, created. " +
				"ctx/location: upstream spells EVENT_CONTEXT.location ctx/location, which the hold-out removes, and " +
				"the codec writes context/_location (ADR 0016). ENTRY language and encoding: the upstream FLAT omits " +
				"them on document_attachment and FLAT encode writes them (4 keys).",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 150, Missing: 41, Extra: 27, Altered: 17},
			Reason: "GAP-B body-form composer: decode refuses the composer keys (4), so the composer's name and " +
				"external_ref are missing and decode fills a PARTY_SELF. UPSTREAM consult_record ctx conflict: the " +
				"FLAT gives ctx/time and context/start_time different values, decode refuses the pair and the " +
				"harness removes ctx/time; the start time left agrees with the canonical. ctx/setting: the FLAT " +
				"writes ctx/setting as bare text, which the codec refuses (it takes ctx/setting|code and |value), so " +
				"decode fills the default 238 other care against the canonical 228 primary medical care. " +
				"ctx/location: the codec refuses ctx/location (it reads context/_location, ADR 0016), so the " +
				"location is missing. LOCATABLE name: the codec refuses _name, so decode names the composition " +
				"OPConsultation from the template against the canonical Routine checkup. Choice element: decode " +
				"refuses media_file/created/date_time_value (the SDK spells it created). Sibling order: FLAT " +
				"carries no order between the document_attachment items; decode lists the media_file cluster first, " +
				"the canonical lists it last, so the item leaves compare at shifted positions. RM-FLOOR " +
				"archetype_details: rm_version is 1.2.0 against 1.0.4 (3 leaves).",
		},
		LegFlatStructured: {
			Outcome: Outcome{Compared: 40, Missing: 12, Extra: 5},
			Reason: "Upstream metadata spelling: the upstream FLAT writes language, territory, setting and location " +
				"as ctx/ short forms and adds a ctx/time, where the upstream STRUCTURED writes the real paths, and " +
				"FlatToStructured, which uses no template, keeps each key as spelled. The upstream FLAT also omits " +
				"the ENTRY language and encoding of document_attachment that the upstream STRUCTURED carries.",
		},
		LegStructuredFlat: {
			Outcome: Outcome{Compared: 25, Extra: 1},
			Reason: "ctx/location: the upstream FLAT spells the location ctx/location, which the hold-out removes, " +
				"while the upstream STRUCTURED spells it context/_location (ADR 0016), which decodes and re-encodes, " +
				"so only our side carries it.",
		},
	},
	"corona": {
		LegCanonicalFlat: {
			Outcome: Outcome{Refused: `path resolves to multiple items: "/content[openEHR-EHR-SECTION.adhoc.v1]"`},
			Reason: "GAP-D reused archetype siblings: the symptome and risikogebiet sections both reuse " +
				"openEHR-EHR-SECTION.adhoc.v1 under content, and FLAT encode refuses the path that resolves to both.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 641, Missing: 565, Altered: 2},
			Reason: "GAP-D reused archetype siblings: decode refuses every key under the symptome and risikogebiet " +
				"sections (94 keys), so both sections are missing from the decoded composition. RM-FLOOR " +
				"archetype_details: rm_version is 1.2.0 against 1.0.4 (2 leaves).",
		},
		LegFlatStructured: {Outcome: Outcome{Compared: 113}},
		LegStructuredFlat: {Outcome: Outcome{Compared: 10}},
	},
	"ehrn_abdm": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 31, Missing: 5, Extra: 1},
			Reason: "GAP-B body-form composer: FLAT encode writes the composer as ctx/composer_name alone, so " +
				"composer|id, |id_scheme and |id_namespace are missing. LOCATABLE name: the upstream FLAT writes the " +
				"composition's own name as _name, which the codec does not carry (deviations.md, LOCATABLE.name). " +
				"Choice element: the template lets media_file/created hold a DV_DATE_TIME or a DV_INTERVAL; upstream " +
				"spells the value created/date_time_value, the SDK's Web Template keeps one value leaf, created.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 150, Missing: 40, Extra: 27, Altered: 15},
			Reason: "GAP-B body-form composer: decode refuses the composer keys (4), so the composer's name and " +
				"external_ref are missing and decode fills a PARTY_SELF. LOCATABLE name: the codec refuses _name, " +
				"so decode names the composition OPConsultation from the template against the canonical Routine " +
				"checkup. Choice element: decode refuses media_file/created/date_time_value (the SDK spells it " +
				"created). Sibling order: FLAT carries no order between the document_attachment items; decode lists " +
				"the media_file cluster first, the canonical lists it last, so the item leaves compare at shifted " +
				"positions. RM-FLOOR archetype_details: rm_version is 1.2.0 against 1.0.4 (3 leaves).",
		},
	},
	"family_history": {
		LegJSONXML: {Outcome: Outcome{Compared: 95}},
	},
	"multi_list": {
		LegFlatStructured: {Outcome: Outcome{Compared: 50}},
		LegStructuredFlat: {Outcome: Outcome{Compared: 41}},
	},
	"multi_occurrence": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 45, Altered: 5},
			Reason: "Upstream date-time spelling: the canonical writes the four event times and the context end " +
				"time with a comma before the fraction, the upstream FLAT with a full stop, and the codecs carry " +
				"the text as written.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 249, Extra: 8, Altered: 9},
			Reason: "RM-FLOOR archetype_details: the two canonical OBSERVATIONs have none, decode adds them (8 " +
				"extra leaves), and rm_version is 1.2.0 against 1.0.4. Upstream date-time spelling: the FLAT writes " +
				"the four event times and the context start and end times with a full stop before the fraction, " +
				"the canonical with a comma. HISTORY.origin: the FLAT has no origin key and decode fills both " +
				"origins from the context start time (deviations.md), where the canonical sets them otherwise.",
		},
	},
	"nested": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 19, Missing: 5, Extra: 8},
			Reason: "GAP-B body-form composer: FLAT encode writes the composer as ctx/composer_name alone, so " +
				"composer|id and |id_namespace are missing. DV_ORDINAL terminology: the ordinal's symbol is coded in " +
				"com.cabolabs.openehr.opt, which the |code form (local implied) cannot carry, so FLAT encode writes " +
				"ordinal|raw where upstream writes |code, |value and |ordinal and loses the terminology. Upstream " +
				"FLAT omissions: the upstream FLAT carries neither the context participation (6 keys) nor the " +
				"ACTIVITY action_archetype_id that the canonical XML holds, and FLAT encode writes both.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 156, Missing: 84, Extra: 48, Altered: 11},
			Reason: "GAP-B body-form composer: decode refuses the composer keys (3), so the composer's name and " +
				"external_ref are missing and decode fills a PARTY_SELF. Upstream FLAT omissions: the FLAT carries " +
				"no context (start time, setting, participation) and no ACTIVITY action_archetype_id, so the decoded " +
				"composition has no context and an empty action_archetype_id. Sibling order: FLAT carries no order " +
				"between the ordinal ELEMENT and the nested CLUSTER of the activity description; decode lists the " +
				"CLUSTER first, the canonical lists it second, so their leaves compare at shifted positions, and the " +
				"ordinal decodes in the local terminology its |code form implies. RM-FLOOR archetype_details: the " +
				"canonical SECTION has none and decode adds it (4 extra leaves), the canonical carries template_id " +
				"on inner archetype roots where decode adds it on the root only, and rm_version is 1.2.0 against " +
				"1.0.2.",
		},
	},
	"persistent_minimal": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 11, Missing: 2, Altered: 1},
			Reason: "GAP-B body-form composer: FLAT encode writes the composer as ctx/composer_name alone, so " +
				"composer|id and |id_namespace are missing. Upstream date-time spelling: the event time is " +
				"21:11:36.700 in the canonical XML and 21:11:36.7 in the upstream FLAT, and the codecs carry the " +
				"text as written.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 72, Missing: 8, Altered: 5},
			Reason: "GAP-B body-form composer: decode refuses the composer keys (3), so the composer's name and " +
				"external_ref are missing and decode fills a PARTY_SELF. Upstream date-time spelling: the event " +
				"time is 21:11:36.7 in the FLAT and 21:11:36.700 in the canonical XML. HISTORY.origin: a persistent " +
				"composition has no context start time, and decode, which fills origin from it (deviations.md), " +
				"leaves the origin empty. RM-FLOOR archetype_details: the canonical carries template_id on the " +
				"inner archetype root where decode adds it on the root only, and rm_version is 1.2.0 against 1.0.2.",
		},
	},
	"test_all_types": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 83, Missing: 11, Extra: 11, Altered: 11},
			Reason: "GAP-B body-form composer: FLAT encode writes the composer as ctx/composer_name alone (3 missing " +
				"keys). Web Template projection: the SDK's Web Template has no in-context time or ism_transition " +
				"node for the section_3 ACTION (webtemplate deviations.md, inContext coverage), and its " +
				"per-careflow-step transition nodes do not match a transition without a careflow_step, so FLAT " +
				"encode writes neither (4 missing keys). Choice element: upstream spells the value of the choice " +
				"element choice/quantity_value, the SDK choice. DV_IDENTIFIER: this upstream FLAT writes the id as " +
				"the bare value, the codec as |id. DV_PROPORTION: upstream writes the derived magnitude as a bare " +
				"value, which the codec does not write (a PROBE-086 residue), and writes the Integer type and the " +
				"Real denominator as 1.0 where the codec writes 1. DV_INTERVAL bounds: the canonical omits the " +
				"RM-mandatory lower_included and upper_included, canjson reads the absence as false, and FLAT encode " +
				"writes false where upstream writes nothing (6 keys). Upstream date and time spelling: the canonical " +
				"writes dates in the basic format (20190114) and date-times with a comma and +00:00, the FLAT in the " +
				"extended format with a full stop and Z (8 leaves). Upstream value disagreement: the canonical " +
				"(all_types_no_multimedia.json) and the FLAT (test_all_types.json) are separate upstream files that " +
				"disagree on duration_any (P1Y2M10DT2H30M against PT30M), on a DV_URI only the canonical carries, " +
				"and on the proportion's precision, which only the canonical carries.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Refused: "unmarshal JSON number 1.0 into Go int32"},
			Reason: "DV_PROPORTION type: the upstream FLAT writes proportion_any|type as 1.0, decode passes it to " +
				"canjson, and canjson refuses 1.0 for the Integer type with an error that names no FLAT key, so the " +
				"reducing decode cannot remove the key and the leg ends.",
		},
	},
}
