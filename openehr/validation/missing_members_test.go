package validation_test

// missing_members_test.go: PROBE-110 and PROBE-081, REQ-112 § Value-typed
// mandatory presence, REQ-175 (wire.md § Missing members on decode), REQ-171
// § Surface and REQ-168. ValidateJSON and ValidateXML decode a document,
// read the members it leaves out as the codecs' WithMissing options report
// them, and validate the decoded value with that report in one walk: each
// missing member is `required` at its own issue path, the one finding there,
// with nothing reported below it, at every level of the document and in
// either format. WithMissing hands a caller's own report to the value-based
// entries with the same result, and the typed wrappers keep their signatures.
// The composition fixture mirrors the codecs' own missing_test.go fixtures
// member for member, and is otherwise valid, so each row's findings are the
// missing members alone.

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Shared fragments of the composition fixture, in canonical JSON.
const (
	mmLanguage = `{"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "ISO_639-1"}, "code_string": "en"}`
	mmTime     = `{"_type": "DV_DATE_TIME", "value": "2026-01-01T00:00:00Z"}`
)

// mmArchetyped returns a complete ARCHETYPED for the archetype id, in
// canonical JSON.
func mmArchetyped(id string) string {
	return `{"_type": "ARCHETYPED", "archetype_id": {"_type": "ARCHETYPE_ID", "value": "` + id + `"}, "rm_version": "1.1.0"}`
}

// mmCompositionJSON is a complete, valid COMPOSITION: every object carries
// every member the RM requires of it, and the rows of mmRows take members
// out of it. It is the canonical-JSON fixture of canjson's missing_test.go,
// with the OBSERVATION's archetype_details, which an ENTRY needs, and one
// item in the CLUSTER, so that its canonical-XML form, mmCompositionXML,
// which has no element for an empty list, is the same instance.
var mmCompositionJSON = `{
  "_type": "COMPOSITION",
  "name": {"_type": "DV_TEXT", "value": "encounter"},
  "archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
  "archetype_details": ` + mmArchetyped("openEHR-EHR-COMPOSITION.encounter.v1") + `,
  "language": ` + mmLanguage + `,
  "territory": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "ISO_3166-1"}, "code_string": "GB"},
  "category": {"_type": "DV_CODED_TEXT", "value": "event", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "433"}},
  "composer": {"_type": "PARTY_SELF"},
  "content": [{
    "_type": "OBSERVATION",
    "name": {"_type": "DV_TEXT", "value": "vitals"},
    "archetype_node_id": "openEHR-EHR-OBSERVATION.vitals.v1",
    "archetype_details": ` + mmArchetyped("openEHR-EHR-OBSERVATION.vitals.v1") + `,
    "language": ` + mmLanguage + `,
    "encoding": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "IANA_character-sets"}, "code_string": "UTF-8"},
    "subject": {"_type": "PARTY_SELF"},
    "data": {
      "_type": "HISTORY",
      "name": {"_type": "DV_TEXT", "value": "history"},
      "archetype_node_id": "at0001",
      "origin": ` + mmTime + `,
      "events": [{
        "_type": "POINT_EVENT",
        "name": {"_type": "DV_TEXT", "value": "event"},
        "archetype_node_id": "at0002",
        "time": ` + mmTime + `,
        "data": {
          "_type": "ITEM_TREE",
          "name": {"_type": "DV_TEXT", "value": "tree"},
          "archetype_node_id": "at0003",
          "items": [
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "weight"}, "archetype_node_id": "at0004", "value": {"_type": "DV_QUANTITY", "magnitude": 80.5, "units": "kg"}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "count"}, "archetype_node_id": "at0005", "value": {"_type": "DV_COUNT", "magnitude": 2}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "flag"}, "archetype_node_id": "at0006", "value": {"_type": "DV_BOOLEAN", "value": true}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "ratio"}, "archetype_node_id": "at0007", "value": {"_type": "DV_PROPORTION", "numerator": 1, "denominator": 2, "type": 0}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "score"}, "archetype_node_id": "at0008", "value": {"_type": "DV_ORDINAL", "value": 1, "symbol": {"_type": "DV_CODED_TEXT", "value": "mild", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "local"}, "code_string": "at0009"}}}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "note"}, "archetype_node_id": "openEHR-EHR-ELEMENT.note.v1", "archetype_details": ` + mmArchetyped("openEHR-EHR-ELEMENT.note.v1") + `, "value": {"_type": "DV_TEXT", "value": "n"}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "range"}, "archetype_node_id": "at0010", "value": {"_type": "DV_INTERVAL", "lower": {"_type": "DV_QUANTITY", "magnitude": 1, "units": "kg"}, "upper": {"_type": "DV_QUANTITY", "magnitude": 2, "units": "kg"}}},
            {"_type": "CLUSTER", "name": {"_type": "DV_TEXT", "value": "cluster"}, "archetype_node_id": "at0011", "items": [
              {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "inner"}, "archetype_node_id": "at0012", "value": {"_type": "DV_TEXT", "value": "i"}}
            ]}
          ]
        }
      }]
    }
  }]
}`

// Shared fragments of the composition fixture, in canonical XML.
const (
	mmNS         = `xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`
	mmLanguageXM = `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`
	mmTimeXML    = `<value>2026-01-01T00:00:00Z</value>`
)

// mmArchetypedXML returns a complete archetype_details element for the
// archetype id.
func mmArchetypedXML(id string) string {
	return `<archetype_details><archetype_id><value>` + id + `</value></archetype_id><rm_version>1.1.0</rm_version></archetype_details>`
}

