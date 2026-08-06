package simplified

// REQ-140 — the party grammar (PARTY_IDENTIFIED / PARTY_RELATED / PARTY_SELF and
// PARTICIPATION) and REQ-053's ENTRY `subject` leaf.
//
// Every path and suffix spelling here is copied out of the vendored PROBE-086
// corpus (ADR 0014 — the reference spelling wins), so a change to the grammar
// that would break an upstream body breaks a test here first.

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// --- context/_health_care_facility --------------------------------------

// TestRMAttrHealthCareFacilityRoundTrip — REQ-140. `context/_health_care_facility`
// is a single PARTY_IDENTIFIED: the four own suffixes decompose its
// `external_ref` (`|id` + `|id_scheme` onto the ref's GENERIC_ID, `|id_namespace`
// onto its namespace) and its `name`, and the nested `_identifier:N` list carries
// its DV_IDENTIFIERs. Exactly the keys
// `ehrbase_conformance_party_identified.json` carries.
func TestRMAttrHealthCareFacilityRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrContext + "/_health_care_facility|id":                     "9091",
		rmattrContext + "/_health_care_facility|id_scheme":              "HOSPITAL-NS",
		rmattrContext + "/_health_care_facility|id_namespace":           "HOSPITAL-NS",
		rmattrContext + "/_health_care_facility|name":                   "Hospital",
		rmattrContext + "/_health_care_facility/_identifier:0|id":       "122",
		rmattrContext + "/_health_care_facility/_identifier:0|issuer":   "issuer",
		rmattrContext + "/_health_care_facility/_identifier:0|assigner": "assigner",
		rmattrContext + "/_health_care_facility/_identifier:0|type":     "type",
	})
	hcf, ok := comp.Context.HealthCareFacility.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("health_care_facility = %T, want *rm.PartyIdentified", comp.Context.HealthCareFacility)
	}
	if hcf.Name == nil || *hcf.Name != "Hospital" {
		t.Errorf("name = %v, want Hospital", hcf.Name)
	}
	if hcf.ExternalRef == nil {
		t.Fatal("external_ref not rebuilt from |id / |id_scheme / |id_namespace")
	}
	gid, ok := hcf.ExternalRef.ID.(*rm.GenericID)
	if !ok {
		t.Fatalf("external_ref.id = %T, want *rm.GenericID (|id_scheme present)", hcf.ExternalRef.ID)
	}
	if gid.Value != "9091" || gid.Scheme != "HOSPITAL-NS" {
		t.Errorf("external_ref.id = %+v", gid)
	}
	if hcf.ExternalRef.Namespace != "HOSPITAL-NS" || hcf.ExternalRef.Type != partyRefType {
		t.Errorf("external_ref namespace/type = %q/%q, want %q/%q",
			hcf.ExternalRef.Namespace, hcf.ExternalRef.Type, "HOSPITAL-NS", partyRefType)
	}
	if len(hcf.Identifiers) != 1 || hcf.Identifiers[0].ID != "122" {
		t.Errorf("identifiers = %+v", hcf.Identifiers)
	}
}

// TestRMAttrHealthCareFacilityRelated — REQ-140. PARTY_RELATED substitutes for
// PARTY_IDENTIFIED at every party position, and it is `/relationship` (a
// DV_CODED_TEXT sub-object) that selects the subtype — the shape
// `ehrbase_conformance_party_related.json` carries.
func TestRMAttrHealthCareFacilityRelated(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrContext + "/_health_care_facility|id":                       "9091",
		rmattrContext + "/_health_care_facility|id_scheme":                "HOSPITAL-NS",
		rmattrContext + "/_health_care_facility|id_namespace":             "HOSPITAL-NS",
		rmattrContext + "/_health_care_facility|name":                     "Hospital",
		rmattrContext + "/_health_care_facility/relationship|code":        "10",
		rmattrContext + "/_health_care_facility/relationship|terminology": "openehr",
		rmattrContext + "/_health_care_facility/relationship|value":       "mother",
	})
	rel, ok := comp.Context.HealthCareFacility.(*rm.PartyRelated)
	if !ok {
		t.Fatalf("health_care_facility = %T, want *rm.PartyRelated (/relationship present)",
			comp.Context.HealthCareFacility)
	}
	if rel.Relationship.Value != "mother" || rel.Relationship.DefiningCode.CodeString != "10" {
		t.Errorf("relationship = %+v", rel.Relationship)
	}
}

