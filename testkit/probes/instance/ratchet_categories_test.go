package instanceprobes_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

func categoryElement() *rm.Element {
	return &rm.Element{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "item"},
	}
}

func categoryNullFlavour() *rm.DVCodedText {
	return &rm.DVCodedText{
		Value: "unknown",
		DefiningCode: rm.CodePhrase{
			CodeString:    "253",
			TerminologyID: rm.TerminologyID{Value: "openehr"},
		},
	}
}

// ratchetCategories runs ValidateRM over root and returns the sorted
// category of every finding at the given code, as issueReason keys it.
func ratchetCategories(root any, code string) []string {
	var cats []string
	for _, iss := range validation.ValidateRM(root).Issues {
		if iss.Code != code {
			continue
		}
		cat, _, _ := strings.Cut(issueReason(iss, instance.ExampleFill), ":")
		cats = append(cats, cat)
	}
	slices.Sort(cats)
	return cats
}

// TestREQ112_RatchetKeysFloorRules pins the census categories for the two
// rm_invariant rules of the REQ-112 catalogue that the generator no longer
// emits (PROBE-027), so a regression of either keeps a precise key.
func TestREQ112_RatchetKeysFloorRules(t *testing.T) {
	both := categoryElement()
	both.Value = &rm.DVText{Value: "x"}
	both.NullFlavour = categoryNullFlavour()

	neither := categoryElement()

	badTime := categoryElement()
	badTime.Value = &rm.DVDateTime{Value: "example"}

	cases := []struct {
		name string
		root any
		want []string
	}{
		{"element with value and null_flavour", both, []string{reasonFloorElementValue}},
		{"element with neither", neither, []string{reasonFloorElementValue}},
		{"element with placeholder date-time", badTime, []string{reasonFloorTemporal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ratchetCategories(tc.root, "rm_invariant")
			if !slices.Equal(got, tc.want) {
				t.Errorf("rm_invariant categories = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestREQ107_HollowBodyFloor pins the coverage floor: a body with no ELEMENT
// is reported, and a body with one is not.
func TestREQ107_HollowBodyFloor(t *testing.T) {
	hollow := &rm.Composition{}
	got, err := bodyReasons(hollow)
	if err != nil {
		t.Fatalf("bodyReasons(hollow): %v", err)
	}
	if _, ok := got[reasonHollowBody+":/"]; !ok {
		t.Errorf("bodyReasons(hollow) = %v, want a %s finding", got, reasonHollowBody)
	}

	el := categoryElement()
	el.Value = &rm.DVText{Value: "x"}
	got, err = bodyReasons(&rm.ItemTree{Items: []rm.Item{el}})
	if err != nil {
		t.Fatalf("bodyReasons(with element): %v", err)
	}
	if _, ok := got[reasonHollowBody+":/"]; ok {
		t.Errorf("bodyReasons(with element) = %v, want no %s finding", got, reasonHollowBody)
	}
}
