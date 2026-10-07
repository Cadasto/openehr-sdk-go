package validation_test

// rmfloor_party_proxy_test.go: REQ-112 — the RM floor walks the party
// proxies (PARTY_SELF, PARTY_IDENTIFIED, PARTY_RELATED) and the
// participations (PARTICIPATION) wherever they sit, and checks them. Each
// test compares the full sorted list of findings ("code path"), so a row
// pins both the finding it expects and the absence of any other.

import (
	json "encoding/json/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// codedText returns a DV_CODED_TEXT with value, coded as code in the
// openehr terminology.
func codedText(value, code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        value,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
	}
}

// validParticipation returns a PARTICIPATION that carries every attribute and
// breaks no RM rule.
func validParticipation() rm.Participation {
	return rm.Participation{
		Function:  rm.DVText{Value: "assistant"},
		Performer: &rm.PartyIdentified{Name: new("Nurse Smith")},
		Mode:      new(codedText("face-to-face communication", "216")),
		Time: &rm.DVInterval[rm.DVDateTime]{
			Lower:         rm.DVDateTime{Value: "2026-10-06T10:00:00Z"},
			Upper:         rm.DVDateTime{Value: "2026-10-06T10:30:00Z"},
			LowerIncluded: true,
			UpperIncluded: true,
		},
	}
}

// partyProxyComposition returns a COMPOSITION that passes the floor with no
// finding and holds a party proxy or a participation at every place the
// REQ-112 catalogue names: the composer, the context's health_care_facility
// and participations, and an EVALUATION's subject, provider and
// other_participations.
func partyProxyComposition() *rm.Composition {
	const compositionID = "openEHR-EHR-COMPOSITION.encounter.v1"
	const evaluationID = "openEHR-EHR-EVALUATION.problem_diagnosis.v1"
	return &rm.Composition{
		ArchetypeNodeID:  compositionID,
		Name:             rm.DVText{Value: "Encounter"},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: compositionID}, RMVersion: "1.1.0"},
		Category:         codedText("event", "433"),
		Composer:         &rm.PartyIdentified{Name: new("Dr Jones")},
		Language:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Territory:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}, CodeString: "NL"},
		Context: &rm.EventContext{
			StartTime:          rm.DVDateTime{Value: "2026-10-06T10:00:00Z"},
			Setting:            codedText("other care", "238"),
			HealthCareFacility: &rm.PartyIdentified{Name: new("Ward A3")},
			Participations:     []rm.Participation{validParticipation()},
		},
		Content: []rm.ContentItem{&rm.Evaluation{
			ArchetypeNodeID:     evaluationID,
			Name:                rm.DVText{Value: "Problem"},
			ArchetypeDetails:    &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: evaluationID}, RMVersion: "1.1.0"},
			Language:            rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
			Encoding:            rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
			Subject:             &rm.PartySelf{},
			Provider:            &rm.PartyIdentified{Name: new("Dr Jones")},
			OtherParticipations: []rm.Participation{validParticipation()},
			Data:                &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "Structure"}},
		}},
	}
}

// evaluationOf returns the EVALUATION partyProxyComposition puts in content.
func evaluationOf(c *rm.Composition) *rm.Evaluation {
	return c.Content[0].(*rm.Evaluation)
}

// refWithoutNamespace returns a PARTY_REF whose id and type are set and whose
// namespace is empty: a present reference that breaks the OBJECT_REF rule.
func refWithoutNamespace() *rm.PartyRef {
	return &rm.PartyRef{
		ID:   &rm.HierObjectID{Value: "9fcc1c70-6349-4a5c-a8f8-4cdbe4ad5b0f"},
		Type: "PERSON",
	}
}

// sortedFindings returns want in the order findingsOf returns findings.
func sortedFindings(want []string) []string {
	return slices.Sorted(slices.Values(want))
}

