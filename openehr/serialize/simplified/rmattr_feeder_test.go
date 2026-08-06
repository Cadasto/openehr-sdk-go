package simplified

// REQ-140 — the `_feeder_audit` family: FEEDER_AUDIT decomposed into its two
// DV_IDENTIFIER lists, its one or two FEEDER_AUDIT_DETAILS sub-objects (each with
// three party positions), and the DV_ENCAPSULATED `original_content` choice.
//
// Every fixture below is copied from the pinned PROBE-086 corpus bodies
// (`ehrbase_conformance_Element_feeder_audit.json`,
// `…_feeder_audit_multimedia.json`, `…_party_related.json`,
// `…_party_self.json`), so the spellings under test are the reference
// implementation's (ADR 0014, design constraint 6). The family appears in 14 of
// the 34 corpus fixtures, on every LOCATABLE kind the template carries.

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// feederAuditCorpusKeys is the `_feeder_audit` instance
// `ehrbase_conformance_Element_feeder_audit.json` hangs off its `dv_quantity`
// ELEMENT, verbatim, re-rooted at base.
func feederAuditCorpusKeys(base string) map[string]any {
	fa := base + "/_feeder_audit"
	keys := map[string]any{
		fa + "/original_content":                      "Hello world!",
		fa + "/original_content|formalism":            "text/plain",
		fa + "/feeder_system_item_id:0|id":            "id1",
		fa + "/feeder_system_item_id:0|issuer":        "issuer1",
		fa + "/feeder_system_item_id:0|assigner":      "assigner1",
		fa + "/feeder_system_item_id:0|type":          "PERSON",
		fa + "/feeder_system_item_id:1|id":            "id2",
		fa + "/feeder_system_item_id:1|issuer":        "issuer2",
		fa + "/feeder_system_item_id:1|assigner":      "assigner2",
		fa + "/feeder_system_item_id:1|type":          "PERSON",
		fa + "/originating_system_item_id:0|id":       "id1",
		fa + "/originating_system_item_id:0|issuer":   "issuer1",
		fa + "/originating_system_item_id:0|assigner": "assigner1",
		fa + "/originating_system_item_id:0|type":     "PERSON",
		fa + "/originating_system_item_id:1|id":       "id2",
		fa + "/originating_system_item_id:1|issuer":   "issuer2",
		fa + "/originating_system_item_id:1|assigner": "assigner2",
		fa + "/originating_system_item_id:1|type":     "PERSON",
	}
	for _, audit := range []string{"originating_system_audit", "feeder_system_audit"} {
		a := fa + "/" + audit
		keys[a+"|system_id"] = "orig"
		keys[a+"|version_id"] = "final"
		keys[a+"|time"] = "2021-12-21T16:02:58.0094262+01:00"
		for _, party := range []struct{ seg, id, name string }{
			{"location", "12342341", "Org 1"},
			{"provider", "456", "Per 1"},
			{"subject", "456", "Per 1"},
		} {
			p := a + "/" + party.seg
			keys[p+"|id"] = party.id
			keys[p+"|id_scheme"] = "NMC"
			keys[p+"|id_namespace"] = "uk.org.nmc"
			keys[p+"|name"] = party.name
		}
	}
	return keys
}

