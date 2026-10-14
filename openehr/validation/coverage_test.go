package validation_test

// coverage_test.go: REQ-171 § Coverage — a result says what it does not
// prove. Stopped lists the path of each max_depth and max_nodes issue, at most
// one per limit; External lists, in walk order, the path of each leaf the
// template binds to a CONSTRAINT_REF; Complete is true exactly when both are
// empty. The zero Coverage, which a guard and ValidateAQL return, is
// complete.

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-171 § Coverage — Complete is true exactly when Stopped and External are
// both empty; the zero Coverage is complete, and an empty list counts as
// empty.
func TestCoverage_REQ171_Complete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		c    validation.Coverage
		want bool
	}{
		{name: "zero", want: true},
		{name: "empty lists", c: validation.Coverage{Stopped: []string{}, External: []string{}}, want: true},
		{name: "stopped", c: validation.Coverage{Stopped: []string{"/items[1]"}}, want: false},
		{name: "external", c: validation.Coverage{External: []string{"/value/defining_code"}}, want: false},
		{name: "both", c: validation.Coverage{Stopped: []string{"/"}, External: []string{"/value/defining_code"}}, want: false},
	}
	for _, tc := range tests {
		if got := tc.c.Complete(); got != tc.want {
			t.Errorf("Coverage{%s}.Complete() = %t, want %t", tc.name, got, tc.want)
		}
	}
}

// REQ-171 § Coverage, REQ-112 § Driver — a walk both limits stop lists both
// paths in Stopped, in the order the walk met them, one per limit: a SECTION
// held by value that holds itself reaches the depth limit first and the visit
// limit later. The package-level floor reports the same coverage.
func TestValidator_REQ171_StoppedListsEachLimitOnce(t *testing.T) {
	r := validateWithin(t, "Validate", visitLimitDeadline, func() validation.Result {
		return validation.New().Validate(selfSharingSection(), nil)
	})
	depth, nodes := pathsOf(r.Issues, "max_depth"), pathsOf(r.Issues, "max_nodes")
	if len(depth) != 1 || len(nodes) != 1 {
		t.Fatalf("Validate(SECTION holding itself by value) reports max_depth %d times and max_nodes %d times, want each once", len(depth), len(nodes))
	}
	if want := []string{depth[0], nodes[0]}; !slices.Equal(r.Coverage.Stopped, want) {
		t.Errorf("Validate(SECTION holding itself by value) Coverage.Stopped = %q, want %q: the max_depth path, then the max_nodes path", r.Coverage.Stopped, want)
	}
	if r.Coverage.Complete() || len(r.Coverage.External) != 0 {
		t.Errorf("Validate(SECTION holding itself by value) Coverage = %+v, want not complete and nothing external", r.Coverage)
	}
	floor := validateWithin(t, "ValidateRM", visitLimitDeadline, func() validation.Result { return validation.ValidateRM(selfSharingSection()) })
	if !slices.Equal(floor.Coverage.Stopped, r.Coverage.Stopped) {
		t.Errorf("ValidateRM(SECTION holding itself by value) Coverage.Stopped = %q, want %q", floor.Coverage.Stopped, r.Coverage.Stopped)
	}
}

