package validation_test

// rmfloor_nil_item_test.go: REQ-112 — an item of a list that holds no RM
// value, a nil element or a nil pointer, is reported as `required` at its
// own path on the RM layer, rule mandatory, and is not walked. It still
// counts toward its list, so a list that holds only such items is present
// and not empty.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// nilItemCluster returns a CLUSTER whose items are items.
func nilItemCluster(items ...rm.Item) *rm.Cluster {
	return &rm.Cluster{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "cluster"}, Items: items}
}

// decodedNullItemCluster decodes a CLUSTER whose items list holds one JSON
// null.
func decodedNullItemCluster(t *testing.T) *rm.Cluster {
	t.Helper()
	const body = `{"_type":"CLUSTER","archetype_node_id":"at0001",` +
		`"name":{"_type":"DV_TEXT","value":"cluster"},"items":[null]}`
	var c rm.Cluster
	if err := canjson.Unmarshal([]byte(body), &c); err != nil {
		t.Fatalf("canjson.Unmarshal(%s): %v", body, err)
	}
	if len(c.Items) != 1 || c.Items[0] != nil {
		t.Fatalf("canjson.Unmarshal(%s) items = %#v, want one nil item", body, c.Items)
	}
	return &c
}

// nilItemSection returns a SECTION whose items are items.
func nilItemSection(items ...rm.ContentItem) *rm.Section {
	return &rm.Section{ArchetypeNodeID: "at0001", Name: &rm.DVText{Value: "section"}, Items: items}
}

