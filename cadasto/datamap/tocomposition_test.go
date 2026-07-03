package datamap

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-058 — write path: a datamap payload + its OPT produces a canonical RM
// COMPOSITION, and round-trips back through FromComposition without losing the
// supplied values. Uses bare at-code keys (the lookup falls back from
// "<at-code>|<label>" to bare "<at-code>").

func loadOPT(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	b, err := os.ReadFile("testdata/fixtures/" + name + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	opt, err := template.ParseOPT(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("ParseOPT(%s): %v", name, err)
	}
	return opt
}

func TestToCompositionRoundTrip(t *testing.T) {
	opt := loadOPT(t, "development-1")

	const archetypeID = "openEHR-EHR-OBSERVATION.development.v0"
	dm := map[string]any{
		"language":  "nl",
		"territory": "NL",
		"composer":  "Dr. Jansen",
		"context":   map[string]any{"start_time": "2026-02-01T09:30:00Z"},
		"content": map[string]any{
			archetypeID: map[string]any{ // bare-id key (no |label) — fallback lookup
				"events": []any{
					map[string]any{
						"time":   "2026-02-01T09:30:00Z",
						"at0004": "Patient developing normally", // DV_TEXT
						"at0006": 12.5,                          // DV_QUANTITY (short form)
						"at0008": 3,                             // DV_COUNT
						"at0013": true,                          // DV_BOOLEAN
					},
				},
			},
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}

	// Structural checks on the canonical composition.
	if comp["_type"] != "COMPOSITION" {
		t.Fatalf("_type: got %v", comp["_type"])
	}
	contentList, _ := comp["content"].([]any)
	if len(contentList) != 1 {
		t.Fatalf("content: want 1 entry, got %d", len(contentList))
	}
	entry := contentList[0].(map[string]any)
	if entry["_type"] != "OBSERVATION" || entry["archetype_node_id"] != archetypeID {
		t.Fatalf("entry: got _type=%v node=%v", entry["_type"], entry["archetype_node_id"])
	}

	// Round-trip back to datamap and confirm the values survived.
	dm2, err := FromComposition(nil, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	content2 := dm2["content"].(map[string]any)
	var root map[string]any
	for _, v := range content2 {
		root = v.(map[string]any)
	}
	if root == nil {
		t.Fatalf("round-trip content empty")
	}
	events := root["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("round-trip events: want 1, got %d", len(events))
	}
	ev := events[0].(map[string]any)
	if ev["time"] != "2026-02-01T09:30:00Z" {
		t.Errorf("round-trip time: got %v", ev["time"])
	}

	got := collectLeafValues(ev)
	assertContains(t, got, "Patient developing normally") // DV_TEXT
	assertContains(t, got, 12.5)                          // DV_QUANTITY magnitude
	assertContains(t, got, 3)                             // DV_COUNT magnitude (JSON number)
	assertContains(t, got, true)                          // DV_BOOLEAN
}

func TestToCompositionEncodesFeederAudit(t *testing.T) {
	opt := loadOPT(t, "development-1")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-02-01T09:30:00Z"},
		"feeder_audit": map[string]any{
			"originating_system_audit": map[string]any{"system_id": "Apple"},
			"originating_system_item_ids": []any{
				map[string]any{"assigner": "Macbook", "id": "C2400001", "issuer": "Apple", "type": "Ordernumber"},
			},
		},
		"content": map[string]any{},
	}
	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}
	fa, ok := comp["feeder_audit"].(map[string]any)
	if !ok {
		t.Fatal("composition mist feeder_audit (encoder dropt 't nog)")
	}
	if fa["_type"] != "FEEDER_AUDIT" {
		t.Errorf("feeder_audit _type = %v, want FEEDER_AUDIT", fa["_type"])
	}
	osa, _ := fa["originating_system_audit"].(map[string]any)
	if osa["_type"] != "FEEDER_AUDIT_DETAILS" || osa["system_id"] != "Apple" {
		t.Errorf("originating_system_audit = %v", osa)
	}
	ids, _ := fa["originating_system_item_ids"].([]any)
	if len(ids) != 1 {
		t.Fatalf("originating_system_item_ids: want 1, got %d", len(ids))
	}
	id0 := ids[0].(map[string]any)
	if id0["_type"] != "DV_IDENTIFIER" || id0["id"] != "C2400001" || id0["type"] != "Ordernumber" || id0["issuer"] != "Apple" || id0["assigner"] != "Macbook" {
		t.Errorf("DV_IDENTIFIER = %v", id0)
	}
}

func TestToCompositionOmitsFeederAuditWithoutSystemID(t *testing.T) {
	// FEEDER_AUDIT.originating_system_audit is RM-verplicht; zonder geldige
	// system_id laten we het hele attribuut weg i.p.v. een door de CDR
	// geweigerde body te bouwen.
	opt := loadOPT(t, "development-1")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-02-01T09:30:00Z"},
		"feeder_audit": map[string]any{
			"originating_system_item_ids": []any{map[string]any{"id": "X"}},
		},
		"content": map[string]any{},
	}
	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}
	if _, ok := comp["feeder_audit"]; ok {
		t.Error("feeder_audit zou weggelaten moeten zijn zonder originating_system_audit.system_id")
	}
}

