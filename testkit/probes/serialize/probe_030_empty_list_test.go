package serializeprobes_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	serializeprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/serialize"
)

// TestProbe030PassesAnEmptyOptionalList pins that PROBE-030 passes a valid
// input that carries a present, empty optional list (REQ-052): the encoder
// leaves it out, and the input's expected form reads it as absent, so the
// input leg finds the first encode wire-equivalent to it.
func TestProbe030PassesAnEmptyOptionalList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		factory func() any
	}{
		{
			name:    "ELEMENT with empty links",
			body:    `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"Systolic"},"value":{"_type":"DV_TEXT","value":"high"},"links":[]}`,
			factory: func() any { return new(rm.Element) },
		},
		{
			name:    "CLUSTER whose ELEMENT has empty links",
			body:    `{"_type":"CLUSTER","archetype_node_id":"at0000","name":{"_type":"DV_TEXT","value":"Blood pressure"},"links":[],"items":[{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"Systolic"},"value":{"_type":"DV_TEXT","value":"high"},"links":[]}]}`,
			factory: func() any { return new(rm.Cluster) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := serializeprobes.Probe030CanjsonRoundTrip([]byte(tc.body), tc.factory)
			if err != nil {
				t.Fatalf("probe framework error: %v", err)
			}
			if r.Status != "pass" {
				t.Errorf("PROBE-030 status = %q (detail: %s); want pass", r.Status, r.Detail)
			}
		})
	}
}
