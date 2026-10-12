package validation_test

// rmfloor_cycle_test.go: REQ-112 § Driver — the RM floor visits each value
// once. A value built in Go can contain itself or share a pointer between two
// attributes; the floor reports a value that recurs inside itself as
// cyclic_value where it recurs, walks a shared value at its first path only,
// and keeps max_depth for a graph that is deep but acyclic. Without these
// guards such a value made the walk branch at every level, which never ends
// in practice, so each case runs under a deadline.

import (
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// walkDeadline bounds each validation in this file. The bounded walk takes
// microseconds; an unbounded one does not finish at all.
const walkDeadline = 10 * time.Second

// validateWithin runs validate and returns its result, failing the test when
// it has not returned within walkDeadline. On that failure the goroutine is
// left running until the test binary exits: the walk it runs cannot be
// stopped from outside.
func validateWithin(t *testing.T, call string, validate func() validation.Result) validation.Result {
	t.Helper()
	done := make(chan validation.Result, 1)
	go func() { done <- validate() }()
	select {
	case r := <-done:
		return r
	case <-time.After(walkDeadline):
		t.Fatalf("%s did not return within %v; want the RM-floor walk bounded", call, walkDeadline)
		return validation.Result{}
	}
}

// REQ-112 — a value that contains itself is reported as cyclic_value at each
// path where it recurs, and the walk does not descend into it again, so it
// ends without reaching max_depth.
func TestValidateRM_REQ112_CyclicValue(t *testing.T) {
	tests := []struct {
		name string
		root func() any
		want []string
	}{
		{
			name: "SECTION holding itself twice",
			root: func() any {
				s := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "s"}}
				s.Items = []rm.ContentItem{s, s}
				return s
			},
			want: []string{"cyclic_value /items[1]", "cyclic_value /items[2]"},
		},
		{
			name: "two CLUSTERs holding each other",
			root: func() any {
				a := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "a"}}
				b := &rm.Cluster{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "b"}, Items: []rm.Item{a}}
				a.Items = []rm.Item{b}
				return a
			},
			want: []string{"cyclic_value /items[at0002]/items[at0001]"},
		},
		{
			name: "DV_CODED_TEXT reached again through a single-valued attribute",
			root: func() any {
				// openehr::671 is in the term mapping purpose group, so the
				// text is a valid purpose for the mapping it carries.
				text := &rm.DVCodedText{
					Value: "research study",
					DefiningCode: rm.CodePhrase{
						TerminologyID: rm.TerminologyID{Value: "openehr"},
						CodeString:    "671",
					},
				}
				target := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"}
				text.Mappings = []rm.TermMapping{{Match: rm.Character("="), Target: target, Purpose: text}}
				return text
			},
			want: []string{"cyclic_value /mappings[1]/purpose"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.root()
			r := validateWithin(t, "ValidateRM", func() validation.Result { return validation.ValidateRM(root) })
			if got := issueKeys(r.Issues); strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("ValidateRM(%s) issues = %q, want %q", tc.name, got, tc.want)
			}
			if r.OK {
				t.Errorf("ValidateRM(%s).OK = true, want false: cyclic_value is an error", tc.name)
			}
		})
	}
}

// REQ-112 — a value two attributes share is walked once, at the first path
// that reaches it. A CLUSTER that holds the next level twice, 40 levels deep,
// reaches its last level by 2^40 paths; the floor reports that level's one
// defect once, at the first of them, and never calls it a cycle.
func TestValidateRM_REQ112_SharedValueWalkedOnce(t *testing.T) {
	const levels = 40
	leaf := &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "leaf"}} // no items: a cardinality breach
	root := leaf
	for range levels {
		root = &rm.Cluster{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.Item{root, root}}
	}

	r := validateWithin(t, "ValidateRM", func() validation.Result { return validation.ValidateRM(root) })

	want := []string{"cardinality " + strings.Repeat("/items[1]", levels) + "/items"}
	if got := issueKeys(r.Issues); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ValidateRM(%d-level shared CLUSTER graph) issues = %q, want %q", levels, got, want)
	}
}

