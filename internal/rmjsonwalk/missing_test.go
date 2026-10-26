package rmjsonwalk_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
)

// missing walks doc with the default model and the Missing hook alone, and
// returns the pointers it reports.
func missing(t *testing.T, doc string, root rmjsonwalk.Root) []string {
	t.Helper()
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	var got []string
	err = m.Walk([]byte(doc), root, rmjsonwalk.Hooks{Missing: func(p string) error {
		got = append(got, p)
		return nil
	}})
	if err != nil {
		t.Fatalf("Walk(%s, %+v) error: %v", doc, root, err)
	}
	return got
}

// TestREQ175_MissingNamesEveryRequiredMemberTheObjectLacks pins what the
// Missing hook reports: for every object the walk reads as an RM class, each
// attribute the RM makes mandatory on that class which the object has no
// member for, or holds null at, as the attribute's name under the object's
// pointer. The members of one object come as the object closes, in the order
// the RM declares them, an ancestor's before the class's own; the objects
// inside an object close first.
func TestREQ175_MissingNamesEveryRequiredMemberTheObjectLacks(t *testing.T) {
	cases := []struct {
		name, doc string
		root      rmjsonwalk.Root
		want      []string
	}{
		{
			name: "root by its _type, the target unknown",
			doc:  `{"_type":"DV_QUANTITY","units":"kg"}`,
			want: []string{"/magnitude"},
		},
		{
			name: "root by the decode target",
			root: rmjsonwalk.Root{Type: "DV_QUANTITY"},
			doc:  `{"units":"kg"}`,
			want: []string{"/magnitude"},
		},
		{
			name: "every member absent",
			doc:  `{"_type":"DV_QUANTITY"}`,
			want: []string{"/magnitude", "/units"},
		},
		{
			// DV_CODED_TEXT inherits value from DV_TEXT and declares
			// defining_code itself: the ancestor's attribute comes first,
			// whatever order the document gives the nulls.
			name: "the RM's order, ancestors first, not the document's",
			doc:  `{"_type":"DV_CODED_TEXT","defining_code":null,"value":null}`,
			want: []string{"/value", "/defining_code"},
		},
		{
			name: "a null member is missing, a null _type reads as elided",
			root: rmjsonwalk.Root{Type: "DV_QUANTITY"},
			doc:  `{"_type":null,"magnitude":null,"units":"kg"}`,
			want: []string{"/magnitude"},
		},
		{
			// Each object's members come as it closes: the objects inside
			// before the one that holds them, in document order.
			name: "inner objects close first",
			doc: `{"_type":"ELEMENT",` +
				`"name":{"_type":"DV_CODED_TEXT","defining_code":{"_type":"CODE_PHRASE","code_string":"c"}},` +
				`"value":{"_type":"DV_QUANTITY","units":"kg"}}`,
			want: []string{
				"/name/defining_code/terminology_id",
				"/name/value",
				"/value/magnitude",
				"/archetype_node_id",
			},
		},
		{
			name: "container elements by position",
			doc: `{"_type":"ITEM_TREE","name":{"value":"n"},"archetype_node_id":"at1","items":[` +
				`{"_type":"ELEMENT","name":{"value":"e"},"archetype_node_id":"at2","value":{"_type":"DV_COUNT"}},` +
				`{"_type":"CLUSTER","name":{"value":"c"},"archetype_node_id":"at3"}]}`,
			want: []string{"/items/0/value/magnitude", "/items/1/items"},
		},
		{
			name: "a Hash entry by its key",
			doc: `{"_type":"RESOURCE_DESCRIPTION","original_author":{"name":"a"},"lifecycle_state":"published",` +
				`"details":{"en":{"purpose":"p"},"de~/":{"language":{"terminology_id":{"value":"ISO_639-1"},"code_string":"de"}}}}`,
			want: []string{"/details/en/language", "/details/de~0~1/purpose"},
		},
		{
			name: "a slice target: each element a root of the element type",
			root: rmjsonwalk.Root{Nest: "[", Type: "DV_QUANTITY"},
			doc:  `[{"units":"kg"},{"_type":"DV_QUANTITY","magnitude":1},null]`,
			want: []string{"/0/magnitude", "/1/units"},
		},
		{
			name: "a map target: each member value a root",
			root: rmjsonwalk.Root{Nest: "{", Type: "DV_QUANTITY"},
			doc:  `{"k":{"units":"kg"},"x/y":{"magnitude":1}}`,
			want: []string{"/k/magnitude", "/x~1y/units"},
		},
		{
			name: "an interval bound bound to a concrete class",
			doc: `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg",` +
				`"normal_range":{"lower":{"magnitude":1},"upper":{"units":"kg"}}}`,
			want: []string{"/normal_range/lower/units", "/normal_range/upper/magnitude"},
		},
		{
			name: "an interval bound by its own _type",
			doc:  `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT"},"upper":{"magnitude":2}}`,
			want: []string{"/lower/magnitude"},
		},
		{
			name: "a generic root bound by the target",
			root: rmjsonwalk.Root{Type: "DV_INTERVAL", Bound: "DV_QUANTITY"},
			doc:  `{"lower":{"units":"kg"},"upper":{"magnitude":2,"units":"kg"}}`,
			want: []string{"/lower/magnitude"},
		},
		{
			name: "a binding overrides the generic payload",
			root: rmjsonwalk.Root{Type: "ORIGINAL_VERSION", Bound: "DV_QUANTITY"},
			doc: `{"_type":"ORIGINAL_VERSION","uid":{"value":"u"},"contribution":{"id":{"_type":"HIER_OBJECT_ID","value":"c"},"namespace":"n","type":"t"},` +
				`"commit_audit":{"system_id":"s","time_committed":{"value":"2026-01-01T00:00:00Z"},"change_type":{"value":"creation","defining_code":{"terminology_id":{"value":"openehr"},"code_string":"249"}},"committer":{"_type":"PARTY_SELF"}},` +
				`"lifecycle_state":{"value":"complete","defining_code":{"terminology_id":{"value":"openehr"},"code_string":"532"}},` +
				`"data":{"units":"kg"}}`,
			want: []string{"/data/magnitude"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missing(t, tc.doc, tc.root); !slices.Equal(got, tc.want) {
				t.Errorf("Walk(%s, %+v) reported missing %q, want %q", tc.doc, tc.root, got, tc.want)
			}
		})
	}
}

