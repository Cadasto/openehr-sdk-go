package simplified_test

// REQ-053 — LOCATABLE.name repopulation on decode via WithTemplate: the decoded
// composition must be RM/OPT-conformant (every mandatory name present), not just
// format-idempotent.
import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func nameRequiredIssues(r validation.Result) []string {
	var out []string
	for _, is := range r.Issues {
		if is.Code == "required" && strings.HasSuffix(is.Path, "/name") {
			out = append(out, is.Path)
		}
	}
	return out
}

func TestDecodeWithTemplateNamesAreConformant(t *testing.T) {
	const id = "Test_dv_quantity_open_constraint.v0"
	optB, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatal(err)
	}
	opt, err := fixtures.ParseOPTBytes(optB)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatal(err)
	}
	compB, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compB, &comp); err != nil {
		t.Fatal(err)
	}
	flat, err := simplified.MarshalFlat(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}

	// Bare decode: names are absent -> the RM-mandatory name surfaces as required.
	bare, err := simplified.UnmarshalFlat(flat, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (bare): %v", err)
	}
	if got := nameRequiredIssues(validation.Validate(bare, compiled)); len(got) == 0 {
		t.Error("bare decode: expected missing-name issues (the gap WithTemplate closes), got none")
	}

	// WithTemplate: names repopulated AND the RM-mandatory attributes the formats
	// omit are completed from ctx defaults -> the decoded composition validates
	// against the OPT.
	named, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	if r := validation.Validate(named, compiled); !r.OK {
		t.Errorf("WithTemplate decode does not validate against the OPT; issues=%v", r.Issues)
	}

	// Names must not leak into FLAT — the round-trip stays idempotent.
	flat2, err := simplified.MarshalFlat(named, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m1, m2 map[string]any
	_ = json.Unmarshal(flat, &m1)
	_ = json.Unmarshal(flat2, &m2)
	if len(m1) != len(m2) {
		t.Errorf("naming changed the FLAT key set: %d -> %d", len(m1), len(m2))
	}
}