// mmCompositionXML is mmCompositionJSON in canonical XML, member for member:
// canxml's missing_test.go fixture with the same two additions.
var mmCompositionXML = `<composition ` + mmNS + ` archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1">
  <name><value>encounter</value></name>
  ` + mmArchetypedXML("openEHR-EHR-COMPOSITION.encounter.v1") + `
  ` + mmLanguageXM + `
  <territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>
  <category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>
  <composer xsi:type="PARTY_SELF"/>
  <content xsi:type="OBSERVATION" archetype_node_id="openEHR-EHR-OBSERVATION.vitals.v1">
    <name><value>vitals</value></name>
    ` + mmArchetypedXML("openEHR-EHR-OBSERVATION.vitals.v1") + `
    ` + mmLanguageXM + `
    <encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>
    <subject xsi:type="PARTY_SELF"/>
    <data xsi:type="HISTORY" archetype_node_id="at0001">
      <name><value>history</value></name>
      <origin>` + mmTimeXML + `</origin>
      <events xsi:type="POINT_EVENT" archetype_node_id="at0002">
        <name><value>event</value></name>
        <time>` + mmTimeXML + `</time>
        <data xsi:type="ITEM_TREE" archetype_node_id="at0003">
          <name><value>tree</value></name>
          <items xsi:type="ELEMENT" archetype_node_id="at0004"><name><value>weight</value></name><value xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0005"><name><value>count</value></name><value xsi:type="DV_COUNT"><magnitude>2</magnitude></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0006"><name><value>flag</value></name><value xsi:type="DV_BOOLEAN"><value>true</value></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0007"><name><value>ratio</value></name><value xsi:type="DV_PROPORTION"><numerator>1</numerator><denominator>2</denominator><type>0</type></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0008"><name><value>score</value></name><value xsi:type="DV_ORDINAL"><value>1</value><symbol><value>mild</value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0009</code_string></defining_code></symbol></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="openEHR-EHR-ELEMENT.note.v1"><name><value>note</value></name>` + mmArchetypedXML("openEHR-EHR-ELEMENT.note.v1") + `<value xsi:type="DV_TEXT"><value>n</value></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0010"><name><value>range</value></name><value xsi:type="DV_INTERVAL"><lower xsi:type="DV_QUANTITY"><magnitude>1</magnitude><units>kg</units></lower><upper xsi:type="DV_QUANTITY"><magnitude>2</magnitude><units>kg</units></upper></value></items>
          <items xsi:type="CLUSTER" archetype_node_id="at0011"><name><value>cluster</value></name><items xsi:type="ELEMENT" archetype_node_id="at0012"><name><value>inner</value></name><value xsi:type="DV_TEXT"><value>i</value></value></items></items>
        </data>
      </events>
    </data>
  </content>
</composition>`

// The pointers of the fixture's objects the rows edit, as the codecs name
// them, and the issue paths the walk writes for the same objects.
const (
	mmEntry   = "/content/0"
	mmHistory = mmEntry + "/data"
	mmItems   = mmHistory + "/events/0/data/items/"

	mmEntryPath   = "/content[openEHR-EHR-OBSERVATION.vitals.v1]"
	mmHistoryPath = mmEntryPath + "/data"
	mmItemsPath   = mmHistoryPath + "/events[at0002]/data/items"
)

// rewriteJSON returns doc with the member at each pointer of edits replaced
// by the raw JSON text it maps to, or dropped when that text is empty, as
// canjson's missing_test.go rewrites its fixture. An edit whose pointer
// names no member of doc fails the test, since it would test nothing.
func rewriteJSON(t *testing.T, doc string, edits map[string]string) []byte {
	t.Helper()
	dec := jsontext.NewDecoder(strings.NewReader(doc))
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out)
	applied := map[string]bool{}
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("rewriteJSON: read: %v", err)
		}
		kind, n := dec.StackIndex(dec.StackDepth())
		if kind == '{' && n%2 == 1 { // tok is a member's name
			ptr := string(dec.StackPointer())
			if repl, ok := edits[ptr]; ok {
				applied[ptr] = true
				name := tok.String() // tok is void once the value is skipped
				if err := dec.SkipValue(); err != nil {
					t.Fatalf("rewriteJSON: skip the value of %s: %v", ptr, err)
				}
				if repl == "" {
					continue
				}
				if err := enc.WriteToken(jsontext.String(name)); err != nil {
					t.Fatalf("rewriteJSON: write %s: %v", name, err)
				}
				if err := enc.WriteValue(jsontext.Value(repl)); err != nil {
					t.Fatalf("rewriteJSON: write %s: %v", repl, err)
				}
				continue
			}
		}
		if err := enc.WriteToken(tok); err != nil {
			t.Fatalf("rewriteJSON: write %s: %v", tok, err)
		}
	}
	for p := range edits {
		if !applied[p] {
			t.Fatalf("rewriteJSON: %s names no member of the document; the edit would test nothing", p)
		}
	}
	return out.Bytes()
}

// mmXMLLists names the multi-valued members the XML fixture carries, whose
// child elements a pointer counts from 0 as the canonical-JSON form does.
var mmXMLLists = map[string]bool{"content": true, "events": true, "items": true}

// rewriteXML returns doc with the element at each pointer of edits replaced
// by the XML text it maps to, or dropped when that text is empty, as
// canxml's missing_test.go rewrites its fixture: a pointer is the element's
// in the canonical-JSON form of the instance. An edit whose pointer names no
// element of doc fails the test.
func rewriteXML(t *testing.T, doc string, edits map[string]string) []byte {
	t.Helper()
	type span struct {
		from, to int
		repl     string
	}
	type frame struct {
		pointer string
		counts  map[string]int
	}
	var spans []span
	stack := []frame{{counts: map[string]int{}}}
	applied := map[string]bool{}
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		from := int(dec.InputOffset())
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("rewriteXML: read: %v", err)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			parent := &stack[len(stack)-1]
			ptr := parent.pointer + "/" + tk.Name.Local
			if mmXMLLists[tk.Name.Local] {
				ptr += "/" + strconv.Itoa(parent.counts[tk.Name.Local])
				parent.counts[tk.Name.Local]++
			}
			if len(stack) == 1 {
				ptr = "" // the root element is the value itself
			}
			if repl, ok := edits[ptr]; ok {
				applied[ptr] = true
				if err := dec.Skip(); err != nil {
					t.Fatalf("rewriteXML: skip %s: %v", ptr, err)
				}
				spans = append(spans, span{from: from, to: int(dec.InputOffset()), repl: repl})
				continue
			}
			stack = append(stack, frame{pointer: ptr, counts: map[string]int{}})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	for p := range edits {
		if !applied[p] {
			t.Fatalf("rewriteXML: %s names no element of the document; the edit would test nothing", p)
		}
	}
	out := []byte(doc)
	for _, s := range slices.Backward(spans) {
		out = slices.Concat(out[:s.from], []byte(s.repl), out[s.to:])
	}
	return out
}

