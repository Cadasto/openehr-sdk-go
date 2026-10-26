package simplified_test

// REQ-053, REQ-116 — the formats have no key for a node's name below the root,
// so a WithTemplate decode names each node it rebuilds from the template.
// Where the template constrains the name as a DV_CODED_TEXT and determines
// its code and its text, decode gives the node that coded name; everywhere
// else the node keeps its DV_TEXT name. These cases edit the name constraint
// of the sdk_crosscheck_values.v1 ELEMENT at0016, write the edited template's
// Medium skeleton as FLAT or STRUCTURED, decode it with the edited template,
// and check the name decode gives the node.

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// valuesTemplate is the template the coded-name cases edit.
const valuesTemplate = "sdk_crosscheck_values.v1"

// nameAttribute matches the one name attribute of valuesTemplate, the one on
// the ELEMENT at0016: a DV_CODED_TEXT child whose attributes end with its
// defining_code.
var nameAttribute = regexp.MustCompile(`(?s)<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>name</rm_attribute_name>.*?<rm_attribute_name>defining_code</rm_attribute_name>.*?</attributes>\s*</children>\s*</attributes>`)

// valuesEdit rewrites the OPT text of valuesTemplate.
type valuesEdit func(t *testing.T, opt string) string

// withName returns an edit that gives the ELEMENT at0016 a name attribute
// with the children children.
func withName(children ...string) valuesEdit {
	return func(t *testing.T, opt string) string {
		t.Helper()
		attr := optAttribute("C_SINGLE_ATTRIBUTE", "name", "1", "", children...)
		return strings.Replace(opt, matchOnce(t, nameAttribute, opt, valuesTemplate), attr, 1)
	}
}

// replacing returns an edit that replaces the one occurrence of old in the OPT
// with replacement.
func replacing(old, replacement string) valuesEdit {
	return func(t *testing.T, opt string) string {
		t.Helper()
		if n := strings.Count(opt, old); n != 1 {
			t.Fatalf("%q occurs %d times in %s, want once — fixture changed?", old, n, valuesTemplate)
		}
		return strings.Replace(opt, old, replacement, 1)
	}
}

// stringValue is a value attribute whose STRING the C_STRING body item
// constrains.
func stringValue(item string) string {
	return optAttribute("C_SINGLE_ATTRIBUTE", "value", "1", "",
		optChild("C_PRIMITIVE_OBJECT", "STRING", "1", "1", "", `<item xsi:type="C_STRING">`+item+`</item>`))
}

// pinnedValue is a value attribute that pins text: a closed C_STRING list of
// that one entry.
func pinnedValue(text string) string {
	return stringValue(`<list>` + text + `</list>`)
}

// codePhrase is a C_CODE_PHRASE child of the terminology terminologyID (none
// where it is "") that lists codes.
func codePhrase(terminologyID string, codes ...string) string {
	var body strings.Builder
	if terminologyID != "" {
		body.WriteString(`<terminology_id><value>` + terminologyID + `</value></terminology_id>`)
	}
	for _, c := range codes {
		body.WriteString(`<code_list>` + c + `</code_list>`)
	}
	return optChild("C_CODE_PHRASE", "CODE_PHRASE", "1", "1", "", body.String())
}

// definingCode is a defining_code attribute with the children children.
func definingCode(children ...string) string {
	return optAttribute("C_SINGLE_ATTRIBUTE", "defining_code", "1", "", children...)
}

// codedText is a DV_CODED_TEXT child, occurring once, with the attributes
// attrs.
func codedText(attrs ...string) string {
	return optStructure("DV_CODED_TEXT", "", attrs...)
}

// localCodes is a DV_CODED_TEXT child whose defining_code lists the local
// codes, after the attributes before.
func localCodes(before string, codes ...string) string {
	return codedText(before, definingCode(codePhrase("local", codes...)))
}

// codedNameJSON is the canonical JSON of a DV_CODED_TEXT name.
func codedNameJSON(value, code, terminologyID string) map[string]any {
	return map[string]any{
		"_type": "DV_CODED_TEXT",
		"value": value,
		"defining_code": map[string]any{
			"_type":          "CODE_PHRASE",
			"code_string":    code,
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": terminologyID},
		},
	}
}

// textNameJSON is the canonical JSON of a DV_TEXT name.
func textNameJSON(value string) map[string]any {
	return map[string]any{"_type": "DV_TEXT", "value": value}
}

// editedValues is valuesTemplate with edits applied: its Web Template, its
// compiled template, and the Medium skeleton it writes.
type editedValues struct {
	wt       *webtemplate.WebTemplate
	compiled *templatecompile.Compiled
	comp     *rm.Composition
}