// TestRMAttrHealthCareFacilityHierObjectID — REQ-140. `|id_scheme` is the
// OBJECT_ID discriminator, as it is for C0's OBJECT_REF families: without it the
// ref's id is a HIER_OBJECT_ID, and the round-trip must not invent a scheme.
func TestRMAttrHealthCareFacilityHierObjectID(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrContext + "/_health_care_facility|id":           "9091",
		rmattrContext + "/_health_care_facility|id_namespace": "HOSPITAL-NS",
	})
	hcf := comp.Context.HealthCareFacility.(*rm.PartyIdentified)
	if _, ok := hcf.ExternalRef.ID.(*rm.HierObjectID); !ok {
		t.Errorf("external_ref.id = %T, want *rm.HierObjectID (no |id_scheme)", hcf.ExternalRef.ID)
	}
}

// --- context/_participation:N and <entry>/_other_participation:N -------

// TestRMAttrParticipationRoundTrip — REQ-140. `context/_participation:N` carries
// the PARTICIPATION grammar: `|function`, `|mode` (the openEHR rubric alone — see
// [participationModes]), the performer's party suffixes inline on the same key
// base, and the performer's identifiers as the reference's **inlined** indexed
// suffixes. Exactly `ehrbase_conformance_party_identified.json`'s keys.
func TestRMAttrParticipationRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrContext + "/_participation:0|function":               "requester",
		rmattrContext + "/_participation:0|mode":                   "face-to-face communication",
		rmattrContext + "/_participation:0|id":                     "199",
		rmattrContext + "/_participation:0|id_scheme":              "HOSPITAL-NS",
		rmattrContext + "/_participation:0|id_namespace":           "HOSPITAL-NS",
		rmattrContext + "/_participation:0|name":                   "Dr. Marcus Johnson",
		rmattrContext + "/_participation:0|identifiers_id:0":       "122",
		rmattrContext + "/_participation:0|identifiers_issuer:0":   "issuer",
		rmattrContext + "/_participation:0|identifiers_assigner:0": "assigner",
		rmattrContext + "/_participation:0|identifiers_type:0":     "type",
		rmattrContext + "/_participation:1|function":               "performer",
		rmattrContext + "/_participation:1|mode":                   "not specified",
	})
	if len(comp.Context.Participations) != 2 {
		t.Fatalf("participations = %d, want 2", len(comp.Context.Participations))
	}
	p := comp.Context.Participations[0]
	if got := rm.DVTextValueOf(p.Function); got != "requester" {
		t.Errorf("function = %q, want requester", got)
	}
	if p.Mode == nil || p.Mode.DefiningCode.CodeString != "216" ||
		p.Mode.DefiningCode.TerminologyID.Value != participationModeTerminology {
		t.Errorf("mode = %+v, want the openehr 216 rubric rebuilt", p.Mode)
	}
	performer, ok := p.Performer.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("performer = %T, want *rm.PartyIdentified", p.Performer)
	}
	if len(performer.Identifiers) != 1 || performer.Identifiers[0].ID != "122" {
		t.Errorf("performer identifiers = %+v", performer.Identifiers)
	}
	// A participation with no party key at all: the performer is the record
	// subject, spelled by absence (`ehrbase_conformance_party_self.json`).
	if _, self := comp.Context.Participations[1].Performer.(*rm.PartySelf); !self {
		t.Errorf("participation:1 performer = %T, want *rm.PartySelf (no party key spelled)",
			comp.Context.Participations[1].Performer)
	}
}