// mmRow is one edit of the composition fixture: the members it takes out of
// the JSON and the XML form, the pointers the codecs report for them, the
// issue paths at which the validators must report each as required, and the
// findings a node's own rule reports at the node's own path from the zero
// value of a member read as absent, which stay (also).
type mmRow struct {
	name      string
	jsonEdits map[string]string
	xmlEdits  map[string]string
	pointers  []string
	paths     []string
	also      []string
}

// dropped returns the edits that leave out the member at each pointer.
func dropped(pointers ...string) map[string]string {
	edits := make(map[string]string, len(pointers))
	for _, p := range pointers {
		edits[p] = ""
	}
	return edits
}

// mmRows puts a missing member everywhere PROBE-110's table reaches, one row
// each: the root, an ENTRY, a HISTORY, a data value of each value-typed
// kind, a nested ARCHETYPED's rm_version and archetype_id; the null and
// xsi:nil forms, which leave the same instance; and every member at once.
//
// A DV_PROPORTION with no denominator also draws its own Valid_denominator
// at its own path: that rule of the node reads the zero denominator, and a
// finding of a node's rule at the node's own path stays, as it does beside
// content the template refuses (REQ-171 § Finding identity). The member's
// path carries the one `required`.
func mmRows() []mmRow {
	row := func(name, pointer, path string) mmRow {
		return mmRow{name: name, jsonEdits: dropped(pointer), xmlEdits: dropped(pointer), pointers: []string{pointer}, paths: []string{path}}
	}
	note := mmItemsPath + "[openEHR-EHR-ELEMENT.note.v1]/archetype_details"
	ratio := row("DV_PROPORTION denominator", mmItems+"3/value/denominator", mmItemsPath+"[at0007]/value/denominator")
	ratio.also = []string{"rm_invariant " + mmItemsPath + "[at0007]/value rm DV_PROPORTION.Valid_denominator"}
	rows := []mmRow{
		row("root language", "/language", "/language"),
		row("ENTRY subject", mmEntry+"/subject", mmEntryPath+"/subject"),
		row("HISTORY origin", mmHistory+"/origin", mmHistoryPath+"/origin"),
		row("DV_QUANTITY magnitude", mmItems+"0/value/magnitude", mmItemsPath+"[at0004]/value/magnitude"),
		row("DV_COUNT magnitude", mmItems+"1/value/magnitude", mmItemsPath+"[at0005]/value/magnitude"),
		row("DV_BOOLEAN value", mmItems+"2/value/value", mmItemsPath+"[at0006]/value/value"),
		ratio,
		row("DV_ORDINAL symbol", mmItems+"4/value/symbol", mmItemsPath+"[at0008]/value/symbol"),
		row("ARCHETYPED rm_version", mmItems+"5/archetype_details/rm_version", note+"/rm_version"),
		row("ARCHETYPED archetype_id", mmItems+"5/archetype_details/archetype_id", note+"/archetype_id"),
		{
			name: "null and xsi:nil members",
			jsonEdits: map[string]string{
				mmEntry + "/subject":          "null",
				mmItems + "0/value/magnitude": "null",
				mmItems + "2/value/value":     "null",
			},
			xmlEdits: map[string]string{
				mmEntry + "/subject":          `<subject xsi:type="PARTY_SELF" xsi:nil="true"/>`,
				mmItems + "0/value/magnitude": `<magnitude xsi:nil="true"/>`,
				mmItems + "2/value/value":     `<value xsi:nil="true"/>`,
			},
			pointers: []string{mmItems + "0/value/magnitude", mmItems + "2/value/value", mmEntry + "/subject"},
			paths:    []string{mmItemsPath + "[at0004]/value/magnitude", mmItemsPath + "[at0006]/value/value", mmEntryPath + "/subject"},
		},
	}
	all := mmRow{name: "every member at once", jsonEdits: map[string]string{}, xmlEdits: map[string]string{}}
	for _, r := range rows[:len(rows)-1] {
		for p := range r.jsonEdits {
			all.jsonEdits[p], all.xmlEdits[p] = "", ""
		}
		all.pointers = append(all.pointers, r.pointers...)
		all.paths = append(all.paths, r.paths...)
		all.also = append(all.also, r.also...)
	}
	return append(rows, all)
}

// wantFindings returns the findings a row must draw, as identityKeys writes
// them: `required` on the RM layer, rule mandatory, at each of the row's
// paths, and the row's other findings.
func (row mmRow) wantFindings() []string {
	out := slices.Clone(row.also)
	for _, p := range row.paths {
		out = append(out, "required "+p+" rm mandatory")
	}
	slices.Sort(out)
	return out
}

// atOrBelow returns the issues of r at path or below it.
func atOrBelow(r validation.Result, path string) []string {
	var out []string
	for _, iss := range r.Issues {
		if rest, ok := strings.CutPrefix(iss.Path, path); ok && (rest == "" || rest[0] == '/' || rest[0] == '[') {
			out = append(out, iss.Code+" "+iss.Path)
		}
	}
	return out
}

