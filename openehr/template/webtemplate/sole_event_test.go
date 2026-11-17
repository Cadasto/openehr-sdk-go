package webtemplate_test

// REQ-106, issue 452 — the reference lifts an event of any subtype that is
// its HISTORY's only event and can occur at most once (openEHR_SDK
// Filter.skip at the commit ADR-0014 pins): the event gives no node, the
// paths below it keep its events[…] segment, and its in-context leaves join
// its other children under the OBSERVATION. An event at max 1 beside other
// events stays a node, whatever its subtype.

import (
	"bytes"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// labEvent is the path of the one event of the corpus laboratory test
// report's OBSERVATION: a POINT_EVENT at 0..1.
const labEvent = "/content[openEHR-EHR-OBSERVATION.laboratory_test.v0]/data[at0001]/events[at0002]"

// labReportAs returns the corpus laboratory test report's OPT with its one
// event's RM type replaced by eventType.
func labReportAs(t *testing.T, eventType string) []byte {
	t.Helper()
	raw := readFile(t, fixtures.TemplateOpt("IDCR - Laboratory Test Report.v0"))
	point := []byte("<rm_type_name>POINT_EVENT</rm_type_name>")
	if n := bytes.Count(raw, point); n != 1 {
		t.Fatalf("the laboratory test report names POINT_EVENT %d times, want once — fixture changed?", n)
	}
	return bytes.Replace(raw, point, []byte("<rm_type_name>"+eventType+"</rm_type_name>"), 1)
}

// TestREQ106SoleEventIsLifted — REQ-106, issue 452. The sole event of the
// laboratory test report gives no node, as a POINT_EVENT and as an
// INTERVAL_EVENT. Its data and its in-context leaves sit under the
// OBSERVATION, so the reference's key laboratory_test_report/laboratory_test/test_name
// names a leaf, and their paths keep the event's segment.
func TestREQ106SoleEventIsLifted(t *testing.T) {
	testName := labEvent + "/data[at0003]/items[at0005]/value"
	for _, tc := range []struct {
		eventType string
		leaves    map[string]string // id under the OBSERVATION -> aqlPath
	}{
		{eventType: "POINT_EVENT", leaves: map[string]string{
			"test_name": testName,
			"time":      labEvent + "/time",
		}},
		{eventType: "INTERVAL_EVENT", leaves: map[string]string{
			"test_name":     testName,
			"time":          labEvent + "/time",
			"math_function": labEvent + "/math_function",
			"width":         labEvent + "/width",
		}},
	} {
		t.Run(tc.eventType, func(t *testing.T) {
			wt := buildOPT(t, labReportAs(t, tc.eventType))
			walkOurTree(wt.Tree, func(n *webtemplate.Node) {
				if n.AQLPath == labEvent {
					t.Errorf("Build kept the sole %s as node %q, want it lifted", n.RMType, n.ID)
				}
			})
			obs := childByID(wt.Tree, "laboratory_test")
			if obs == nil {
				t.Fatalf("root children %v, want laboratory_test", childIDs(wt.Tree))
			}
			for id, path := range tc.leaves {
				leaf := childByID(obs, id)
				if leaf == nil {
					t.Errorf("laboratory_test children %v, want %q", childIDs(obs), id)
					continue
				}
				if leaf.AQLPath != path {
					t.Errorf("laboratory_test/%s aqlPath = %q, want %q", id, leaf.AQLPath, path)
				}
			}
		})
	}
}

// TestREQ106EventWithSiblingsStaysANode — REQ-106, issue 452. The
// Demonstration template's OBSERVATION has four events, each at max 1: an
// abstract EVENT, two POINT_EVENTs and an INTERVAL_EVENT. None is its
// HISTORY's only event, so each stays a node, the abstract EVENT included.
func TestREQ106EventWithSiblingsStaysANode(t *testing.T) {
	wt := buildOPT(t, readFile(t, fixtures.TemplateOpt("Demonstration.v1")))
	const events = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data[at0001]/events"
	want := map[string]string{
		events + "[at0002]": "EVENT",
		events + "[at0033]": "POINT_EVENT",
		events + "[at0034]": "INTERVAL_EVENT",
		events + "[at0035]": "POINT_EVENT",
	}
	got := map[string]string{}
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if _, ok := want[n.AQLPath]; ok {
			got[n.AQLPath] = n.RMType
		}
	})
	for path, rmType := range want {
		if got[path] != rmType {
			t.Errorf("node at %s has rmType %q, want a %s node", path, got[path], rmType)
		}
	}
}
