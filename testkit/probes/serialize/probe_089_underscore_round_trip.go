package serializeprobes

// PROBE-089 — underscore-attribute round-trip (REQ-140; exercises REQ-053).
// For each SDK-authored per-family FLAT fixture below — one per row of the
// REQ-140 grammar table, recursion and refusals included — four legs:
//
//   - (a) **decode** — the body decodes into the typed RM attribute and
//     re-encodes byte-for-byte, over the *whole* body, so a family cannot be
//     carried at the cost of a key beside it;
//   - (b) **encode** — the decoded composition is taken through canonical JSON
//     and back, and encoding *that* composition emits exactly the fixture's
//     family key set: no silent drop, no invented key, and no `|raw` at a base
//     the grammar carries. The canonical transit is what makes this an encode
//     assertion rather than a second reading of leg (a) — the attribute has to
//     survive as canonical RM, so an underscore value parked anywhere the
//     canonical form does not model would vanish here;
//   - (c) **refusals** — [Probe089RefusedFamilies], below;
//   - (d) **STRUCTURED** — FLAT → STRUCTURED → FLAT preserves the underscore
//     vocabulary (`_`-keys as array-valued members) and the OPT-driven
//     STRUCTURED round-trip returns the same FLAT.
//
// The distinct assertion versus [Probe086UpstreamFlatParity] is the *fixture*:
// PROBE-086 measures how much of one upstream corpus this codec carries, and a
// key it refuses is counted rather than failed. Here every key MUST round-trip,
// the fixtures reach shapes the corpus does not write (a nested `_identifier:N`
// under a FEEDER_AUDIT_DETAILS `provider`), and the deliberate refusals are
// asserted as refusals rather than tallied.
//
// The distinct assertion versus the package tests in
// openehr/serialize/simplified/rmattr*_test.go is the leg set: those pin one
// family's shape and its typed RM result from inside the package; this probe
// asserts the whole-body byte-exactness, the canonical-transit encode, and the
// STRUCTURED vocabulary from outside it, over one fixture per grammar row. A
// family deleted from the codec fails here whether or not its package test
// survives.
//
// Modes: In-repo (Sandbox) only — no backend, no cassette/live mode.

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	conformance "github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
)

// FLAT path fragments of the vendored PROBE-086 corpus template — the one OPT
// every fixture here instantiates, so each path and suffix spelling is the
// reference implementation's (ADR 0014).
const (
	probe089Root     = "conformance-ehrbase.de.v0"
	probe089Context  = probe089Root + "/context"
	probe089Section  = probe089Root + "/conformance_section"
	probe089Obs      = probe089Section + "/conformance_observation"
	probe089Action   = probe089Section + "/conformance_action"
	probe089Instr    = probe089Section + "/conformance_instruction"
	probe089Event    = probe089Obs + "/any_event:0"
	probe089Qty      = probe089Event + "/dv_quantity"
	probe089Count    = probe089Event + "/dv_count"
	probe089Ordinal  = probe089Event + "/dv_ordinal"
	probe089DateTime = probe089Event + "/dv_date_time"
	probe089Text     = probe089Event + "/dv_text"
	probe089MM       = probe089Event + "/dv_multimedia"
	probe089Parsable = probe089Event + "/dv_parsable"
	// The interval leaves sit under their own OBSERVATION in this template.
	probe089IntervalEvent = probe089Section + "/conformance_interval/any_event:0"
	probe089Interval      = probe089IntervalEvent + "/interval_dv_quantity"
)

// Probe089Case is one SDK-authored per-family FLAT fixture.
type Probe089Case struct {
	// Name identifies the case in reports and sub-tests.
	Name string
	// Rows names the REQ-140 grammar-table rows this fixture covers, so a
	// reader can check the table against the fixture set.
	Rows []string
	// Keys is the fixture's own key set — the assertion surface. It is merged
	// with the shared scaffolding (see [Probe089Case.Body]) to make one
	// complete, decodable FLAT body, and every key in it MUST survive all four
	// legs.
	Keys map[string]any
}

// Body is the complete FLAT payload: the minimal decodable scaffolding every
// fixture shares — the mandatory `ctx/` context plus the OBSERVATION's own
// in-context leaves — merged with the fixture's own keys. The scaffolding
// carries no clinical leaf value deliberately: `_null_flavour` has to stand
// beside an **absent** one.
func (c Probe089Case) Body() map[string]any {
	body := map[string]any{
		"ctx/language":  "en",
		"ctx/territory": "US",
		"ctx/time":      "2021-12-21T14:19:31.649613+01:00",
		// `category` is a template-constrained leaf on its own FLAT path, not
		// `ctx/` metadata, and COMPOSITION.category is RM-mandatory and
		// value-typed: leaving it out would have the encoder emit it empty (see
		// simplified/deviations.md § zero-valued mandatory in-context
		// attributes), which is a scaffolding artefact, not a family under test.
		probe089Root + "/category|code":        "433",
		probe089Root + "/category|value":       "event",
		probe089Root + "/category|terminology": "openehr",
		probe089Obs + "/language|code":         "en",
		probe089Obs + "/language|terminology":  "ISO_639-1",
		probe089Event + "/time":                "2021-12-21T16:02:58.0094262+01:00",
	}
	maps.Copy(body, c.Keys)
	return body
}