// decodeJSONFixture decodes doc with the canonical-JSON codec's default
// decode, as a caller that validates afterwards does.
func decodeJSONFixture(t *testing.T, doc []byte) *rm.Composition {
	t.Helper()
	comp := new(rm.Composition)
	if err := canjson.Unmarshal(doc, comp); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}
	return comp
}

// TestValidateJSON_PROBE110_FixtureIsOneValidInstance pins the fixture the
// rows edit: its two forms decode to one instance, which both entries find
// valid, so every finding a row draws comes from what the row leaves out.
func TestValidateJSON_PROBE110_FixtureIsOneValidInstance(t *testing.T) {
	t.Parallel()
	fromJSON := decodeJSONFixture(t, []byte(mmCompositionJSON))
	fromXML := new(rm.Composition)
	if err := canxml.Unmarshal([]byte(mmCompositionXML), fromXML); err != nil {
		t.Fatalf("canxml.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(fromJSON, fromXML) {
		t.Fatalf("the JSON and XML fixtures decode to different instances:\n%#v\n%#v", fromJSON, fromXML)
	}
	for name, r := range map[string]validation.Result{
		"ValidateJSON": validation.ValidateJSON([]byte(mmCompositionJSON), new(rm.Composition), nil),
		"ValidateXML":  validation.ValidateXML([]byte(mmCompositionXML), new(rm.Composition), nil),
	} {
		if !r.OK || len(r.Issues) != 0 {
			t.Errorf("%s(fixture) = OK %t, issues %q; want OK with none", name, r.OK, identityKeys(r.Issues))
		}
	}
}

// TestValidateJSON_PROBE110_MissingMembersAreRequired is PROBE-110's
// validator half (REQ-112 § Value-typed mandatory presence, REQ-175). Each
// row leaves members out of the fixture, and ValidateJSON and ValidateXML
// each report `required`, rule mandatory, at each member's issue path, the
// one finding at that path or below it, and nothing else a node's own rule
// does not report at its own path ([mmRows]). The two formats
// give the same issues, root holds what the default decode gives, and
// New().Validate(root, nil, WithMissing(pointers)), with the pointers the
// codecs report written out by hand, returns the very result ValidateJSON
// returns.
func TestValidateJSON_PROBE110_MissingMembersAreRequired(t *testing.T) {
	t.Parallel()
	for _, row := range mmRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			jsonDoc := rewriteJSON(t, mmCompositionJSON, row.jsonEdits)
			xmlDoc := rewriteXML(t, mmCompositionXML, row.xmlEdits)
			jsonRoot, xmlRoot := new(rm.Composition), new(rm.Composition)
			rJ := validation.ValidateJSON(jsonDoc, jsonRoot, nil)
			rX := validation.ValidateXML(xmlDoc, xmlRoot, nil)

			if got, want := identityKeys(rJ.Issues), row.wantFindings(); !slices.Equal(got, want) {
				t.Errorf("ValidateJSON(%s) findings = %q, want %q", row.name, got, want)
			}
			for _, p := range row.paths {
				if got, want := atOrBelow(rJ, p), []string{"required " + p}; !slices.Equal(got, want) {
					t.Errorf("ValidateJSON(%s) reports %q at or below %s, want %q alone", row.name, got, p, want)
				}
			}
			if rJ.OK {
				t.Errorf("ValidateJSON(%s).OK = true, want false", row.name)
			}
			if got, want := identityKeys(rX.Issues), identityKeys(rJ.Issues); !slices.Equal(got, want) {
				t.Errorf("ValidateXML(%s) issues = %q, want ValidateJSON's %q", row.name, got, want)
			}

			def := decodeJSONFixture(t, jsonDoc)
			if !reflect.DeepEqual(jsonRoot, def) {
				t.Errorf("ValidateJSON(%s) left root\n%#v\nwant the default decode's\n%#v", row.name, jsonRoot, def)
			}
			byHand := validation.New().Validate(def, nil, validation.WithMissing(row.pointers))
			if !slices.Equal(byHand.Issues, rJ.Issues) || !reflect.DeepEqual(byHand.Coverage, rJ.Coverage) {
				t.Errorf("New().Validate(%s, nil, WithMissing(%q)) = %q %+v, want ValidateJSON's %q %+v", row.name, row.pointers, identityKeys(byHand.Issues), byHand.Coverage, identityKeys(rJ.Issues), rJ.Coverage)
			}
		})
	}
}

// TestValidateRM_PROBE110_WithoutTheReportNumbersValidateClean is
// PROBE-110's can-fail control (REQ-112 § Value-typed mandatory presence):
// with the report withheld, a document that leaves out a number or a
// boolean decodes to a value the Reference Model rules find valid, since the
// zero value is a valid one. That is the blind spot the report closes:
// ValidateJSON, which reads it, finds the same document not OK.
func TestValidateRM_PROBE110_WithoutTheReportNumbersValidateClean(t *testing.T) {
	t.Parallel()
	for _, row := range mmRows() {
		if !strings.Contains(row.name, "magnitude") && row.name != "DV_BOOLEAN value" {
			continue
		}
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			doc := rewriteJSON(t, mmCompositionJSON, row.jsonEdits)
			if r := validation.ValidateRM(decodeJSONFixture(t, doc)); !r.OK || len(r.Issues) != 0 {
				t.Errorf("ValidateRM(decoded %s) = OK %t, issues %q; want OK with none: the value alone cannot say the member is missing", row.name, r.OK, identityKeys(r.Issues))
			}
			if r := validation.ValidateJSON(doc, new(rm.Composition), nil); r.OK {
				t.Errorf("ValidateJSON(%s).OK = true, want false", row.name)
			}
		})
	}
}