// TestRMAttrOtherParticipationRoundTrip — REQ-140. `_other_participation:N` is
// the same grammar on an ENTRY owner, and PARTY_RELATED substitutes for the
// performer there too (`ehrbase_conformance_party_related.json`).
func TestRMAttrOtherParticipationRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/_other_participation:0|function":                 "requester",
		rmattrObs + "/_other_participation:0|mode":                     "face-to-face communication",
		rmattrObs + "/_other_participation:0|id":                       "1234-5678",
		rmattrObs + "/_other_participation:0|id_scheme":                "UUID",
		rmattrObs + "/_other_participation:0|id_namespace":             "EHR.NETWORK",
		rmattrObs + "/_other_participation:0|name":                     "Silvia Blake",
		rmattrObs + "/_other_participation:0/relationship|code":        "10",
		rmattrObs + "/_other_participation:0/relationship|terminology": "openehr",
		rmattrObs + "/_other_participation:0/relationship|value":       "mother",
	})
	obs := firstObservation(t, comp)
	if len(obs.OtherParticipations) != 1 {
		t.Fatalf("other_participations = %d, want 1", len(obs.OtherParticipations))
	}
	related, ok := obs.OtherParticipations[0].Performer.(*rm.PartyRelated)
	if !ok {
		t.Fatalf("performer = %T, want *rm.PartyRelated (/relationship present)",
			obs.OtherParticipations[0].Performer)
	}
	if related.Relationship.Value != "mother" {
		t.Errorf("relationship = %+v", related.Relationship)
	}
}

// TestRMAttrParticipationRefusals — REQ-140. The two identifier spellings are
// position-specific and the vendored mode vocabulary is closed, so each wrong
// shape is a typed refusal naming the key.
func TestRMAttrParticipationRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	base := map[string]any{
		rmattrContext + "/_participation:0|function": "requester",
	}
	for name, tc := range map[string]struct {
		keys map[string]any
		want error
		name string
	}{
		// The nested spelling standalone parties use is refused *here*: a
		// participation inlines its performer's identifiers, and accepting both
		// would give one RM list two FLAT surfaces.
		"nested identifier on a participation": {
			keys: map[string]any{rmattrContext + "/_participation:0/_identifier:0|id": "122"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_participation:0/_identifier:0|id",
		},
		"unindexed inline identifier": {
			keys: map[string]any{rmattrContext + "/_participation:0|identifiers_id": "122"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_participation:0|identifiers_id",
		},
		"sparse inline identifier index": {
			keys: map[string]any{rmattrContext + "/_participation:0|identifiers_id:1": "122"},
			want: ErrUnknownPath,
			name: rmattrContext + "/_participation:0",
		},
		"unknown mode rubric": {
			keys: map[string]any{rmattrContext + "/_participation:0|mode": "by carrier pigeon"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_participation:0|mode",
		},
		"participation on an entry": {
			keys: map[string]any{rmattrObs + "/_participation:0|function": "requester"},
			want: ErrUnknownPath,
			name: rmattrObs + "/_participation:0",
		},
		"other participation on a section": {
			keys: map[string]any{rmattrSection + "/_other_participation:0|function": "requester"},
			want: ErrUnknownPath,
			name: rmattrSection + "/_other_participation:0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(base)
			maps.Copy(body, tc.keys)
			err := decodeRMAttrErr(t, wt, body)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("err = %v, want it to name %q", err, tc.name)
			}
		})
	}
}

// TestRMAttrParticipationFunctionRequired — REQ-140. PARTICIPATION.function is
// RM-mandatory and the grammar's only channel for it is `|function`, so a
// participation spelled without one is refused rather than decoded to an empty
// text.
func TestRMAttrParticipationFunctionRequired(t *testing.T) {
	wt, _ := conformanceWT(t)
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrContext + "/_participation:0|name": "Dr. Marcus Johnson",
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "|function") {
		t.Errorf("err = %v, want ErrUnsupportedDatatype naming |function", err)
	}
}