// constraintRefValueChild is an ELEMENT value child DV_CODED_TEXT whose
// defining_code is a CONSTRAINT_REF: a value set the validator cannot
// consult.
const constraintRefValueChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="CONSTRAINT_REF">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <reference>ac0001</reference>
          </children>
        </attributes>
      </children>`

// constraintRefStringChild and constraintRefWrappedChild are an ELEMENT value
// child DV_TEXT whose value, a STRING, is a CONSTRAINT_REF: once directly and
// once inside a C_PRIMITIVE_OBJECT, both of which compile to a leaf on the
// AOM primitive short name STRING.
const (
	constraintRefStringChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="CONSTRAINT_REF">
            <rm_type_name>STRING</rm_type_name>
            <node_id></node_id>
            <reference>ac0001</reference>
          </children>
        </attributes>
      </children>`
	constraintRefWrappedChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id></node_id>
            <item xsi:type="CONSTRAINT_REF">
              <reference>ac0001</reference>
            </item>
          </children>
        </attributes>
      </children>`
)

// REQ-171 § Coverage — a leaf the template binds to a CONSTRAINT_REF is listed
// in Coverage.External at its issue path, and the result is not complete. The
// same value under a C_CODE_PHRASE, which the validator checks locally, is
// listed nowhere. A CONSTRAINT_REF on a primitive short name is listed
// too. In the Demonstration.v1 corpus composition the leaf the
// CONSTRAINT_REF walk test breaks is among those listed. The floor alone
// binds no template, so it lists nothing external.
func TestValidator_REQ171_ExternalListsConstraintRefLeaves(t *testing.T) {
	coded := rm.DVCodedText{
		Value:        "coded",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"},
	}
	tests := []struct {
		name string
		opt  string
		want []string
	}{
		{name: "CONSTRAINT_REF", opt: elementValueOPT(constraintRefValueChild), want: []string{"/value/defining_code"}},
		{name: "C_CODE_PHRASE", opt: elementValueOPT(externalCodeChild)},
		{name: "CONSTRAINT_REF on STRING", opt: elementValueOPT(constraintRefStringChild), want: []string{"/value/value"}},
		{name: "CONSTRAINT_REF in C_PRIMITIVE_OBJECT", opt: elementValueOPT(constraintRefWrappedChild), want: []string{"/value/value"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.New().Validate(valueElement(coded), mustCompileInline(t, tc.opt))
			if !r.OK {
				t.Fatalf("Validate(coded ELEMENT) issues = %q, want none", issueKeys(r.Issues))
			}
			if !slices.Equal(r.Coverage.External, tc.want) || len(r.Coverage.Stopped) != 0 {
				t.Errorf("Validate(coded ELEMENT) Coverage = %+v, want External %q and nothing stopped", r.Coverage, tc.want)
			}
			if got, want := r.Coverage.Complete(), len(tc.want) == 0; got != want {
				t.Errorf("Validate(coded ELEMENT) Coverage.Complete() = %t, want %t", got, want)
			}
		})
	}

	comp, _ := constraintRefCode(t)
	r := validation.New().Validate(comp, mustCompile(t, "Demonstration.v1"))
	if !slices.Contains(r.Coverage.External, constraintRefCodePath) || r.Coverage.Complete() {
		t.Errorf("Validate(Demonstration.v1) Coverage = %+v, want External to hold %q and not complete", r.Coverage, constraintRefCodePath)
	}
	if floor := validation.ValidateRM(comp); len(floor.Coverage.External) != 0 {
		t.Errorf("ValidateRM(Demonstration.v1) Coverage.External = %q, want none: the floor binds no template", floor.Coverage.External)
	}
}

// corpusExternal is, for each corpus document whose OPT holds a
// CONSTRAINT_REF, the path of each leaf the document binds to one, in walk
// order. Each was checked against the OPT and the document: Demonstration.v1
// constrains ELEMENT at0011 and at0025 that way in each of its four events,
// and the document holds a coded at0011 in the one cluster of the first three
// events and in both clusters of the fourth, but at0025 only as a
// DV_QUANTITY, which binds to another alternative. Address.v2 constrains
// at0009, which the document holds coded. TestPerson.v2 constrains at0012,
// at0034 and the address's at0009; the document holds at0012 and the
// address's at0009 coded, and at0034 as a DV_TEXT, which binds to no
// CONSTRAINT_REF.
var corpusExternal = func() map[string][]string {
	const demo = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events"
	const code = "/items[at0011]/value/defining_code"
	return map[string][]string{
		"Demonstration.v1": {
			demo + "[at0002]/data/items[at0004]" + code,
			demo + "[at0033]/data/items[at0004]" + code,
			demo + "[at0034]/data/items[at0004]" + code,
			demo + "[at0035]/data/items[2]" + code,
			demo + "[at0035]/data/items[3]" + code,
		},
		"Address.v2": {"/details/items[at0009]/value/defining_code"},
		"TestPerson.v2": {
			"/details/items[at0001]/items[at0012]/value/defining_code",
			"/contacts[at0003]/addresses[openEHR-DEMOGRAPHIC-ADDRESS.address.v2]/details/items[at0009]/value/defining_code",
		},
	}
}()

// corpusDocument is one vendored corpus document, decoded, with its compiled
// OPT when the corpus holds one.
type corpusDocument struct {
	rel  fixtures.CompositionJSONRel
	root any
	c    *templatecompile.Compiled // nil when the corpus holds no OPT for it
}

// corpusDocuments decodes every vendored corpus document the fixtures list
// that decodes, and compiles the OPT of each composition-kind document whose
// OPT the corpus holds. It fails the test when it finds none of either.
func corpusDocuments(t *testing.T) []corpusDocument {
	t.Helper()
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("ListCompositionJSON: %v", err)
	}
	var out []corpusDocument
	templated := 0
	for _, rel := range rels {
		factory, ok := fixtures.FactoryForJSONRel(rel)
		if !ok {
			continue
		}
		body, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel.Rel, err)
		}
		root := factory()
		if err := canjson.Unmarshal(body, root); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		doc := corpusDocument{rel: rel, root: root}
		if rel.Kind == "compositions" {
			if c, ok := compileCorpusTemplate(t, rel.Template); ok {
				doc.c = c
				templated++
			}
		}
		out = append(out, doc)
	}
	if len(out) == 0 || templated == 0 {
		t.Fatalf("decoded %d corpus documents, %d with a template; the corpus listing has gone blind", len(out), templated)
	}
	return out
}

// REQ-171 Acceptance — every vendored corpus document validated against its
// OPT stops at no limit, and lists in External exactly the CONSTRAINT_REF
// leaves it binds (corpusExternal), so Complete is true for each document
// that binds none, the BMI composition among them.
func TestValidator_REQ171_CorpusCoverage(t *testing.T) {
	v := validation.New()
	complete := map[string]bool{}
	for _, doc := range corpusDocuments(t) {
		if doc.c == nil {
			continue
		}
		r := v.Validate(doc.root, doc.c)
		want := corpusExternal[doc.rel.Template]
		if len(r.Coverage.Stopped) != 0 {
			t.Errorf("Validate(%s) Coverage.Stopped = %q, want none: no corpus document comes near a limit", doc.rel.Rel, r.Coverage.Stopped)
		}
		if !slices.Equal(r.Coverage.External, want) {
			t.Errorf("Validate(%s) Coverage.External = %q, want %q", doc.rel.Rel, r.Coverage.External, want)
		}
		if got := r.Coverage.Complete(); got != (len(want) == 0) {
			t.Errorf("Validate(%s) Coverage.Complete() = %t, want %t", doc.rel.Rel, got, len(want) == 0)
		}
		complete[doc.rel.Template] = r.Coverage.Complete()
	}
	for id := range corpusExternal {
		if _, seen := complete[id]; !seen {
			t.Errorf("corpus document %s was not validated against its OPT; the CONSTRAINT_REF cases have gone blind", id)
		}
	}
	if !complete["BMI"] {
		t.Errorf("Validate(BMI) is not complete, or was not validated; want the corpus composition that meets no limit and no CONSTRAINT_REF complete")
	}
}

// REQ-171 § Coverage — a guard, ValidateAQL and ValidateAQLWithTypeRelation
// return the zero Coverage, which is complete.
func TestREQ171_GuardsAndAQLReturnTheZeroCoverage(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	tests := []struct {
		name string
		r    validation.Result
	}{
		{name: "Validate(nil, c)", r: validation.Validate(nil, c)},
		{name: "Validate(root, nil)", r: validation.Validate(validVitalSignsComposition(), nil)},
		{name: "New().Validate(root, template without a root)", r: validation.New().Validate(validVitalSignsComposition(), &templatecompile.Compiled{})},
		{name: "ValidateRM(nil)", r: validation.ValidateRM(nil)},
		{name: "ValidateRM(root outside the RM set)", r: validation.ValidateRM(struct{}{})},
		{name: "ValidateRMFolder(nil)", r: validation.ValidateRMFolder(nil)},
		{name: "ValidateRMEHRStatusBytes(array)", r: validation.ValidateRMEHRStatusBytes([]byte(`[]`))},
		{name: "ValidateAQL", r: validation.ValidateAQL(aql.NewQuery("SELECT * FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1]"), c)},
		{name: "ValidateAQL(syntax error)", r: validation.ValidateAQL(aql.NewQuery("SELEC x"), c)},
		{name: "ValidateAQLWithTypeRelation", r: validation.ValidateAQLWithTypeRelation(aql.NewQuery("SELEC x"), c, contain.Default())},
	}
	for _, tc := range tests {
		if len(tc.r.Issues) == 0 {
			t.Errorf("%s reported nothing; want an issue, so the case is not vacuous", tc.name)
		}
		if tc.r.Coverage.Stopped != nil || tc.r.Coverage.External != nil || !tc.r.Coverage.Complete() {
			t.Errorf("%s Coverage = %+v, want the zero Coverage", tc.name, tc.r.Coverage)
		}
	}
}

// REQ-171 § Coverage, REQ-112 — the presence-aware EHR_STATUS entry keeps the
// coverage of the floor it runs when it adds the findings of an absent key:
// an EHR_STATUS that omits subject and holds more ELEMENTs in other_details
// than the default visit limit lets the walk reach reports both, and lists
// where the visit limit stopped the walk. (The decoder refuses JSON nested
// deep enough to reach the default depth limit.)
func TestValidateRMEHRStatusBytes_REQ171_KeepsCoverage(t *testing.T) {
	const element = `{"_type": "ELEMENT", "archetype_node_id": "at0002", "name": {"_type": "DV_TEXT", "value": "e"}, "value": {"_type": "DV_BOOLEAN", "value": true}}`
	items := strings.Repeat(element+",", 20_000) + element
	data := []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"archetype_details": {"_type": "ARCHETYPED",
			"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"},
			"rm_version": "1.1.0"},
		"is_modifiable": true,
		"is_queryable": true,
		"other_details": {"_type": "ITEM_TREE", "archetype_node_id": "at0001", "name": {"_type": "DV_TEXT", "value": "t"}, "items": [` + items + `]}
	}`)
	r := validation.ValidateRMEHRStatusBytes(data)
	stopped := pathsOf(r.Issues, "max_nodes")
	if !containsIssue(r.Issues, "/subject", "required") || len(stopped) != 1 {
		t.Fatalf("ValidateRMEHRStatusBytes(no subject, 20,001 ELEMENTs) issues = %d, max_nodes at %q; want required at /subject and one max_nodes", len(r.Issues), stopped)
	}
	if !slices.Equal(r.Coverage.Stopped, stopped) || r.Coverage.Complete() {
		t.Errorf("ValidateRMEHRStatusBytes(no subject, 20,001 ELEMENTs) Coverage = %+v, want Stopped %q, not complete", r.Coverage, stopped)
	}
}

