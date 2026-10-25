package instance_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// anyArchetype is the RM default of ACTIVITY.action_archetype_id, which the
// BMM documents as "Defaults to /.*/, meaning any archetype".
const anyArchetype = "/.*/"

// activityInstructionOPT is an INSTRUCTION template with one ACTIVITY
// at0002 whose description is an empty ITEM_TREE and whose other
// attributes are attrs.
func activityInstructionOPT(attrs ...string) string {
	activity := optNode("ACTIVITY", "at0002", append([]string{optSingle("description", emptyTree)}, attrs...)...)
	return optTemplate("INSTRUCTION", append(entryAttrs,
		optSingle("narrative", optNode("DV_TEXT", "")),
		optMultiple("activities", activity))...)
}

// onlyActivity is the one ACTIVITY of a generated INSTRUCTION root.
func onlyActivity(t *testing.T, out any) rm.Activity {
	t.Helper()
	in, ok := out.(*rm.Instruction)
	if !ok {
		t.Fatalf("root is %T, want *rm.Instruction", out)
	}
	if len(in.Activities) != 1 {
		t.Fatalf("INSTRUCTION.activities has %d members, want 1", len(in.Activities))
	}
	return in.Activities[0]
}

// TestREQ107_ActivityActionArchetypeIDTakesTheRMDefault is the REQ-107 RM
// defaults check on an ACTIVITY's action_archetype_id. Where the OPT leaves
// it silent, or puts on it a C_STRING with neither a pattern nor a list,
// the generator writes the RM default /.*/, not the open-string example.
// Where the OPT gives a pattern, it writes an id the pattern matches, and
// where it gives a list, a listed id (Precedence 1). Every root passes the
// RM floor and the template validator, under every policy, both value
// fills and both compile modes.
func TestREQ107_ActivityActionArchetypeIDTakesTheRMDefault(t *testing.T) {
	const pattern = `openEHR-EHR-ACTION\.medication\.v1`
	matches := regexp.MustCompile(`^(?:` + pattern + `)$`)
	cases := []struct {
		name  string
		opt   string
		check func(got string) bool
		want  string
	}{
		{
			name:  "silent",
			opt:   activityInstructionOPT(),
			check: func(got string) bool { return got == anyArchetype },
			want:  anyArchetype,
		},
		{
			name:  "C_STRING with neither a pattern nor a list",
			opt:   activityInstructionOPT(optStringAttr("action_archetype_id", "")),
			check: func(got string) bool { return got == anyArchetype },
			want:  anyArchetype,
		},
		{
			name:  "C_STRING pattern",
			opt:   activityInstructionOPT(optStringAttr("action_archetype_id", "<pattern>"+pattern+"</pattern>")),
			check: matches.MatchString,
			want:  "an id " + pattern + " matches",
		},
		{
			name:  "C_STRING list",
			opt:   activityInstructionOPT(optStringAttr("action_archetype_id", "<list>openEHR-EHR-ACTION.procedure.v1</list>")),
			check: func(got string) bool { return got == "openEHR-EHR-ACTION.procedure.v1" },
			want:  "openEHR-EHR-ACTION.procedure.v1",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Territory, opts.Composer = "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := onlyActivity(t, out).ActionArchetypeID; !tc.check(got) {
						t.Errorf("ACTIVITY.action_archetype_id = %q, want %s", got, tc.want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