// TestValidateRM_REQ112_NilListItem pins the REQ-112 rule for an item of a
// list that holds no RM value. Each row lists the whole result, by code,
// path, layer and rule.
func TestValidateRM_REQ112_NilListItem(t *testing.T) {
	const obsPath = "/items[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	unnamed := validBloodPressureObservation()
	unnamed.Name = nil
	tests := []struct {
		name string
		root func(t *testing.T) any
		want []string
	}{
		{
			name: "CLUSTER with a typed-nil pointer item",
			root: func(*testing.T) any { return nilItemCluster((*rm.Element)(nil)) },
			want: []string{"required /items[1] rm mandatory"},
		},
		{
			name: "CLUSTER with an untyped nil item",
			root: func(*testing.T) any { return nilItemCluster(nil) },
			want: []string{"required /items[1] rm mandatory"},
		},
		{
			name: "CLUSTER decoded from items [null]",
			root: func(t *testing.T) any { return decodedNullItemCluster(t) },
			want: []string{"required /items[1] rm mandatory"},
		},
		{
			name: "SECTION with a nil item before a valid OBSERVATION",
			root: func(*testing.T) any { return nilItemSection(nil, validBloodPressureObservation()) },
			want: []string{"required /items[1] rm mandatory"},
		},
		{
			// The OBSERVATION after the nil item is walked as usual, so a
			// fault inside it is reported too.
			name: "SECTION with a nil item before an OBSERVATION without a name",
			root: func(*testing.T) any { return nilItemSection(nil, unnamed) },
			want: []string{
				"required /items[1] rm mandatory",
				"required " + obsPath + "/name rm mandatory",
			},
		},
		{
			name: "COMPOSITION with a typed-nil content item",
			root: func(*testing.T) any {
				comp := validVitalSignsComposition()
				comp.Content = []rm.ContentItem{(*rm.Observation)(nil)}
				return comp
			},
			want: []string{"required /content[1] rm mandatory"},
		},
		{
			// Control: an empty list is present and holds no item, which
			// stays the RM's cardinality.
			name: "CLUSTER with no item",
			root: func(*testing.T) any { return nilItemCluster([]rm.Item{}...) },
			want: []string{"cardinality /items rm cardinality"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root(t))
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// walkOrderKeys renders each issue as "code path layer rule", in the order
// the walk reported it, so a test can tell what came after a stop.
func walkOrderKeys(issues []validation.Issue) []string {
	keys := make([]string, 0, len(issues))
	for _, i := range issues {
		keys = append(keys, i.Code+" "+i.Path+" "+i.Layer.String()+" "+i.Rule)
	}
	return keys
}

// valuedElement returns an ELEMENT the RM rules find nothing wrong with.
func valuedElement() *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0002", Name: &rm.DVText{Value: "element"}, Value: &rm.DVText{Value: "text"}}
}

// TestValidator_REQ112_REQ171_NilListItemAfterVisitLimit pins the REQ-112
// nil-item rule against the REQ-171 visit limit: once the walk stops, a nil
// item later in the same list, an untyped nil or a typed-nil pointer, is not
// reported, so max_nodes is the last issue. A nil item counts as one visit,
// so a list of nil items alone reaches the limit too, and the item past it
// is reported as max_nodes, not as required. The control row walks a list
// under the default limits, where both nil items are reported.
func TestValidator_REQ112_REQ171_NilListItemAfterVisitLimit(t *testing.T) {
	// Five visits reach the first ELEMENT's name, so the walk stops inside
	// the first item and every item after it comes after the stop.
	const nodes = 5
	// The walk visits the CLUSTER and three values of its attributes before
	// its items: its archetype_node_id, its name and the name's value. Under
	// a limit of seven visits, the first three nil items spend the last three
	// (4 + 3 = 7), and the fourth is past the limit.
	const nilOnlyNodes = 7
	tests := []struct {
		name  string
		nodes int
		items []rm.Item
		// want lists every issue in walk order: nothing may follow
		// max_nodes.
		want        []string
		wantStopped []string
	}{
		{
			name:        "untyped nil after the stop",
			nodes:       nodes,
			items:       []rm.Item{valuedElement(), nil},
			want:        []string{"max_nodes /items[at0002]/name  "},
			wantStopped: []string{"/items[at0002]/name"},
		},
		{
			name:        "typed-nil pointer after the stop",
			nodes:       nodes,
			items:       []rm.Item{valuedElement(), (*rm.Element)(nil)},
			want:        []string{"max_nodes /items[at0002]/name  "},
			wantStopped: []string{"/items[at0002]/name"},
		},
		{
			name:        "both nil items, a valued ELEMENT between, after the stop",
			nodes:       nodes,
			items:       []rm.Item{valuedElement(), nil, valuedElement(), (*rm.Element)(nil)},
			want:        []string{"max_nodes /items[1]/name  "},
			wantStopped: []string{"/items[1]/name"},
		},
		{
			name:  "a hundred nil items and nothing else",
			nodes: nilOnlyNodes,
			items: slices.Repeat([]rm.Item{nil}, 100),
			want: []string{
				"required /items[1] rm mandatory",
				"required /items[2] rm mandatory",
				"required /items[3] rm mandatory",
				"max_nodes /items[4]  ",
			},
			wantStopped: []string{"/items[4]"},
		},
		{
			// Control: under the default limits the walk does not stop,
			// so both nil items of the same list are reported.
			name:  "the same list under the default limits",
			items: []rm.Item{valuedElement(), nil, valuedElement(), (*rm.Element)(nil)},
			want: []string{
				"required /items[2] rm mandatory",
				"required /items[4] rm mandatory",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := validation.New(validation.WithLimits(validation.Limits{Nodes: tc.nodes}))
			r := v.Validate(nilItemCluster(tc.items...), nil)
			if got := walkOrderKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s, Nodes %d) issues in walk order (code path layer rule) = %q, want %q", tc.name, tc.nodes, got, tc.want)
			}
			if !slices.Equal(r.Coverage.Stopped, tc.wantStopped) {
				t.Errorf("Validate(%s, Nodes %d) Coverage.Stopped = %q, want %q", tc.name, tc.nodes, r.Coverage.Stopped, tc.wantStopped)
			}
			if got, want := r.Coverage.Complete(), len(tc.wantStopped) == 0; got != want {
				t.Errorf("Validate(%s, Nodes %d) Coverage.Complete() = %t, want %t", tc.name, tc.nodes, got, want)
			}
		})
	}
}