// TestREQ112_PartyProxyAndParticipationFindings checks the floor's findings
// on a COMPOSITION with one fault planted in a party proxy or a
// participation at each place it can sit (REQ-112). The unchanged
// composition is the negative control: a composer with only a name, a
// facility, a provider and a bare PARTY_SELF subject, and two complete
// participations whose mode and time are walked, give no finding.
func TestREQ112_PartyProxyAndParticipationFindings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(c *rm.Composition)
		want   []string
	}{
		{
			name:   "valid party proxies and participations",
			mutate: func(*rm.Composition) {},
		},
		{
			name: "issue acceptance: composer external_ref without namespace, participation without function and with an empty performer",
			mutate: func(c *rm.Composition) {
				c.Composer = &rm.PartyIdentified{ExternalRef: refWithoutNamespace()}
				c.Context.Participations = []rm.Participation{{Performer: &rm.PartyIdentified{}}}
			},
			want: []string{
				"required /context/participations[0]/function",
				"rm_invariant /composer/external_ref/namespace",
				"rm_invariant /context/participations[0]/performer",
			},
		},
		{
			name: "composer external_ref with no part set",
			mutate: func(c *rm.Composition) {
				c.Composer = &rm.PartyIdentified{ExternalRef: &rm.PartyRef{}}
			},
			want: []string{
				"rm_invariant /composer/external_ref/id",
				"rm_invariant /composer/external_ref/namespace",
				"rm_invariant /composer/external_ref/type",
			},
		},
		{
			name:   "composer PARTY_IDENTIFIED with no name, identifiers or external_ref",
			mutate: func(c *rm.Composition) { c.Composer = &rm.PartyIdentified{} },
			want:   []string{"rm_invariant /composer"},
		},
		{
			name:   "composer PARTY_IDENTIFIED in value form",
			mutate: func(c *rm.Composition) { c.Composer = rm.PartyIdentified{} },
			want:   []string{"rm_invariant /composer"},
		},
		{
			name:   "health_care_facility with no name, identifiers or external_ref",
			mutate: func(c *rm.Composition) { c.Context.HealthCareFacility = &rm.PartyIdentified{} },
			want:   []string{"rm_invariant /context/health_care_facility"},
		},
		{
			name:   "ENTRY provider with no name, identifiers or external_ref",
			mutate: func(c *rm.Composition) { evaluationOf(c).Provider = &rm.PartyIdentified{} },
			want:   []string{"rm_invariant /content[0]/provider"},
		},
		{
			name:   "participation performer with no name, identifiers or external_ref",
			mutate: func(c *rm.Composition) { c.Context.Participations[0].Performer = &rm.PartyIdentified{} },
			want:   []string{"rm_invariant /context/participations[0]/performer"},
		},
		{
			name:   "context participation without function",
			mutate: func(c *rm.Composition) { c.Context.Participations[0].Function = nil },
			want:   []string{"required /context/participations[0]/function"},
		},
		{
			name:   "context participation whose function value is empty",
			mutate: func(c *rm.Composition) { c.Context.Participations[0].Function = rm.DVText{} },
			want:   []string{"required /context/participations[0]/function"},
		},
		{
			name:   "context participation without performer",
			mutate: func(c *rm.Composition) { c.Context.Participations[0].Performer = nil },
			want:   []string{"required /context/participations[0]/performer"},
		},
		{
			name:   "ENTRY other_participation without function",
			mutate: func(c *rm.Composition) { evaluationOf(c).OtherParticipations[0].Function = nil },
			want:   []string{"required /content[0]/other_participations[0]/function"},
		},
		{
			name:   "ENTRY other_participation without performer",
			mutate: func(c *rm.Composition) { evaluationOf(c).OtherParticipations[0].Performer = nil },
			want:   []string{"required /content[0]/other_participations[0]/performer"},
		},
		{
			name:   "participation mode with an empty code_string",
			mutate: func(c *rm.Composition) { c.Context.Participations[0].Mode.DefiningCode.CodeString = "" },
			want: []string{
				"required /context/participations[0]/mode/defining_code/code_string",
				"rm_invariant /context/participations[0]/mode/defining_code",
			},
		},
		{
			name: "participation time with an invalid bound",
			mutate: func(c *rm.Composition) {
				c.Context.Participations[0].Time.Upper = rm.DVDateTime{Value: "example"}
			},
			want: []string{"rm_invariant /context/participations[0]/time/upper"},
		},
		{
			name:   "composer name present but empty",
			mutate: func(c *rm.Composition) { c.Composer = &rm.PartyIdentified{Name: new("")} },
			want:   []string{"rm_invariant /composer/name"},
		},
		{
			name: "composer identifiers present but empty",
			mutate: func(c *rm.Composition) {
				c.Composer = &rm.PartyIdentified{Identifiers: []rm.DVIdentifier{}}
			},
			want: []string{"rm_invariant /composer/identifiers"},
		},
		{
			name: "composer identifier without its mandatory id",
			mutate: func(c *rm.Composition) {
				c.Composer = &rm.PartyIdentified{Identifiers: []rm.DVIdentifier{{}}}
			},
			want: []string{"required /composer/identifiers[0]/id"},
		},
		{
			name: "composer identifier with an id",
			mutate: func(c *rm.Composition) {
				c.Composer = &rm.PartyIdentified{Identifiers: []rm.DVIdentifier{{ID: "123", Issuer: new("NHS")}}}
			},
		},
		{
			name: "ENTRY subject PARTY_RELATED without relationship",
			mutate: func(c *rm.Composition) {
				evaluationOf(c).Subject = &rm.PartyRelated{Name: new("Jane")}
			},
			want: []string{"required /content[0]/subject/relationship"},
		},
		{
			name: "ENTRY subject PARTY_RELATED with only a relationship",
			mutate: func(c *rm.Composition) {
				evaluationOf(c).Subject = &rm.PartyRelated{Relationship: codedText("mother", "10")}
			},
			want: []string{"rm_invariant /content[0]/subject"},
		},
		{
			name: "ENTRY subject PARTY_RELATED with an empty name and empty identifiers",
			mutate: func(c *rm.Composition) {
				evaluationOf(c).Subject = &rm.PartyRelated{
					Name: new(""), Identifiers: []rm.DVIdentifier{},
					Relationship: codedText("mother", "10"),
				}
			},
			want: []string{
				"rm_invariant /content[0]/subject/identifiers",
				"rm_invariant /content[0]/subject/name",
			},
		},
		{
			name: "ENTRY subject PARTY_RELATED complete",
			mutate: func(c *rm.Composition) {
				evaluationOf(c).Subject = &rm.PartyRelated{
					Name:         new("Jane"),
					Relationship: codedText("mother", "10"),
				}
			},
		},
		{
			name: "ENTRY subject PARTY_SELF with an external_ref without namespace",
			mutate: func(c *rm.Composition) {
				evaluationOf(c).Subject = &rm.PartySelf{ExternalRef: refWithoutNamespace()}
			},
			want: []string{"rm_invariant /content[0]/subject/external_ref/namespace"},
		},
		{
			name:   "ENTRY subject PARTY_SELF in value form",
			mutate: func(c *rm.Composition) { evaluationOf(c).Subject = rm.PartySelf{} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := partyProxyComposition()
			tc.mutate(c)
			got := findingsOf(validation.ValidateRM(c))
			if want := sortedFindings(tc.want); !slices.Equal(got, want) {
				t.Errorf("ValidateRM(COMPOSITION, %s) findings = %q, want %q", tc.name, got, want)
			}
		})
	}
}