// TestFeederAuditRoundTrip — REQ-140. The whole corpus instance, byte-exact in
// both directions, on the collapsed ELEMENT the fixture hangs it off.
func TestFeederAuditRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, feederAuditCorpusKeys(rmattrElement))

	el := feederAuditElement(t, comp)
	fa := el.FeederAudit
	if fa == nil {
		t.Fatal("feeder_audit not decoded onto the ELEMENT")
	}
	if len(fa.OriginatingSystemItemIds) != 2 || fa.OriginatingSystemItemIds[1].ID != "id2" {
		t.Errorf("originating_system_item_ids = %+v", fa.OriginatingSystemItemIds)
	}
	if len(fa.FeederSystemItemIds) != 2 {
		t.Errorf("feeder_system_item_ids = %+v", fa.FeederSystemItemIds)
	}
	osa := fa.OriginatingSystemAudit
	if osa.SystemID != "orig" || osa.VersionID == nil || *osa.VersionID != "final" {
		t.Errorf("originating_system_audit = %+v", osa)
	}
	if osa.Time == nil || osa.Time.Value != "2021-12-21T16:02:58.0094262+01:00" {
		t.Errorf("originating_system_audit.time = %+v", osa.Time)
	}
	loc, ok := osa.Location.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("location = %T, want *rm.PartyIdentified", osa.Location)
	}
	if loc.Name == nil || *loc.Name != "Org 1" || loc.ExternalRef == nil {
		t.Errorf("location = %+v", loc)
	}
	if _, ok := osa.Subject.(*rm.PartyIdentified); !ok {
		t.Errorf("subject = %T, want *rm.PartyIdentified", osa.Subject)
	}
	if fa.FeederSystemAudit == nil {
		t.Error("feeder_system_audit not decoded")
	}
	parsable, ok := fa.OriginalContent.(*rm.DVParsable)
	if !ok {
		t.Fatalf("original_content = %T, want *rm.DVParsable", fa.OriginalContent)
	}
	if parsable.Value != "Hello world!" || parsable.Formalism != "text/plain" {
		t.Errorf("original_content = %+v", parsable)
	}
}

// TestFeederAuditOwners — REQ-140. `feeder_audit` is a LOCATABLE attribute, so the
// family reaches every LOCATABLE the walk produces: the corpus writes it on the
// composition root, the SECTION, all five ENTRY subtypes, the EVENT, the CLUSTER
// and the collapsed ELEMENT.
func TestFeederAuditOwners(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, base := range []string{
		rmattrRoot, rmattrSection, rmattrObs, rmattrAction, rmattrEvent,
		rmattrEvent + "/conformance_cluster", rmattrElement,
	} {
		t.Run(base, func(t *testing.T) {
			assertRMAttrRoundTrip(t, wt, feederAuditCorpusKeys(base))
		})
	}
}

// TestFeederAuditRefusedOnNonLocatable — REQ-140. EVENT_CONTEXT and ISM_TRANSITION
// are not LOCATABLEs and declare no `feeder_audit`, so the family is ErrUnknownPath
// there — read off the RM, not off a list.
func TestFeederAuditRefusedOnNonLocatable(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, base := range []string{rmattrRoot + "/context", rmattrAction + "/transition"} {
		t.Run(base, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
				base + "/_feeder_audit/originating_system_audit|system_id": "orig",
			}))
			if !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// TestFeederAuditMultimediaChoice — REQ-140. FEEDER_AUDIT.original_content is a
// DV_ENCAPSULATED, and the reference disambiguates the choice **by key name**:
// `original_content` is the DV_PARSABLE, `original_content_multimedia` the
// DV_MULTIMEDIA (`ehrbase_conformance_feeder_audit_multimedia.json`). Both
// directions.
func TestFeederAuditMultimediaChoice(t *testing.T) {
	wt, _ := conformanceWT(t)
	fa := rmattrObs + "/_feeder_audit"
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		fa + "/originating_system_audit|system_id":    "orig",
		fa + "/original_content_multimedia":           "http://med.tube.com/sample",
		fa + "/original_content_multimedia|mediatype": "video/H261",
		fa + "/original_content_multimedia|size":      504903212,
	})
	obs := firstObservation(t, comp)
	if obs.FeederAudit == nil {
		t.Fatal("feeder_audit not decoded")
	}
	mm, ok := obs.FeederAudit.OriginalContent.(*rm.DVMultimedia)
	if !ok {
		t.Fatalf("original_content = %T, want *rm.DVMultimedia", obs.FeederAudit.OriginalContent)
	}
	if mm.MediaType.CodeString != "video/H261" || mm.Size != 504903212 {
		t.Errorf("original_content_multimedia = %+v", mm)
	}
}

