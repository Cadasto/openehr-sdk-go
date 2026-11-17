package simplified_test

// REQ-053, REQ-106 — a composition named by its template. The Web Template
// root's name is the name the template pins on the composition, where it pins
// one, so a FLAT body without `<root>/_name` decodes to that name, and a
// composition carrying it encodes without the key.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// vitalSignsPinnedName is the name the IDCR - Vital Signs Encounter.v1
// template pins on its composition, whose archetype term is "Encounter".
const vitalSignsPinnedName = "Vital Signs Observations"

// vitalSignsWT compiles the IDCR - Vital Signs Encounter.v1 corpus template
// and builds its Web Template.
func vitalSignsWT(t *testing.T) *webtemplate.WebTemplate {
	t.Helper()
	opt, err := readOPTFile(fixtures.TemplateOpt("IDCR - Vital Signs Encounter.v1"))
	if err != nil {
		t.Fatalf("parse IDCR - Vital Signs Encounter.v1 OPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile IDCR - Vital Signs Encounter.v1 OPT: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build IDCR - Vital Signs Encounter.v1 Web Template: %v", err)
	}
	return wt
}

// TestREQ053CompositionPinnedNameRoundTrips — REQ-053, REQ-106: decode of a
// FLAT body that spells no `<root>/_name` names the composition with the name
// its template pins, not the archetype's term, and encode of a composition
// carrying that name as a plain DV_TEXT writes no `<root>/_name`.
func TestREQ053CompositionPinnedNameRoundTrips(t *testing.T) {
	wt := vitalSignsWT(t)
	nameKey := wt.Tree.ID + "/_name"
	body, err := json.Marshal(map[string]any{"ctx/language": "en", "ctx/territory": "GB", "ctx/composer_name": "Composer"})
	if err != nil {
		t.Fatal(err)
	}
	comp, err := simplified.UnmarshalFlat(body, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat(%s): %v", body, err)
	}
	if got := comp.Name; got == nil || got.GetValue() != vitalSignsPinnedName {
		t.Errorf("decoded name = %#v, want %q", got, vitalSignsPinnedName)
	}

	comp.Name = &rm.DVText{Value: vitalSignsPinnedName}
	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var flat map[string]any
	if err := json.Unmarshal(data, &flat); err != nil {
		t.Fatalf("unmarshal FLAT: %v", err)
	}
	if v, ok := flat[nameKey]; ok {
		t.Errorf("FLAT %q = %#v, want no key for the name the template pins", nameKey, v)
	}
}

// TestREQ053CodedPinnedNameKeysTheElement — REQ-053, REQ-106, issue 455: the
// IDCR - Vital Signs Encounter.v1 template names the pulse rate ELEMENT with
// the local code at1027, whose term text is "Heart Rate", so its FLAT key is
// heart_rate, not the archetype term's rate. A strict decode with the template
// reads the heart_rate quantity and names the ELEMENT with that coded name.
func TestREQ053CodedPinnedNameKeysTheElement(t *testing.T) {
	const element = "/content[openEHR-EHR-SECTION.vital_signs.v1]/items[openEHR-EHR-OBSERVATION.pulse.v1]/data/events[at0003]/data/items[at0004]"
	wt, compiled := templateOf(t, "IDCR - Vital Signs Encounter.v1")
	key := wt.Tree.ID + "/vital_signs/pulse_heart_beat/heart_rate"
	body, err := json.Marshal(map[string]any{
		"ctx/language": "en", "ctx/territory": "GB", "ctx/composer_name": "Composer",
		key + "|magnitude": 72, key + "|unit": "/min",
	})
	if err != nil {
		t.Fatal(err)
	}
	comp, err := simplified.UnmarshalFlat(body, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(%s, WithStrict) = %v, want the composition", body, err)
	}
	node, ok := nodeAt(canonicalDoc(t, comp), element)
	if !ok {
		t.Fatalf("decoded composition has no ELEMENT at %s", element)
	}
	if want := codedNameJSON("Heart Rate", "at1027", "local"); !reflect.DeepEqual(node["name"], want) {
		t.Errorf("ELEMENT name = %s, want %s", jsonText(node["name"]), jsonText(want))
	}
	value, _ := node["value"].(map[string]any)
	if value["magnitude"] != 72.0 || value["units"] != "/min" {
		t.Errorf("ELEMENT value = %s, want 72 /min", jsonText(value))
	}
}