// TestValidateJSON_PROBE081_EHRStatusSubjectInBothFormats is PROBE-081's
// subject arm in both formats (REQ-112): an EHR_STATUS that leaves its
// subject out is `required` at /subject, alone, and one with a bare
// PARTY_SELF draws nothing, though both decode to the same zero
// rm.PartySelf.
func TestValidateJSON_PROBE081_EHRStatusSubjectInBothFormats(t *testing.T) {
	t.Parallel()
	statusJSON := func(subject string) []byte {
		return []byte(`{"_type": "EHR_STATUS", "name": {"_type": "DV_TEXT", "value": "s"}, "archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
			"archetype_details": ` + mmArchetyped("openEHR-EHR-EHR_STATUS.generic.v1") + `,` + subject + ` "is_queryable": true, "is_modifiable": true}`)
	}
	statusXML := func(subject string) []byte {
		return []byte(`<ehr_status ` + mmNS + ` archetype_node_id="openEHR-EHR-EHR_STATUS.generic.v1"><name><value>s</value></name>` +
			mmArchetypedXML("openEHR-EHR-EHR_STATUS.generic.v1") + subject + `<is_queryable>true</is_queryable><is_modifiable>true</is_modifiable></ehr_status>`)
	}
	for _, tc := range []struct {
		name string
		r    validation.Result
		want []string
	}{
		{name: "JSON without subject", r: validation.ValidateJSON(statusJSON(""), new(rm.EHRStatus), nil), want: []string{"required /subject rm mandatory"}},
		{name: "XML without subject", r: validation.ValidateXML(statusXML(""), new(rm.EHRStatus), nil), want: []string{"required /subject rm mandatory"}},
		{name: "JSON with a bare subject", r: validation.ValidateJSON(statusJSON(`"subject": {"_type": "PARTY_SELF"},`), new(rm.EHRStatus), nil)},
		{name: "XML with a bare subject", r: validation.ValidateXML(statusXML(`<subject/>`), new(rm.EHRStatus), nil)},
	} {
		if got := identityKeys(tc.r.Issues); !slices.Equal(got, tc.want) {
			t.Errorf("%s: issues = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestValidateJSON_PROBE110_TemplateReportsTheMissingMember pins the walk
// with a template (REQ-112 § Value-typed mandatory presence, REQ-102): the
// corpus vital-signs composition, whose systolic magnitude is out of the
// template's range, draws the constraint's out_of_range there; with that
// magnitude left out it draws `required` at the magnitude instead, and no
// out_of_range, its other findings unchanged. New().Validate with the
// codec's pointer, and package Validate with the same option, return the
// same result.
func TestValidateJSON_PROBE110_TemplateReportsTheMissingMember(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	raw, err := os.ReadFile(fixtures.CompositionJSON("vital_signs"))
	if err != nil {
		t.Fatalf("read vital_signs.json: %v", err)
	}
	const (
		pointer = "/content/0/data/events/0/data/items/0/value/magnitude"
		bp      = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items"
		temp    = "/content[openEHR-EHR-OBSERVATION.body_temperature.v1]/data/events[at0003]/data/item/value"
	)
	whole := validation.ValidateJSON(raw, new(rm.Composition), c)
	if !containsIssue(whole.Issues, bp+"[at0004]/value", "primitive_out_of_range") {
		t.Fatalf("precondition: ValidateJSON(vital_signs.json, c) issues = %q, want primitive_out_of_range at %s[at0004]/value", identityKeys(whole.Issues), bp)
	}
	doc := rewriteJSON(t, string(raw), dropped(pointer))
	r := validation.ValidateJSON(doc, new(rm.Composition), c)
	want := []string{
		"primitive_unit_unknown " + bp + "[at0005]/value template unit_unknown",
		"primitive_unit_unknown " + temp + " template unit_unknown",
		"required " + bp + "[at0004]/value/magnitude rm mandatory",
	}
	if got := identityKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateJSON(vital_signs.json without the systolic magnitude, c) issues = %q, want %q", got, want)
	}
	comp := decodeJSONFixture(t, doc)
	for name, other := range map[string]validation.Result{
		"New().Validate":   validation.New().Validate(comp, c, validation.WithMissing([]string{pointer})),
		"package Validate": validation.Validate(comp, c, validation.WithMissing([]string{pointer})),
	} {
		if !slices.Equal(other.Issues, r.Issues) {
			t.Errorf("%s(decoded, c, WithMissing(%s)) issues = %q, want ValidateJSON's %q", name, pointer, identityKeys(other.Issues), identityKeys(r.Issues))
		}
	}
	if without := validation.ValidateComposition(comp, c); containsIssue(without.Issues, bp+"[at0004]/value/magnitude", "required") {
		t.Errorf("ValidateComposition(decoded, c) reports required at the magnitude; want it silent there: it has no report")
	}
}

// TestValidateJSON_PROBE110_PastTheDepthLimitDrawsNothing pins the walk's
// limits over the report (REQ-112 § Value-typed mandatory presence, REQ-171
// § Coverage): a member the walk never reaches, below the depth limit of
// the Validator's own limits, draws nothing, a member above the limit is
// still reported, and the depth stop is reported once, where
// Coverage.Stopped lists it.
func TestValidateJSON_PROBE110_PastTheDepthLimitDrawsNothing(t *testing.T) {
	t.Parallel()
	deep := mmItemsPath + "[at0004]/value/magnitude"
	doc := rewriteJSON(t, mmCompositionJSON, dropped("/language", mmItems+"0/value/magnitude"))
	r := validation.New(validation.WithLimits(validation.Limits{Depth: 3})).ValidateJSON(doc, new(rm.Composition), nil)
	if !containsIssue(r.Issues, "/language", "required") {
		t.Errorf("ValidateJSON(Depth 3) issues = %q, want required at /language, above the limit", issueKeys(r.Issues))
	}
	if got := atOrBelow(r, deep); len(got) != 0 {
		t.Errorf("ValidateJSON(Depth 3) reports %q at or below %s, past the limit; want nothing", got, deep)
	}
	stops := pathsOf(r.Issues, "max_depth")
	if len(stops) != 1 || !slices.Equal(r.Coverage.Stopped, stops) {
		t.Errorf("ValidateJSON(Depth 3) max_depth at %q, Coverage.Stopped %q; want one stop, listed", stops, r.Coverage.Stopped)
	}
	if full := validation.ValidateJSON(doc, new(rm.Composition), nil); !containsIssue(full.Issues, deep, "required") {
		t.Errorf("precondition: ValidateJSON(default limits) issues = %q, want required at %s", issueKeys(full.Issues), deep)
	}
}

// TestWithMissing_PROBE110_ReadsTheReportOverTheValue pins that a reported
// member is read as absent whatever the Go value holds there (REQ-112 §
// Value-typed mandatory presence), and how a pointer reaches it: into a list
// by the item's position, into a Hash by the entry's key, unescaped as RFC
// 6901 says, so the entry keyed "de/x" is "de~1x". A pointer that names no
// attribute the walk visits, an item past the end of a list, a member no
// class has, the root itself, draws nothing.
func TestWithMissing_PROBE110_ReadsTheReportOverTheValue(t *testing.T) {
	t.Parallel()
	comp := decodeJSONFixture(t, []byte(mmCompositionJSON))
	language := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"}
	desc := &rm.ResourceDescription{
		LifecycleState: "published",
		OriginalAuthor: map[string]string{"name": "a"},
		Details: map[string]rm.ResourceDescriptionItem{
			"de/x": {Language: language, Purpose: "p"},
			"en":   {Language: language, Purpose: "q"},
		},
	}
	for _, tc := range []struct {
		name     string
		root     any
		pointers []string
		want     []string
	}{
		{
			name:     "a list item by its position",
			root:     comp,
			pointers: []string{mmEntry + "/subject", mmItems + "4/value/symbol"},
			want:     []string{"required " + mmItemsPath + "[at0008]/value/symbol", "required " + mmEntryPath + "/subject"},
		},
		{
			name:     "a Hash entry by its key",
			root:     desc,
			pointers: []string{"/details/de~1x/purpose"},
			want:     []string{"required /details[1]/purpose"},
		},
		{
			name:     "pointers that name nothing the walk visits",
			root:     comp,
			pointers: []string{"", "/", mmEntry + "/no_such_member", "/content/1/subject", "/details/en/purpose", "content/0/subject"},
		},
		{
			name: "the Hash with no report",
			root: desc,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if r := validation.ValidateRM(tc.root); len(r.Issues) != 0 {
				t.Fatalf("precondition: ValidateRM(%s) issues = %q, want none: the value holds every member", tc.name, issueKeys(r.Issues))
			}
			if got := issueKeys(validation.ValidateRM(tc.root, validation.WithMissing(tc.pointers)).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s, WithMissing(%q)) issues = %q, want %q", tc.name, tc.pointers, got, tc.want)
			}
		})
	}
}

// TestWithMissing_REQ112_OptionsReadAsTheUnion pins how a call reads its
// options (REQ-112 § Value-typed mandatory presence: a nil option, an empty
// list and a repeated option are read as the union of what was given): no
// option, a nil one, a nil list and an empty list leave the result as it is,
// and two lists, one repeating a pointer of the other, give what one list of
// their union gives, each member reported once.
func TestWithMissing_REQ112_OptionsReadAsTheUnion(t *testing.T) {
	t.Parallel()
	comp := decodeJSONFixture(t, []byte(mmCompositionJSON))
	base := validation.ValidateRM(comp)
	for name, r := range map[string]validation.Result{
		"a nil option":  validation.ValidateRM(comp, nil),
		"a nil list":    validation.ValidateRM(comp, validation.WithMissing(nil)),
		"an empty list": validation.New().Validate(comp, nil, validation.WithMissing([]string{})),
	} {
		if !slices.Equal(r.Issues, base.Issues) || !reflect.DeepEqual(r.Coverage, base.Coverage) {
			t.Errorf("ValidateRM(fixture, %s) = %q %+v, want what no option gives, %q %+v", name, issueKeys(r.Issues), r.Coverage, issueKeys(base.Issues), base.Coverage)
		}
	}
	a, b := "/language", mmItems+"0/value/magnitude"
	repeated := validation.ValidateRM(comp, validation.WithMissing([]string{a}), nil, validation.WithMissing([]string{b, a}))
	union := validation.ValidateRM(comp, validation.WithMissing([]string{a, b}))
	want := []string{"required " + mmItemsPath + "[at0004]/value/magnitude", "required /language"}
	if got := issueKeys(repeated.Issues); !slices.Equal(got, want) || !slices.Equal(repeated.Issues, union.Issues) {
		t.Errorf("ValidateRM(fixture, WithMissing(%q), nil, WithMissing(%q)) issues = %q, want %q, what one list of their union gives (%q)", []string{a}, []string{b, a}, got, want, issueKeys(union.Issues))
	}
}

// REQ-171 § Surface, REQ-112 § Value-typed mandatory presence: the typed
// wrappers take no option, for a value with no report, the two decode
// entries take the bytes, the root and a template, and the value-based
// entries take the trailing options. A change of signature fails to compile
// here.
var (
	_ func(*rm.Composition, *templatecompile.Compiled) validation.Result                   = validation.ValidateComposition
	_ func(rm.Party, *templatecompile.Compiled) validation.Result                          = validation.ValidateDemographic
	_ func(*rm.Folder, *templatecompile.Compiled) validation.Result                        = validation.ValidateFolder
	_ func(*rm.EHRStatus, *templatecompile.Compiled) validation.Result                     = validation.ValidateEHRStatus
	_ func(*rm.Folder) validation.Result                                                   = validation.ValidateRMFolder
	_ func(*rm.EHRStatus) validation.Result                                                = validation.ValidateRMEHRStatus
	_ func(*rm.EHRAccess) validation.Result                                                = validation.ValidateRMEHRAccess
	_ func(rm.Party) validation.Result                                                     = validation.ValidateRMDemographic
	_ func([]byte) validation.Result                                                       = validation.ValidateRMEHRStatusBytes
	_ func([]byte, any, *templatecompile.Compiled) validation.Result                       = validation.ValidateJSON
	_ func([]byte, any, *templatecompile.Compiled) validation.Result                       = validation.ValidateXML
	_ func(any, *templatecompile.Compiled, ...validation.ValidateOption) validation.Result = validation.Validate
	_ func(any, ...validation.ValidateOption) validation.Result                            = validation.ValidateRM
)

// rankedItemsOPT is a CLUSTER whose items take two ELEMENT at0001 siblings
// that pin no name, so an item ranks among both and binds by trial
// (REQ-102, binding items): the first is required once and
// admits a DV_COUNT magnitude from 5 to 10, the second is optional and
// admits one from 0 to 10. The CLUSTER's archetype_details is declared with
// no child, so the template walks it with the RM rules alone.
func rankedItemsOPT(t *testing.T) string {
	t.Helper()
	const (
		fiveToTen = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>5</lower><upper>10</upper></range>`
		zeroToTen = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>10</upper></range>`
		optional = `<lower>0</lower><upper>1</upper>`
		details  = `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE">`
	)
	opt := clusterItemsOPT(
		primitiveAttrChild("DV_COUNT", "magnitude", "INTEGER", "C_INTEGER", fiveToTen),
		primitiveAttrChild("DV_COUNT", "magnitude", "INTEGER", "C_INTEGER", zeroToTen),
	)
	opt = mustReplaceOnce(t, opt, optional, `<lower>1</lower><upper>1</upper>`)
	return mustReplaceOnce(t, opt, details, `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>archetype_details</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
    </attributes>
    `+details)
}

// TestValidate_PROBE110_TemplateTrialsReadTheReport pins that the report
// reaches every place the walk binds a value with a template (REQ-112 §
// Value-typed mandatory presence, REQ-171 § One walk): the trials that bind
// a list item among siblings of one rank read the item's magnitude as absent
// too, so it validates under neither sibling and binds to the first, which
// needs it; read from the Go value, the magnitude 0 would fail the first and
// bind the item to the second, leaving the first without its item. An
// attribute the template declares with no child is walked with the report
// as well.
func TestValidate_PROBE110_TemplateTrialsReadTheReport(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, rankedItemsOPT(t))
	cluster := func() *rm.Cluster {
		return &rm.Cluster{
			ArchetypeNodeID:  "openEHR-EHR-CLUSTER.items.v1",
			Name:             &rm.DVText{Value: "cluster"},
			ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.items.v1"}, RMVersion: "1.1.0"},
			Items:            []rm.Item{&rm.Element{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "count"}, Value: &rm.DVCount{Magnitude: 0}}},
		}
	}
	if got, want := issueKeys(validation.Validate(cluster(), c).Issues), []string{"cardinality /items[at0001]"}; !slices.Equal(got, want) {
		t.Fatalf("precondition: Validate(CLUSTER with magnitude 0, c) issues = %q, want %q: the item binds to the second sibling", got, want)
	}
	for _, tc := range []struct {
		pointer string
		want    []string
	}{
		{pointer: "/items/0/value/magnitude", want: []string{"required /items[at0001]/value/magnitude"}},
		{pointer: "/archetype_details/rm_version", want: []string{"cardinality /items[at0001]", "required /archetype_details/rm_version"}},
	} {
		r := validation.Validate(cluster(), c, validation.WithMissing([]string{tc.pointer}))
		if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
			t.Errorf("Validate(CLUSTER, c, WithMissing(%s)) issues = %q, want %q", tc.pointer, got, tc.want)
		}
	}
}