// editValues builds valuesTemplate with edits applied in order, and writes its
// Medium skeleton, which need not validate: its name at at0016 has no FLAT
// key, so the decode never reads it.
func editValues(t *testing.T, edits ...valuesEdit) editedValues {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(valuesTemplate))
	if err != nil {
		t.Fatalf("read %s: %v", valuesTemplate, err)
	}
	opt := string(raw)
	for _, edit := range edits {
		opt = edit(t, opt)
	}
	parsed, err := template.ParseOPT(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ParseOPT(edited %s): %v", valuesTemplate, err)
	}
	wt, compiled := compileTemplate(t, valuesTemplate, parsed)
	composer := "Composer"
	comp, err := composition.NewSkeleton(t.Context(), compiled,
		composition.WithPolicy(instance.Medium),
		composition.WithComposer(&rm.PartyIdentified{Name: &composer}),
		composition.WithTerritory("NL"),
		composition.WithNow(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("NewSkeleton(edited %s): %v", valuesTemplate, err)
	}
	return editedValues{wt: wt, compiled: compiled, comp: comp}
}

// TestREQ053CodedNameRules — REQ-053, REQ-116: decode gives a node a coded
// name only where the template constrains the name as a DV_CODED_TEXT, through
// one child or alternatives that agree, whose one C_CODE_PHRASE lists one
// code, or several of which only one has the pinned text as its term text.
// The text is the pinned one, else the code's term text: the node's archetype
// term for a local code, its rubric for an openehr one. Everywhere else, a
// code or a text the template leaves open included, the node keeps the DV_TEXT
// name the index gives it. Where decode gives a coded name, the round trip
// validates against the edited template.
func TestREQ053CodedNameRules(t *testing.T) {
	cases := []struct {
		name  string
		edits []valuesEdit
		want  map[string]any
	}{
		{
			name:  "one listed code, no pinned text",
			edits: []valuesEdit{withName(localCodes("", "at0017"))},
			want:  codedNameJSON("First option", "at0017", "local"),
		},
		{
			name:  "one listed code and a pinned text",
			edits: []valuesEdit{withName(localCodes(pinnedValue("Second option"), "at0017"))},
			want:  codedNameJSON("Second option", "at0017", "local"),
		},
		{
			name:  "several codes, one with the pinned text as its term",
			edits: []valuesEdit{withName(localCodes(pinnedValue("First option"), "at0017", "at0018"))},
			want:  codedNameJSON("First option", "at0017", "local"),
		},
		{
			name:  "several codes, no pinned text",
			edits: []valuesEdit{withName(localCodes("", "at0017", "at0018"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a pinned text no listed code has",
			edits: []valuesEdit{withName(localCodes(pinnedValue("Third option"), "at0017", "at0018"))},
			want:  textNameJSON("Third option"),
		},
		{
			name: "a pinned text two listed codes have",
			edits: []valuesEdit{
				replacing(`<items id="text">First option</items>`, `<items id="text">Second option</items>`),
				withName(localCodes(pinnedValue("Second option"), "at0017", "at0018")),
			},
			want: textNameJSON("Second option"),
		},
		{
			name:  "an open list, which pins no text",
			edits: []valuesEdit{withName(localCodes(stringValue(`<list>First option</list><list_open>true</list_open>`), "at0017", "at0018"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a list of two texts, which pins none",
			edits: []valuesEdit{withName(localCodes(stringValue(`<list>First option</list><list>Second option</list>`), "at0017", "at0018"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a pattern that rejects the code's term text",
			edits: []valuesEdit{withName(localCodes(stringValue(`<pattern>Other.*</pattern>`), "at0017"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a code phrase that lists no code",
			edits: []valuesEdit{withName(localCodes(""))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a code phrase that names no terminology",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("", "248152002"))))},
			want:  textNameJSON("Coded name"),
		},
		{
			name: "an external reference set",
			edits: []valuesEdit{withName(codedText(definingCode(optChild("C_CODE_REFERENCE", "CODE_PHRASE", "1", "1", "",
				`<referenceSetUri>terminology:example?subset=names</referenceSetUri>`))))},
			want: textNameJSON("Coded name"),
		},
		{
			name:  "a local code with no term",
			edits: []valuesEdit{withName(localCodes("", "at0099"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name: "a local code whose term has no text",
			edits: []valuesEdit{
				replacing(`<items id="text">First option</items>`, `<items id="text"></items>`),
				withName(localCodes("", "at0017")),
			},
			want: textNameJSON("Coded name"),
		},
		{
			name:  "an openehr code",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("openehr", "433"))))},
			want:  codedNameJSON("event", "433", "openehr"),
		},
		{
			name:  "an openehr code that two groups give two rubrics",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("openehr", "532"))))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "an openehr code no group holds",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("openehr", "99999"))))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "an external code, its term under the code alone",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("SNOMED-CT", "248152002"))))},
			want:  codedNameJSON("Female", "248152002", "SNOMED-CT"),
		},
		{
			name: "an external code, its term under the terminology and the code",
			edits: []valuesEdit{
				replacing(`<term_definitions code="248152002">`, `<term_definitions code="SNOMED-CT::248152002">`),
				withName(codedText(definingCode(codePhrase("SNOMED-CT", "248152002")))),
			},
			want: codedNameJSON("Female", "248152002", "SNOMED-CT"),
		},
		{
			name:  "an external at-code, whose bare key names an archetype term",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("SNOMED-CT", "at0017"))))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "defining_code alternatives",
			edits: []valuesEdit{withName(codedText(definingCode(codePhrase("local", "at0017"), codePhrase("local", "at0018"))))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "name alternatives that agree",
			edits: []valuesEdit{withName(localCodes("", "at0017"), localCodes("", "at0017"))},
			want:  codedNameJSON("First option", "at0017", "local"),
		},
		{
			name:  "name alternatives that disagree",
			edits: []valuesEdit{withName(localCodes("", "at0017"), localCodes("", "at0018"))},
			want:  textNameJSON("Coded name"),
		},
		{
			name:  "a DV_TEXT name alternative",
			edits: []valuesEdit{withName(optStructure("DV_TEXT", ""), localCodes("", "at0017"))},
			want:  textNameJSON("Coded name"),
		},
	}
	for _, tc := range cases {
		e := editValues(t, tc.edits...)
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				back := bmiRoundTrip(t, format, e.comp, e.wt, simplified.WithTemplate(e.compiled), simplified.WithStrict())
				got, _ := nameAt(t, back, codedNameNode)
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("decoded name at %s = %s, want %s", codedNameNode, jsonText(got), jsonText(tc.want))
				}
				if tc.want["_type"] != "DV_CODED_TEXT" {
					return
				}
				for _, is := range validation.ValidateComposition(back, e.compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
			})
		}
	}
}