func TestFeederAuditRoundTripOptIn(t *testing.T) {
	opt := loadOPT(t, "development-1")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-02-01T09:30:00Z"},
		"feeder_audit": map[string]any{
			"originating_system_audit": map[string]any{"system_id": "Apple"},
			"originating_system_item_ids": []any{
				map[string]any{"id": "C2400001", "type": "Ordernumber", "issuer": "Apple", "assigner": "Macbook"},
			},
		},
		"content": map[string]any{},
	}
	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}

	// Zonder optie: feeder_audit NIET in de decoded datamap (default short).
	plain, err := FromComposition(opt, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	if _, ok := plain["feeder_audit"]; ok {
		t.Error("feeder_audit zou afwezig moeten zijn zonder WithFeederAudit()")
	}

	// Met optie: round-trip terug naar de platte vorm.
	got, err := FromComposition(opt, comp, WithFeederAudit())
	if err != nil {
		t.Fatalf("FromComposition(WithFeederAudit): %v", err)
	}
	fa, ok := got["feeder_audit"].(map[string]any)
	if !ok {
		t.Fatal("feeder_audit ontbreekt met WithFeederAudit()")
	}
	ids, _ := fa["originating_system_item_ids"].([]any)
	if len(ids) != 1 {
		t.Fatalf("item_ids: %v", fa)
	}
	id0 := ids[0].(map[string]any)
	if id0["id"] != "C2400001" || id0["type"] != "Ordernumber" || id0["issuer"] != "Apple" || id0["assigner"] != "Macbook" {
		t.Errorf("round-trip id = %v", id0)
	}
	if osa, _ := fa["originating_system_audit"].(map[string]any); osa["system_id"] != "Apple" {
		t.Errorf("round-trip system_id = %v", fa["originating_system_audit"])
	}
}

func TestToCompositionRequiresStartTime(t *testing.T) {
	opt := loadOPT(t, "development-1")
	_, err := ToComposition(opt, map[string]any{"content": map[string]any{}})
	if err == nil {
		t.Error("expected error when context.start_time is missing")
	}
}

// actionRoot loads the minimal ACTION fixture and returns its content
// archetype root, for unit-testing encodeAction directly (no INSTRUCTION/
// activity scaffolding needed — ACTION.description is a plain ITEM_TREE).
func actionRoot(t *testing.T) contentRoot {
	t.Helper()
	opt := loadOPT(t, "minimal_action_2")
	root, _ := opt.Root().(template.ObjectNode)
	for _, r := range findContentArchetypeRoots(root) {
		if r.node.RMTypeName() == "ACTION" {
			return r
		}
	}
	t.Fatal("minimal_action_2 fixture: no ACTION content root found")
	return contentRoot{}
}