// TestFeederAuditBothContentSpellingsRefused — REQ-140. The two spellings address
// one RM attribute, so a body carrying both is a typed error rather than a silent
// pick.
func TestFeederAuditBothContentSpellingsRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	fa := rmattrObs + "/_feeder_audit"
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		fa + "/originating_system_audit|system_id":    "orig",
		fa + "/original_content":                      "Hello world!",
		fa + "/original_content|formalism":            "text/plain",
		fa + "/original_content_multimedia":           "http://med.tube.com/sample",
		fa + "/original_content_multimedia|mediatype": "video/H261",
		fa + "/original_content_multimedia|size":      1,
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "original_content_multimedia") {
		t.Errorf("err = %v, want it to name both spellings", err)
	}
}

// TestFeederAuditPartyRelatedSubject — REQ-140. The three party positions are the
// C2 party grammar at three more positions, PARTY_RELATED's `/relationship` and the
// nested `_identifier:N` included — the shapes `ehrbase_conformance_party_related`
// writes under its feeder audits.
func TestFeederAuditPartyRelatedSubject(t *testing.T) {
	wt, _ := conformanceWT(t)
	audit := rmattrObs + "/_feeder_audit/originating_system_audit"
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		audit + "|system_id":                        "orig",
		audit + "/subject|name":                     "Per 1",
		audit + "/subject/relationship|code":        "10",
		audit + "/subject/relationship|value":       "mother",
		audit + "/subject/relationship|terminology": "openehr",
		audit + "/provider|name":                    "Per 1",
		audit + "/provider/_identifier:0|id":        "55175056",
		audit + "/provider/_identifier:0|issuer":    "issuer",
		audit + "/provider/_identifier:0|assigner":  "assigner",
		audit + "/provider/_identifier:0|type":      "type",
	})
	osa := firstObservation(t, comp).FeederAudit.OriginatingSystemAudit
	related, ok := osa.Subject.(*rm.PartyRelated)
	if !ok {
		t.Fatalf("subject = %T, want *rm.PartyRelated", osa.Subject)
	}
	if related.Relationship.Value != "mother" {
		t.Errorf("relationship = %+v", related.Relationship)
	}
	prov, ok := osa.Provider.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("provider = %T", osa.Provider)
	}
	if len(prov.Identifiers) != 1 || prov.Identifiers[0].ID != "55175056" {
		t.Errorf("provider identifiers = %+v", prov.Identifiers)
	}
}

// TestFeederAuditPartySelfSubject — REQ-140 (**corpus correction**). The
// FEEDER_AUDIT_DETAILS `subject` is the one PARTY_PROXY position the reference
// spells PARTY_SELF **explicitly**: `ehrbase_conformance_party_self.json` writes
// `originating_system_audit/subject|_type: "PARTY_SELF"`. It has to — the
// attribute is RM-optional there, so absence already means absent, unlike the
// PARTICIPATION performer and the ENTRY `subject` where PARTY_SELF *is* the
// absence.
func TestFeederAuditPartySelfSubject(t *testing.T) {
	wt, _ := conformanceWT(t)
	audit := rmattrObs + "/_feeder_audit/originating_system_audit"
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		audit + "|system_id":     "orig",
		audit + "/subject|_type": "PARTY_SELF",
	})
	osa := firstObservation(t, comp).FeederAudit.OriginatingSystemAudit
	if _, ok := osa.Subject.(*rm.PartySelf); !ok {
		t.Fatalf("subject = %T, want *rm.PartySelf", osa.Subject)
	}
}