// TestREQ112_PartyProxyDetailsAreValueFree pins that the findings on a party
// proxy name the attribute and the RM rule, never a value the proxy holds
// (REQ-112, REQ-093). Three PARTY_RELATED proxies carry distinctive strings
// wherever a value can sit, and each draws findings at its own node. The
// composer has a name, a present but empty identifiers, an external_ref
// whose id is set and whose namespace is empty, and no relationship. The
// ENTRY subject has an empty name, one identifier with an id, and the same
// kind of external_ref. The participation performer has only a
// relationship, so it breaks Basic_validity. No marker may appear in the
// Path, Code, Detail or Severity of any finding.
func TestREQ112_PartyProxyDetailsAreValueFree(t *testing.T) {
	const (
		nameMarker         = "Dr Jones Marker"
		identifierMarker   = "IDENTIFIER-MARKER-4417"
		refMarker          = "REF-MARKER-9fcc1c70"
		relationshipMarker = "RELATIONSHIP-MARKER-mother"
	)
	refMarked := func() *rm.PartyRef {
		return &rm.PartyRef{ID: &rm.HierObjectID{Value: refMarker}, Type: "PERSON"}
	}
	c := partyProxyComposition()
	c.Composer = &rm.PartyRelated{
		Name:        new(nameMarker),
		Identifiers: []rm.DVIdentifier{},
		ExternalRef: refMarked(),
	}
	evaluationOf(c).Subject = &rm.PartyRelated{
		Name:         new(""),
		Identifiers:  []rm.DVIdentifier{{ID: identifierMarker}},
		ExternalRef:  refMarked(),
		Relationship: codedText(relationshipMarker, "10"),
	}
	c.Context.Participations[0].Performer = &rm.PartyRelated{Relationship: codedText(relationshipMarker, "10")}

	r := validation.ValidateRM(c)
	// Every rule must fire, or the check below would pass on a report that
	// holds none of them.
	want := sortedFindings([]string{
		"required /composer/relationship",
		"rm_invariant /composer/external_ref/namespace",
		"rm_invariant /composer/identifiers",
		"rm_invariant /content[0]/subject/external_ref/namespace",
		"rm_invariant /content[0]/subject/name",
		"rm_invariant /context/participations[0]/performer",
	})
	if got := findingsOf(r); !slices.Equal(got, want) {
		t.Fatalf("ValidateRM(COMPOSITION with marked party proxies) findings = %q, want %q", got, want)
	}
	markers := []string{nameMarker, identifierMarker, refMarker, relationshipMarker}
	for _, issue := range r.Issues {
		fields := []struct{ name, text string }{
			{name: "Path", text: issue.Path},
			{name: "Code", text: issue.Code},
			{name: "Detail", text: issue.Detail},
			{name: "Severity", text: issue.Severity.String()},
		}
		for _, f := range fields {
			for _, marker := range markers {
				if strings.Contains(f.text, marker) {
					t.Errorf("ValidateRM(COMPOSITION with marked party proxies): issue %s at %q has %s %q, which echoes the value %q", issue.Code, issue.Path, f.name, f.text, marker)
				}
			}
		}
	}
}