// PROBE-0553 proves REQ-0029 — encodeAction honors a payload-supplied
// current_state (enrollment lifecycle) and preserves the completed default.
func TestEncodeAction_CurrentStateFromPayload(t *testing.T) {
	out := map[string]any{}
	got, err := encodeAction(out, actionRoot(t), map[string]any{
		"current_state": map[string]any{"code": "245", "value": "active", "terminology": "openehr"},
	}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("encodeAction: %v", err)
	}
	ism := got["ism_transition"].(map[string]any)
	cs := ism["current_state"].(map[string]any)
	dv := cs["defining_code"].(map[string]any) // dvCodedText nests the code under defining_code
	if dv["code_string"] != "245" {
		t.Fatalf("current_state code = %v, want 245", dv["code_string"])
	}
}

// PROBE-0554 proves REQ-0029 — absent current_state → completed(532) default (backward compat).
func TestEncodeAction_DefaultCompletedPreserved(t *testing.T) {
	got, err := encodeAction(map[string]any{}, actionRoot(t), map[string]any{}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("encodeAction: %v", err)
	}
	ism := got["ism_transition"].(map[string]any)
	cs := ism["current_state"].(map[string]any)
	dv := cs["defining_code"].(map[string]any)
	if dv["code_string"] != "532" {
		t.Fatalf("default current_state code = %v, want 532", dv["code_string"])
	}
}

// PROBE-0569 proves REQ-0029 — encodeAction emits a payload-supplied
// careflow_step alongside current_state on the ism_transition.
func TestEncodeAction_CareflowStepFromPayload(t *testing.T) {
	got, err := encodeAction(map[string]any{}, actionRoot(t), map[string]any{
		"current_state": map[string]any{"code": "245", "value": "active", "terminology": "openehr"},
		"careflow_step": map[string]any{"code": "at0003", "value": "active", "terminology": "local"},
	}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("encodeAction: %v", err)
	}
	ism := got["ism_transition"].(map[string]any)
	step, ok := ism["careflow_step"].(map[string]any)
	if !ok {
		t.Fatalf("careflow_step missing on ism_transition: %v", ism)
	}
	dv := step["defining_code"].(map[string]any)
	if dv["code_string"] != "at0003" {
		t.Fatalf("careflow_step code = %v, want at0003", dv["code_string"])
	}
	// current_state must still be honored alongside careflow_step.
	cs := ism["current_state"].(map[string]any)
	if cs["defining_code"].(map[string]any)["code_string"] != "245" {
		t.Fatalf("current_state code = %v, want 245", cs["defining_code"])
	}
}

// PROBE-0570 proves REQ-0029 — a present-but-malformed current_state (missing
// code) is rejected by codedTextFromPayload and falls back to completed(532).
func TestEncodeAction_MalformedCurrentStateFallsBack(t *testing.T) {
	got, err := encodeAction(map[string]any{}, actionRoot(t), map[string]any{
		"current_state": map[string]any{"value": "active", "terminology": "openehr"}, // no code
	}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("encodeAction: %v", err)
	}
	ism := got["ism_transition"].(map[string]any)
	cs := ism["current_state"].(map[string]any)
	dv := cs["defining_code"].(map[string]any)
	if dv["code_string"] != "532" {
		t.Fatalf("malformed current_state code = %v, want 532 (default fallback)", dv["code_string"])
	}
}

// collectLeafValues gathers all scalar leaf values (recursing maps/slices) so
// the round-trip assertion is robust to the exact key labels.
func collectLeafValues(v any) []any {
	var out []any
	switch t := v.(type) {
	case map[string]any:
		for _, vv := range t {
			out = append(out, collectLeafValues(vv)...)
		}
	case []any:
		for _, vv := range t {
			out = append(out, collectLeafValues(vv)...)
		}
	default:
		out = append(out, v)
	}
	return out
}

