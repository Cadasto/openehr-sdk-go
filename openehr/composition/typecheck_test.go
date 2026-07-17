package composition

import (
	"errors"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestCheckRMTypeTypedInterval pins the bound-aware DV_INTERVAL
// matching (ADR 0013 review round): a compiled node declares the
// ITS-JSON parameterised name, the registry reverses the bare name,
// and rmnames.TypedIntervalName bridges the two — bound-checked, so a
// DV_INTERVAL<DV_COUNT> value cannot pass a DV_INTERVAL<DV_QUANTITY>
// node. Typed-nil pointers fail as unrecognised.
func TestCheckRMTypeTypedInterval(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("Demonstration.v1"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := tcimpl.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	nodes := c.AllByRMType("DV_INTERVAL<DV_QUANTITY>")
	if len(nodes) == 0 {
		t.Fatal("Demonstration.v1 has no DV_INTERVAL<DV_QUANTITY> node — fixture drift")
	}
	node := nodes[0]

	if err := checkRMType(node, rm.DVInterval[rm.DVQuantity]{}); err != nil {
		t.Errorf("value form: checkRMType = %v, want nil", err)
	}
	if err := checkRMType(node, &rm.DVInterval[rm.DVQuantity]{}); err != nil {
		t.Errorf("pointer form: checkRMType = %v, want nil", err)
	}
	if err := checkRMType(node, rm.DVInterval[rm.DVCount]{}); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("bound mismatch (DV_COUNT at DV_QUANTITY node): err = %v, want ErrTypeMismatch", err)
	}
	if err := checkRMType(node, (*rm.DVInterval[rm.DVQuantity])(nil)); !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("typed-nil: err = %v, want ErrTypeMismatch (unrecognised)", err)
	}
}