// TestFeederAuditTypeSuffixBounds — REQ-140. `|_type` is not a general party
// suffix: it carries PARTY_SELF and nothing else, it is legal only at the
// PARTY_PROXY-typed `subject`, and it may not stand beside party keys that would
// contradict it.
func TestFeederAuditTypeSuffixBounds(t *testing.T) {
	wt, _ := conformanceWT(t)
	audit := rmattrObs + "/_feeder_audit/originating_system_audit"
	for name, extra := range map[string]map[string]any{
		"other subtype":                  {audit + "/subject|_type": "PARTY_IDENTIFIED"},
		"beside a party key":             {audit + "/subject|_type": "PARTY_SELF", audit + "/subject|name": "Per 1"},
		"at a PARTY_IDENTIFIED position": {audit + "/provider|_type": "PARTY_SELF"},
		"at a standalone party":          {rmattrContext + "/_health_care_facility|_type": "PARTY_SELF"},
	} {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{audit + "|system_id": "orig"}
			maps.Copy(body, extra)
			if err := decodeRMAttrErr(t, wt, rmattrBody(body)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// TestFeederAuditOtherDetailsRefused — REQ-140. FEEDER_AUDIT_DETAILS.other_details
// is an ITEM_STRUCTURE the reference's suffix set has no channel for and the pinned
// corpus never writes; it is deferred as a typed refusal naming the key, not
// dropped.
func TestFeederAuditOtherDetailsRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	key := rmattrObs + "/_feeder_audit/originating_system_audit/other_details|x"
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrObs + "/_feeder_audit/originating_system_audit|system_id": "orig",
		key: "boom",
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "other_details") {
		t.Errorf("err = %v, want it to name other_details", err)
	}
}

// TestFeederAuditGrammarBounds — REQ-140. The family's grammar is closed: an own
// suffix (FEEDER_AUDIT has none — every attribute is a sub-path), an unknown
// sub-path, an unknown FEEDER_AUDIT_DETAILS suffix, an indexed audit, a sparse
// item-id sequence and a missing RM-mandatory `system_id` all refuse loudly,
// naming the offending FLAT key.
func TestFeederAuditGrammarBounds(t *testing.T) {
	wt, _ := conformanceWT(t)
	fa := rmattrObs + "/_feeder_audit"
	audit := fa + "/originating_system_audit"
	for name, tc := range map[string]struct {
		extra map[string]any
		names string
	}{
		"own suffix on the family": {
			extra: map[string]any{fa + "|typo": "x"}, names: fa + "|typo",
		},
		"bare family value": {
			extra: map[string]any{fa: "x"}, names: fa,
		},
		"unknown sub-path": {
			extra: map[string]any{fa + "/bogus|x": "y"}, names: fa + "/bogus|x",
		},
		"unknown audit suffix": {
			extra: map[string]any{audit + "|typo": "x"}, names: audit + "|typo",
		},
		"indexed audit": {
			extra: map[string]any{fa + "/originating_system_audit:1|system_id": "x"},
			names: "originating_system_audit",
		},
		"sparse item ids": {
			extra: map[string]any{fa + "/originating_system_item_id:1|id": "id2"},
			names: "originating_system_item_id",
		},
		"missing system_id": {
			extra: map[string]any{audit + "|version_id": "final"}, names: "system_id",
		},
		"missing originating audit": {
			extra: map[string]any{fa + "/feeder_system_audit|system_id": "orig"},
			names: "originating_system_audit",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{}
			if name != "missing system_id" && name != "missing originating audit" && name != "indexed audit" {
				body[audit+"|system_id"] = "orig"
			}
			maps.Copy(body, tc.extra)
			err := decodeRMAttrErr(t, wt, rmattrBody(body))
			if !errors.Is(err, ErrUnsupportedDatatype) && !errors.Is(err, ErrUnknownPath) {
				t.Fatalf("err = %v, want a typed refusal", err)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("err = %v, want it to name %q", err, tc.names)
			}
		})
	}
}