// subjectStopStatus is an EHR_STATUS that omits subject and whose name holds
// enough TERM_MAPPINGs, with a formatting and one preferred_term beside
// them, that the default visit limit falls exactly on the zero subject the
// decode leaves: the path the key-presence check reports as required. Each
// mapping costs the walk six visits: the mapping, its match, its target, the
// target's code_string and terminology_id, and the value of that
// TERMINOLOGY_ID, which the walk enters (REQ-171 Structural access); the
// preferred mapping costs one more, for its preferred_term.
func subjectStopStatus() []byte {
	const mapping = `{"_type": "TERM_MAPPING", "match": "=", "target": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "SNOMED-CT"}, "code_string": "1"}}`
	const preferred = `{"_type": "TERM_MAPPING", "match": "=", "target": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "SNOMED-CT"}, "code_string": "1", "preferred_term": "p"}}`
	return []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status", "formatting": "plain", "mappings": [` + preferred + strings.Repeat(","+mapping, 16_664) + `]},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"archetype_details": {"_type": "ARCHETYPED",
			"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"},
			"rm_version": "1.1.0"},
		"is_modifiable": true,
		"is_queryable": true
	}`)
}

// REQ-171 § Coverage, REQ-112 — when the visit limit falls on the path of an
// absent key, the presence-aware EHR_STATUS entry keeps the max_nodes issue
// there beside its own required, so the issue and Coverage.Stopped agree and
// max_nodes is reported once.
func TestValidateRMEHRStatusBytes_REQ171_KeepsAStopOnAnAbsentKey(t *testing.T) {
	r := validation.ValidateRMEHRStatusBytes(subjectStopStatus())
	if !slices.Equal(r.Coverage.Stopped, []string{"/subject"}) {
		t.Fatalf("precondition: Coverage.Stopped = %q, want [/subject]; the visit count of the recipe drifted, adjust the number of mappings", r.Coverage.Stopped)
	}
	if got := pathsOf(r.Issues, "max_nodes"); !slices.Equal(got, r.Coverage.Stopped) {
		t.Errorf("ValidateRMEHRStatusBytes(visit limit on /subject) reports max_nodes at %q, want once, at each path Coverage.Stopped lists (%q)", got, r.Coverage.Stopped)
	}
	if !containsIssue(r.Issues, "/subject", "required") {
		t.Errorf("ValidateRMEHRStatusBytes(visit limit on /subject) issues = %q, want required at /subject", issueKeys(r.Issues))
	}
}