// TestREQ112_PartyProxyAndParticipationRoots checks the floor on a party
// proxy or a participation handed to ValidateRM as the root, in pointer and
// value form (REQ-112): the PARTY_IDENTIFIED rules fire at "/", the
// mandatory attributes are reported at their own paths, and a bare
// PARTY_SELF gives no finding.
func TestREQ112_PartyProxyAndParticipationRoots(t *testing.T) {
	cases := []struct {
		name string
		root any
		want []string
	}{
		{name: "empty PARTY_IDENTIFIED", root: &rm.PartyIdentified{}, want: []string{"rm_invariant /"}},
		{name: "empty PARTY_IDENTIFIED value", root: rm.PartyIdentified{}, want: []string{"rm_invariant /"}},
		{name: "PARTY_IDENTIFIED with a name", root: &rm.PartyIdentified{Name: new("Dr Jones")}},
		{
			name: "PARTY_IDENTIFIED with an empty name and empty identifiers",
			root: &rm.PartyIdentified{Name: new(""), Identifiers: []rm.DVIdentifier{}},
			want: []string{"rm_invariant /identifiers", "rm_invariant /name"},
		},
		{
			name: "PARTY_IDENTIFIED with an external_ref without namespace",
			root: &rm.PartyIdentified{ExternalRef: refWithoutNamespace()},
			want: []string{"rm_invariant /external_ref/namespace"},
		},
		{name: "empty PARTY_RELATED", root: &rm.PartyRelated{}, want: []string{"required /relationship", "rm_invariant /"}},
		{name: "empty PARTY_RELATED value", root: rm.PartyRelated{}, want: []string{"required /relationship", "rm_invariant /"}},
		{name: "bare PARTY_SELF", root: &rm.PartySelf{}},
		{name: "bare PARTY_SELF value", root: rm.PartySelf{}},
		{
			name: "PARTY_SELF value with an external_ref without namespace",
			root: rm.PartySelf{ExternalRef: refWithoutNamespace()},
			want: []string{"rm_invariant /external_ref/namespace"},
		},
		{name: "empty PARTICIPATION", root: &rm.Participation{}, want: []string{"required /function", "required /performer"}},
		{name: "empty PARTICIPATION value", root: rm.Participation{}, want: []string{"required /function", "required /performer"}},
		{
			name: "PARTICIPATION with an empty performer",
			root: &rm.Participation{Function: rm.DVText{Value: "assistant"}, Performer: &rm.PartyIdentified{}},
			want: []string{"rm_invariant /performer"},
		},
		{name: "complete PARTICIPATION", root: new(validParticipation())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findingsOf(validation.ValidateRM(tc.root))
			if want := sortedFindings(tc.want); !slices.Equal(got, want) {
				t.Errorf("ValidateRM(%T) findings = %q, want %q", tc.root, got, want)
			}
		})
	}
}

