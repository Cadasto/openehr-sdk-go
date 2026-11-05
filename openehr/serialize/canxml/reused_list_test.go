package canxml_test

// reused_list_test.go: REQ-056 (wire.md § REQ-056) for a list member decoded
// into a value that already holds items. Canonical JSON replaces an array a
// document carries, so canonical XML replaces the list too: the member holds
// the items of the document's elements alone, and a member the document
// leaves out keeps what the value held, in both codecs.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// reusedListRow is one document and a value that already holds items of the
// list member the document carries, or leaves out.
type reusedListRow struct {
	name  string
	in    string
	stale func() any // a value holding an item the document does not carry
	fresh func() any // a zero value of the same type
}

func reusedListRows() []reusedListRow {
	stalePlain := func() *rm.DVText {
		return &rm.DVText{Value: "old", Mappings: []rm.TermMapping{{Match: "=", Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "1"}}}}
	}
	return []reusedListRow{
		{
			// A list of a primitive type, read element by element as text.
			name:  "a list of strings",
			in:    `<description ` + ucNS + `><original_author id="name">a</original_author><other_contributors>x</other_contributors><other_contributors>z</other_contributors><lifecycle_state>published</lifecycle_state></description>`,
			stale: func() any { return &rm.ResourceDescription{OtherContributors: []string{"y"}} },
			fresh: func() any { return new(rm.ResourceDescription) },
		},
		{
			// A list of a class with a decoder of its own.
			name:  "a list of structs",
			in:    `<value ` + ucNS + `><value>new</value><mappings><match>&gt;</match><target><terminology_id><value>ICD10</value></terminology_id><code_string>A1</code_string></target></mappings></value>`,
			stale: func() any { return stalePlain() },
			fresh: func() any { return new(rm.DVText) },
		},
		{
			// A list of an abstract class, dispatched on xsi:type.
			name: "a list of polymorphic items",
			in:   `<data ` + ucNS + ` archetype_node_id="at0003"><name><value>tree</value></name><items xsi:type="ELEMENT" archetype_node_id="at0005"><name><value>new</value></name></items></data>`,
			stale: func() any {
				return &rm.ItemTree{Items: []rm.Item{&rm.Element{ArchetypeNodeID: "at0004", Name: &rm.DVText{Value: "old"}}}}
			},
			fresh: func() any { return new(rm.ItemTree) },
		},
		{
			// An optional list, held as a pointer to a slice.
			name:  "an optional list",
			in:    `<children ` + ucNS + `><rm_type_name>CODE_PHRASE</rm_type_name><node_id/><terminology_id><value>local</value></terminology_id><code_list>at2</code_list></children>`,
			stale: func() any { return &aom14.CCodePhrase{CodeList: &[]string{"at1"}} },
			fresh: func() any { return new(aom14.CCodePhrase) },
		},
		{
			// The bytes of an xml processing instruction in a comment make a
			// decode given no option decode the document again, which keeps
			// what the document leaves out, as the first decode does.
			name:  "control: xml instruction bytes in a comment",
			in:    `<value ` + ucNS + `><!-- <?xml version="1.0"?> --><value>new</value></value>`,
			stale: func() any { return stalePlain() },
			fresh: func() any { return new(rm.DVText) },
		},
		{
			// A list member the document leaves out keeps the value's items.
			name:  "control: a list the document leaves out",
			in:    `<description ` + ucNS + `><original_author id="name">a</original_author><lifecycle_state>published</lifecycle_state></description>`,
			stale: func() any { return &rm.ResourceDescription{OtherContributors: []string{"y"}} },
			fresh: func() any { return new(rm.ResourceDescription) },
		},
	}
}