// Probe089Inputs is one fixture per row of the REQ-140 grammar table, including
// the recursive shapes (`_feeder_audit/…/provider/_identifier:N`,
// `dv_multimedia/_thumbnail`) and `_null_flavour` beside an absent bare value.
// Values are copied from the pinned corpus bodies wherever the corpus writes
// the shape, so the vocabulary under test is the reference's.
var Probe089Inputs = []Probe089Case{
	{
		Name: "locatable_uid_and_link",
		Rows: []string{"any LOCATABLE: _uid", "any LOCATABLE: _link:N"},
		Keys: map[string]any{
			// The concrete UID_BASED_ID subtype is re-derived from the lexical
			// form, so the root's three-part id and the bare UUIDs below must
			// come back as the subtypes they arrived as.
			probe089Root + "/_uid":            "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1",
			probe089Section + "/_uid":         "9fcc1c70-9349-444d-b9cb-8fa817697f5e",
			probe089Obs + "/_uid":             "347a5490-55ee-4da9-b91a-9bba710f730e",
			probe089Qty + "/_uid":             "a4a17f52-e2a1-4b41-a0f2-1a0d3ad0f5a1",
			probe089Root + "/_link:0|meaning": "problem related note",
			probe089Root + "/_link:0|type":    "problem",
			probe089Root + "/_link:0|target":  "ehr://ehr.network/347a5490-55ee-4da9-b91a-9bba710f730e",
			// The Web Template folds ELEMENT.value into its leaf node, so these
			// two belong to the ELEMENT one attribute up.
			probe089Qty + "/_link:0|meaning": "problem related note",
			probe089Qty + "/_link:0|type":    "problem",
			probe089Qty + "/_link:0|target":  "ehr://ehr.network/347a5490-55ee-4da9-b91a-9bba710f730e",
			probe089Qty + "|magnitude":       65.9,
			probe089Qty + "|unit":            "unit",
		},
	},
	{
		Name: "entry_object_refs",
		Rows: []string{"ENTRY: _work_flow_id", "ENTRY: _guideline_id"},
		Keys: map[string]any{
			probe089Obs + "/_work_flow_id|id":        "335645",
			probe089Obs + "/_work_flow_id|id_scheme": "HOSPITAL-NS",
			probe089Obs + "/_work_flow_id|namespace": "HOSPITAL-NS",
			probe089Obs + "/_work_flow_id|type":      "WORKFLOW",
			probe089Obs + "/_guideline_id|id":        "3445",
			probe089Obs + "/_guideline_id|id_scheme": "HOSPITAL-NS",
			probe089Obs + "/_guideline_id|namespace": "HOSPITAL-NS",
			probe089Obs + "/_guideline_id|type":      "GUIDELINE",
			probe089Qty + "|magnitude":               65.9,
			probe089Qty + "|unit":                    "unit",
		},
	},
	{
		Name: "event_context_optionals",
		Rows: []string{"EVENT_CONTEXT: _end_time", "EVENT_CONTEXT: _location"},
		Keys: map[string]any{
			// ADR 0016: the EVENT_CONTEXT optionals ride this grammar at their
			// real paths rather than gaining `ctx/` short forms.
			probe089Context + "/_end_time": "2021-12-21T15:19:31.649613+01:00",
			probe089Context + "/_location": "microbiology lab 2",
			probe089Qty + "|magnitude":     65.9,
			probe089Qty + "|unit":          "unit",
		},
	},
	{
		Name: "value_decorations_quantity",
		Rows: []string{"DV_ORDERED leaf: _normal_range", "DV_ORDERED leaf: _other_reference_ranges:N"},
		Keys: map[string]any{
			probe089Qty + "|magnitude":                                     65.9,
			probe089Qty + "|unit":                                          "unit",
			probe089Qty + "/_normal_range/lower|magnitude":                 20.5,
			probe089Qty + "/_normal_range/lower|unit":                      "unit",
			probe089Qty + "/_normal_range/upper|magnitude":                 66.6,
			probe089Qty + "/_normal_range/upper|unit":                      "unit",
			probe089Qty + "/_normal_range|lower_included":                  false,
			probe089Qty + "/_normal_range|upper_included":                  false,
			probe089Qty + "/_other_reference_ranges:0/lower|magnitude":     70.5,
			probe089Qty + "/_other_reference_ranges:0/lower|unit":          "unit",
			probe089Qty + "/_other_reference_ranges:0/upper|magnitude":     77.6,
			probe089Qty + "/_other_reference_ranges:0/upper|unit":          "unit",
			probe089Qty + "/_other_reference_ranges:0/meaning|code":        "260360000",
			probe089Qty + "/_other_reference_ranges:0/meaning|value":       "very high",
			probe089Qty + "/_other_reference_ranges:0|lower_included":      false,
			probe089Qty + "/_other_reference_ranges:0|upper_included":      false,
			probe089Qty + "/_other_reference_ranges:0/meaning|terminology": "SNOMED-CT",
		},
	},
	{
		Name: "value_decorations_boundary_flags",
		Rows: []string{
			"DV_ORDERED leaf: _normal_range (bare and suffixed bounds)",
			"interval boundary flags (the asymmetric defaults)",
		},
		Keys: map[string]any{
			// A bare-bound anchor (DV_COUNT) with **no** flags at all: both
			// bounds present, both included — the closed-endpoint default that
			// must not gain a key on re-encode.
			probe089Count:                                        7,
			probe089Count + "/_normal_range/lower":               1,
			probe089Count + "/_normal_range/upper":               8,
			probe089Count + "/_other_reference_ranges:0/lower":   8,
			probe089Count + "/_other_reference_ranges:0/upper":   10,
			probe089Count + "/_other_reference_ranges:0/meaning": "high",
			// A suffixed-bound anchor (DV_ORDINAL) whose upper end is unbounded:
			// `|upper_unbounded: true` pairs with `|upper_included: false`, the
			// only spelling under which the corpus round-trips byte-exactly.
			probe089Ordinal + "|code":                                      "at0015",
			probe089Ordinal + "|value":                                     "value1",
			probe089Ordinal + "|ordinal":                                   1,
			probe089Ordinal + "/_other_reference_ranges:0/lower|code":      "at0016",
			probe089Ordinal + "/_other_reference_ranges:0/lower|value":     "value2",
			probe089Ordinal + "/_other_reference_ranges:0/lower|ordinal":   2,
			probe089Ordinal + "/_other_reference_ranges:0/meaning":         "high",
			probe089Ordinal + "/_other_reference_ranges:0|upper_included":  false,
			probe089Ordinal + "/_other_reference_ranges:0|upper_unbounded": true,
		},
	},
	{
		Name: "text_mappings_and_members",
		Rows: []string{
			"DV_TEXT leaf: _mapping:N",
			"DV_TEXT leaf: _language, _encoding",
			"CODE_PHRASE member: |preferred_term",
		},
		Keys: map[string]any{
			probe089Text:                                     "DV_TEXT value",
			probe089Text + "|formatting":                     "plain",
			probe089Text + "/_language|code":                 "en",
			probe089Text + "/_language|terminology":          "ISO_639-1",
			probe089Text + "/_language|preferred_term":       "English",
			probe089Text + "/_encoding|code":                 "UTF-8",
			probe089Text + "/_encoding|terminology":          "IANA_character-sets",
			probe089Text + "/_mapping:0|match":               "=",
			probe089Text + "/_mapping:0/target|code":         "21794005",
			probe089Text + "/_mapping:0/target|terminology":  "SNOMED-CT",
			probe089Text + "/_mapping:0/purpose|code":        "671",
			probe089Text + "/_mapping:0/purpose|terminology": "openehr",
			probe089Text + "/_mapping:0/purpose|value":       "research study",
			probe089Text + "/_mapping:1|match":               "=",
			probe089Text + "/_mapping:1/target|code":         "W.11.7",
			probe089Text + "/_mapping:1/target|terminology":  "RTX",
		},
	},
	{
		Name: "null_flavour_beside_absent_value",
		Rows: []string{"collapsed ELEMENT: _null_flavour", "collapsed ELEMENT: _null_reason"},
		Keys: map[string]any{
			// No bare leaf key at all: the ELEMENT carries a null flavour and no
			// value, which is what `ehrbase_conformance_Element_null_flavor`
			// writes and what the "typed-nil pointer is a skipped leaf" rule
			// must not swallow.
			probe089Qty + "/_null_flavour|code":        "253",
			probe089Qty + "/_null_flavour|value":       "unknown",
			probe089Qty + "/_null_flavour|terminology": "openehr",
			probe089Qty + "/_null_reason":              "sample reason",
		},
	},
	{
		Name: "temporal_accuracy",
		Rows: []string{"DV_DATE / DV_DATE_TIME / DV_TIME leaf: _accuracy"},
		Keys: map[string]any{
			probe089DateTime:                          "2022-01-12T13:22:34.000868+01:00",
			probe089DateTime + "/_accuracy":           "P2DT9H52M",
			probe089DateTime + "/_normal_range/lower": "2022-01-12T13:22:34.000868+01:00",
			probe089DateTime + "/_normal_range/upper": "2022-02-12T13:22:34.000868+01:00",
			probe089DateTime + "|magnitude_status":    "~",
		},
	},
	{
		Name: "context_party_and_participations",
		Rows: []string{
			"EVENT_CONTEXT: _health_care_facility",
			"EVENT_CONTEXT: _participation:N",
			"PARTY_IDENTIFIED / PARTY_RELATED: nested _identifier:N, /relationship",
			"PARTICIPATION: inlined |identifiers_*:N",
		},
		Keys: map[string]any{
			probe089Context + "/_health_care_facility|id":                   "9091",
			probe089Context + "/_health_care_facility|id_scheme":            "HOSPITAL-NS",
			probe089Context + "/_health_care_facility|id_namespace":         "HOSPITAL-NS",
			probe089Context + "/_health_care_facility|name":                 "Hospital",
			probe089Context + "/_health_care_facility/_identifier:0|id":     "122",
			probe089Context + "/_health_care_facility/_identifier:0|issuer": "issuer",
			// One participation with a full PARTY_IDENTIFIED performer and its
			// identifiers in the reference's **inlined** spelling …
			probe089Context + "/_participation:0|function":               "requester",
			probe089Context + "/_participation:0|mode":                   "face-to-face communication",
			probe089Context + "/_participation:0|id":                     "199",
			probe089Context + "/_participation:0|id_scheme":              "HOSPITAL-NS",
			probe089Context + "/_participation:0|id_namespace":           "HOSPITAL-NS",
			probe089Context + "/_participation:0|name":                   "Dr. Marcus Johnson",
			probe089Context + "/_participation:0|identifiers_id:0":       "122",
			probe089Context + "/_participation:0|identifiers_issuer:0":   "issuer",
			probe089Context + "/_participation:0|identifiers_assigner:0": "assigner",
			probe089Context + "/_participation:0|identifiers_type:0":     "type",
			// … and one whose performer is spelled by the **absence** of every
			// party key: an RM-mandatory PARTY_PROXY, so no-keys is PARTY_SELF.
			probe089Context + "/_participation:1|function": "performer",
			probe089Context + "/_participation:1|mode":     "not specified",
			probe089Qty + "|magnitude":                     65.9,
			probe089Qty + "|unit":                          "unit",
		},
	},
	{
		Name: "entry_participation_subject_provider",
		Rows: []string{
			"ENTRY: _other_participation:N",
			"ENTRY: subject (PARTY_PROXY leaf, REQ-053 closure)",
			"ENTRY: _provider",
		},
		Keys: map[string]any{
			probe089Obs + "/_other_participation:0|function":                 "requester",
			probe089Obs + "/_other_participation:0|mode":                     "face-to-face communication",
			probe089Obs + "/_other_participation:0|name":                     "Silvia Blake",
			probe089Obs + "/_other_participation:0/relationship|code":        "10",
			probe089Obs + "/_other_participation:0/relationship|value":       "mother",
			probe089Obs + "/_other_participation:0/relationship|terminology": "openehr",
			probe089Obs + "/subject|id":                                      "1234-5678",
			probe089Obs + "/subject|id_scheme":                               "UUID",
			probe089Obs + "/subject|id_namespace":                            "EHR.NETWORK",
			probe089Obs + "/subject|name":                                    "Silvia Blake",
			probe089Obs + "/subject/_identifier:0|id":                        "122",
			probe089Obs + "/subject/_identifier:0|type":                      "type",
			// ENTRY.provider is the second RM-optional PARTY_PROXY position, so
			// PARTY_SELF is spelled explicitly there — pinned in the feeder
			// fixture below; here the party form.
			probe089Obs + "/_provider|name": "Dr. Marcus Johnson",
			probe089Qty + "|magnitude":      65.9,
			probe089Qty + "|unit":           "unit",
		},
	},
	{
		Name: "feeder_audit_recursive",
		Rows: []string{
			"any LOCATABLE: _feeder_audit",
			"FEEDER_AUDIT: originating_system_item_id:N / feeder_system_item_id:N",
			"FEEDER_AUDIT_DETAILS: |system_id, |version_id, |time, /location, /subject, /provider",
			"FEEDER_AUDIT: original_content (DV_PARSABLE)",
			"PARTY_SELF at an RM-optional position: |_type",
		},
		Keys: map[string]any{
			probe089Obs + "/_feeder_audit/originating_system_item_id:0|id":                "id1",
			probe089Obs + "/_feeder_audit/originating_system_item_id:0|issuer":            "issuer1",
			probe089Obs + "/_feeder_audit/originating_system_item_id:0|assigner":          "assigner1",
			probe089Obs + "/_feeder_audit/originating_system_item_id:0|type":              "PERSON",
			probe089Obs + "/_feeder_audit/originating_system_item_id:1|id":                "id2",
			probe089Obs + "/_feeder_audit/originating_system_item_id:1|issuer":            "issuer2",
			probe089Obs + "/_feeder_audit/originating_system_item_id:1|assigner":          "assigner2",
			probe089Obs + "/_feeder_audit/originating_system_item_id:1|type":              "PERSON",
			probe089Obs + "/_feeder_audit/feeder_system_item_id:0|id":                     "id1",
			probe089Obs + "/_feeder_audit/feeder_system_item_id:0|issuer":                 "issuer1",
			probe089Obs + "/_feeder_audit/originating_system_audit|system_id":             "orig",
			probe089Obs + "/_feeder_audit/originating_system_audit|version_id":            "final",
			probe089Obs + "/_feeder_audit/originating_system_audit|time":                  "2021-12-21T16:02:58.0094262+01:00",
			probe089Obs + "/_feeder_audit/originating_system_audit/location|id":           "12342341",
			probe089Obs + "/_feeder_audit/originating_system_audit/location|id_scheme":    "NMC",
			probe089Obs + "/_feeder_audit/originating_system_audit/location|id_namespace": "uk.org.nmc",
			probe089Obs + "/_feeder_audit/originating_system_audit/location|name":         "Org 1",
			// The deepest shape in the grammar: a party inside a family, with a
			// nested `_identifier:N` of its own — three tail levels.
			probe089Obs + "/_feeder_audit/originating_system_audit/provider|name":                 "Per 1",
			probe089Obs + "/_feeder_audit/originating_system_audit/provider/_identifier:0|id":     "55175056",
			probe089Obs + "/_feeder_audit/originating_system_audit/provider/_identifier:0|issuer": "issuer",
			// PARTY_SELF at an RM-optional PARTY_PROXY: the discriminator is
			// written, because absence there already means absent.
			probe089Obs + "/_feeder_audit/originating_system_audit/subject|_type": "PARTY_SELF",
			probe089Obs + "/_feeder_audit/feeder_system_audit|system_id":          "feeder",
			probe089Obs + "/_feeder_audit/original_content":                       "Hello world!",
			probe089Obs + "/_feeder_audit/original_content|formalism":             "text/plain",
			probe089Qty + "|magnitude":                                            65.9,
			probe089Qty + "|unit":                                                 "unit",
		},
	},
	{
		Name: "feeder_audit_multimedia_choice",
		Rows: []string{"FEEDER_AUDIT: original_content_multimedia (the choice by key name)"},
		Keys: map[string]any{
			probe089Obs + "/_feeder_audit/originating_system_audit|system_id":    "orig",
			probe089Obs + "/_feeder_audit/original_content_multimedia":           "http://med.tube.com/sample",
			probe089Obs + "/_feeder_audit/original_content_multimedia|mediatype": "video/H261",
			probe089Obs + "/_feeder_audit/original_content_multimedia|size":      504903212,
			probe089Qty + "|magnitude":                                           65.9,
			probe089Qty + "|unit":                                                "unit",
		},
	},
	{
		Name: "encapsulated_leaves",
		Rows: []string{
			"DV_MULTIMEDIA leaf (bare uri + suffixes)",
			"DV_MULTIMEDIA value: _thumbnail (nested DV_MULTIMEDIA)",
			"DV_PARSABLE leaf (bare value + |formalism)",
			"DV_MULTIMEDIA / DV_PARSABLE value: _charset, _language",
		},
		Keys: map[string]any{
			probe089MM:                                  "http://med.tube.com/sample",
			probe089MM + "|mediatype":                   "video/H261",
			probe089MM + "|size":                        504903212,
			probe089MM + "|alternatetext":               "alternate text",
			probe089MM + "|compression_algorithm":       "zlib",
			probe089MM + "|integrity_check":             "b90360558e5420cef47015b1afbd70a156f940afa470b0515f95eacc2edcef6a",
			probe089MM + "|integrity_check_algorithm":   "SHA-256",
			probe089MM + "/_charset|code":               "UTF-8",
			probe089MM + "/_charset|terminology":        "IANA_character-sets",
			probe089MM + "/_language|code":              "en",
			probe089MM + "/_language|terminology":       "ISO_639-1",
			probe089MM + "/_thumbnail|data":             "Z2hnZ2pnamdnag==",
			probe089MM + "/_thumbnail|mediatype":        "image/png",
			probe089MM + "/_thumbnail|size":             504,
			probe089Parsable:                            "Formal instructions on carrying out the procedure...",
			probe089Parsable + "|formalism":             "GLIF 1.0",
			probe089Parsable + "/_charset|code":         "UTF-8",
			probe089Parsable + "/_charset|terminology":  "IANA_character-sets",
			probe089Parsable + "/_language|code":        "en",
			probe089Parsable + "/_language|terminology": "ISO_639-1",
		},
	},
	{
		Name: "interval_leaf",
		Rows: []string{"DV_INTERVAL<T> leaf (the interval grammar at a modelled node)"},
		Keys: map[string]any{
			probe089IntervalEvent + "/time":       "2021-12-21T16:02:58.0094262+01:00",
			probe089Interval + "/lower|magnitude": 72.83,
			probe089Interval + "/lower|unit":      "Unit",
			probe089Interval + "|lower_included":  false,
			probe089Interval + "|upper_included":  false,
			probe089Interval + "|upper_unbounded": true,
		},
	},
}