// TestREQ112_EHRStatusSubjectPartySelf checks the floor on an EHR_STATUS's
// value-typed PARTY_SELF subject (REQ-112): a bare subject gives no finding,
// and a subject whose external_ref breaks the OBJECT_REF rule is reported
// below /subject.
func TestREQ112_EHRStatusSubjectPartySelf(t *testing.T) {
	const statusID = "openEHR-EHR-EHR_STATUS.generic.v1"
	cases := []struct {
		name    string
		subject rm.PartySelf
		want    []string
	}{
		{name: "bare subject"},
		{
			name:    "subject with an external_ref without namespace",
			subject: rm.PartySelf{ExternalRef: refWithoutNamespace()},
			want:    []string{"rm_invariant /subject/external_ref/namespace"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := &rm.EHRStatus{
				ArchetypeNodeID:  statusID,
				Name:             rm.DVText{Value: "EHR Status"},
				ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: statusID}, RMVersion: "1.1.0"},
				Subject:          tc.subject,
				IsModifiable:     true,
				IsQueryable:      true,
			}
			got := findingsOf(validation.ValidateRMEHRStatus(status))
			if want := sortedFindings(tc.want); !slices.Equal(got, want) {
				t.Errorf("ValidateRMEHRStatus(%s) findings = %q, want %q", tc.name, got, want)
			}
		})
	}
}

// composerJSON returns an otherwise valid COMPOSITION body whose composer is
// the given member value, verbatim.
func composerJSON(composer string) string {
	return `{"_type":"COMPOSITION",
		"name":{"_type":"DV_TEXT","value":"Encounter"},
		"archetype_node_id":"openEHR-EHR-COMPOSITION.encounter.v1",
		"archetype_details":` + archetypedJSON("openEHR-EHR-COMPOSITION.encounter.v1") + `,
		"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},
		"territory":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"},"code_string":"NL"},
		"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"433"}},
		"composer":` + composer + `}`
}

// TestREQ112_PartyIdentifiedDecodedPresence checks that the PARTY_IDENTIFIED
// rules read presence from the decoded value (REQ-112): a decoded
// "name":"" or "identifiers":[] is present and empty and is reported, while
// an absent key and a JSON null both read as absent and are not. A body with
// every attribute absent or null breaks Basic_validity.
func TestREQ112_PartyIdentifiedDecodedPresence(t *testing.T) {
	const identifier = `[{"_type":"DV_IDENTIFIER","id":"123"}]`
	cases := []struct {
		name     string
		composer string
		want     []string
	}{
		{
			name:     "empty name",
			composer: `{"_type":"PARTY_IDENTIFIED","name":""}`,
			want:     []string{"rm_invariant /composer/name"},
		},
		{
			name:     "empty identifiers",
			composer: `{"_type":"PARTY_IDENTIFIED","name":"Dr Jones","identifiers":[]}`,
			want:     []string{"rm_invariant /composer/identifiers"},
		},
		{
			name:     "absent identifiers",
			composer: `{"_type":"PARTY_IDENTIFIED","name":"Dr Jones"}`,
		},
		{
			name:     "null identifiers",
			composer: `{"_type":"PARTY_IDENTIFIED","name":"Dr Jones","identifiers":null}`,
		},
		{
			name:     "absent name",
			composer: `{"_type":"PARTY_IDENTIFIED","identifiers":` + identifier + `}`,
		},
		{
			name:     "null name",
			composer: `{"_type":"PARTY_IDENTIFIED","name":null,"identifiers":` + identifier + `}`,
		},
		{
			name:     "every attribute absent",
			composer: `{"_type":"PARTY_IDENTIFIED"}`,
			want:     []string{"rm_invariant /composer"},
		},
		{
			name:     "every attribute null",
			composer: `{"_type":"PARTY_IDENTIFIED","name":null,"identifiers":null,"external_ref":null}`,
			want:     []string{"rm_invariant /composer"},
		},
		{
			name: "external_ref without namespace",
			composer: `{"_type":"PARTY_IDENTIFIED","external_ref":{"_type":"PARTY_REF",
				"id":{"_type":"HIER_OBJECT_ID","value":"9fcc1c70-6349-4a5c-a8f8-4cdbe4ad5b0f"},
				"namespace":"","type":"PERSON"}}`,
			want: []string{"rm_invariant /composer/external_ref/namespace"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var comp rm.Composition
			if err := json.Unmarshal([]byte(composerJSON(tc.composer)), &comp); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", tc.name, err)
			}
			got := findingsOf(validation.ValidateRM(&comp))
			if want := sortedFindings(tc.want); !slices.Equal(got, want) {
				t.Errorf("ValidateRM(decoded composer, %s) findings = %q, want %q", tc.name, got, want)
			}
		})
	}
}
