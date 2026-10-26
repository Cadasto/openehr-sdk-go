package simplified_test

// REQ-053 — a FLAT or STRUCTURED body decoded with WithTemplate validates
// against the OPT it was written from. The formats fold every ITEM_STRUCTURE
// away, so decode takes the folded structure's concrete subtype from the
// compiled template; and FLAT carries order only among the instances of one
// node, so decode places the members of one container list in the Web
// Template's node order.

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// bmiTemplate builds the Web Template and the compiled template of the BMI
// corpus OPT. Its OBSERVATION protocol is an ITEM_LIST of three ELEMENTs whose
// FLAT ids (method, formula, comment) sort in the reverse of their template
// order (at0006, at0010, at0011).
func bmiTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt("BMI"))
	if err != nil {
		t.Fatalf("ParseFile(BMI): %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(BMI): %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(BMI): %v", err)
	}
	return wt, compiled
}

// bmiSkeleton is the composition NewSkeleton writes for the BMI template under
// instance.Medium, on a pinned clock.
func bmiSkeleton(t *testing.T, compiled *templatecompile.Compiled) *rm.Composition {
	t.Helper()
	name := "Composer"
	comp, err := composition.NewSkeleton(t.Context(), compiled,
		composition.WithPolicy(instance.Medium),
		composition.WithComposer(&rm.PartyIdentified{Name: &name}),
		composition.WithTerritory("NL"),
		composition.WithNow(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("NewSkeleton(BMI): %v", err)
	}
	if r := validation.ValidateComposition(comp, compiled); !r.OK {
		t.Fatalf("the written BMI skeleton is invalid: %v", r.Issues)
	}
	return comp
}

// bmiRoundTrip encodes comp in format ("FLAT" or "STRUCTURED") and decodes it
// back with opts.
func bmiRoundTrip(t *testing.T, format string, comp *rm.Composition, wt *webtemplate.WebTemplate, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	encode, decode := simplified.MarshalFlat, simplified.UnmarshalFlat
	if format == "STRUCTURED" {
		encode, decode = simplified.MarshalStructured, simplified.UnmarshalStructured
	}
	body, err := encode(comp, wt)
	if err != nil {
		t.Fatalf("Marshal %s: %v", format, err)
	}
	back, err := decode(body, wt, opts...)
	if err != nil {
		t.Fatalf("Unmarshal %s: %v", format, err)
	}
	return back
}

// bmiProtocol returns the canonical JSON of the BMI OBSERVATION's protocol.
func bmiProtocol(t *testing.T, comp *rm.Composition) map[string]any {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc struct {
		Content []struct {
			Protocol map[string]any `json:"protocol"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal canonical JSON: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Protocol == nil {
		t.Fatalf("canonical JSON carries %d content items, want one OBSERVATION with a protocol", len(doc.Content))
	}
	return doc.Content[0].Protocol
}

// memberIDs returns the archetype_node_id of each member of a structure's
// items list, in list order.
func memberIDs(structure map[string]any) []string {
	items, _ := structure["items"].([]any)
	ids := make([]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		id, _ := m["archetype_node_id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// TestREQ053WithTemplateRoundTripValidatesBMI — REQ-053: the BMI skeleton,
// written as FLAT or STRUCTURED and decoded with WithTemplate, validates
// against the OPT. Its protocol comes back as the ITEM_LIST the template
// names, not the ITEM_TREE the path alone would suggest, and its members in
// the order they were written.
func TestREQ053WithTemplateRoundTripValidatesBMI(t *testing.T) {
	wt, compiled := bmiTemplate(t)
	comp := bmiSkeleton(t, compiled)
	written := bmiProtocol(t, comp)
	if written["_type"] != "ITEM_LIST" {
		t.Fatalf("the written protocol is a %v, want ITEM_LIST — fixture changed?", written["_type"])
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			got := bmiProtocol(t, back)
			if got["_type"] != "ITEM_LIST" {
				t.Errorf("decoded protocol _type = %v, want ITEM_LIST", got["_type"])
			}
			if g, w := memberIDs(got), memberIDs(written); !slices.Equal(g, w) {
				t.Errorf("decoded protocol members = %v, want the written order %v", g, w)
			}
		})
	}
}

// TestREQ053OPTFreeDecodeKeepsTheInference — REQ-053: without WithTemplate the
// path alone names the folded structure, and `items` stays ITEM_TREE. The
// members still come back in template order, which the Web Template alone
// gives.
func TestREQ053OPTFreeDecodeKeepsTheInference(t *testing.T) {
	wt, compiled := bmiTemplate(t)
	comp := bmiSkeleton(t, compiled)
	written := memberIDs(bmiProtocol(t, comp))
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			got := bmiProtocol(t, bmiRoundTrip(t, format, comp, wt))
			if got["_type"] != "ITEM_TREE" {
				t.Errorf("OPT-free decoded protocol _type = %v, want ITEM_TREE", got["_type"])
			}
			if g := memberIDs(got); !slices.Equal(g, written) {
				t.Errorf("OPT-free decoded protocol members = %v, want the written order %v", g, written)
			}
		})
	}
}