// TestParticipationModeVocabularyIsInvertible — REQ-140. `|mode` carries the
// rubric alone, so the vendored group is read backwards on decode. Two codes
// sharing a rubric would make that lookup pick one by map order — a
// non-deterministic round-trip.
func TestParticipationModeVocabularyIsInvertible(t *testing.T) {
	if len(participationModeCodes) != len(participationModes) {
		t.Errorf("the vendored `participation mode` group has %d codes but only %d distinct rubrics; "+
			"the rubric->code lookup decode uses would pick one by map order",
			len(participationModes), len(participationModeCodes))
	}
}

// TestParticipationEncodeRefusals — REQ-140. Encode never drops an in-scope
// value: what the grammar cannot carry is a typed error, and none of these
// families has a `|raw` carrier to fall back to.
func TestParticipationEncodeRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	openehrCode := func(code, value string) rm.DVCodedText {
		return rm.DVCodedText{
			DVText:       rm.DVText{Value: value},
			DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "openehr"}},
		}
	}
	for name, mutate := range map[string]func(*rm.Composition){
		"participation time": func(c *rm.Composition) {
			c.Context.Participations = []rm.Participation{{
				Function:  rm.DVText{Value: "requester"},
				Performer: rm.PartySelf{},
				Time: &rm.DVInterval[rm.DVDateTime]{
					Interval: rm.Interval[rm.DVDateTime]{
						Lower: rm.DVDateTime{Value: "2021-12-21T14:19:31+01:00"},
					},
				},
			}}
		},
		"coded function": func(c *rm.Composition) {
			fn := openehrCode("216", "requester")
			c.Context.Participations = []rm.Participation{{Function: &fn, Performer: rm.PartySelf{}}}
		},
		"mode outside the vendored group": func(c *rm.Composition) {
			mode := openehrCode("999", "by carrier pigeon")
			c.Context.Participations = []rm.Participation{{
				Function: rm.DVText{Value: "requester"}, Mode: &mode, Performer: rm.PartySelf{},
			}}
		},
		"mode in another terminology": func(c *rm.Composition) {
			mode := rm.DVCodedText{
				DVText:       rm.DVText{Value: "face-to-face communication"},
				DefiningCode: rm.CodePhrase{CodeString: "216", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
			}
			c.Context.Participations = []rm.Participation{{
				Function: rm.DVText{Value: "requester"}, Mode: &mode, Performer: rm.PartySelf{},
			}}
		},
		"mode value disagreeing with the rubric": func(c *rm.Composition) {
			mode := openehrCode("216", "in person")
			c.Context.Participations = []rm.Participation{{
				Function: rm.DVText{Value: "requester"}, Mode: &mode, Performer: rm.PartySelf{},
			}}
		},
		"party self carrying an external ref": func(c *rm.Composition) {
			c.Context.Participations = []rm.Participation{{
				Function: rm.DVText{Value: "requester"},
				Performer: rm.PartySelf{ExternalRef: &rm.PartyRef{
					ObjectRef: rm.ObjectRef{ID: rm.HierObjectID{Value: "x"}, Namespace: "n", Type: partyRefType},
				}},
			}}
		},
		"external ref with an empty type": func(c *rm.Composition) {
			who := "Hospital"
			c.Context.HealthCareFacility = rm.PartyIdentified{Name: &who, ExternalRef: &rm.PartyRef{
				ObjectRef: rm.ObjectRef{ID: rm.HierObjectID{Value: "9091"}, Namespace: "HOSPITAL-NS"},
			}}
		},
		"external ref with an indistinguishable object id": func(c *rm.Composition) {
			who := "Hospital"
			c.Context.HealthCareFacility = rm.PartyIdentified{Name: &who, ExternalRef: &rm.PartyRef{
				// TEMPLATE_ID is scheme-less, so `|id_scheme`'s absence would read
				// it back as a HIER_OBJECT_ID.
				ObjectRef: rm.ObjectRef{ID: rm.TemplateID{Value: "t"}, Namespace: "HOSPITAL-NS", Type: partyRefType},
			}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			comp := decodeRMAttr(t, wt, rmattrBody(nil))
			mutate(comp)
			if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("MarshalFlat err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// --- the ENTRY `subject` leaf (REQ-053) ---------------------------------

// TestSubjectLeafRoundTrip — REQ-053. The ENTRY `subject` in-context leaf is a
// PARTY_PROXY, which the codec skipped in both directions until REQ-140's party
// grammar gave it a spelling. Three key shapes address one RM value here — the
// leaf's own suffixes, the nested `_identifier:N` and PARTY_RELATED's
// `/relationship` — exactly as `ehrbase_conformance_party_identified.json` and
// `…_party_related.json` write them.
func TestSubjectLeafRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/subject|id":                     "1234-5678",
		rmattrObs + "/subject|id_scheme":              "UUID",
		rmattrObs + "/subject|id_namespace":           "EHR.NETWORK",
		rmattrObs + "/subject|name":                   "Silvia Blake",
		rmattrObs + "/subject/_identifier:0|id":       "122",
		rmattrObs + "/subject/_identifier:0|issuer":   "issuer",
		rmattrObs + "/subject/_identifier:0|assigner": "assigner",
		rmattrObs + "/subject/_identifier:0|type":     "type",
	})
	obs := firstObservation(t, comp)
	subject, ok := obs.Subject.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("subject = %T, want *rm.PartyIdentified", obs.Subject)
	}
	if subject.Name == nil || *subject.Name != "Silvia Blake" {
		t.Errorf("subject name = %v", subject.Name)
	}
	if len(subject.Identifiers) != 1 || subject.Identifiers[0].ID != "122" {
		t.Errorf("subject identifiers = %+v", subject.Identifiers)
	}
	if subject.ExternalRef == nil || subject.ExternalRef.Namespace != "EHR.NETWORK" {
		t.Errorf("subject external_ref = %+v", subject.ExternalRef)
	}
}

// TestSubjectLeafRelatedRoundTrip — REQ-053. `subject/relationship` selects
// PARTY_RELATED at the leaf exactly as it does inside a family.
func TestSubjectLeafRelatedRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/subject|name":                     "Silvia Blake",
		rmattrObs + "/subject/relationship|code":        "10",
		rmattrObs + "/subject/relationship|terminology": "openehr",
		rmattrObs + "/subject/relationship|value":       "mother",
	})
	related, ok := firstObservation(t, comp).Subject.(*rm.PartyRelated)
	if !ok {
		t.Fatalf("subject = %T, want *rm.PartyRelated", firstObservation(t, comp).Subject)
	}
	if related.Relationship.Value != "mother" {
		t.Errorf("relationship = %+v", related.Relationship)
	}
}

