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
// Six rules hold for every record, and [CheckRecords] enforces them. A count
// is never negative. A refusal carries no counts. A record that is not a
// refusal compares at least one key or leaf, since agreement over an empty
// set is vacuous. Missing plus altered do not exceed compared. A refusal, a
// difference, or a non-zero excluded count states its Reason. A clean record,
// one with no refusal and every count except compared at zero, carries no Reason, so a closed gap
// cannot leave a stale reason behind.
type Record struct {
	// Outcome is the expected outcome. For a refusal, Refused is a stable
	// substring of the codec's error rather than the whole message.
	Outcome Outcome
	// Reason names the cause of every refusal, difference and non-zero
	// excluded count.
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
	if o.Compared < 0 || o.Missing < 0 || o.Extra < 0 || o.Altered < 0 || o.Excluded < 0 {
		errs = append(errs, errors.New("a count is negative"))
	}
	if o.Refused != "" {
		if o.Compared != 0 || o.Missing != 0 || o.Extra != 0 || o.Altered != 0 || o.Excluded != 0 {
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
		errs = append(errs, errors.New("a refusal, difference or non-zero excluded count must state why it exists"))
	case o.Clean() && reason != "":
		errs = append(errs, errors.New("a clean record carries no reason; remove the stale one"))
	}
	return errors.Join(errs...)
}

// CheckRecords reports every way table breaks the record rules for sets: a
// set without records, a record for a set the corpus lacks, and, for each
// set that has records, every problem [CheckSetRecords] reports.
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
// corpus, with the reason for every refusal, difference and non-zero excluded
// count. A leg whose measured outcome changes fails PROBE-105 until its record
// here changes in the same commit, so a gap opens or closes only
// deliberately. The harness regenerates CENSUS.md, which sits beside it and
// publishes this table.
//
// Reasons name their causes with labels, including these:
//
//   - canxml Array<Octet>: canonical XML reads DV_MULTIMEDIA.data and
//     integrity_check as one element per byte, where ITS-XML types them
//     xs:base64Binary.
//   - party reference type: the reference writes no |type at a party and
//     hardcodes PARTY, while the codec writes |type only where a reference's
//     type is not PARTY and reads an absent |type as PARTY. So where the
//     canonical types the composer's reference PERSON, FLAT encode writes
//     composer|type, which the upstream FLAT omits, and decode of the upstream
//     FLAT gives the reference the type PARTY.
//   - HISTORY.origin: FLAT encode writes an origin that is not, as written,
//     the earliest event time as history_origin, while the reference compares
//     the two to the second, so its FLAT omits an origin a fraction of a
//     second before the earliest event time.
//   - RM-FLOOR archetype_details: decode adds archetype_details on archetype
//     roots and stamps the SDK's RM release in rm_version; upstream canonical
//     documents lack some of them, give rm_version as 1.0.4 or 1.0.2, or carry a template_id
//     below the root that decode does not add.
var Recorded = map[string]map[Leg]Record{
	"alternative_events": {
		LegCanonicalFlat: {Outcome: Outcome{Compared: 21}},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 115, Missing: 8, Extra: 12, Altered: 13},
			Reason: "Sibling order: FLAT carries no order " +
				"between the birth_en and any_event_en events; decode lists them in template order " +
				"(openehr/serialize/simplified/deviations.md), any_event_en before birth_en, the canonical lists " +
				"Birth first, so the leaves of all three events compare at shifted positions, the event types and " +
				"the interval event's math_function and width (8 leaves each way) among them. HISTORY.origin: the " +
				"FLAT has no origin key and decode gives it the earliest event time, the Birth time, as the canonical " +
				"does (openehr/serialize/simplified/deviations.md). RM-FLOOR archetype_details: the canonical OBSERVATION has none, " +
				"decode adds it (4 extra leaves), and rm_version is 1.2.0 against 1.0.4.",
		},
	},
	"consult_record": {
		LegJSONXML: {Outcome: Outcome{Compared: 150}},
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 26, Missing: 1, Extra: 6},
			Reason: "Choice element: the template lets media_file/created hold a DV_DATE_TIME or a DV_INTERVAL; upstream " +
				"spells the value created/date_time_value, the SDK's Web Template keeps one value leaf, created. " +
				"ctx/location: upstream spells EVENT_CONTEXT.location ctx/location, which the hold-out removes, and " +
				"the codec writes context/_location. ENTRY language and encoding: the upstream FLAT omits " +
				"them on document_attachment and FLAT encode writes them (4 keys).",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 150, Missing: 7, Altered: 4, Excluded: 3},
			Reason: "The 3 excluded keys are ctx/time, refused, " +
				"and ctx/location and created/date_time_value, reported as unmatched content, as " +
				"follows. UPSTREAM consult_record ctx conflict: the " +
				"FLAT gives ctx/time and context/start_time different values, decode refuses the pair and the " +
				"harness removes ctx/time; the start time left agrees with the canonical. " +
				"ctx/location: the codec does not read ctx/location (it reads context/_location), so the " +
				"location is missing. Choice element: decode does not read media_file/created/date_time_value " +
				"(the SDK spells it created), so the media_file cluster lacks its created element (6 leaves). " +
				"RM-FLOOR archetype_details: rm_version is 1.2.0 against 1.0.4 (4 leaves).",
		},
		LegFlatStructured: {
			Outcome: Outcome{Compared: 40, Missing: 12, Extra: 5},
			Reason: "Upstream metadata spelling: the upstream FLAT writes language, territory, setting and location " +
				"as ctx/ short forms and adds a ctx/time, where the upstream STRUCTURED writes the real paths, and " +
				"FlatToStructured, which uses no template, keeps each key as spelled. The upstream FLAT also omits " +
				"the ENTRY language and encoding of document_attachment that the upstream STRUCTURED carries.",
		},
		LegStructuredFlat: {
			Outcome: Outcome{Compared: 29, Extra: 1, Excluded: 1},
			Reason: "ctx/location: the upstream FLAT spells the location ctx/location, which the hold-out removes, " +
				"while the upstream STRUCTURED spells it context/_location, which decodes and re-encodes, " +
				"so only our side carries it. Excluded: one family drops out of both sides of the comparison: " +
				"decode reports media_file/created/date_time_value (choice element) as unmatched content.",
		},
	},
	"corona": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 104, Altered: 1},
			Reason:  "Upstream number spelling: the upstream FLAT writes the body temperature's magnitude as 39.0, where the codec writes 39.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 641, Altered: 15},
			Reason:  "RM-FLOOR archetype_details: rm_version is 1.2.0 against 1.0.4 (15 leaves).",
		},
		LegFlatStructured: {Outcome: Outcome{Compared: 113}},
		LegStructuredFlat: {Outcome: Outcome{Compared: 104}},
	},
	"ehrn_abdm": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 31, Missing: 1, Extra: 1},
			Reason: "Choice element: the template lets media_file/created hold a DV_DATE_TIME or a DV_INTERVAL; upstream " +
				"spells the value created/date_time_value, the SDK's Web Template keeps one value leaf, created.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 150, Missing: 6, Altered: 4, Excluded: 1},
			Reason: "The excluded key is created/date_time_value, reported as unmatched content, as follows. " +
				"Choice element: decode does not read media_file/created/date_time_value (the SDK " +
				"spells it created), so the media_file cluster lacks its created element (6 leaves). RM-FLOOR " +
				"archetype_details: rm_version is 1.2.0 against 1.0.4 (4 leaves).",
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
			Outcome: Outcome{Compared: 45, Extra: 1, Altered: 5},
			Reason: "Upstream date-time spelling: the canonical writes the four event times and the context end " +
				"time with a comma before the fraction, the upstream FLAT with a full stop, and the codecs carry " +
				"the text as written. HISTORY.origin: the first OBSERVATION's origin is 2 ms before its events, so " +
				"FLAT encode writes it as history_origin, which the upstream FLAT omits.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 249, Extra: 8, Altered: 9},
			Reason: "RM-FLOOR archetype_details: the two canonical OBSERVATIONs have none, decode adds them (8 " +
				"extra leaves), and rm_version is 1.2.0 against 1.0.4. Upstream date-time spelling: the FLAT writes " +
				"the four event times and the context start and end times with a full stop before the fraction, " +
				"the canonical with a comma. HISTORY.origin: the FLAT has no origin key and decode gives each origin " +
				"the earliest time its events carry (openehr/serialize/simplified/deviations.md): the first " +
				"OBSERVATION's canonical origin is earlier than its events, and the second's differs only in the " +
				"comma before the fraction.",
		},
	},
	"nested": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 19, Missing: 3, Extra: 9},
			Reason: "party reference type: the canonical types the composer's reference PERSON, so FLAT encode " +
				"writes composer|type, which the upstream FLAT omits. DV_ORDINAL terminology: the ordinal's symbol is coded in " +
				"com.cabolabs.openehr.opt, which the |code form (local implied) cannot carry, so FLAT encode writes " +
				"ordinal|raw where upstream writes |code, |value and |ordinal and loses the terminology. Upstream " +
				"FLAT omissions: the upstream FLAT carries neither the context participation (6 keys) nor the " +
				"ACTIVITY action_archetype_id that the canonical XML holds, and FLAT encode writes both.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 156, Missing: 34, Extra: 4, Altered: 8},
			Reason: "party reference type: the upstream FLAT writes no composer|type, so decode gives the " +
				"composer's reference the type PARTY against the canonical's PERSON. Upstream FLAT " +
				"omissions: the FLAT carries no context (start time, setting, participation) and no ACTIVITY action_archetype_id, so the decoded " +
				"composition has no context, and the ACTIVITY takes the RM default action_archetype_id /.*/, since " +
				"nested.en.v1.opt does not constrain it. DV_ORDINAL terminology: the " +
				"ordinal decodes in the local terminology its |code form implies, against the canonical's " +
				"com.cabolabs.openehr.opt. RM-FLOOR archetype_details: the " +
				"canonical SECTION has none and decode adds it (4 extra leaves), the canonical carries template_id " +
				"on inner archetype roots where decode adds it on the root only, and rm_version is 1.2.0 against " +
				"1.0.2.",
		},
	},
	"persistent_minimal": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 11, Extra: 2, Altered: 1},
			Reason: "party reference type: the canonical types the composer's reference PERSON, so FLAT encode " +
				"writes composer|type, which the upstream FLAT omits. Upstream date-time spelling: the event time is " +
				"21:11:36.700 in the canonical XML and 21:11:36.7 in the upstream FLAT, and the codecs carry the " +
				"text as written. HISTORY.origin: the origin is 48 ms before the event time, so FLAT encode writes " +
				"it as history_origin, which the upstream FLAT omits.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 72, Missing: 2, Altered: 5},
			Reason: "party reference type: the upstream FLAT writes no composer|type, so decode gives the " +
				"composer's reference the type PARTY against the canonical's PERSON. Upstream date-time " +
				"spelling: the event time is 21:11:36.7 in the FLAT and 21:11:36.700 in the canonical XML. HISTORY.origin: the " +
				"FLAT has no origin key and decode gives it the event time (openehr/serialize/simplified/deviations.md), " +
				"where the canonical sets it earlier. RM-FLOOR archetype_details: the canonical carries template_id on the " +
				"inner archetype root where decode adds it on the root only, and rm_version is 1.2.0 against 1.0.2.",
		},
	},
	"test_all_types": {
		LegCanonicalFlat: {
			Outcome: Outcome{Compared: 83, Missing: 4, Extra: 7, Altered: 11},
			Reason: "party reference type: the canonical types the composer's reference PERSON, so FLAT encode " +
				"writes composer|type, which the upstream FLAT omits. HISTORY.origin: the origin is 6 ms before " +
				"the event time, so FLAT encode writes it as history_origin, which the upstream FLAT omits. " +
				"Choice element: upstream spells the value of the choice " +
				"element choice/quantity_value, the SDK choice. DV_IDENTIFIER: this upstream FLAT writes the id as " +
				"the bare value, the codec as |id. DV_PROPORTION: upstream writes the derived magnitude as a bare " +
				"value, which the codec does not write (a PROBE-086 residue), and writes the Integer type and the " +
				"Real denominator as 1.0 where the codec writes 1. Upstream date and time spelling: the canonical " +
				"writes dates in the basic format (20190114) and date-times with a comma and +00:00, the FLAT in the " +
				"extended format with a full stop and Z (8 leaves). Upstream conversion differences: upstream " +
				"openEHR_SDK's FlatJsonMarshallerTest.toFlatJsonAllTypesErrors encodes this canonical " +
				"(all_types_no_multimedia.json) as FLAT and expects these differences against this FLAT " +
				"(test_all_types.json): duration_any (P1Y2M10DT2H30M against PT30M), a DV_URI only the canonical " +
				"carries and the proportion's precision, which only the canonical carries, besides the DV_IDENTIFIER " +
				"and proportion type spellings above.",
		},
		LegFlatCanonical: {
			Outcome: Outcome{Compared: 423, Missing: 40, Extra: 40, Altered: 32, Excluded: 3},
			Reason: "The 3 excluded keys are " +
				"choice/quantity_value|magnitude and |unit (2) and the bare proportion_any (1), reported as " +
				"unmatched content, as follows. ISM transition careflow step: the upstream FLAT gives the " +
				"section_3 ACTION's transition a current state (526, planned) and no careflow step, so decode " +
				"gives it the one careflow step of the template whose current states hold 526 " +
				"(openehr/serialize/simplified/deviations.md), where the canonical has none (6 extra leaves); " +
				"this is the intended reading of such a body, not a gap. " +
				"party reference type: the upstream FLAT writes no composer|type, so decode gives the composer's " +
				"reference the type PARTY against the canonical's PERSON. " +
				"Choice element: decode does not read choice/quantity_value (the SDK spells it choice), " +
				"so the choice element is missing. DV_PROPORTION: decode reports the bare proportion_any, the " +
				"derived magnitude, as unmatched content, and reads |type 1.0 as the Integer 1. " +
				"Upstream conversion differences: a DV_URI and the proportion's " +
				"precision only the canonical carries, and duration_any (P1Y2M10DT2H30M against PT30M). Upstream " +
				"date and time spelling: the canonical writes dates in the basic format (20190114) and date-times " +
				"with a comma and +00:00, the FLAT in the extended format with a full stop and Z (7 leaves). " +
				"HISTORY.origin: the FLAT has no origin key and decode gives it the event time " +
				"(openehr/serialize/simplified/deviations.md), where the canonical sets it earlier. Shifted " +
				"positions: the elements decode lacks move their later siblings up, the DV_URI and the choice " +
				"element in the EVALUATION, so the EVALUATION's intervals and cluster compare against the " +
				"canonical's next element; this accounts for most of the missing, extra and altered leaves. RM-FLOOR " +
				"archetype_details: the canonical SECTION and the " +
				"ACTION's description CLUSTER have none, decode adds them (8 extra leaves), and rm_version is " +
				"1.2.0 against 1.0.2 and 1.0.4.",
		},
	},
}