// TestValidate_PROBE110_ASharedItemIsTriedPerReport pins that a verdict a
// trial decides holds for one reading of the value only (REQ-171 § One walk,
// Trials: the same Go value read the same way): an ELEMENT held twice in a
// CLUSTER's items, whose magnitude the report names at the first place only,
// is read as absent there and as 7 at the second. At the first place it
// validates under neither sibling and binds to the first, whose range 0 to 4
// it would break; at the second it validates under the second sibling, whose
// range 5 to 10 admits 7, and binds there, drawing nothing. Were the verdicts
// of the first place reused at the second, the item would bind to the first
// sibling again and break its range.
func TestValidate_PROBE110_ASharedItemIsTriedPerReport(t *testing.T) {
	t.Parallel()
	const (
		zeroToFour = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>4</upper></range>`
		fiveToTen = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>5</lower><upper>10</upper></range>`
	)
	opt := clusterItemsOPT(
		primitiveAttrChild("DV_COUNT", "magnitude", "INTEGER", "C_INTEGER", zeroToFour),
		primitiveAttrChild("DV_COUNT", "magnitude", "INTEGER", "C_INTEGER", fiveToTen),
	)
	c := mustCompileInline(t, mustReplaceOnce(t, opt, `<lower>0</lower><upper>1</upper>`, `<lower>0</lower><upper>2</upper>`))
	shared := &rm.Element{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "count"}, Value: &rm.DVCount{Magnitude: 7}}
	cluster := &rm.Cluster{ArchetypeNodeID: "openEHR-EHR-CLUSTER.items.v1", Name: &rm.DVText{Value: "cluster"}, Items: []rm.Item{shared, shared}}
	r := validation.Validate(cluster, c, validation.WithMissing([]string{"/items/0/value/magnitude"}))
	if got, want := issueKeys(r.Issues), []string{"required /items[1]/value/magnitude"}; !slices.Equal(got, want) {
		t.Errorf("Validate(CLUSTER holding one ELEMENT twice, c, WithMissing(/items/0/value/magnitude)) issues = %q, want %q", got, want)
	}
}

