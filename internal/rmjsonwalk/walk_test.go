package rmjsonwalk_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
)

// unmatched walks doc with the default model and returns the pointers it
// reports.
func unmatched(t *testing.T, doc, rootType string) []string {
	t.Helper()
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	var got []string
	err = m.Unmatched([]byte(doc), rootType, func(p string) error {
		got = append(got, p)
		return nil
	})
	if err != nil {
		t.Fatalf("Unmatched(%s, %q) error: %v", doc, rootType, err)
	}
	return got
}

// TestREQ170_UnmatchedNamesEveryMemberTheRMDoesNotDefine pins what the walk
// reports, in document order and by RFC 6901 pointer: a member whose name is
// neither "_type" nor an attribute of the RM type its object decodes as, at
// any depth. Each case isolates one way the walk reads an object's type.
func TestREQ170_UnmatchedNamesEveryMemberTheRMDoesNotDefine(t *testing.T) {
	cases := []struct {
		name, root, doc string
		want            []string
	}{
		{
			name: "root by its _type, the target unknown",
			doc:  `{"_type":"DV_TEXT","value":"v","x-a":1}`,
			want: []string{"/x-a"},
		},
		{
			name: "root by the decode target",
			root: "DV_TEXT",
			doc:  `{"value":"v","x-a":1}`,
			want: []string{"/x-a"},
		},
		{
			// JSON member order is free: the members before "_type" are
			// read against the type it names.
			name: "_type after the members it decides",
			doc:  `{"x-a":1,"value":"v","x-b":{"value":2},"_type":"DV_TEXT"}`,
			want: []string{"/x-a", "/x-b"},
		},
		{
			name: "by-value class from its attribute",
			doc:  `{"_type":"CODE_PHRASE","terminology_id":{"x-a":1,"value":"openehr"},"code_string":"433"}`,
			want: []string{"/terminology_id/x-a"},
		},
		{
			name: "polymorphic slot by its _type, container elements",
			doc: `{"_type":"ITEM_TREE","name":{"value":"n"},"archetype_node_id":"at1","items":[` +
				`{"_type":"ELEMENT","name":{"value":"e"},"archetype_node_id":"at2","value":{"_type":"DV_COUNT","magnitude":1,"x-a":1}},` +
				`{"_type":"CLUSTER","name":{"value":"c"},"archetype_node_id":"at3","x-b":true,"items":[]}]}`,
			want: []string{"/items/0/value/x-a", "/items/1/x-b"},
		},
		{
			name: "interval bound bound to a concrete class",
			doc: `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg",` +
				`"normal_range":{"lower":{"magnitude":1,"units":"kg","x-a":1},"x-b":1}}`,
			want: []string{"/normal_range/lower/x-a", "/normal_range/x-b"},
		},
		{
			name: "interval bound by its own _type",
			doc:  `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":1,"x-a":1},"upper":{"magnitude":2,"x-b":1}}`,
			want: []string{"/lower/x-a"},
		},
		{
			name: "an empty _type reads as elided",
			doc:  `{"_type":"CODE_PHRASE","terminology_id":{"_type":"","value":"openehr","x-a":1},"code_string":"433"}`,
			want: []string{"/terminology_id/x-a"},
		},
		{
			name: "a null member and a null _type",
			root: "DV_TEXT",
			doc:  `{"_type":null,"value":"v","language":null,"x-a":null}`,
			want: []string{"/x-a"},
		},
		{
			// RFC 6901 § 3: "~" is written "~0" and "/" is written "~1".
			name: "names RFC 6901 escapes",
			doc:  `{"_type":"DV_TEXT","a~b/c":1,"value":"v","~1":2}`,
			want: []string{"/a~0b~1c", "/~01"},
		},
		{
			name: "a root array reads each element as a root",
			doc:  `[{"_type":"DV_TEXT","value":"v","x-a":1},{"value":"w","x-b":1},{"_type":"DV_COUNT","magnitude":1,"x-c":1}]`,
			want: []string{"/0/x-a", "/2/x-c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unmatched(t, tc.doc, tc.root); !slices.Equal(got, tc.want) {
				t.Errorf("Unmatched(%s, %q) reported %q, want %q", tc.doc, tc.root, got, tc.want)
			}
		})
	}
}

