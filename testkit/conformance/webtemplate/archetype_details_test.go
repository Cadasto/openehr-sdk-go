package webtemplate_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	conformance "github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
)

// minDecodedCorpusBodies is how many upstream bodies decode whole today with
// the template attached. The floor keeps an emptied or unreadable corpus from
// passing with nothing asserted; raise it as decode gaps close.
const minDecodedCorpusBodies = 24

// TestREQ053_REQ112_DecodedCorpusPassesRMFloor decodes every body of the
// upstream FLAT corpus as written, with the template attached, and requires
// each one that decodes whole to pass the RM floor (REQ-112). FLAT has no key
// for archetype_details, so the floor's archetype-root rule holds only if
// decode rebuilds it from the Web Template (REQ-053).
//
// The decode is strict, so a body carrying unmatched content (REQ-170) is
// refused rather than decoded without it: what is left of such a body can be
// a node whose only data the codec does not read (the CLUSTER of
// `ehrbase_conformance_cluster`, whose one item has no Web Template node), and
// that says nothing about decode. A body the codec refuses is not this test's
// concern: PROBE-086 counts those keys. What is asserted is that a body the
// codec reads whole never yields a composition the SDK's own floor rejects.
func TestREQ053_REQ112_DecodedCorpusPassesRMFloor(t *testing.T) {
	target, err := conformance.NewTarget()
	if err != nil {
		t.Fatalf("build target: %v", err)
	}
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("enumerate cases: %v", err)
	}

	var decoded int
	for _, c := range cases {
		raw, err := os.ReadFile(c.Flat)
		if err != nil {
			t.Fatalf("read %s: %v", c.Name, err)
		}
		comp, err := simplified.UnmarshalFlat(raw, target.Web, simplified.WithTemplate(target.Compiled), simplified.WithStrict())
		if err != nil {
			continue
		}
		decoded++
		t.Run(c.Name, func(t *testing.T) {
			res := validation.ValidateRM(comp)
			if res.OK {
				return
			}
			var b strings.Builder
			for _, is := range res.Issues {
				b.WriteString("\n  " + is.Code + " at " + is.Path + ": " + is.Detail)
			}
			t.Errorf("validation.ValidateRM(UnmarshalFlat(%s)) is not OK:%s", c.Name, b.String())
		})
	}
	if decoded < minDecodedCorpusBodies {
		t.Errorf("%d of %d corpus bodies decoded, want at least %d — corpus missing or decode regressed",
			decoded, len(cases), minDecodedCorpusBodies)
	}
}
