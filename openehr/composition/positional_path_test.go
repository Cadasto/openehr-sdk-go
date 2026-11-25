package composition_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// keyedParticipationConstraint is participationConstraint with the node id
// at0001, so its compiled path takes that id as its predicate.
var keyedParticipationConstraint = strings.Replace(participationConstraint,
	"<rm_type_name>PARTICIPATION</rm_type_name><node_id></node_id>",
	"<rm_type_name>PARTICIPATION</rm_type_name><node_id>at0001</node_id>", 1)

// mappingConstraint is a TERM_MAPPING child constraint with a required
// CODE_PHRASE target. A TERM_MAPPING is no LOCATABLE and carries no node
// id, so its compiled path takes a positional key.
const mappingConstraint = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>TERM_MAPPING</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>target</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id></children></attributes>
</children>
`

// mappedParticipation returns participationConstraint with its DV_TEXT
// function constraining mappings with n TERM_MAPPING child constraints.
func mappedParticipation(n int) string {
	mappings := `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>mappings</rm_attribute_name>` + requiredExistence + "\n" +
		strings.Repeat(mappingConstraint, n) +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality></attributes>
`
	return strings.Replace(participationConstraint,
		`<rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>`,
		`<rm_type_name>DV_TEXT</rm_type_name><node_id></node_id>`+"\n"+mappings+`</children>`, 1)
}

// participationsOPT returns a COMPOSITION template whose
// EVENT_CONTEXT.participations lists the given child constraints, in order.
func participationsOPT(children ...string) string {
	return participationsOPTHead + strings.Join(children, "") + participationsOPTTail
}

// positionalMark is the value the positional-path tests set: a function text
// or a mapping target code that the skeleton never writes.
const positionalMark = "SECOND"

// positionalValue returns the value set at path: a CODE_PHRASE at a mapping
// target, else a DV_TEXT.
func positionalValue(path string) any {
	if strings.HasSuffix(path, "/target") {
		return &rm.CodePhrase{CodeString: positionalMark, TerminologyID: rm.TerminologyID{Value: "local"}}
	}
	return &rm.DVText{Value: positionalMark}
}

// positionalWrites lists where comp holds positionalMark: each participation
// whose function text, or whose function's mapping target code, it is.
func positionalWrites(comp *rm.Composition) []string {
	if comp == nil || comp.Context == nil {
		return nil
	}
	var writes []string
	for i, p := range comp.Context.Participations {
		if p.Function == nil {
			continue
		}
		if p.Function.GetValue() == positionalMark {
			writes = append(writes, "participations["+strconv.Itoa(i)+"].function")
		}
		f, ok := p.Function.(*rm.DVText)
		if !ok {
			continue
		}
		for j, m := range f.Mappings {
			if m.Target.CodeString == positionalMark {
				writes = append(writes, "participations["+strconv.Itoa(i)+"].function.mappings["+strconv.Itoa(j)+"].target")
			}
		}
	}
	return writes
}

// TestREQ101_SetRefusesAPositionalSegmentOfSeveralChildren checks, for
// REQ-101, that Set returns ErrInvalidPath, Build carries it and nothing is
// written, for a path through a positional segment ([@N], the key the
// compile step gives a list child with no archetype id, node id or slot
// predicate) under a multi-valued attribute for which the template lists
// more than one child of any kind. The skeleton's list order does not follow
// the template's, and a PARTICIPATION or TERM_MAPPING carries no node id to
// match, so the builder cannot tell which item the position names. Every
// positional segment on the path counts, the outer one as well as the inner.
func TestREQ101_SetRefusesAPositionalSegmentOfSeveralChildren(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, opt, path string
	}{
		{
			name: "second of two positional PARTICIPATIONs",
			opt:  participationsOPT(participationConstraint, participationConstraint),
			path: "/context/participations[@2]/function",
		},
		{
			name: "first of two positional PARTICIPATIONs",
			opt:  participationsOPT(participationConstraint, participationConstraint),
			path: "/context/participations[@1]/function",
		},
		{
			name: "positional PARTICIPATION after one with a node id",
			opt:  participationsOPT(keyedParticipationConstraint, participationConstraint),
			path: "/context/participations[@2]/function",
		},
		{
			name: "positional PARTICIPATION before one with a node id",
			opt:  participationsOPT(participationConstraint, keyedParticipationConstraint),
			path: "/context/participations[@1]/function",
		},
		{
			name: "outer segment of two PARTICIPATIONs, inner of one TERM_MAPPING",
			opt:  participationsOPT(mappedParticipation(1), mappedParticipation(1)),
			path: "/context/participations[@2]/function/mappings[@1]/target",
		},
		{
			name: "outer segment of one PARTICIPATION, inner of two TERM_MAPPINGs",
			opt:  participationsOPT(mappedParticipation(2)),
			path: "/context/participations[@1]/function/mappings[@2]/target",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), compileInline(t, tc.opt),
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			v := positionalValue(tc.path)
			setErr := b.Set(tc.path, v)
			if !errors.Is(setErr, composition.ErrInvalidPath) {
				t.Errorf("Set(%s, %T) = %v, want ErrInvalidPath", tc.path, v, setErr)
			}
			if setErr != nil {
				msg := setErr.Error()
				if !strings.Contains(msg, tc.path) || !strings.Contains(msg, "more than one child") {
					t.Errorf("Set(%s) error %q, want it to name the path and say the template lists more than one child", tc.path, msg)
				}
				if strings.Contains(msg, positionalMark) {
					t.Errorf("Set(%s) error %q carries the value %q", tc.path, msg, positionalMark)
				}
			}
			comp, err := b.Build()
			if !errors.Is(err, composition.ErrInvalidPath) {
				t.Errorf("Build after Set(%s) = %v, want ErrInvalidPath", tc.path, err)
			}
			if writes := positionalWrites(comp); len(writes) > 0 {
				t.Errorf("Build after Set(%s) wrote the value at %v, want it written nowhere", tc.path, writes)
			}
		})
	}
}

// TestREQ101_SetWritesThroughASolePositionalChild checks, for REQ-101, that a
// path through positional segments each of which is the only child the
// template lists for its multi-valued attribute is settable: every member
// the skeleton holds there was built from that child, so Set writes the
// value into the first, and Build returns it with no error.
func TestREQ101_SetWritesThroughASolePositionalChild(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, opt, path, want string
	}{
		{
			name: "the only PARTICIPATION",
			opt:  participationOPT,
			path: "/context/participations[@1]/function",
			want: "participations[0].function",
		},
		{
			name: "the only TERM_MAPPING of the only PARTICIPATION",
			opt:  participationsOPT(mappedParticipation(1)),
			path: "/context/participations[@1]/function/mappings[@1]/target",
			want: "participations[0].function.mappings[0].target",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), compileInline(t, tc.opt),
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			v := positionalValue(tc.path)
			if err := b.Set(tc.path, v); err != nil {
				t.Fatalf("Set(%s, %T) = %v, want nil", tc.path, v, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s) = %v, want nil", tc.path, err)
			}
			if writes := positionalWrites(comp); len(writes) != 1 || writes[0] != tc.want {
				t.Errorf("Build after Set(%s) wrote the value at %v, want [%s]", tc.path, writes, tc.want)
			}
		})
	}
}
