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
