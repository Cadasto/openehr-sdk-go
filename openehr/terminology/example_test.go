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