// TestSubjectLeafPartySelfEmitsNothing — REQ-053. PARTY_SELF is spelled by the
// absence of every party key, which is what makes the leaf symmetric with the
// `WithTemplate` RM-mandatory completion: that default is a PARTY_SELF, so a
// conformant decode's re-encode must not gain a single `subject` key.
func TestSubjectLeafPartySelfEmitsNothing(t *testing.T) {
	wt, c := conformanceWT(t)
	body := rmattrBody(nil)
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := UnmarshalFlat(b, wt, WithTemplate(c))
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if _, self := firstObservation(t, comp).Subject.(*rm.PartySelf); !self {
		t.Fatalf("WithTemplate default subject = %T, want *rm.PartySelf — the symmetry under test",
			firstObservation(t, comp).Subject)
	}
	for k := range reencodeRMAttr(t, wt, comp) {
		if strings.Contains(k, "/subject") {
			t.Errorf("re-encode emitted %q for a PARTY_SELF subject; PARTY_SELF is spelled by absence", k)
		}
	}
}

// TestSubjectLeafRefusals — REQ-053 / REQ-140. The leaf carries the same closed
// grammar as every other party position, and a party leaf is single-valued.
func TestSubjectLeafRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		keys map[string]any
		want error
		name string
	}{
		"unknown suffix": {
			keys: map[string]any{rmattrObs + "/subject|id_typo": "x"},
			want: ErrUnsupportedDatatype,
			name: rmattrObs + "/subject|id_typo",
		},
		"unknown sub-path": {
			keys: map[string]any{rmattrObs + "/subject/relation|code": "10"},
			want: ErrUnsupportedDatatype,
			name: rmattrObs + "/subject/relation|code",
		},
		"indexed party leaf": {
			keys: map[string]any{rmattrObs + "/subject:1|name": "Silvia Blake"},
			want: ErrUnknownPath,
			name: rmattrObs + "/subject:1",
		},
		"raw fragment": {
			keys: map[string]any{rmattrObs + "/subject|raw": map[string]any{"_type": "PARTY_SELF"}},
			want: ErrUnsupportedDatatype,
			name: rmattrObs + "/subject|raw",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(tc.keys))
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("err = %v, want it to name %q", err, tc.name)
			}
		})
	}
}