// TestFeederAuditOriginalContentBeyondGrammarRidesRaw — REQ-140. Neither
// `original_content` spelling carries the DV_ENCAPSULATED CODE_PHRASE members as
// sub-paths (the corpus never writes them there), so a charset or a language on
// the encapsulated value rides that key's own `|raw` — lossless, and the shape
// stays unwritten.
func TestFeederAuditOriginalContentBeyondGrammarRidesRaw(t *testing.T) {
	out := map[string]any{}
	fa := &rm.FeederAudit{
		OriginatingSystemAudit: rm.FeederAuditDetails{SystemID: "orig"},
		OriginalContent: &rm.DVParsable{
			Value: "Hello world!", Formalism: "text/plain",
			Charset: &rm.CodePhrase{CodeString: "UTF-8"},
		},
	}
	if err := feederAuditRMAttr(out, "p/_feeder_audit", fa); err != nil {
		t.Fatalf("feederAuditRMAttr: %v", err)
	}
	if _, raw := out["p/_feeder_audit/original_content|raw"]; !raw {
		t.Errorf("a charset-carrying original_content should ride |raw, got %#v", out)
	}
}

// TestFeederAuditOtherDetailsRefusedOnEncode — REQ-140. The deferral is symmetric:
// a populated `other_details` is a typed error on the way out, never a silent drop.
func TestFeederAuditOtherDetailsRefusedOnEncode(t *testing.T) {
	out := map[string]any{}
	fa := &rm.FeederAudit{OriginatingSystemAudit: rm.FeederAuditDetails{
		SystemID:     "orig",
		OtherDetails: &rm.ItemTree{},
	}}
	err := feederAuditRMAttr(out, "p/_feeder_audit", fa)
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "other_details") {
		t.Errorf("err = %v, want it to name other_details", err)
	}
}

// TestRMAttrProviderRoundTrip — REQ-140. ENTRY.provider is the second RM-optional
// PARTY_PROXY position, so it rides the same grammar as the FEEDER_AUDIT_DETAILS
// subject: the party keys for an identified provider, `|_type` for a PARTY_SELF one.
// The corpus writes a bare `|name` on `ehrbase_conformance_observation`.
func TestRMAttrProviderRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/_provider|name": "Dr. Marcus Johnson",
	})
	prov, ok := firstObservation(t, comp).Provider.(*rm.PartyIdentified)
	if !ok {
		t.Fatalf("provider = %T, want *rm.PartyIdentified", firstObservation(t, comp).Provider)
	}
	if prov.Name == nil || *prov.Name != "Dr. Marcus Johnson" {
		t.Errorf("provider = %+v", prov)
	}

	self := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrObs + "/_provider|_type": "PARTY_SELF",
	})
	if _, ok := firstObservation(t, self).Provider.(*rm.PartySelf); !ok {
		t.Errorf("provider = %T, want *rm.PartySelf", firstObservation(t, self).Provider)
	}
}

// TestRMAttrProviderOwnerAdmission — REQ-140. `provider` is declared on ENTRY, so
// the family reaches the five ENTRY subtypes and nothing else — read off the RM.
func TestRMAttrProviderOwnerAdmission(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, base := range []string{rmattrRoot, rmattrSection, rmattrElement, rmattrEvent} {
		t.Run(base, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{base + "/_provider|name": "x"}))
			if !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// feederAuditElement returns the collapsed ELEMENT the corpus hangs its
// `_feeder_audit` off.
func feederAuditElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	obs := firstObservation(t, comp)
	for _, ev := range obs.Data.Events {
		pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
		if !ok {
			continue
		}
		tree, ok := pe.Data.(*rm.ItemTree)
		if !ok {
			continue
		}
		for _, it := range tree.Items {
			if el, ok := it.(*rm.Element); ok && el.FeederAudit != nil {
				return el
			}
		}
	}
	t.Fatal("no ELEMENT carrying a feeder_audit found")
	return nil
}