// TestREQ170_UnmatchedLeavesOpaqueValuesAlone pins where the walk does not
// look: a value it cannot read as an RM class is neither descended nor
// reported, so a member inside it is never named. Each case carries a member
// that would be unmatched if the walk read the value as the class its "_type"
// names; the root's own extension member shows the walk ran.
func TestREQ170_UnmatchedLeavesOpaqueValuesAlone(t *testing.T) {
	cases := []struct {
		name, root, doc string
		want            []string
	}{
		{
			name: "a class outside the RM at the root",
			root: "C_COMPLEX_OBJECT",
			doc:  `{"_type":"C_COMPLEX_OBJECT","rm_type_name":"OBSERVATION","x-a":1,"attributes":[{"_type":"C_SINGLE_ATTRIBUTE","x-b":1}]}`,
		},
		{
			name: "a class outside the RM in a slot",
			doc:  `{"_type":"ELEMENT","name":{"value":"e"},"archetype_node_id":"at1","value":{"_type":"NO_SUCH_CLASS","x-a":1},"x-root":1}`,
			want: []string{"/x-root"},
		},
		{
			name: "the generic payload of ORIGINAL_VERSION",
			doc:  `{"_type":"ORIGINAL_VERSION","data":{"_type":"DV_TEXT","value":"v","x-a":1},"x-root":1}`,
			want: []string{"/x-root"},
		},
		{
			name: "an elided _type where the declared type is abstract",
			doc:  `{"_type":"ELEMENT","name":{"value":"e"},"archetype_node_id":"at1","value":{"value":"v","x-a":1},"x-root":1}`,
			want: []string{"/x-root"},
		},
		{
			name: "an elided _type behind an unbound type parameter",
			doc:  `{"_type":"DV_INTERVAL","lower":{"magnitude":1,"x-a":1},"x-root":1}`,
			want: []string{"/x-root"},
		},
		{
			name: "a root of unknown type with no _type",
			doc:  `{"value":"v","x-a":1}`,
		},
		{
			name: "a primitive attribute holding an object",
			doc:  `{"_type":"RESOURCE_DESCRIPTION_ITEM","other_details":{"x-a":{"_type":"DV_TEXT","x-b":1}},"x-root":1}`,
			want: []string{"/x-root"},
		},
		{
			name: "a value of the wrong JSON kind",
			doc:  `{"_type":"DV_TEXT","value":"v","mappings":{"x-a":1},"language":["x-b"],"x-root":1}`,
			want: []string{"/x-root"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unmatched(t, tc.doc, tc.root); !slices.Equal(got, tc.want) {
				t.Errorf("Unmatched(%s, %q) reported %q, want %q", tc.doc, tc.root, got, tc.want)
			}
		})
	}
}

// TestUnmatchedStopsAtTheReportsError pins that a report error stops the walk
// at once and comes back as it is: a strict decode refuses at the first
// unmatched member and sees no later one.
func TestUnmatchedStopsAtTheReportsError(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	stop := errors.New("stop")
	var got []string
	err = m.Unmatched([]byte(`{"_type":"DV_TEXT","x-a":1,"value":"v","x-b":2}`), "", func(p string) error {
		got = append(got, p)
		return stop
	})
	if err != stop { //nolint:errorlint // identity by design: the report's own error comes back unwrapped
		t.Errorf("Unmatched error = %v, want the report's own error", err)
	}
	if want := []string{"/x-a"}; !slices.Equal(got, want) {
		t.Errorf("Unmatched reported %q before stopping, want %q", got, want)
	}
}

// TestUnmatchedReportsMalformedInput pins that a document jsontext refuses is
// an error, not an empty report.
func TestUnmatchedReportsMalformedInput(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	for _, doc := range []string{`{"_type":"DV_TEXT","value":`, `{"value":1,"value":2}`} {
		err := m.Unmatched([]byte(doc), "DV_TEXT", func(string) error { return nil })
		if err == nil {
			t.Errorf("Unmatched(%s) error = nil, want jsontext's refusal", doc)
		}
	}
}

// TestUnmatchedPointersCarryNoValue pins that a pointer names the member only:
// the value the member holds never reaches the report.
func TestUnmatchedPointersCarryNoValue(t *testing.T) { // REQ-170
	const secret = "planted-secret-value"
	got := unmatched(t, `{"_type":"DV_TEXT","x-a":"`+secret+`","value":"v","x-b":{"k":"`+secret+`"}}`, "")
	for _, p := range got {
		if strings.Contains(p, secret) {
			t.Errorf("Unmatched reported %q, which carries the member's value", p)
		}
	}
	if len(got) != 2 {
		t.Errorf("Unmatched reported %q, want two pointers", got)
	}
}
