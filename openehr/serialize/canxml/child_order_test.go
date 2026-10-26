package canxml_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

func codedText(value, terminology, code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        value,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: terminology}, CodeString: code},
	}
}

func auditDetails() *rm.AuditDetails {
	return &rm.AuditDetails{
		SystemID:      "example.org",
		TimeCommitted: rm.DVDateTime{Value: "2026-10-26T10:00:00Z"},
		ChangeType:    codedText("creation", "openehr", "249"),
		Committer:     &rm.PartySelf{},
	}
}

// TestMarshalOriginalVersionChildOrder pins the child order of an
// ORIGINAL_VERSION with every element present: the VERSION elements first
// (contribution, commit_audit, signature), then the ORIGINAL_VERSION ones in
// the ITS-XML sequence, which puts data second and lifecycle_state last.
//
// REQ-056: the encoder writes a class's children in the order of its ITS-XML
// schema sequence.
func TestMarshalOriginalVersionChildOrder(t *testing.T) {
	signature := "c2lnbmF0dXJl"
	composition := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Language:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Territory:       rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}, CodeString: "GB"},
		Category:        codedText("event", "openehr", "433"),
		Composer:        &rm.PartySelf{},
	}
	version := &rm.OriginalVersion[*rm.Composition]{
		Contribution: &rm.ObjectRef{
			ID:        &rm.HierObjectID{Value: "0a4b6c8e-1f2d-4e3a-9b5c-7d6e8f9a0b1c"},
			Namespace: "local",
			Type:      "CONTRIBUTION",
		},
		CommitAudit:           auditDetails(),
		Signature:             &signature,
		UID:                   rm.ObjectVersionID{Value: "8849182c-82ad-4088-a07f-48ead4180515::example.org::2"},
		Data:                  &composition,
		PrecedingVersionUID:   &rm.ObjectVersionID{Value: "8849182c-82ad-4088-a07f-48ead4180515::example.org::1"},
		OtherInputVersionUids: []rm.ObjectVersionID{{Value: "5a1e2b3c-4d5e-4f60-8a7b-9c0d1e2f3a4b::example.org::1"}},
		Attestations: []rm.Attestation{{
			AuditDetails: *auditDetails(),
			Reason:       rm.DVText{Value: "signed"},
		}},
		LifecycleState: codedText("complete", "openehr", "532"),
	}

	got, err := canxml.Marshal(version)
	if err != nil {
		t.Fatalf("Marshal(ORIGINAL_VERSION): %v", err)
	}
	want := []string{
		"contribution", "commit_audit", "signature",
		"uid", "data", "preceding_version_uid", "other_input_version_uids", "attestations", "lifecycle_state",
	}
	if children := rootChildren(t, got); !slices.Equal(children, want) {
		t.Errorf("ORIGINAL_VERSION children = %q\nwant %q\ndocument:\n%s", children, want, got)
	}
}

// TestMarshalObservationChildOrder pins the child order of an OBSERVATION:
// the LOCATABLE elements, then ENTRY's, then CARE_ENTRY's, then the
// OBSERVATION's own, each block in its ITS-XML sequence. Before the schemas
// gave the order, protocol came before name.
//
// REQ-056: the encoder writes a class's children in the order of its ITS-XML
// schema sequence.
func TestMarshalObservationChildOrder(t *testing.T) {
	obs := &rm.Observation{
		ArchetypeNodeID:  "openEHR-EHR-OBSERVATION.body_weight.v2",
		Name:             rm.DVText{Value: "Body weight"},
		UID:              &rm.HierObjectID{Value: "9b2c3d4e-5f60-4a7b-8c9d-0e1f2a3b4c5d"},
		Links:            []rm.Link{{Meaning: rm.DVText{Value: "follow-up"}, Type: rm.DVText{Value: "issue"}, Target: rm.DVEHRURI{Value: "ehr://example/"}}},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.body_weight.v2"}, RMVersion: "1.1.0"},
		FeederAudit: &rm.FeederAudit{OriginatingSystemAudit: rm.FeederAuditDetails{
			SystemID: "lab.example.org",
		}},
		Language:            rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Encoding:            rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
		Subject:             &rm.PartySelf{},
		Provider:            &rm.PartySelf{},
		OtherParticipations: []rm.Participation{{Function: rm.DVText{Value: "witness"}, Performer: &rm.PartySelf{}}},
		WorkflowID:          &rm.ObjectRef{ID: &rm.HierObjectID{Value: "wf-1"}, Namespace: "local", Type: "WORKFLOW"},
		Protocol:            &rm.ItemTree{ArchetypeNodeID: "at0011", Name: rm.DVText{Value: "Protocol"}},
		GuidelineID:         &rm.ObjectRef{ID: &rm.HierObjectID{Value: "gl-1"}, Namespace: "local", Type: "GUIDELINE"},
		Data: rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0002",
			Name:            rm.DVText{Value: "History"},
			Origin:          rm.DVDateTime{Value: "2026-10-26T10:00:00Z"},
		},
		State: &rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0003",
			Name:            rm.DVText{Value: "State"},
			Origin:          rm.DVDateTime{Value: "2026-10-26T10:00:00Z"},
		},
	}

	got, err := canxml.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal(OBSERVATION): %v", err)
	}
	want := []string{
		"name", "uid", "links", "archetype_details", "feeder_audit",
		"language", "encoding", "subject", "provider", "other_participations", "workflow_id",
		"protocol", "guideline_id",
		"data", "state",
	}
	if children := rootChildren(t, got); !slices.Equal(children, want) {
		t.Errorf("OBSERVATION children = %q\nwant %q\ndocument:\n%s", children, want, got)
	}
}
