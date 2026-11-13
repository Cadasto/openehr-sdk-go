package composition_test

// alternative_paths_test.go: Set at a path below one of several nodes that
// share a path, ISM_TRANSITION alternatives and same-id siblings, where
// Compiled.NodeAt returns the node below the first of them (REQ-101 Type
// enforcement and Skeleton policy).

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// allTypesAction is the path of the ACTION in the cross-format
// test_all_types template, whose ism_transition lists three ISM_TRANSITION
// alternatives: at0003 (planned, 526), at0004 (active, 245) and at0005
// (completed, 532), each with its own careflow step.
const allTypesAction = "/content[openEHR-EHR-SECTION.test_all_types.v1]/items[at0001]/items[at0002]/items[openEHR-EHR-ACTION.test_all_types.v1]"

// compileAllTypes compiles the cross-format test_all_types template.
func compileAllTypes(t testing.TB) *templatecompile.Compiled {
	t.Helper()
	opt, err := readOPTFile(filepath.Join(fixtures.CrossFormatRoot(), "test_all_types", "template.opt"))
	if err != nil {
		t.Fatalf("readOPTFile test_all_types: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile test_all_types: %v", err)
	}
	return c
}

// TestREQ101_SetBelowALaterTransitionAlternative checks Set at the
// current_state and careflow_step of an ISM_TRANSITION, whose paths NodeAt
// resolves below the first of three alternatives. Set checks each value's
// type alone, so it accepts a later alternative's state and step, and Build
// writes them; the validator then binds the transition to that alternative
// and adds no finding. Set leaves the transition's consistency to the
// validator: with the later step alone, beside the state the skeleton built
// from the first alternative, Set and Build still return nil, and the
// validator, which binds the transition to the first alternative by its
// state, reports a finding at the step's code.
func TestREQ101_SetBelowALaterTransitionAlternative(t *testing.T) {
	t.Parallel()
	c := compileAllTypes(t)
	coded := func(terminology, code, text string) *rm.DVCodedText {
		return &rm.DVCodedText{Value: text, DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology}}}
	}
	const (
		state = allTypesAction + "/ism_transition/current_state"
		step  = allTypesAction + "/ism_transition/careflow_step"
	)
	cases := []struct {
		name string
		set  map[string]*rm.DVCodedText
		// wantAt is the path of each finding the validator adds, "" for none.
		wantAt string
	}{
		{
			name: "state and step of the at0004 alternative",
			set: map[string]*rm.DVCodedText{
				state: coded("openehr", "245", "active"),
				step:  coded("local", "at0004", "Active"),
			},
		},
		{
			name:   "step of the at0004 alternative beside the skeleton's state",
			set:    map[string]*rm.DVCodedText{step: coded("local", "at0004", "Active")},
			wantAt: step + "/defining_code",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), c,
				composition.WithTerritory("NL"), composition.WithComposer(testComposer()))
			if err != nil {
				t.Fatalf("NewBuilder(test_all_types): %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			before := validation.ValidateComposition(skeleton, c)
			for path, v := range tc.set {
				if err := b.Set(path, v); err != nil {
					t.Fatalf("Set(%s, %s::%s) = %v, want nil", path, v.DefiningCode.TerminologyID.Value, v.DefiningCode.CodeString, err)
				}
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build = %v, want nil", err)
			}
			for path, v := range tc.set {
				got, err := rmpath.ItemAtPath(comp, path)
				if err != nil {
					t.Fatalf("ItemAtPath(%s) on the built composition: %v", path, err)
				}
				text, ok := got.(rm.DVTextLike)
				if !ok {
					t.Fatalf("built composition holds %T at %s, want a coded text", got, path)
				}
				if code, _ := text.GetDefiningCode(); code.CodeString != v.DefiningCode.CodeString {
					t.Errorf("built composition holds a text coded %q at %s, want the value Set was given, coded %q", code.CodeString, path, v.DefiningCode.CodeString)
				}
			}
			added := newIssues(before, validation.ValidateComposition(comp, c))
			if tc.wantAt == "" && len(added) > 0 {
				t.Errorf("ValidateComposition after Set reports new findings %q, want none", added)
			}
			if tc.wantAt != "" && len(added) == 0 {
				t.Errorf("ValidateComposition after Set reports no new finding, want one at %s", tc.wantAt)
			}
			for _, issue := range added {
				if _, at, _ := strings.Cut(issue, " at "); tc.wantAt != "" && at != tc.wantAt {
					t.Errorf("ValidateComposition after Set reports the new finding %q, want each at %s", issue, tc.wantAt)
				}
			}
		})
	}
}

// TestREQ101_SetBelowASameIDSibling checks Set at the value of two ELEMENT
// siblings of one node id that pin no name, a DV_TEXT under the first and a
// DV_COUNT under the second. They share the path, and NodeAt returns the
// value under the first, so Set reaches the first use alone: it accepts a
// DV_TEXT and refuses a DV_COUNT, the second sibling's type, with
// ErrTypeMismatch.
func TestREQ101_SetBelowASameIDSibling(t *testing.T) {
	t.Parallel()
	c := compileFixture(t, "sdk_crosscheck_siblings.v1")
	const path = "/context/other_context/items[at0002]/value"
	cases := []struct {
		name    string
		v       any
		wantErr error
	}{
		{name: "DV_TEXT, the first sibling's type", v: &rm.DVText{Value: "first"}},
		{name: "DV_COUNT, the second sibling's type", v: &rm.DVCount{Magnitude: 2}, wantErr: composition.ErrTypeMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), c,
				composition.WithTerritory("NL"), composition.WithComposer(testComposer()))
			if err != nil {
				t.Fatalf("NewBuilder(sdk_crosscheck_siblings.v1): %v", err)
			}
			if err := b.Set(path, tc.v); !errors.Is(err, tc.wantErr) {
				t.Errorf("Set(%s, %T) = %v, want %v", path, tc.v, err, tc.wantErr)
			}
		})
	}
}