// TestREQ175_MissingNeverNamesAPresentMember pins that a present member is
// never missing, whatever it holds, and that three kinds of attribute are
// never missing at all: one no serialization carries (a function the model
// computes, such as EVENT.offset, and RESOURCE_DESCRIPTION.parent_resource),
// the four interval boundary flags, and "_type".
func TestREQ175_MissingNeverNamesAPresentMember(t *testing.T) {
	cases := []struct {
		name, doc string
		root      rmjsonwalk.Root
	}{
		{
			name: "empty string, zero, empty list, empty object, bare PARTY_SELF",
			doc: `{"_type":"OBSERVATION","name":{"value":""},"archetype_node_id":"",` +
				`"language":{"terminology_id":{"value":""},"code_string":""},"encoding":{"terminology_id":{"value":""},"code_string":""},` +
				`"subject":{"_type":"PARTY_SELF"},` +
				`"data":{"name":{"value":"h"},"archetype_node_id":"at1","origin":{"value":""},"events":[` +
				`{"_type":"POINT_EVENT","name":{"value":"e"},"archetype_node_id":"at2","time":{"value":""},"data":{"_type":"ITEM_TREE","name":{"value":"t"},"archetype_node_id":"at3","items":[` +
				`{"_type":"ELEMENT","name":{"value":"e"},"archetype_node_id":"at4","value":{"_type":"DV_QUANTITY","magnitude":0,"units":""}},` +
				`{"_type":"CLUSTER","name":{"value":"c"},"archetype_node_id":"at5","items":[]}]}}]}}`,
		},
		{
			// A value of the wrong JSON kind is present: the decoder
			// refuses it, the walk does not report it.
			name: "a value of the wrong JSON kind",
			doc:  `{"_type":"DV_QUANTITY","magnitude":"80","units":["kg"]}`,
		},
		{
			name: "the interval boundary flags",
			doc:  `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":1}}`,
		},
		{
			name: "the interval boundary flags of a BASE interval class",
			root: rmjsonwalk.Root{Type: "Point_interval", Bound: "DV_COUNT"},
			doc:  `{"lower":{"magnitude":1}}`,
		},
		{
			name: "EVENT.offset, a function the model computes",
			doc:  `{"_type":"POINT_EVENT","name":{"value":"e"},"archetype_node_id":"at1","time":{"value":"2026-01-01T00:00:00Z"},"data":{"_type":"ITEM_TREE","name":{"value":"t"},"archetype_node_id":"at2"}}`,
		},
		{
			name: "RESOURCE_DESCRIPTION.parent_resource",
			doc:  `{"_type":"RESOURCE_DESCRIPTION","original_author":{},"lifecycle_state":"","details":{}}`,
		},
		{
			name: "_type",
			root: rmjsonwalk.Root{Type: "DV_COUNT"},
			doc:  `{"magnitude":1}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missing(t, tc.doc, tc.root); len(got) != 0 {
				t.Errorf("Walk(%s, %+v) reported missing %q, want nothing", tc.doc, tc.root, got)
			}
		})
	}
}

// TestREQ175_MissingLeavesOpaqueValuesAlone pins where the Missing hook does
// not look, which is where the Unmatched hook does not either: an object the
// walk cannot read as an RM class is not checked, and nothing inside it is.
// Each case leaves members out of such an object; the root's own missing
// member shows the walk ran.
func TestREQ175_MissingLeavesOpaqueValuesAlone(t *testing.T) {
	cases := []struct {
		name, doc string
		root      rmjsonwalk.Root
		want      []string
	}{
		{
			name: "a class outside the RM at the root",
			root: rmjsonwalk.Root{Type: "C_COMPLEX_OBJECT"},
			doc:  `{"_type":"C_COMPLEX_OBJECT","attributes":[{"_type":"C_SINGLE_ATTRIBUTE"}]}`,
		},
		{
			name: "a class outside the RM in a slot",
			doc:  `{"_type":"ELEMENT","name":{"value":"e"},"value":{"_type":"NO_SUCH_CLASS"}}`,
			want: []string{"/archetype_node_id"},
		},
		{
			name: "the generic payload of ORIGINAL_VERSION",
			doc:  `{"_type":"ORIGINAL_VERSION","uid":{"value":"u"},"contribution":{"id":{"_type":"HIER_OBJECT_ID","value":"c"},"namespace":"n","type":"t"},"lifecycle_state":{"value":"c","defining_code":{"terminology_id":{"value":"openehr"},"code_string":"532"}},"data":{"_type":"DV_QUANTITY","units":"kg"}}`,
			want: []string{"/commit_audit"},
		},
		{
			name: "an elided _type where the declared type is abstract",
			doc:  `{"_type":"ELEMENT","name":{"value":"e"},"value":{"units":"kg"}}`,
			want: []string{"/archetype_node_id"},
		},
		{
			name: "an elided _type behind an unbound type parameter",
			doc:  `{"_type":"DV_INTERVAL","lower":{"units":"kg"}}`,
		},
		{
			name: "a root of unknown type with no _type",
			doc:  `{"units":"kg"}`,
		},
		{
			name: "a primitive attribute holding an object",
			doc:  `{"_type":"RESOURCE_DESCRIPTION_ITEM","purpose":"p","other_details":{"x":{"_type":"DV_QUANTITY"}}}`,
			want: []string{"/language"},
		},
		{
			name: "a Hash of plain values",
			doc:  `{"_type":"RESOURCE_DESCRIPTION","original_author":{"x":{"_type":"DV_QUANTITY"}},"lifecycle_state":"published"}`,
			want: []string{"/details"},
		},
		{
			name: "a value of the wrong JSON kind for its attribute",
			doc:  `{"_type":"DV_CODED_TEXT","defining_code":[{"_type":"CODE_PHRASE"}]}`,
			want: []string{"/value"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missing(t, tc.doc, tc.root); !slices.Equal(got, tc.want) {
				t.Errorf("Walk(%s, %+v) reported missing %q, want %q", tc.doc, tc.root, got, tc.want)
			}
		})
	}
}

// TestREQ175_WalkServesBothHooksInOnePass pins that one walk serves both
// hooks: unmatched members come in document order as they are met, missing
// members as their object closes, so the two reports interleave by position
// in the document. Two passes would give one report whole before the other.
func TestREQ175_WalkServesBothHooksInOnePass(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	const doc = `{"_type":"DV_CODED_TEXT","x-a":1,"defining_code":{"_type":"CODE_PHRASE","x-b":1,"code_string":"c"},"x-c":1}`
	var log []string
	err = m.Walk([]byte(doc), rmjsonwalk.Root{}, rmjsonwalk.Hooks{
		Unmatched: func(p string) error { log = append(log, "unmatched "+p); return nil },
		Missing:   func(p string) error { log = append(log, "missing "+p); return nil },
	})
	if err != nil {
		t.Fatalf("Walk(%s) error: %v", doc, err)
	}
	want := []string{
		"unmatched /x-a",
		"unmatched /defining_code/x-b",
		"missing /defining_code/terminology_id",
		"unmatched /x-c",
		"missing /value",
	}
	if !slices.Equal(log, want) {
		t.Errorf("Walk(%s) reported\n%q\nwant, in one pass,\n%q", doc, log, want)
	}
}

// TestREQ175_WalkWithNilHooksReportsNothing pins that a nil hook asks for
// nothing: the walk still reads the document, so a malformed one is an error,
// and reports nothing to a hook that is not there.
func TestREQ175_WalkWithNilHooksReportsNothing(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	const doc = `{"_type":"DV_QUANTITY","x-a":1}`
	if err := m.Walk([]byte(doc), rmjsonwalk.Root{}, rmjsonwalk.Hooks{}); err != nil {
		t.Errorf("Walk(%s, no hooks) error = %v, want nil", doc, err)
	}
	var got []string
	err = m.Walk([]byte(doc), rmjsonwalk.Root{}, rmjsonwalk.Hooks{Missing: func(p string) error {
		got = append(got, p)
		return nil
	}})
	if err != nil {
		t.Fatalf("Walk(%s, Missing) error: %v", doc, err)
	}
	if want := []string{"/magnitude", "/units"}; !slices.Equal(got, want) {
		t.Errorf("Walk(%s, Missing) reported %q, want %q: an unmatched member is not the Missing hook's", doc, got, want)
	}
	if err := m.Walk([]byte(`{"_type":"DV_QUANTITY",`), rmjsonwalk.Root{}, rmjsonwalk.Hooks{}); err == nil {
		t.Error("Walk(malformed, no hooks) error = nil, want jsontext's refusal")
	}
}

// TestREQ175_MissingStopsAtTheHooksError pins that the Missing hook's error
// stops the walk at once and comes back as it is.
func TestREQ175_MissingStopsAtTheHooksError(t *testing.T) {
	m, err := rmjsonwalk.Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	stop := errors.New("stop")
	var got []string
	err = m.Walk([]byte(`{"_type":"DV_QUANTITY"}`), rmjsonwalk.Root{}, rmjsonwalk.Hooks{Missing: func(p string) error {
		got = append(got, p)
		return stop
	}})
	if err != stop { //nolint:errorlint // identity by design: the hook's own error comes back unwrapped
		t.Errorf("Walk error = %v, want the hook's own error", err)
	}
	if want := []string{"/magnitude"}; !slices.Equal(got, want) {
		t.Errorf("Walk reported %q before stopping, want %q", got, want)
	}
}

// TestREQ175_MissingPointersCarryNoValue pins that a pointer is built from
// attribute names, positions and Hash keys only: a value the document holds
// never reaches the report.
func TestREQ175_MissingPointersCarryNoValue(t *testing.T) {
	const secret = "planted-secret-value"
	got := missing(t, `{"_type":"DV_CODED_TEXT","value":"`+secret+`","defining_code":{"code_string":"`+secret+`"}}`, rmjsonwalk.Root{})
	for _, p := range got {
		if strings.Contains(p, secret) {
			t.Errorf("Walk reported %q, which carries a value from the document", p)
		}
	}
	if want := []string{"/defining_code/terminology_id"}; !slices.Equal(got, want) {
		t.Errorf("Walk reported %q, want %q", got, want)
	}
}