// TestREQ053OPTFreeDecodeGivesNoCodedName — REQ-053: without WithTemplate the
// decode has no template to name a node from, and the node whose name the
// template codes comes back with no name, as every node below the root does.
func TestREQ053OPTFreeDecodeGivesNoCodedName(t *testing.T) {
	wt, compiled := templateOf(t, valuesTemplate)
	comp := skeletonOf(t, compiled, instance.Medium)
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			doc := canonicalDoc(t, bmiRoundTrip(t, format, comp, wt))
			node, ok := nodeAt(doc, codedNameNode)
			if !ok {
				t.Fatalf("OPT-free decode has no node at %s — the body should keep it", codedNameNode)
			}
			if name, has := node["name"]; has {
				t.Errorf("OPT-free decoded name at %s = %s, want none", codedNameNode, jsonText(name))
			}
		})
	}
}

// TestREQ053CodedNameTheTemplateLeavesOpen — REQ-053: the Referral Request.v1
// annotation's name is a DV_CODED_TEXT whose code comes from an external
// reference set, so the template determines no code, and FLAT has no key for
// the name. Decode invents none: the node keeps its DV_TEXT name, and the
// round trip reports the name's type against the template. This is the
// registered format limit.
func TestREQ053CodedNameTheTemplateLeavesOpen(t *testing.T) {
	const annotation = "/context/other_context/items[openEHR-EHR-CLUSTER.annotations.v1]/items[at0001]"
	wt, compiled := templateOf(t, "Referral Request.v1")
	comp := skeletonOf(t, compiled, instance.Medium)
	if written, _ := nameAt(t, comp, annotation); written["_type"] != "DV_CODED_TEXT" {
		t.Fatalf("the written name at %s is %s, want a DV_CODED_TEXT — fixture changed?", annotation, jsonText(written))
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			if got, want := mustName(t, back, annotation), textNameJSON("Annotation"); !reflect.DeepEqual(got, want) {
				t.Errorf("decoded name at %s = %s, want %s", annotation, jsonText(got), jsonText(want))
			}
			var mismatch bool
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				if is.Code == "rm_type_mismatch" && is.Path == annotation+"/name" {
					mismatch = true
					continue
				}
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			if !mismatch {
				t.Errorf("ValidateComposition reports no rm_type_mismatch at %s/name, want the registered limit", annotation)
			}
		})
	}
}

// mustName returns the name of the node at path in comp's canonical JSON,
// failing the test where there is none.
func mustName(t *testing.T, comp *rm.Composition, path string) map[string]any {
	t.Helper()
	name, ok := nameAt(t, comp, path)
	if !ok {
		t.Fatalf("no named node at %s", path)
	}
	return name
}
