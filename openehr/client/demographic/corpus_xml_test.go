package demographic_test

// corpus_xml_test.go — the canonical XML twins of the demographic corpus
// bodies (REQ-056): each is what canxml writes for its canonical JSON twin,
// and decodes back to what that twin decodes to.

import (
	"bytes"
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestCorpusXMLTwins pins the eight XML twins under
// testkit/corpus/its_rest/demographic: each holds the bytes canxml writes
// for the value its JSON twin decodes to, and decodes back, strictly, to a
// value whose canonical JSON equals the JSON twin's, as the cross-format
// round trip of the codec compares them.
//
// The ORIGINAL_VERSION twin differs from what canxml writes by one
// attribute: the xsi:type the ITS-XML schemas need on the generic `data`,
// which the encoder does not write.
func TestCorpusXMLTwins(t *testing.T) { // REQ-056
	twins := []struct {
		name  string
		new   func() any
		fixup func([]byte) []byte
	}{
		{name: "person", new: func() any { return new(rm.Person) }},
		{name: "organisation", new: func() any { return new(rm.Organisation) }},
		{name: "group", new: func() any { return new(rm.Group) }},
		{name: "agent", new: func() any { return new(rm.Agent) }},
		{name: "role", new: func() any { return new(rm.Role) }},
		{name: "versioned_party", new: func() any { return new(rm.VersionedParty) }},
		{name: "revision_history", new: func() any { return new(rm.RevisionHistory) }},
		{
			name: "original_version",
			new:  func() any { return new(rm.OriginalVersion[*rm.Person]) },
			fixup: func(b []byte) []byte {
				return bytes.Replace(b, []byte("<data "), []byte(`<data xsi:type="PERSON" `), 1)
			},
		},
	}
	for _, tw := range twins {
		t.Run(tw.name, func(t *testing.T) {
			fromJSON := tw.new()
			if err := canjson.Unmarshal(fixture(t, tw.name+".json"), fromJSON, canjson.WithStrict()); err != nil {
				t.Fatalf("decode %s.json: %v", tw.name, err)
			}
			wantXML, err := canxml.Marshal(fromJSON)
			if err != nil {
				t.Fatalf("encode %s as XML: %v", tw.name, err)
			}
			if tw.fixup != nil {
				wantXML = tw.fixup(wantXML)
			}
			gotXML := fixture(t, tw.name+".xml")
			if !bytes.Equal(gotXML, wantXML) {
				t.Errorf("%s.xml is not what canxml writes for %s.json:\n got %s\nwant %s", tw.name, tw.name, gotXML, wantXML)
			}

			fromXML := tw.new()
			if err := canxml.Unmarshal(gotXML, fromXML, canxml.WithStrict()); err != nil {
				t.Fatalf("decode %s.xml: %v", tw.name, err)
			}
			a := normalisedJSON(t, fromJSON)
			d := normalisedJSON(t, fromXML)
			if !reflect.DeepEqual(a, d) {
				t.Errorf("%s: JSON→XML→JSON differs from JSON→JSON:\n JSON %v\n  XML %v", tw.name, a, d)
			}
		})
	}
}

// normalisedJSON returns v's canonical JSON as a generic tree, without
// null members and empty arrays: canonical XML writes no element for
// either, so the XML leg reads them back as absent.
func normalisedJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := canjson.Marshal(v)
	if err != nil {
		t.Fatalf("encode %T as JSON: %v", v, err)
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatalf("read back %T: %v", v, err)
	}
	return stripEmpty(tree)
}

func stripEmpty(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			cleaned := stripEmpty(val)
			if arr, ok := cleaned.([]any); cleaned == nil || ok && len(arr) == 0 {
				continue
			}
			out[k] = cleaned
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripEmpty(e)
		}
		return out
	default:
		return v
	}
}