// TestValidateJSON_REQ168_InputThatDoesNotDecode is the invalid_shape row of
// both decode entries (REQ-112 § Value-typed mandatory presence, REQ-168):
// input that does not decode into root is one `invalid_shape` at "/", an
// error with no layer or rule, whose Detail quotes nothing of the input and
// whose Value holds the decode error. JSON null raises no decode error, so
// its Value is empty. A member name given twice is refused as the canonical
// JSON codec refuses it, an error that is typereg.ErrInvalidShape.
func TestValidateJSON_REQ168_InputThatDoesNotDecode(t *testing.T) {
	t.Parallel()
	const marker = "MARKER-31337"
	for _, tc := range []struct {
		name      string
		r         validation.Result
		nullValue bool
		is        error
	}{
		{name: "malformed JSON", r: validation.ValidateJSON([]byte(`{"_type": "COMPOSITION", "name": "`+marker+`"`), new(rm.Composition), nil)},
		{name: "JSON null", r: validation.ValidateJSON([]byte(" null "), new(rm.Composition), nil), nullValue: true},
		{name: "a JSON array", r: validation.ValidateJSON([]byte(`["`+marker+`"]`), new(rm.Composition), nil)},
		{name: "a JSON string", r: validation.ValidateJSON([]byte(`"`+marker+`"`), new(rm.Composition), nil)},
		{name: "a JSON number", r: validation.ValidateJSON([]byte(`42`), new(rm.Composition), nil)},
		{name: "a value after the object", r: validation.ValidateJSON([]byte(mmCompositionJSON+` "`+marker+`"`), new(rm.Composition), nil)},
		{name: "a member name twice", r: validation.ValidateJSON([]byte(`{"_type": "DV_TEXT", "value": "`+marker+`", "value": "b"}`), new(rm.DVText), nil), is: typereg.ErrInvalidShape},
		{name: "a JSON magnitude that fails its decode", r: validation.ValidateJSON(ehrStatusWithQuotedMagnitude(marker), new(rm.EHRStatus), nil)},
		{name: "XML content after the root", r: validation.ValidateXML([]byte(`<dv_text `+mmNS+`><value>x</value></dv_text><extra>`+marker+`</extra>`), new(rm.DVText), nil)},
		{name: "XML content before the root", r: validation.ValidateXML([]byte(marker+`<dv_text `+mmNS+`><value>x</value></dv_text>`), new(rm.DVText), nil)},
		{name: "an XML root that fails its decode", r: validation.ValidateXML([]byte(`<dv_quantity `+mmNS+`><magnitude>`+marker+`</magnitude><units>kg</units></dv_quantity>`), new(rm.DVQuantity), nil)},
		{name: "malformed XML", r: validation.ValidateXML([]byte(`<dv_text `+mmNS+`><value>`+marker+`</value>`), new(rm.DVText), nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if len(tc.r.Issues) != 1 || tc.r.OK {
				t.Fatalf("%s: OK %t, issues %+v; want one invalid_shape and not OK", tc.name, tc.r.OK, tc.r.Issues)
			}
			iss := tc.r.Issues[0]
			if iss.Code != "invalid_shape" || iss.Path != "/" || iss.Severity != validation.Error || iss.Layer != validation.LayerNone || iss.Rule != "" {
				t.Errorf("%s: issue %s at %q, severity %s, layer %s, rule %q; want invalid_shape at \"/\", an error with no layer or rule", tc.name, iss.Code, iss.Path, iss.Severity, iss.Layer, iss.Rule)
			}
			assertValueFree(t, tc.r, marker)
			err, isErr := iss.Value.Reveal().(error)
			switch {
			case tc.nullValue && iss.Value.Reveal() != nil:
				t.Errorf("%s: Value.Reveal() = %v, want nothing: null raises no decode error", tc.name, iss.Value.Reveal())
			case !tc.nullValue && (!isErr || err == nil):
				t.Errorf("%s: Value.Reveal() = %T, want the decode error", tc.name, iss.Value.Reveal())
			case tc.is != nil && !errors.Is(err, tc.is):
				t.Errorf("%s: Value.Reveal() = %v, want an error that is %v", tc.name, err, tc.is)
			}
			if !reflect.DeepEqual(tc.r.Coverage, validation.Coverage{}) {
				t.Errorf("%s: Coverage = %+v, want the zero Coverage: nothing was walked", tc.name, tc.r.Coverage)
			}
		})
	}
}