func assertContains(t *testing.T, haystack []any, want any) {
	t.Helper()
	if slices.Contains(haystack, want) {
		return
	}
	t.Errorf("round-trip lost value %v (got leaves %v)", want, haystack)
}

func TestEncodeExpandedValue_DVIdentifierNoEmptyFields(t *testing.T) {
	// Cadasto weigert een DV_IDENTIFIER met lege issuer/assigner/type (400).
	// De encoder mag die dus NIET toevoegen: alleen meegegeven velden blijven.
	got := encodeExpandedValue(map[string]any{"rmType": "DV_IDENTIFIER", "id": "C2400002"})
	if got["_type"] != "DV_IDENTIFIER" || got["id"] != "C2400002" {
		t.Fatalf("base velden fout: %+v", got)
	}
	for _, f := range []string{"issuer", "assigner", "type"} {
		if _, ok := got[f]; ok {
			t.Errorf("veld %q zou afwezig moeten zijn (lege velden breken de CDR), kreeg %v", f, got[f])
		}
	}
	// Meegegeven waarden blijven wél behouden.
	got2 := encodeExpandedValue(map[string]any{"rmType": "DV_IDENTIFIER", "id": "X", "issuer": "GLIMS"})
	if got2["issuer"] != "GLIMS" {
		t.Errorf("issuer = %v, want GLIMS", got2["issuer"])
	}
	if _, ok := got2["assigner"]; ok {
		t.Errorf("assigner zou afwezig moeten zijn, kreeg %v", got2["assigner"])
	}
}

// REQ-058 — INSTRUCTION write path: protocol + payload-narrative. Regressie voor
// de bug waarbij encodeInstruction de protocol-ITEM_TREE wegliet (order-id/status
// verdwenen) en de payload-narrative negeerde (altijd template-term). Gebruikt de
// order-template Laboratorium opdracht.v1 (heeft een protocol-constraint at0008
// met at0010/at0127); development-3 heeft geen protocol-attribuut.
func TestToCompositionInstructionProtocolAndNarrative(t *testing.T) {
	opt := loadOPT(t, "laborder")

	const archetypeID = "openEHR-EHR-INSTRUCTION.request-lab_test.v1|Aanvraag laboratorium onderzoek"
	dm := map[string]any{
		"language":  "nl",
		"territory": "NL",
		"composer":  "Dr. Jansen",
		"context":   map[string]any{"start_time": "2026-06-03T08:30:00Z"},
		"content": map[string]any{
			archetypeID: map[string]any{
				"narrative": "Graag met spoed", // payload-narrative, niet de template-fallback
				"protocol": map[string]any{
					"at0010|Aanvrager ID":    "ORD-100", // order-id
					"at0127|Status aanvraag": "NW",      // status
				},
				"activities": []any{
					map[string]any{"at0121|Aanvraagde dienst": "Natrium"},
				},
			},
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}
	raw, _ := json.Marshal(comp)
	s := string(raw)

	// protocol overleeft (vóór de fix volledig gedropt voor INSTRUCTION)
	if !strings.Contains(s, "ORD-100") {
		t.Errorf("order-id (protocol at0010) niet ge-encodeerd:\n%s", s)
	}
	if !strings.Contains(s, `"NW"`) {
		t.Errorf("status (protocol at0127) niet ge-encodeerd:\n%s", s)
	}
	// payload-narrative gebruikt (niet de template-fallback "Instruction")
	if !strings.Contains(s, "Graag met spoed") {
		t.Errorf("payload-narrative niet gebruikt:\n%s", s)
	}
	// de aangevraagde bepaling overleeft
	if !strings.Contains(s, "Natrium") {
		t.Errorf("activity (at0121) niet ge-encodeerd:\n%s", s)
	}
}
