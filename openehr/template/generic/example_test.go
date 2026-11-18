package generic_test

// REQ-178: the compiled template drives instance.Generate and the validator.

import (
	"context"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/generic"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// A default EHR_STATUS built from the compiled template, then checked
// against it.
func ExampleEHRStatus() {
	c := generic.EHRStatus()
	fmt.Println(c.TemplateID(), c.Root().RMTypeName())

	root, err := instance.Generate(context.Background(), c, instance.Options{Policy: instance.Required})
	if err != nil {
		fmt.Println(err)
		return
	}
	status, ok := root.(*rm.EHRStatus)
	if !ok {
		fmt.Printf("unexpected root %T\n", root)
		return
	}
	fmt.Println(status.ArchetypeNodeID, validation.ValidateEHRStatus(status, c).OK)
	// Output:
	// generic_ehr_status.v1 EHR_STATUS
	// openEHR-EHR-EHR_STATUS.generic.v1 true
}