// TestRMAttrPartyRefTypeRoundTrip — REQ-140. `|type` is emitted only when the
// reference is typed outside [partyRefType], which is what keeps a corpus body
// byte-exact (no `|type` at all) while a `PERSON` or `ORGANISATION` reference —
// the shape the vendored PROBE-076 fixture `clinical_content_validation.json`
// carries — still round-trips instead of being dropped.
func TestRMAttrPartyRefTypeRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrContext + "/_health_care_facility|id":           "9905",
		rmattrContext + "/_health_care_facility|id_scheme":    "scheme",
		rmattrContext + "/_health_care_facility|id_namespace": "namespace",
		rmattrContext + "/_health_care_facility|type":         "ORGANISATION",
		rmattrContext + "/_health_care_facility|name":         "Company",
	})
	// The default is spelled by absence: an untyped body must not gain a key.
	comp := decodeRMAttr(t, wt, rmattrBody(map[string]any{
		rmattrContext + "/_health_care_facility|id":           "9091",
		rmattrContext + "/_health_care_facility|id_namespace": "HOSPITAL-NS",
	}))
	got := reencodeRMAttr(t, wt, comp)
	if v, emitted := got[rmattrContext+"/_health_care_facility|type"]; emitted {
		t.Errorf("re-encode invented |type = %#v for the default %q reference type", v, partyRefType)
	}
}

// TestRMAttrPartyRefusals — REQ-140. The party grammar is closed: a suffix
// outside it, an `external_ref` missing its RM-mandatory namespace, and a scheme
// with no id to qualify are all typed refusals naming the offending FLAT key.
func TestRMAttrPartyRefusals(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		keys map[string]any
		want error
		name string
	}{
		"unknown suffix": {
			keys: map[string]any{rmattrContext + "/_health_care_facility|id_typo": "9091"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_health_care_facility|id_typo",
		},
		"unknown sub-path": {
			keys: map[string]any{rmattrContext + "/_health_care_facility/relation|code": "10"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_health_care_facility/relation|code",
		},
		"external ref without namespace": {
			keys: map[string]any{rmattrContext + "/_health_care_facility|id": "9091"},
			want: ErrUnsupportedDatatype,
			name: "id_namespace",
		},
		"scheme without id": {
			keys: map[string]any{rmattrContext + "/_health_care_facility|id_scheme": "HOSPITAL-NS"},
			want: ErrUnsupportedDatatype,
			name: "id_scheme",
		},
		"indexed relationship": {
			keys: map[string]any{rmattrContext + "/_health_care_facility/relationship:1|code": "10"},
			want: ErrUnsupportedDatatype,
			name: rmattrContext + "/_health_care_facility/relationship:1|code",
		},
		"sparse identifier index": {
			keys: map[string]any{rmattrContext + "/_health_care_facility/_identifier:1|id": "122"},
			want: ErrUnknownPath,
			name: "_identifier",
		},
		"health care facility on an entry": {
			keys: map[string]any{rmattrObs + "/_health_care_facility|name": "Hospital"},
			want: ErrUnknownPath,
			name: rmattrObs + "/_health_care_facility",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(tc.keys))
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("err = %v, want it to name %q", err, tc.name)
			}
		})
	}
}

