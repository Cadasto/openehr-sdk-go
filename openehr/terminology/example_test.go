package terminology_test

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// Look up the rubric of a composition category code. A rubric belongs to a
// code within one group, so look the code up on the group that governs it.
func ExampleGroup_Rubric() {
	rubric, ok := terminology.CompositionCategory.Rubric("433")
	fmt.Println("433:", rubric, ok)

	// 225 is a setting code. The composition category group does not know
	// it, and says so with a false second result.
	_, ok = terminology.CompositionCategory.Rubric("225")
	fmt.Println("225 is a composition category:", ok)

	rubric, ok = terminology.Setting.Rubric("225")
	fmt.Println("225:", rubric, ok)
	// Output:
	// 433: event true
	// 225 is a composition category: false
	// 225: home true
}

// Check codes against code sets. The ISO and IANA code sets ignore letter
// case, as those registers do; the openEHR code sets match exactly. Only a
// code the pinned set lists is a member: ISO-8859-1 is an alias the IANA
// register gives ISO_8859-1:1987, and the pinned set lists only the latter.
func ExampleCodeSet_Has() {
	fmt.Println(terminology.Languages.ExternalID(), terminology.Languages.Has("en-US"))
	fmt.Println(terminology.CharacterSets.Has("iso_8859-1:1987"), terminology.CharacterSets.Has("ISO-8859-1"))
	fmt.Println(terminology.NormalStatuses.Has("H"), terminology.NormalStatuses.Has("h"))
	// Output:
	// ISO_639-1 true
	// true false
	// true false
}

// Look up the units of the physical property a C_DV_QUANTITY constraint
// names, openehr::122 (Length). Membership is exact, as UCUM codes are
// case-sensitive, and mg measures mass, not length.
func ExampleQuantityPropertyByCode() {
	length, ok := terminology.QuantityPropertyByCode("122")
	fmt.Println(length.Name(), ok)
	fmt.Println(length.Has("cm"), length.Has("CM"), length.Has("mg"))
	for u := range length.All() {
		if u.Primary {
			fmt.Println("primary:", u.UCUM)
		}
	}
	// Output:
	// Length true
	// true false false
	// primary: m
}
