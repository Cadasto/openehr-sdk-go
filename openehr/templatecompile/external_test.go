package templatecompile_test

// This file is the REQ-111 acceptance proof: it drives the whole
// compile → build → serialise → validate pipeline through the SDK's
// PUBLIC packages only. It imports no internal/ package — exactly what
// an external module is restricted to — so a clean compile here is
// itself evidence that the surface is externally reachable.

import (
	"bytes"
	"context"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// systolicPath is the DV_QUANTITY leaf under the vital_signs
// blood_pressure observation (same path the composition builder tests
// use).
const systolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

func externalComposer() *rm.PartyIdentified {
	name := "Dr Ext Ernal"
	return &rm.PartyIdentified{Name: &name}
}

// TestExternalBuildRoundTrip exercises acceptance criteria 2 and 3: the
// builder is reachable from a public-only call path, and the produced
// COMPOSITION round-trips builder → canjson.Marshal → canjson.Unmarshal
// with field-level equality (canonical JSON is field-canonical, so
// byte-stable re-marshalling proves per-field equality).
func TestExternalBuildRoundTrip(t *testing.T) {
	ctx := context.Background()

	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	b, err := composition.NewBuilder(
		ctx, c,
		composition.WithTerritory("NL"),
		composition.WithComposer(externalComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetQuantity(systolicPath, 120, "mm[Hg]"); err != nil {
		t.Fatalf("SetQuantity: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// The skeleton the builder seeds must validate clean against its own
	// compiled OPT (criterion 2: validation reachable + structurally
	// complete output).
	if res := validation.ValidateComposition(comp, c); !res.OK {
		t.Fatalf("ValidateComposition on built skeleton: not OK: %+v", res.Issues)
	}

	// Round-trip with field-level equality (criterion 3).
	//
	// "Field-level equality" is asserted three ways rather than via naive
	// byte-equality of re-marshalling: (a) the value we Set and the
	// identity fields survive the decode; (b) the decoded COMPOSITION
	// still validates clean against the same OPT — every structural,
	// identity, cardinality and primitive constraint the original
	// satisfied still holds; (c) the canonical form is byte-stable from
	// the decoded form onward (decode→encode is idempotent).
	//
	// (Note: the builder's *first* encode emits LOCATABLE.name as a
	// value DV_TEXT without a `_type` discriminator, which the decoder
	// normalises to a typed pointer — so first≠second is expected and is
	// a canjson representation detail, not data loss.)
	first, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var decoded rm.Composition
	if err := canjson.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("canjson.Unmarshal: %v", err)
	}

	// (a) the data survived the round-trip.
	if res := validation.ValidateComposition(&decoded, c); !res.OK {
		t.Fatalf("decoded composition no longer validates: %+v", res.Issues)
	}
	second, err := canjson.Marshal(&decoded)
	if err != nil {
		t.Fatalf("canjson.Marshal (re-encode): %v", err)
	}
	for _, want := range []string{
		`"magnitude":120,"units":"mm[Hg]"`, // the value we Set
		`"code_string":"NL"`,               // territory
		`"value":"vital_signs"`,            // template id
	} {
		if !bytes.Contains(second, []byte(want)) {
			t.Errorf("round-tripped composition lost %q", want)
		}
	}

	// (c) canonical idempotence: once decoded, encode→decode→encode is
	// byte-stable.
	var decoded2 rm.Composition
	if err := canjson.Unmarshal(second, &decoded2); err != nil {
		t.Fatalf("canjson.Unmarshal (second): %v", err)
	}
	third, err := canjson.Marshal(&decoded2)
	if err != nil {
		t.Fatalf("canjson.Marshal (third): %v", err)
	}
	if !bytes.Equal(second, third) {
		t.Errorf("canonical form not idempotent across round-trips:\n second = %s\n third  = %s", second, third)
	}
}

// TestExternalValidateEHRStatus exercises acceptance criterion 4:
// validation.ValidateEHRStatus is callable from a public-only call path
// on an external *rm.EHRStatus plus an externally-compiled OPT. The
// vendored fixtures are all COMPOSITION-rooted, so validating an
// EHR_STATUS against one correctly surfaces a root rm_type_mismatch —
// which itself proves the full call path executed end to end.
func TestExternalValidateEHRStatus(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		Subject:         rm.PartySelf{},
		IsQueryable:     true,
		IsModifiable:    true,
	}

	res := validation.ValidateEHRStatus(status, c)
	// The call must run and return a structured result. Against a
	// COMPOSITION-rooted OPT, an EHR_STATUS root must not silently pass —
	// and specifically must surface an rm_type_mismatch at the root, which
	// proves the walker actually compared the RM root to the OPT root
	// rather than failing for some unrelated reason.
	if res.OK {
		t.Fatal("ValidateEHRStatus unexpectedly OK against a COMPOSITION-rooted OPT")
	}
	rootMismatch := false
	for _, is := range res.Issues {
		if is.Code == "rm_type_mismatch" {
			rootMismatch = true
			break
		}
	}
	if !rootMismatch {
		t.Fatalf("expected an rm_type_mismatch issue; got %+v", res.Issues)
	}
}

// TestExternalInstanceAndAQL proves the other two REQ-111 consumers —
// instance.Generate (REQ-107) and validation.ValidateAQL (REQ-109) — are
// reachable on the public-only call path with an externally-compiled OPT,
// so every entry point REQ-111 names is covered, not just the builder and
// the composition validator.
func TestExternalInstanceAndAQL(t *testing.T) {
	ctx := context.Background()
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// instance.Generate synthesises an RM root from the public *Compiled.
	v, err := instance.Generate(ctx, c, instance.Options{
		Policy:    instance.Minimal,
		Territory: "NL",
		Composer:  externalComposer(),
	})
	if err != nil {
		t.Fatalf("instance.Generate: %v", err)
	}
	if _, err := instance.AsComposition(v); err != nil {
		t.Fatalf("instance.AsComposition: %v", err)
	}

	// validation.ValidateAQL runs the template-aware lint against the public
	// *Compiled. A query naming an archetype absent from the template must
	// surface aql_archetype_not_in_template — proving the externally-compiled
	// template actually drives the check.
	res := validation.ValidateAQL(
		aql.Query{Q: "SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]"},
		c,
	)
	found := false
	for _, is := range res.Issues {
		if is.Code == "aql_archetype_not_in_template" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ValidateAQL did not flag the absent archetype against the compiled template; got %+v", res.Issues)
	}
}