// TestREQ056_XMLListReplacesAReusedValue decodes each row's document into the
// row's stale value, through canxml.Unmarshal and a canxml.Decoder, and into
// the same stale value through canonical JSON, from the canonical-JSON form of
// the document's instance: the two codecs give one value, and a list the
// document carries holds its items alone.
func TestREQ056_XMLListReplacesAReusedValue(t *testing.T) {
	for _, row := range reusedListRows() {
		t.Run(row.name, func(t *testing.T) {
			fresh := row.fresh()
			if err := canxml.Unmarshal([]byte(row.in), fresh); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) into a zero value = %v; want nil", row.in, err)
			}
			doc, err := canjson.Marshal(fresh)
			if err != nil {
				t.Fatalf("canjson.Marshal(the decoded value) = %v; want nil", err)
			}
			fromJSON := row.stale()
			if err := canjson.Unmarshal(doc, fromJSON); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) into the stale value = %v; want nil", doc, err)
			}

			viaUnmarshal := row.stale()
			if err := canxml.Unmarshal([]byte(row.in), viaUnmarshal); err != nil {
				t.Fatalf("canxml.Unmarshal into the stale value = %v; want nil", err)
			}
			viaDecoder := row.stale()
			if err := canxml.NewDecoder(strings.NewReader(row.in)).Decode(viaDecoder); err != nil {
				t.Fatalf("Decoder.Decode into the stale value = %v; want nil", err)
			}
			for name, got := range map[string]any{"canxml.Unmarshal": viaUnmarshal, "Decoder.Decode": viaDecoder} {
				if !reflect.DeepEqual(got, fromJSON) {
					t.Errorf("%s into the stale value gave\n%#v\nwant what canonical JSON gives\n%#v", name, got, fromJSON)
				}
			}
			if !strings.HasPrefix(row.name, "control:") && !reflect.DeepEqual(viaUnmarshal, fresh) {
				t.Errorf("canxml.Unmarshal into the stale value gave\n%#v\nwant the value a zero value decodes to\n%#v", viaUnmarshal, fresh)
			}
		})
	}
}

// TestREQ056_XMLSecondDecodeKeepsAReusedValue decodes a slot whose second
// include's item declares its `xs` prefix on the item's parent, which a decode
// given no option reads twice, into a value that holds an include and an
// exclude. Through canxml.Unmarshal and a canxml.Decoder given no option, the
// value is the one a decode that reads the document once gives,
// canxml.WithMissing's: the two includes the document carries, and the
// exclude it leaves out kept.
//
// Can-fail control: clear the value before the second decode, and the exclude
// is lost; let the second decode add to the includes the first began, and the
// slot holds three.
func TestREQ056_XMLSecondDecodeKeepsAReusedValue(t *testing.T) {
	in := streamAgain.slot()
	stale := func() *aom14.ArchetypeSlot {
		return &aom14.ArchetypeSlot{Includes: []aom14.Assertion{{Tag: new("old")}}, Excludes: []aom14.Assertion{{Tag: new("kept")}}}
	}
	once := stale()
	if err := canxml.Unmarshal([]byte(in), once, canxml.WithMissing(func(string) {})); err != nil {
		t.Fatalf("canxml.Unmarshal WithMissing into the stale value = %v; want nil", err)
	}
	if len(once.Includes) != 2 || len(once.Excludes) != 1 || *once.Excludes[0].Tag != "kept" {
		t.Fatalf("canxml.Unmarshal WithMissing into the stale value gave %d includes and %d excludes; want the document's 2 and the 1 kept", len(once.Includes), len(once.Excludes))
	}
	viaUnmarshal := stale()
	if err := canxml.Unmarshal([]byte(in), viaUnmarshal); err != nil {
		t.Fatalf("canxml.Unmarshal into the stale value = %v; want nil", err)
	}
	viaDecoder := stale()
	if err := canxml.NewDecoder(strings.NewReader(in)).Decode(viaDecoder); err != nil {
		t.Fatalf("Decoder.Decode into the stale value = %v; want nil", err)
	}
	for name, got := range map[string]*aom14.ArchetypeSlot{"canxml.Unmarshal": viaUnmarshal, "Decoder.Decode": viaDecoder} {
		if !reflect.DeepEqual(got, once) {
			t.Errorf("%s into the stale value gave %d includes and %d excludes\n%#v\nwant what one decode gives\n%#v", name, len(got.Includes), len(got.Excludes), got, once)
		}
	}
}
