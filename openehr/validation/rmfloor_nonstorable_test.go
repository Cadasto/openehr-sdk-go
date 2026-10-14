package validation_test

// rmfloor_nonstorable_test.go: REQ-112 Driver — the floor's required-set
// check leaves out an RM-mandatory attribute that no serialization carries,
// the set rminfo.IsNonStorableAttr names. RESOURCE_DESCRIPTION parent_resource
// is one: the back-reference to the resource that holds the description,
// typed AUTHORED_RESOURCE, a class with no concrete descendant, so no value
// can be held there.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_REQ112_ResourceDescriptionParentResourceLeftOut pins REQ-112
// Driver on RESOURCE_DESCRIPTION: the floor never reports parent_resource. A
// filled description decoded from JSON draws no issue at all. The zero
// description still draws `required`, rule mandatory, at each of the other
// RM-mandatory attributes it lacks, so the check leaves out parent_resource
// alone and not the whole class.
func TestValidateRM_REQ112_ResourceDescriptionParentResourceLeftOut(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want []string // code path layer rule, sorted
	}{
		{
			name: "filled description",
			body: `{"_type":"RESOURCE_DESCRIPTION","original_author":{"name":"A"},"lifecycle_state":"published",` +
				`"details":{"en":{"_type":"RESOURCE_DESCRIPTION_ITEM","language":{"terminology_id":{"value":"ISO_639-1"},"code_string":"en"},"purpose":"p"}}}`,
			want: nil,
		},
		{
			name: "zero description",
			body: `{"_type":"RESOURCE_DESCRIPTION"}`,
			want: []string{
				"required /details rm mandatory",
				"required /lifecycle_state rm mandatory",
				"required /original_author rm mandatory",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, err := typereg.Default.Decode([]byte(tc.body))
			if err != nil {
				t.Fatalf("typereg.Default.Decode(%s): %v", tc.name, err)
			}
			r := validation.ValidateRM(root)
			for _, issue := range r.Issues {
				if issue.Path == "/parent_resource" {
					t.Errorf("ValidateRM(%s) reports %s, rule %s, at /parent_resource; no value can be held there", tc.name, issue.Code, issue.Rule)
				}
			}
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues (code path layer rule) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