// REQ-112 — a leaf the floor does not descend, here a PARTY_REF two
// performers share, is also checked once: its breach appears at the first
// path only.
func TestValidateRM_REQ112_SharedLeafCheckedOnce(t *testing.T) {
	ref := &rm.PartyRef{ID: &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"}, Type: "PERSON"} // namespace missing
	ctx := &rm.EventContext{
		StartTime: rm.DVDateTime{Value: "2026-10-12T09:00:00Z"},
		Participations: []rm.Participation{
			{Function: &rm.DVText{Value: "first"}, Performer: &rm.PartySelf{ExternalRef: ref}},
			{Function: &rm.DVText{Value: "second"}, Performer: &rm.PartySelf{ExternalRef: ref}},
		},
	}

	r := validateWithin(t, "ValidateRM", func() validation.Result { return validation.ValidateRM(ctx) })

	first := "/participations[1]/performer/external_ref/namespace"
	second := "/participations[2]/performer/external_ref/namespace"
	if n := countIssue(r.Issues, first, "rm_invariant"); n != 1 {
		t.Errorf("ValidateRM(shared PARTY_REF) carries rm_invariant at %s %d times, want once; issues %q", first, n, issueKeys(r.Issues))
	}
	if n := countIssue(r.Issues, second, "rm_invariant"); n != 0 {
		t.Errorf("ValidateRM(shared PARTY_REF) carries rm_invariant at %s %d times, want none: a shared value carries its first path only; issues %q", second, n, issueKeys(r.Issues))
	}
}

// REQ-112 — a graph that is deep but has no cycle still stops at max_depth,
// at the node the limit is reached on, and is not reported as cyclic.
func TestValidateRM_REQ112_DeepAcyclicGraphStopsAtMaxDepth(t *testing.T) {
	const levels = 300 // past the walk's depth limit of 256
	root := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
	for range levels {
		root = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{root}}
	}

	r := validateWithin(t, "ValidateRM", func() validation.Result { return validation.ValidateRM(root) })

	deepest := strings.Repeat("/items[at0001]", 256)
	if !containsIssue(r.Issues, deepest, "max_depth") {
		t.Errorf("ValidateRM(%d nested SECTIONs) has no max_depth at the SECTION 256 levels down; issues %q", levels, issueKeys(r.Issues))
	}
	if containsCode(r.Issues, "cyclic_value") {
		t.Errorf("ValidateRM(%d nested SECTIONs) reports cyclic_value, want none: no SECTION holds itself; issues %q", levels, issueKeys(r.Issues))
	}
}

// REQ-112, REQ-102 — the template-driven entries run the same floor, so a
// cycle in a part of the composition the template does not model is bounded
// there too and reported as cyclic_value. The device CLUSTER fills a slot of
// vital_signs.opt, so only the floor walks it.
func TestValidateComposition_REQ112_CyclicValueBounded(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	tree, ok := bpObservation(t, comp).Protocol.(*rm.ItemTree)
	if !ok || len(tree.Items) != 1 {
		t.Fatalf("fixture protocol is %T, want an ITEM_TREE holding the device CLUSTER", bpObservation(t, comp).Protocol)
	}
	device, ok := tree.Items[0].(*rm.Cluster)
	if !ok {
		t.Fatalf("fixture protocol item is %T, want *rm.Cluster", tree.Items[0])
	}
	device.Items = append(device.Items, device, device)

	r := validateWithin(t, "ValidateComposition", func() validation.Result { return validation.ValidateComposition(comp, c) })

	const clusterPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol/items[openEHR-EHR-CLUSTER.device.v1]"
	want := []string{"cyclic_value " + clusterPath + "/items[2]", "cyclic_value " + clusterPath + "/items[3]"}
	if got := issueKeys(r.Issues); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ValidateComposition(device CLUSTER holding itself twice) issues = %q, want %q", got, want)
	}
}