// Probe089Refusal is one deliberately-refused key set: REQ-140's boundary,
// asserted as a typed error naming the key rather than as a census tally.
type Probe089Refusal struct {
	// Name identifies the case in reports and sub-tests.
	Name string
	// Keys are added to a decodable body; the decode MUST fail.
	Keys map[string]any
	// Want is the sentinel the error MUST match under errors.Is.
	Want error
	// Names are substrings the message MUST carry — the offending FLAT key, so
	// the PROBE-086 census can scope the exclusion to it, plus the citation
	// that says *why* it is refused where a boundary decision owns it.
	Names []string
}

// Probe089Refusals are REQ-140's deliberate exclusions. Each one MUST fail on
// decode: a decode-and-drop would satisfy no assertion here, which is the
// point — these are the shapes a permissive codec loses silently.
var Probe089Refusals = []Probe089Refusal{
	{
		// ADR 0015: no `ctx/` short form can carry the composer's external_ref,
		// and encode is `ctx/`-only, so a composer decoded from these keys
		// could not be re-emitted.
		Name:  "composer_external_ref",
		Keys:  map[string]any{probe089Root + "/composer|id": "1234-5678", probe089Root + "/composer|id_scheme": "UUID"},
		Want:  simplified.ErrUnsupportedDatatype,
		Names: []string{"composer"},
	},
	{
		Name:  "composer_nested_identifier",
		Keys:  map[string]any{probe089Root + "/composer/_identifier:0|id": "122"},
		Want:  simplified.ErrUnsupportedDatatype,
		Names: []string{probe089Root + "/composer/_identifier:0|id", "ADR 0015"},
	},
	{
		Name:  "composer_relationship",
		Keys:  map[string]any{probe089Root + "/composer/relationship|code": "10"},
		Want:  simplified.ErrUnsupportedDatatype,
		Names: []string{probe089Root + "/composer/relationship|code", "ADR 0015"},
	},
	{
		// Spec-named, corpus-unexercised as a decodable grammar: deferred as a
		// typed refusal until a pinnable fixture lands, never decoded-and-dropped.
		Name: "instruction_details_deferred",
		Keys: map[string]any{
			probe089Action + "/_instruction_details|activity_id":     "activities[at0001]",
			probe089Action + "/_instruction_details|composition_uid": "4cdc3017-d8c5-4cd3-9900-f3bb7171d006",
		},
		Want:  simplified.ErrUnknownPath,
		Names: []string{probe089Action + "/_instruction_details"},
	},
	{
		Name: "wf_definition_deferred",
		Keys: map[string]any{
			probe089Instr + "/_wf_definition|value":     "wf_definition",
			probe089Instr + "/_wf_definition|formalism": "formalism",
		},
		Want:  simplified.ErrUnknownPath,
		Names: []string{probe089Instr + "/_wf_definition"},
	},
	{
		// FEEDER_AUDIT_DETAILS.other_details is an ITEM_STRUCTURE the
		// reference's suffix set has no channel for — the third deferral.
		Name: "feeder_audit_other_details_deferred",
		Keys: map[string]any{
			probe089Obs + "/_feeder_audit/originating_system_audit|system_id":       "orig",
			probe089Obs + "/_feeder_audit/originating_system_audit/other_details|x": "boom",
		},
		Want:  simplified.ErrUnsupportedDatatype,
		Names: []string{"other_details"},
	},
	{
		// Not a family at all. `_identifier:N` is reached only *inside* a party,
		// so an ENTRY owner names no family — and a typo must not be mistaken
		// for one.
		Name:  "unknown_family",
		Keys:  map[string]any{probe089Obs + "/_nonsense|id": "x"},
		Want:  simplified.ErrUnknownPath,
		Names: []string{probe089Obs + "/_nonsense"},
	},
}

