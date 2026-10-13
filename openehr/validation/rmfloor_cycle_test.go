package validation_test

// rmfloor_cycle_test.go: REQ-112 § Driver — the RM floor visits each value
// held by pointer once. A value built in Go can contain itself or share a
// pointer between two attributes; the floor reports a value that recurs
// inside itself as cyclic_value where it recurs, walks a shared value at its
// first path only, and keeps max_depth for a graph that is deep but acyclic.
// A value held by value can contain itself through a list that shares its own
// backing array, with no pointer to recognise; the walk stops after a fixed
// number of visits and reports max_nodes once. Without these guards such a
// value made the walk branch at every level, which never ends in practice, so
// each case runs under a deadline.

import (
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// walkDeadline bounds each validation in this file that the visit record
// ends. The bounded walk takes microseconds; an unbounded one does not finish
// at all.
const walkDeadline = 10 * time.Second

// visitLimitDeadline bounds a validation that only the visit limit ends: the
// walk runs to the limit first, which takes a fraction of a second, longer
// under the race detector. The deadline stops a walk the limit fails to end;
// it is wide, so a loaded machine running many tests at once does not miss
// it. What the walk visits is pinned by the visit count elsewhere, not by
// this deadline.
var visitLimitDeadline = func() time.Duration {
	if raceEnabled {
		return 30 * time.Second
	}
	return 10 * time.Second
}()

// validateWithin runs validate and returns its result, failing the test when
// it has not returned within deadline. On that failure the goroutine is left
// running until the test binary exits: the walk it runs cannot be stopped
// from outside.
func validateWithin(t *testing.T, call string, deadline time.Duration, validate func() validation.Result) validation.Result {
	t.Helper()
	done := make(chan validation.Result, 1)
	go func() { done <- validate() }()
	select {
	case r := <-done:
		return r
	case <-time.After(deadline):
		t.Fatalf("%s did not return within %v; want the RM-floor walk bounded", call, deadline)
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
			r := validateWithin(t, "ValidateRM", walkDeadline, func() validation.Result { return validation.ValidateRM(root) })
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

	r := validateWithin(t, "ValidateRM", walkDeadline, func() validation.Result { return validation.ValidateRM(root) })

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

	r := validateWithin(t, "ValidateRM", walkDeadline, func() validation.Result { return validation.ValidateRM(ctx) })

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
// once, at the first node the limit is reached on, and is not reported as
// cyclic. That node is the name of the SECTION 255 levels down, which the
// walk reaches before the SECTION 256 levels down.
func TestValidateRM_REQ112_DeepAcyclicGraphStopsAtMaxDepth(t *testing.T) {
	const levels = 300 // past the walk's depth limit of 256
	root := nestedSections(levels)

	r := validateWithin(t, "ValidateRM", walkDeadline, func() validation.Result { return validation.ValidateRM(root) })

	first := firstDepthStop("/items[at0001]", defaultDepth)
	if got := pathsOf(r.Issues, "max_depth"); len(got) != 1 || got[0] != first {
		t.Errorf("ValidateRM(%d nested SECTIONs) reports max_depth at %q, want once, at the name of the SECTION 255 levels down; issues %q", levels, got, issueKeys(r.Issues))
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

	r := validateWithin(t, "ValidateComposition", walkDeadline, func() validation.Result { return validation.ValidateComposition(comp, c) })

	const clusterPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol/items[openEHR-EHR-CLUSTER.device.v1]"
	want := []string{"cyclic_value " + clusterPath + "/items[2]", "cyclic_value " + clusterPath + "/items[3]"}
	if got := issueKeys(r.Issues); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ValidateComposition(device CLUSTER holding itself twice) issues = %q, want %q", got, want)
	}
}

// selfSharingSection returns a SECTION held by value whose items list shares
// its own backing array: each item is a copy of the section, and every copy's
// items are that same array. The section therefore contains itself twice, on
// every level, with no pointer the visit record could recognise.
func selfSharingSection() rm.Section {
	s := rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "s"}}
	s.Items = make([]rm.ContentItem, 2)
	s.Items[0], s.Items[1] = s, s
	return s
}

// countCode counts the issues that carry code.
func countCode(issues []validation.Issue, code string) int {
	n := 0
	for _, i := range issues {
		if i.Code == code {
			n++
		}
	}
	return n
}

// assertStoppedAtMaxNodes fails the test unless r reports max_nodes exactly
// once, as its last issue: the limit ends the whole walk, so nothing is
// reported after it.
func assertStoppedAtMaxNodes(t *testing.T, call string, r validation.Result) {
	t.Helper()
	if n := countCode(r.Issues, "max_nodes"); n != 1 {
		t.Fatalf("%s reports max_nodes %d times, want once", call, n)
	}
	if last := r.Issues[len(r.Issues)-1]; last.Code != "max_nodes" {
		t.Errorf("%s last issue is %s at %s, want max_nodes: the visit limit ends the walk", call, last.Code, last.Path)
	}
	if r.OK {
		t.Errorf("%s.OK = true, want false: max_nodes is an error", call)
	}
}

// REQ-112 — a value held by value that contains itself through its own list
// is not recognised by the visit record, so the walk stops after its fixed
// number of visits and reports max_nodes once, within the deadline. The stop ends
// the whole walk: the INSTRUCTION's narrative, an RM-mandatory attribute the
// walk would check after the protocol that holds the loop, is not reported.
func TestValidateRM_REQ112_SelfSharingValueStopsAtMaxNodes(t *testing.T) {
	loop := rm.Cluster{ArchetypeNodeID: "at0099", Name: rm.DVText{Value: "loop"}}
	loop.Items = make([]rm.Item, 2)
	loop.Items[0], loop.Items[1] = loop, loop
	tests := []struct {
		name string
		root any
	}{
		{name: "SECTION holding itself by value", root: selfSharingSection()},
		{
			name: "INSTRUCTION whose protocol holds such a CLUSTER, narrative absent",
			root: &rm.Instruction{
				ArchetypeNodeID: "openEHR-EHR-INSTRUCTION.request.v0",
				Name:            rm.DVText{Value: "request"},
				Protocol:        &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "protocol"}, Items: []rm.Item{loop}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validateWithin(t, "ValidateRM", visitLimitDeadline, func() validation.Result { return validation.ValidateRM(tc.root) })
			assertStoppedAtMaxNodes(t, "ValidateRM("+tc.name+")", r)
		})
	}
}

// REQ-112, REQ-102 — the template-driven entries run the same floor, so a
// CLUSTER held by value that contains itself through its own items list stops
// at the visit limit there too. The device CLUSTER fills a slot of
// vital_signs.opt, so only the floor walks what it holds.
func TestValidateComposition_REQ112_SelfSharingValueStopsAtMaxNodes(t *testing.T) {
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
	loop := rm.Cluster{ArchetypeNodeID: "at0099", Name: rm.DVText{Value: "loop"}}
	loop.Items = make([]rm.Item, 2)
	loop.Items[0], loop.Items[1] = loop, loop
	device.Items = append(device.Items, loop)

	r := validateWithin(t, "ValidateComposition", visitLimitDeadline, func() validation.Result { return validation.ValidateComposition(comp, c) })
	assertStoppedAtMaxNodes(t, "ValidateComposition(device CLUSTER holding a self-sharing CLUSTER by value)", r)
}
