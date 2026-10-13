package validation_test

// issue_err_test.go: REQ-102 § Sentinels. Issue.Err bridges each code the
// table names to its sentinel. Every other code returns nil: the guards, the
// walk conditions, and the codes of the RM rules, which every instance
// validator reports, the template-driven ones included.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

func TestIssueErr_REQ102_Sentinels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code string
		want error // nil when no sentinel maps
	}{
		{code: "required", want: validation.ErrRequired},
		{code: "cardinality", want: validation.ErrCardinality},
		{code: "rm_type_mismatch", want: validation.ErrTypeMismatch},
		{code: "alternative_mismatch", want: validation.ErrTypeMismatch},
		{code: "archetype_id_mismatch", want: validation.ErrTypeMismatch},
		{code: "node_id_mismatch", want: validation.ErrTypeMismatch},
		{code: "primitive_out_of_range", want: validation.ErrPrimitive},
		{code: "primitive_not_in_list", want: validation.ErrPrimitive},
		{code: "slot_fill", want: validation.ErrSlotFill},
		{code: "aql_syntax", want: validation.ErrAQLSyntax},
		{code: "aql_empty", want: validation.ErrAQLSyntax},
		{code: "nil_root"},
		{code: "nil_template"},
		{code: "nil_composition"},
		{code: "nil_party"},
		{code: "nil_folder"},
		{code: "nil_ehr_status"},
		{code: "nil_ehr_access"},
		{code: "rm_type_unknown"},
		{code: "invalid_shape"},
		{code: "max_depth"},
		{code: "max_nodes"},
		{code: "cyclic_value"},
		{code: "rm_invariant"},
		{code: "mappings_valid"},
		{code: "term_mapping_match"},
		{code: "lower_included_valid"},
		{code: "upper_included_valid"},
		{code: "code_not_in_value_set"},
		{code: "is_archetype_root"},
		{code: "rm_version_valid"},
		{code: "aql_from_archetype"},
	} {
		got := validation.Issue{Code: tc.code}.Err()
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("Issue{Code: %q}.Err() = %v, want nil", tc.code, got)
		case tc.want != nil && !errors.Is(got, tc.want):
			t.Errorf("Issue{Code: %q}.Err() = %v, want %v", tc.code, got, tc.want)
		}
	}
}
