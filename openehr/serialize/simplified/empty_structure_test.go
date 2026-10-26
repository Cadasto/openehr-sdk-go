package simplified_test

// REQ-053 — the formats have no key for a structure with no member, so a
// WithTemplate decode rebuilds one, empty, under a node the body rebuilt,
// where the template or the RM requires the attribute that holds it and the
// template gives one structure there that it lets hold no member. These cases
// edit the sdk_crosscheck_structure.v1 OPT, decode the body its unedited
// Required skeleton writes (an event with no data, an observation with no
// protocol) against the edited template, and check what decode rebuilds.

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const (
	// structureTemplate is the template the cases edit.
	structureTemplate = "sdk_crosscheck_structure.v1"
	// structureObservation is the path of its OBSERVATION.
	structureObservation = "/content[openEHR-EHR-OBSERVATION.sdk_structure.v1]"
)

// optInterval is an AOM interval body from lower to upper, or with no upper
// bound where upper is "*".
func optInterval(lower, upper string) string {
	if upper == "*" {
		return `<lower_included>true</lower_included><upper_included>false</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>` + lower + `</lower>`
	}
	return `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>` + lower + `</lower><upper>` + upper + `</upper>`
}

// optChild is a child of the OPT type xsiType and the RM type rmType,
// occurring lower..upper times, with the node id nodeID (none where it is
// ""), and then body: its attributes, or a slot's includes.
func optChild(xsiType, rmType, lower, upper, nodeID, body string) string {
	return `<children xsi:type="` + xsiType + `"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences>` + optInterval(lower, upper) + `</occurrences><node_id>` + nodeID + `</node_id>` + body + `</children>`
}

// optStructure is a C_COMPLEX_OBJECT child of the RM type rmType, occurring
// once, with the node id nodeID and the attributes attrs.
func optStructure(rmType, nodeID string, attrs ...string) string {
	return optChild("C_COMPLEX_OBJECT", rmType, "1", "1", nodeID, strings.Join(attrs, ""))
}