// TestValidateJSON_REQ112_RootGuards pins the guards of both decode entries
// (REQ-112 § Value-typed mandatory presence): a nil root, untyped or typed,
// is `nil_root` with an empty path, and a root that is no registered RM
// value `rm_type_unknown` at "/", with or without a template, as ValidateRM
// reports them; the guard comes first, so input that would not decode is
// not read.
func TestValidateJSON_REQ112_RootGuards(t *testing.T) {
	t.Parallel()
	type notRM struct{ X int }
	c := mustCompile(t, "vital_signs")
	entries := map[string]func([]byte, any, *templatecompile.Compiled) validation.Result{
		"ValidateJSON": validation.ValidateJSON,
		"ValidateXML":  validation.ValidateXML,
	}
	for name, validate := range entries {
		for _, tc := range []struct {
			root       any
			code, path string
		}{
			{root: nil, code: "nil_root", path: ""},
			{root: (*rm.Composition)(nil), code: "nil_root", path: ""},
			{root: &notRM{}, code: "rm_type_unknown", path: "/"},
			{root: new(map[string]any), code: "rm_type_unknown", path: "/"},
		} {
			for _, tpl := range []*templatecompile.Compiled{nil, c} {
				r := validate([]byte("{<"), tc.root, tpl)
				if got, want := issueKeys(r.Issues), []string{tc.code + " " + tc.path}; !slices.Equal(got, want) || r.OK {
					t.Errorf("%s(malformed, %T, template %t) = OK %t, issues %q; want %q", name, tc.root, tpl != nil, r.OK, got, want)
				} else if !strings.HasPrefix(r.Issues[0].Detail, name+":") {
					t.Errorf("%s(%T) Detail = %q, want it to name the entry", name, tc.root, r.Issues[0].Detail)
				}
			}
		}
	}
}