// NewProbe089Target compiles and exports the corpus OPT once, for reuse across
// every fixture in a run. It is the same vendored template PROBE-086 measures,
// which is what makes the census movement and this probe two views of one
// landing.
func NewProbe089Target() (*conformance.Target, error) {
	return conformance.NewTarget()
}

// Probe089UnderscoreRoundTrip runs legs (a), (b) and (d) for one fixture.
// Status is "pass" when every leg holds; "fail" names the leg and the keys that
// broke it. Framework misuse (nil target, a fixture asserting nothing, a
// corpus template that is not the one the fixtures are written against) returns
// a non-nil error, so a harness can tell "could not run" from "the codec is
// wrong". There is no "skip": every fixture here is authored against a
// vendored template this SDK builds, so an unmodelled shape is a failure.
func Probe089UnderscoreRoundTrip(target *conformance.Target, c Probe089Case) (Result, error) {
	r := Result{Probe: "PROBE-089"}
	if target == nil || target.Web == nil {
		return r, errors.New("PROBE-089: nil target")
	}
	if len(c.Keys) == 0 {
		return r, fmt.Errorf("PROBE-089: case %q asserts no keys", c.Name)
	}
	if len(c.Rows) == 0 {
		return r, fmt.Errorf("PROBE-089: case %q names no grammar-table row", c.Name)
	}
	if target.Root != probe089Root {
		return r, fmt.Errorf("PROBE-089: corpus template root is %q, want %q — the fixtures are authored against the vendored PROBE-086 corpus OPT",
			target.Root, probe089Root)
	}
	body, err := json.Marshal(c.Body())
	if err != nil {
		return r, fmt.Errorf("PROBE-089: case %q: marshal fixture: %w", c.Name, err)
	}

	// (a) decode → re-encode, byte-exact over the whole body.
	comp, err := simplified.UnmarshalFlat(body, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat: "+err.Error()
		return r, nil
	}
	out, err := simplified.MarshalFlat(comp, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat: "+err.Error()
		return r, nil
	}
	if d := flatDiff(body, out); d != "" {
		r.Status, r.Detail = "fail", "decode → re-encode is not byte-exact: "+d
		return r, nil
	}

	// (b) encode from a composition this codec did not build: the decoded RM
	// goes out through canonical JSON and comes back, and the family's key set
	// must be reproduced exactly off *that* composition.
	canonical, err := canjson.Marshal(comp)
	if err != nil {
		r.Status, r.Detail = "fail", "canjson.Marshal: "+err.Error()
		return r, nil
	}
	var viaCanonical rm.Composition
	if err := canjson.Unmarshal(canonical, &viaCanonical); err != nil {
		r.Status, r.Detail = "fail", "canjson.Unmarshal: "+err.Error()
		return r, nil
	}
	reencoded, err := simplified.MarshalFlat(&viaCanonical, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (via canonical): "+err.Error()
		return r, nil
	}
	if d := probe089EncodeDiff(c.Keys, reencoded); d != "" {
		r.Status, r.Detail = "fail", "encode from the canonical composition: "+d
		return r, nil
	}

	// (d) STRUCTURED — the OPT-free interconversion preserves the vocabulary,
	// and the OPT-driven round-trip returns the same FLAT.
	structured, err := simplified.FlatToStructured(body)
	if err != nil {
		r.Status, r.Detail = "fail", "FlatToStructured: "+err.Error()
		return r, nil
	}
	if d := probe089VocabularyDiff(c.Body(), structured); d != "" {
		r.Status, r.Detail = "fail", "STRUCTURED vocabulary: "+d
		return r, nil
	}
	back, err := simplified.StructuredToFlat(structured)
	if err != nil {
		r.Status, r.Detail = "fail", "StructuredToFlat: "+err.Error()
		return r, nil
	}
	// The interconversion normalises `:index` (STRUCTURED is arrays-always), so
	// the assertion is the semantic one PROBE-076 makes: the interconverted FLAT
	// decodes and re-encodes to the same canonical FLAT.
	compIC, err := simplified.UnmarshalFlat(back, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	outIC, err := simplified.MarshalFlat(compIC, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (interconverted): "+err.Error()
		return r, nil
	}
	if d := flatDiff(body, outIC); d != "" {
		r.Status, r.Detail = "fail", "FLAT ↔ STRUCTURED interconversion loses data: "+d
		return r, nil
	}
	wtStructured, err := simplified.MarshalStructured(comp, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalStructured: "+err.Error()
		return r, nil
	}
	compS, err := simplified.UnmarshalStructured(wtStructured, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "UnmarshalStructured: "+err.Error()
		return r, nil
	}
	outS, err := simplified.MarshalFlat(compS, target.Web)
	if err != nil {
		r.Status, r.Detail = "fail", "MarshalFlat (from structured): "+err.Error()
		return r, nil
	}
	if d := flatDiff(body, outS); d != "" {
		r.Status, r.Detail = "fail", "OPT-driven STRUCTURED round-trip diverges from FLAT: "+d
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("%d keys round-tripped byte-exactly across %d grammar-table row(s): %s",
		len(c.Keys), len(c.Rows), strings.Join(c.Rows, "; "))
	return r, nil
}

// Probe089RefusedFamilies runs leg (c) for one deliberate refusal: the body
// MUST fail to decode, with the sentinel the boundary declares and a message
// naming the offending key. A successful decode is a failure however faithful
// the rest of the body is — that is the decode-and-drop this REQ forbids.
func Probe089RefusedFamilies(target *conformance.Target, ref Probe089Refusal) (Result, error) {
	r := Result{Probe: "PROBE-089"}
	if target == nil || target.Web == nil {
		return r, errors.New("PROBE-089: nil target")
	}
	if len(ref.Keys) == 0 || ref.Want == nil {
		return r, fmt.Errorf("PROBE-089: refusal %q has no keys or no sentinel", ref.Name)
	}
	body, err := json.Marshal(Probe089Case{Keys: ref.Keys}.Body())
	if err != nil {
		return r, fmt.Errorf("PROBE-089: refusal %q: marshal fixture: %w", ref.Name, err)
	}
	comp, err := simplified.UnmarshalFlat(body, target.Web)
	if err == nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("%s: decoded without error (%d content items) — a deliberate exclusion MUST refuse, not decode and drop",
			ref.Name, len(comp.Content))
		return r, nil
	}
	if !errors.Is(err, ref.Want) {
		r.Status, r.Detail = "fail", fmt.Sprintf("%s: err = %v, want %v", ref.Name, err, ref.Want)
		return r, nil
	}
	for _, want := range ref.Names {
		if !strings.Contains(err.Error(), want) {
			r.Status, r.Detail = "fail", fmt.Sprintf("%s: err = %v, want it to name %q", ref.Name, err, want)
			return r, nil
		}
	}
	r.Status, r.Detail = "pass", fmt.Sprintf("%s refused: %v", ref.Name, err)
	return r, nil
}

// probe089EncodeDiff compares the family key set the encoder emitted against
// the fixture's own keys. "Family surface" is every emitted key that either
// carries a `_`-prefixed path segment or shares a base path with one of the
// fixture's keys — the latter is what brings the composite leaves (the ENTRY
// `subject`, a `DV_INTERVAL<T>`, a DV_MULTIMEDIA) into scope, since their keys
// carry no underscore of their own.
func probe089EncodeDiff(keys map[string]any, emitted []byte) string {
	got, err := decodeNumberMap(emitted)
	if err != nil {
		return "emitted FLAT is not a JSON object: " + err.Error()
	}
	want, err := decodeNumberMap(mustMarshalFlat(keys))
	if err != nil {
		return "fixture keys are not JSON-encodable: " + err.Error()
	}
	bases := map[string]bool{}
	for k := range want {
		bases[probe089BaseOf(k)] = true
	}
	var dropped, invented, raw, changed []string
	for k, v := range want {
		have, ok := got[k]
		if !ok {
			dropped = append(dropped, k)
			continue
		}
		if !sameFlatValue(v, have) {
			changed = append(changed, fmt.Sprintf("%s (fixture=%v emitted=%v)", k, v, have))
		}
	}
	for k := range got {
		if _, expected := want[k]; expected {
			continue
		}
		if !probe089HasUnderscoreSegment(k) && !bases[probe089BaseOf(k)] {
			continue
		}
		if strings.HasSuffix(k, "|raw") {
			raw = append(raw, k)
			continue
		}
		invented = append(invented, k)
	}
	var b strings.Builder
	for _, part := range []struct {
		label string
		keys  []string
	}{
		{"dropped", dropped},
		{"rode |raw where the grammar carries the value", raw},
		{"invented", invented},
		{"value changed", changed},
	} {
		if len(part.keys) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %s", part.label, strings.Join(slices.Sorted(slices.Values(part.keys)), ", "))
	}
	return b.String()
}

// probe089VocabularyDiff asserts the STRUCTURED body carries exactly the
// underscore vocabulary the FLAT body spells, as array-valued members: the
// `_family` names must match on both sides, and none may collapse to a scalar
// (STRUCTURED is arrays-always, which is how `:N` survives without an OPT).
func probe089VocabularyDiff(flat map[string]any, structured []byte) string {
	want := map[string]bool{}
	for k := range flat {
		for seg := range strings.SplitSeq(probe089BaseOf(k), "/") {
			if name, _, _ := strings.Cut(seg, ":"); strings.HasPrefix(name, "_") {
				want[name] = true
			}
		}
	}
	var doc any
	if err := json.Unmarshal(structured, &doc); err != nil {
		return "STRUCTURED is not JSON: " + err.Error()
	}
	got := map[string]bool{}
	var scalar []string
	probe089WalkMembers(doc, func(name string, value any) {
		if !strings.HasPrefix(name, "_") {
			return
		}
		got[name] = true
		if _, ok := value.([]any); !ok {
			scalar = append(scalar, name)
		}
	})
	var missing, extra []string
	for name := range want {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if !want[name] {
			extra = append(extra, name)
		}
	}
	var b strings.Builder
	for _, part := range []struct {
		label string
		names []string
	}{
		{"members the interconversion dropped", missing},
		{"members it invented", extra},
		{"members that are not arrays", scalar},
	} {
		if len(part.names) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s: %s", part.label, strings.Join(slices.Sorted(slices.Values(part.names)), ", "))
	}
	return b.String()
}

// probe089WalkMembers visits every object member of a decoded JSON document.
func probe089WalkMembers(node any, visit func(name string, value any)) {
	switch n := node.(type) {
	case map[string]any:
		for name, value := range n {
			visit(name, value)
			probe089WalkMembers(value, visit)
		}
	case []any:
		for _, item := range n {
			probe089WalkMembers(item, visit)
		}
	}
}

// flatDiff reports how two FLAT payloads differ, or "" when they are equal.
// Both sides are compared through json.Number, so integers above 2^53 compare
// exactly (see [flatMapsEqual]).
func flatDiff(want, got []byte) string {
	mw, err := decodeNumberMap(want)
	if err != nil {
		return "left side is not a JSON object: " + err.Error()
	}
	mg, err := decodeNumberMap(got)
	if err != nil {
		return "right side is not a JSON object: " + err.Error()
	}
	var missing, extra, mismatched []string
	for k, v := range mw {
		have, ok := mg[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		if !sameFlatValue(v, have) {
			mismatched = append(mismatched, fmt.Sprintf("%s (want=%v got=%v)", k, v, have))
		}
	}
	for k := range mg {
		if _, ok := mw[k]; !ok {
			extra = append(extra, k)
		}
	}
	var b strings.Builder
	for _, part := range []struct {
		label string
		keys  []string
	}{
		{"missing", missing},
		{"extra", extra},
		{"mismatched", mismatched},
	} {
		if len(part.keys) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %s", part.label, strings.Join(slices.Sorted(slices.Values(part.keys)), ", "))
	}
	return b.String()
}

// sameFlatValue compares two decoded FLAT leaf values. json.Number compares as
// its literal text, which is exact — and deliberately strict about a re-encode
// that reformats a number.
func sameFlatValue(a, b any) bool {
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

// probe089BaseOf is the FLAT key without its `|suffix`.
func probe089BaseOf(key string) string {
	base, _, _ := strings.Cut(key, "|")
	return base
}

// probe089HasUnderscoreSegment reports whether any path segment of the key is a
// `_`-prefixed RM attribute.
func probe089HasUnderscoreSegment(key string) bool {
	for seg := range strings.SplitSeq(probe089BaseOf(key), "/") {
		if strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

// mustMarshalFlat encodes a fixture key map. The maps are literals in this
// file, so a failure is a programming error in the fixture, not a codec fault;
// it surfaces as an unparsable payload in the caller's diff.
func mustMarshalFlat(m map[string]any) []byte {
	b, err := json.Marshal(m)
	if err != nil {
		return []byte(`"` + err.Error() + `"`)
	}
	return b
}