// optAttribute is an attribute of the OPT type xsiType named name, with the
// existence lower..1, the children children, and then tail (a cardinality).
func optAttribute(xsiType, name, lower, tail string, children ...string) string {
	return `<attributes xsi:type="` + xsiType + `"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + optInterval(lower, "1") + `</existence>` + strings.Join(children, "") + tail + `</attributes>`
}

// optProtocol is the OBSERVATION's protocol attribute with the existence
// lower..1 and the children children.
func optProtocol(lower string, children ...string) string {
	return optAttribute("C_SINGLE_ATTRIBUTE", "protocol", lower, "", children...)
}

// protocolAttribute matches the protocol attribute of structureTemplate,
// which holds no attribute of its own.
var protocolAttribute = regexp.MustCompile(`(?s)<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>protocol</rm_attribute_name>.*?</attributes>`)

// eventDataLower matches the existence lower bound of the event's data
// attribute in structureTemplate (at0002 is the event).
var eventDataLower = regexp.MustCompile(`(?s)(<node_id>at0002</node_id>\s*<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>data</rm_attribute_name>\s*<existence>.*?<lower>)1(</lower>)`)

// eventContext matches the composition's EVENT_CONTEXT child in
// structureTemplate, which ends with its other_context; otherContext matches
// that other_context attribute, which holds an ITEM_TREE with no attribute,
// and otherContextLower its existence lower bound.
var (
	eventContext      = regexp.MustCompile(`(?s)<children xsi:type="C_COMPLEX_OBJECT">\s*<rm_type_name>EVENT_CONTEXT</rm_type_name>.*?<node_id>at0001</node_id>\s*</children>\s*</attributes>\s*</children>`)
	otherContext      = regexp.MustCompile(`(?s)<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>other_context</rm_attribute_name>.*?</attributes>`)
	otherContextLower = regexp.MustCompile(`(?s)(<rm_attribute_name>other_context</rm_attribute_name>\s*<existence>.*?<lower>)0(</lower>)`)
)

// optEdit rewrites the OPT text of structureTemplate.
type optEdit func(t *testing.T, opt string) string

// matchOnce returns the one match of re in text, named what for a failure.
func matchOnce(t *testing.T, re *regexp.Regexp, text, what string) string {
	t.Helper()
	m := re.FindAllString(text, -1)
	if len(m) != 1 {
		t.Fatalf("%s matches %d times in %s, want once — fixture changed?", re, len(m), what)
	}
	return m[0]
}

// replaceOnce returns an edit that replaces the one match of re in the OPT
// with repl, as regexp.ReplaceAllString expands it.
func replaceOnce(re *regexp.Regexp, repl string) optEdit {
	return func(t *testing.T, opt string) string {
		t.Helper()
		matchOnce(t, re, opt, structureTemplate)
		return re.ReplaceAllString(opt, repl)
	}
}

// withProtocol returns an edit that makes attrs the OBSERVATION's protocol
// attribute and whatever attributes follow it.
func withProtocol(attrs string) optEdit {
	return func(t *testing.T, opt string) string {
		t.Helper()
		return strings.Replace(opt, matchOnce(t, protocolAttribute, opt, structureTemplate), attrs, 1)
	}
}

// contextEdit rewrites the EVENT_CONTEXT child of structureTemplate.
type contextEdit func(t *testing.T, ctx string) string

// keepContext leaves the EVENT_CONTEXT as the template writes it, with an
// optional other_context.
func keepContext(_ *testing.T, ctx string) string { return ctx }

// requireOtherContext makes the EVENT_CONTEXT require its other_context.
func requireOtherContext(t *testing.T, ctx string) string {
	t.Helper()
	matchOnce(t, otherContextLower, ctx, "its EVENT_CONTEXT")
	return otherContextLower.ReplaceAllString(ctx, "${1}1${2}")
}

// dropOtherContext leaves the other_context out of the EVENT_CONTEXT.
func dropOtherContext(t *testing.T, ctx string) string {
	t.Helper()
	return strings.Replace(ctx, matchOnce(t, otherContext, ctx, "its EVENT_CONTEXT"), "", 1)
}

// withContextAlternatives returns an edit that gives the composition's context
// two EVENT_CONTEXT alternatives, the template's own rewritten by first and by
// second.
func withContextAlternatives(first, second contextEdit) optEdit {
	return func(t *testing.T, opt string) string {
		t.Helper()
		ctx := matchOnce(t, eventContext, opt, structureTemplate)
		return strings.Replace(opt, ctx, first(t, ctx)+second(t, ctx), 1)
	}
}

// editedStructureTemplate builds the Web Template and the compiled template of
// structureTemplate with edits applied in order.
func editedStructureTemplate(t *testing.T, edits ...optEdit) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(structureTemplate))
	if err != nil {
		t.Fatalf("read %s: %v", structureTemplate, err)
	}
	opt := string(raw)
	for _, edit := range edits {
		opt = edit(t, opt)
	}
	parsed, err := template.ParseOPT(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ParseOPT(edited %s): %v", structureTemplate, err)
	}
	return compileTemplate(t, structureTemplate, parsed)
}

// emptyStructureJSON is the canonical JSON of a structure with no member, as
// the generator writes it and decode rebuilds it.
func emptyStructureJSON(rmType, nodeID, name string) map[string]any {
	return map[string]any{
		"_type":             rmType,
		"archetype_node_id": nodeID,
		"name":              map[string]any{"_type": "DV_TEXT", "value": name},
	}
}

// TestREQ053EmptyStructureRebuildRules — REQ-053: decode rebuilds a structure
// with no member only where the template, or the RM, requires the attribute
// that holds it, and the template gives there one ITEM_STRUCTURE node, or
// alternatives that agree, that it lets hold no member and whose name it
// gives. It never rebuilds a slot, an archetype root, a node with no node id,
// a node of another class, nor an abstract one, nor one under a list, nor one
// where the nodes at the holder's path disagree. Where it rebuilds one, the
// round trip validates against the edited template.
func TestREQ053EmptyStructureRebuildRules(t *testing.T) {
	const (
		protocol    = structureObservation + "/protocol"
		eventData   = structureObservation + "/data/events[at0002]/data"
		contextData = "/context/other_context"
	)
	tree := func(attrs ...string) string { return optStructure("ITEM_TREE", "at0010", attrs...) }
	cases := []struct {
		name  string
		edits []optEdit
		path  string
		want  map[string]any // nil: decode rebuilds nothing at path
	}{
		{name: "a protocol the template leaves optional", path: protocol},
		{
			name: "a protocol the template requires", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree()))},
			want:  emptyStructureJSON("ITEM_TREE", "at0010", "Protocol"),
		},
		{
			name: "a required protocol whose name the template codes", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(optAttribute("C_SINGLE_ATTRIBUTE", "name", "1", "", localCodes("", "at0010")))))},
			want: map[string]any{
				"_type":             "ITEM_TREE",
				"archetype_node_id": "at0010",
				"name":              codedNameJSON("Protocol", "at0010", "local"),
			},
		},
		{
			name: "event data the RM alone requires", path: eventData,
			edits: []optEdit{replaceOnce(eventDataLower, "${1}0${2}")},
			want:  emptyStructureJSON("ITEM_TREE", "at0003", "Tree"),
		},
		{
			name: "an ITEM_SINGLE, whose item the RM requires", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optStructure("ITEM_SINGLE", "at0010")))},
		},
		{
			name: "a structure whose uid the template requires", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(optAttribute("C_SINGLE_ATTRIBUTE", "uid", "1", ""))))},
		},
		{
			name: "a structure whose required items ask for a member", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(optAttribute("C_MULTIPLE_ATTRIBUTE", "items", "1",
				`<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>`+optInterval("1", "*")+`</interval></cardinality>`))))},
		},
		{
			name: "alternatives that disagree on the type", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(), optStructure("ITEM_LIST", "at0010")))},
		},
		{
			name: "alternatives that agree", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(), tree()))},
			want:  emptyStructureJSON("ITEM_TREE", "at0010", "Protocol"),
		},
		{
			name: "an alternative the template does not allow", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", tree(), optChild("C_COMPLEX_OBJECT", "ITEM_LIST", "0", "0", "at0010", "")))},
			want:  emptyStructureJSON("ITEM_TREE", "at0010", "Protocol"),
		},
		{
			name: "a slot", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optChild("ARCHETYPE_SLOT", "ITEM_TREE", "1", "1", "at0010",
				`<includes><string_expression>archetype_id/value matches {/.*/}</string_expression></includes>`)))},
		},
		{
			name: "an archetype root", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optChild("C_ARCHETYPE_ROOT", "ITEM_TREE", "1", "1", "at0000",
				`<archetype_id><value>openEHR-EHR-ITEM_TREE.sdk_protocol.v1</value></archetype_id>`+
					`<term_definitions code="at0000"><items id="text">Protocol</items></term_definitions>`)))},
		},
		{
			name: "a structure with no node id", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optStructure("ITEM_TREE", "")))},
		},
		{
			name: "an ELEMENT", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optStructure("ELEMENT", "at0010")))},
		},
		{
			name: "the abstract ITEM_STRUCTURE", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optStructure("ITEM_STRUCTURE", "at0010")))},
		},
		{
			name: "a structure whose node id names no term", path: protocol,
			edits: []optEdit{withProtocol(optProtocol("1", optStructure("ITEM_TREE", "at0099")))},
		},
		{
			name: "a structure under a list", path: structureObservation + "/links",
			edits: []optEdit{withProtocol(optProtocol("0", tree()) +
				optAttribute("C_MULTIPLE_ATTRIBUTE", "links", "1", "", optStructure("ITEM_TREE", "at0010")))},
		},
		{
			name: "context alternatives that agree", path: contextData,
			edits: []optEdit{withContextAlternatives(requireOtherContext, requireOtherContext)},
			want:  emptyStructureJSON("ITEM_TREE", "at0001", "Tree"),
		},
		{
			name: "context alternatives, the first requiring other_context", path: contextData,
			edits: []optEdit{withContextAlternatives(requireOtherContext, keepContext)},
		},
		{
			name: "context alternatives, the second requiring other_context", path: contextData,
			edits: []optEdit{withContextAlternatives(keepContext, requireOtherContext)},
		},
		{
			name: "context alternatives, one with no other_context", path: contextData,
			edits: []optEdit{withContextAlternatives(dropOtherContext, requireOtherContext)},
		},
	}
	original, compiled := templateOf(t, structureTemplate)
	body, err := simplified.MarshalFlat(skeletonOf(t, compiled, instance.Required), original)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	for _, tc := range cases {
		wt, edited := editedStructureTemplate(t, tc.edits...)
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				back := decodeAs(t, format, body, wt, simplified.WithTemplate(edited), simplified.WithStrict())
				got, rebuilt := nodeAt(canonicalDoc(t, back), tc.path)
				if tc.want == nil {
					if rebuilt {
						t.Errorf("decode rebuilt %s at %s, want none", jsonText(got), tc.path)
					}
					return
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("decoded structure at %s = %s, want %s", tc.path, jsonText(got), jsonText(tc.want))
				}
				for _, is := range validation.ValidateComposition(back, edited).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
			})
		}
	}
}

// TestREQ053FloorKeepsNoStructureListNonEmpty — REQ-053: decode judges a
// structure's lists by the template alone, because the floor keeps no list of
// an ITEM_STRUCTURE non-empty once present. A list of one there would make
// the template's existence on it ask for a member, as the validator reads it,
// and decode would have to read it too.
func TestREQ053FloorKeepsNoStructureListNonEmpty(t *testing.T) {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default gives no class hierarchy")
	}
	for _, l := range rminfo.NonEmptyLists() {
		if conforms, _ := h.ConformsTo(l.Class, "ITEM_STRUCTURE"); conforms {
			t.Errorf("the floor keeps %s.%s non-empty (%s): decode's empty-structure rule must read it", l.Class, l.List, l.Invariant)
		}
	}
}