// --- party validity refusals ---------------------------------------------

// A PARTY_IDENTIFIED that satisfies none of `name`, `identifiers` or
// `external_ref` writes no key at all, and the absence of every party key is how
// this grammar spells PARTY_SELF at an RM-mandatory position. Encoding it would
// round-trip the party back as a *different subtype*, so it is refused — which
// is also what the RM already says (invariant `Basic_validity`).
func TestEmptyPartyIdentifiedRefused(t *testing.T) {
	empty := ""
	for name, party := range map[string]rm.PartyProxy{
		"no name, no identifier, no external_ref": &rm.PartyIdentified{},
		"present but empty name":                  &rm.PartyIdentified{Name: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			out := map[string]any{}
			if err := partyRMAttr(out, "x/_provider", party); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if len(out) != 0 {
				t.Errorf("a refused party still wrote %v", out)
			}
		})
	}
}

// A party carrying any one of the three is fine — the refusal above must not
// widen into "a party needs a name".
func TestPartyIdentifiedWithOnlyIdentifierEncodes(t *testing.T) {
	out := map[string]any{}
	party := &rm.PartyIdentified{Identifiers: []rm.DVIdentifier{{ID: "id-0"}}}
	if err := partyRMAttr(out, "x/_provider", party); err != nil {
		t.Fatalf("partyRMAttr: %v", err)
	}
	if got := out["x/_provider/_identifier:0|id"]; got != "id-0" {
		t.Errorf("_identifier:0|id = %v, want id-0", got)
	}
}

// PARTICIPATION.performer is RM-mandatory, and at a mandatory position the
// absence of every party key is PARTY_SELF. A nil performer written as absence
// would have decode invent a PARTY_SELF nobody put there.
func TestParticipationNilPerformerRefused(t *testing.T) {
	out := map[string]any{}
	err := participationsRMAttr(out, "x", "_participation", []rm.Participation{{
		Function: rm.DVText{Value: "requester"},
	}})
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "performer") {
		t.Errorf("err = %v, want it to name the performer", err)
	}
}

// TestParticipationModeDecorationRefused — REQ-140. `|mode` carries the openEHR
// `participation mode` **rubric alone**, so a DV_CODED_TEXT decoration has no
// channel and must be refused rather than narrowed away. The sibling checks
// (foreign terminology, value/rubric mismatch) were pinned; this one was not, so
// deleting it survived the whole suite and the decoration was silently dropped.
func TestParticipationModeDecorationRefused(t *testing.T) {
	pt := "face to face"
	mode := rm.DVCodedText{
		DVText:       rm.DVText{Value: "face-to-face communication"},
		DefiningCode: rm.CodePhrase{CodeString: "216", TerminologyID: rm.TerminologyID{Value: "openehr"}, PreferredTerm: &pt},
	}
	out := map[string]any{}
	err := participationsRMAttr(out, "x", "_participation", []rm.Participation{{
		Function:  rm.DVText{Value: "requester"},
		Mode:      &mode,
		Performer: &rm.PartyIdentified{Name: func(s string) *string { return &s }("Dr Test")},
	}})
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("err = %v, want it to name |mode", err)
	}
}
