package templatecompile_test

// compile_model_test.go: how Compile reads the operational template model it
// is given: the refusals and the readings of § REQ-111 Reading the model, on
// models built in code, where the corpus golden covers the corpus.

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// once is the occurrences or existence interval 1..1.
var once = rm.Interval[aom14.Integer]{Lower: 1, Upper: 1, LowerIncluded: true, UpperIncluded: true}

// complexObject is a C_COMPLEX_OBJECT of rmType at nodeID, occurring once,
// over attrs.
func complexObject(rmType, nodeID string, attrs ...aom14.CAttribute) *aom14.CComplexObject {
	return &aom14.CComplexObject{RMTypeName: rmType, NodeID: nodeID, Occurrences: once, Attributes: attrs}
}

// single is a C_SINGLE_ATTRIBUTE name of existence 1..1 over children.
func single(name string, children ...aom14.CObject) *aom14.CSingleAttribute {
	return &aom14.CSingleAttribute{RMAttributeName: name, Existence: once, Children: children}
}

// multiple is a C_MULTIPLE_ATTRIBUTE name of existence 1..1 and cardinality
// 0..* over children.
func multiple(name string, children ...aom14.CObject) *aom14.CMultipleAttribute {
	return &aom14.CMultipleAttribute{
		RMAttributeName: name,
		Existence:       once,
		Cardinality:     aom14.Cardinality{Interval: rm.Interval[aom14.Integer]{LowerIncluded: true, UpperUnbounded: true}},
		Children:        children,
	}
}

// modelOf is an operational template whose definition is a COMPOSITION
// archetype root over attrs.
func modelOf(attrs ...aom14.CAttribute) *aom14.OperationalTemplate {
	return &aom14.OperationalTemplate{
		TemplateID: rm.TemplateID{Value: "model"},
		Concept:    "model",
		Language:   rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Definition: aom14.CArchetypeRoot{
			CComplexObject: aom14.CComplexObject{RMTypeName: "COMPOSITION", NodeID: "at0000", Occurrences: once, Attributes: attrs},
			ArchetypeID:    rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.model.v1"},
		},
	}
}

// nested is a model whose definition holds a chain of levels CLUSTER
// objects, each under the items of the one above, the deepest levels below
// the definition.
func nested(levels int) *aom14.OperationalTemplate {
	var below aom14.CObject
	for i := levels; i >= 1; i-- {
		var attrs []aom14.CAttribute
		if below != nil {
			attrs = append(attrs, multiple("items", below))
		}
		below = complexObject("CLUSTER", "at0001", attrs...)
	}
	return modelOf(multiple("content", below))
}

// TestREQ111_CompileRefusesModel pins the refusals of § REQ-111 Reading the
// model: each wraps ErrInvalidInput, so a caller tells a template Compile
// will not take from a failure of its own. A definition that holds nothing,
// which the model cannot tell from a missing one, is refused as no
// definition, and one that holds something but names no RM type as such, so
// that the parity check tells the two apart.
func TestREQ111_CompileRefusesModel(t *testing.T) {
	const (
		noDefinition = "template has no definition"
		noRMType     = "template definition names no RM type"
	)
	cases := []struct {
		name  string
		model func() *aom14.OperationalTemplate
		// msg, when set, is the text the error ends with, after the
		// ErrInvalidInput it wraps.
		msg string
	}{
		{name: "nil template", model: func() *aom14.OperationalTemplate { return nil }},
		{name: "no template_id", model: func() *aom14.OperationalTemplate {
			m := modelOf()
			m.TemplateID = rm.TemplateID{}
			return m
		}},
		{name: "a blank template_id", model: func() *aom14.OperationalTemplate {
			m := modelOf()
			m.TemplateID.Value = " \n "
			return m
		}},
		{name: "no definition", msg: noDefinition, model: func() *aom14.OperationalTemplate {
			m := modelOf()
			m.Definition = aom14.CArchetypeRoot{}
			return m
		}},
		{name: "a definition that names no RM type", msg: noRMType, model: func() *aom14.OperationalTemplate {
			m := modelOf(single("name", complexObject("DV_TEXT", "")))
			m.Definition.RMTypeName = "  "
			return m
		}},
		{name: "a definition that holds only an archetype id", msg: noRMType, model: func() *aom14.OperationalTemplate {
			m := modelOf()
			m.Definition = aom14.CArchetypeRoot{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.model.v1"}}
			return m
		}},
		{name: "a definition that holds only occurrences", msg: noRMType, model: func() *aom14.OperationalTemplate {
			m := modelOf()
			m.Definition = aom14.CArchetypeRoot{Occurrences: once}
			return m
		}},
		{name: "inverted occurrences", model: func() *aom14.OperationalTemplate {
			c := complexObject("SECTION", "at0001")
			c.Occurrences = rm.Interval[aom14.Integer]{Lower: 3, Upper: 1, LowerIncluded: true, UpperIncluded: true}
			return modelOf(multiple("content", c))
		}},
		{name: "occurrences closing leaves empty", model: func() *aom14.OperationalTemplate {
			c := complexObject("SECTION", "at0001")
			c.Occurrences = rm.Interval[aom14.Integer]{Lower: 2, Upper: 3}
			return modelOf(multiple("content", c))
		}},
		{name: "inverted existence", model: func() *aom14.OperationalTemplate {
			a := single("context", complexObject("EVENT_CONTEXT", ""))
			a.Existence = rm.Interval[aom14.Integer]{Lower: 1, Upper: 0, LowerIncluded: true, UpperIncluded: true}
			return modelOf(a)
		}},
		{name: "inverted cardinality", model: func() *aom14.OperationalTemplate {
			a := multiple("content", complexObject("SECTION", "at0001"))
			a.Cardinality.Interval = rm.Interval[aom14.Integer]{Lower: 5, Upper: 2, LowerIncluded: true, UpperIncluded: true}
			return modelOf(a)
		}},
		{name: "a nil constraint object", model: func() *aom14.OperationalTemplate {
			return modelOf(multiple("content", nil))
		}},
		{name: "a nil pointer to a constraint object", model: func() *aom14.OperationalTemplate {
			return modelOf(multiple("content", (*aom14.CComplexObject)(nil)))
		}},
		{name: "a nil attribute", model: func() *aom14.OperationalTemplate {
			return modelOf(nil)
		}},
		{name: "a nil pointer to an attribute", model: func() *aom14.OperationalTemplate {
			return modelOf((*aom14.CMultipleAttribute)(nil))
		}},
	}
	for _, tc := range cases {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
				_, err := templatecompile.Compile(tc.model(), templatecompile.Options{Strict: strict})
				if !errors.Is(err, templatecompile.ErrInvalidInput) {
					t.Fatalf("Compile error = %v, want an error wrapping ErrInvalidInput", err)
				}
				if tc.msg != "" && !strings.HasSuffix(err.Error(), ": "+tc.msg) {
					t.Errorf("Compile error = %q, want it to end with %q", err, tc.msg)
				}
			})
		}
	}
}

// TestREQ111_CompileReadsWeakValuesUnlessStrict pins § REQ-111 What the
// compile step tolerates on whole models: in its default mode Compile reads a
// value the model holds in the weaker form the parser's default mode reads
// it, and with Options.Strict it refuses it with an error wrapping
// ErrInvalidInput.
func TestREQ111_CompileReadsWeakValuesUnlessStrict(t *testing.T) {
	// leaf is a model whose definition holds o at /context/start_time/value.
	leaf := func(o aom14.CObject) *aom14.OperationalTemplate {
		return modelOf(single("context", complexObject("EVENT_CONTEXT", "",
			single("start_time", complexObject("DV_DATE_TIME", "", single("value", o))))))
	}
	tz := func(k aom14.TimezoneValidityKind) *aom14.TimezoneValidityKind { return &k }
	cases := []struct {
		name  string
		model *aom14.OperationalTemplate
		want  constraints.PrimitiveConstraint // the default mode's reading
	}{
		{
			name:  "a C_DATE whose timezone validity is mandatory",
			model: leaf(&aom14.CPrimitiveObject{RMTypeName: "DATE", Occurrences: once, Item: &aom14.CDate{TimezoneValidity: tz(aom14.TimezoneValidityKindMandatory)}}),
			want:  constraints.CDate{TimezoneValidity: constraints.ValidityOptional},
		},
		{
			name:  "a C_DATE_TIME whose timezone validity names no VALIDITY_KIND",
			model: leaf(&aom14.CPrimitiveObject{RMTypeName: "DATE_TIME", Occurrences: once, Item: &aom14.CDateTime{TimezoneValidity: tz(7)}}),
			want:  constraints.CDateTime{TimezoneValidity: constraints.ValidityOptional},
		},
		{
			name: "a C_DATE_TIME range bound that is not a date-time",
			model: leaf(&aom14.CPrimitiveObject{RMTypeName: "DATE_TIME", Occurrences: once, Item: &aom14.CDateTime{
				Range: &rm.Interval[string]{Lower: "yesterday", LowerIncluded: true, UpperUnbounded: true},
			}}),
			want: constraints.CDateTime{Range: constraints.TemporalRange{LowerUnbounded: true, UpperUnbounded: true}},
		},
		{
			name:  "a CONSTRAINT_REF with a blank reference",
			model: leaf(&aom14.ConstraintRef{RMTypeName: "CODE_PHRASE", Occurrences: once, Reference: " "}),
			want:  constraints.ConstraintRef{},
		},
		{
			name:  "a C_PRIMITIVE_OBJECT with no item",
			model: leaf(&aom14.CPrimitiveObject{RMTypeName: "DATE_TIME", Occurrences: once}),
			want:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := templatecompile.Compile(tc.model)
			if err != nil {
				t.Fatalf("Compile = %v, want the weaker reading", err)
			}
			n, err := c.NodeAt("/context/start_time/value")
			if err != nil {
				t.Fatalf("NodeAt(/context/start_time/value) = %v", err)
			}
			if got := n.PrimitiveConstraint(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PrimitiveConstraint() = %#v, want %#v", got, tc.want)
			}

			if _, err := templatecompile.Compile(tc.model, templatecompile.Options{Strict: true}); !errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Errorf("Compile(Strict) error = %v, want an error wrapping ErrInvalidInput", err)
			}
		})
	}
}

// TestREQ111_CompileRefusesUnhandledKind pins that a constraint object or an
// attribute of a kind Compile does not handle, such as one held by value,
// which the model's decode never produces, is refused with an error that does
// not wrap ErrInvalidInput: the template is not at fault.
func TestREQ111_CompileRefusesUnhandledKind(t *testing.T) {
	cases := map[string]*aom14.OperationalTemplate{
		"a constraint object held by value": modelOf(multiple("content", aom14.CComplexObject{RMTypeName: "SECTION", NodeID: "at0001", Occurrences: once})),
		"an attribute held by value":        modelOf(aom14.CSingleAttribute{RMAttributeName: "context", Existence: once}),
	}
	for name, model := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := templatecompile.Compile(model)
			if err == nil || errors.Is(err, templatecompile.ErrInvalidInput) {
				t.Fatalf("Compile = %v, want an error that does not wrap ErrInvalidInput", err)
			}
		})
	}
}

// TestREQ108_CompileRefusesDeepModel pins the depth limit of REQ-108 that
// Compile holds a model to, which a model decoded or built without the
// model's reader has not been held to: 128 levels of constraint objects below
// the definition compile, and 129 are refused with an error wrapping
// ErrInvalidInput.
func TestREQ108_CompileRefusesDeepModel(t *testing.T) {
	if _, err := templatecompile.Compile(nested(128)); err != nil {
		t.Fatalf("Compile(128 levels) = %v, want no error", err)
	}
	if _, err := templatecompile.Compile(nested(129)); !errors.Is(err, templatecompile.ErrInvalidInput) {
		t.Fatalf("Compile(129 levels) = %v, want an error wrapping ErrInvalidInput", err)
	}
}

// TestREQ111_CompileReadsModelTrimmed pins the readings of § REQ-111 Reading
// the model on padded values: identifiers, codes, list entries and term texts
// are read without their surrounding white space, a blank term, item or
// binding is left out, and an ELEMENT's null_flavor is read as null_flavour.
func TestREQ111_CompileReadsModelTrimmed(t *testing.T) {
	name := &aom14.CPrimitiveObject{RMTypeName: "STRING", Occurrences: once, Item: &aom14.CString{List: []string{" Padded name "}}}
	codes := []string{" at0002 ", " "}
	element := complexObject(" ELEMENT ", " at0001 ",
		single(" name ", complexObject("DV_TEXT", "", single("value", name))),
		single(" null_flavor ", complexObject("DV_CODED_TEXT", "")),
		single("value", complexObject("DV_CODED_TEXT", "", single("defining_code",
			&aom14.CCodePhrase{RMTypeName: "CODE_PHRASE", Occurrences: once, TerminologyID: &rm.TerminologyID{Value: " local "}, CodeList: &codes}))),
	)
	model := modelOf(multiple(" content ", &aom14.CArchetypeRoot{
		CComplexObject: *complexObject(" OBSERVATION ", " at0000 ", single("data", complexObject("HISTORY", "at0001",
			multiple("events", complexObject("POINT_EVENT", "at0002", single("data", complexObject("ITEM_TREE", "at0003", multiple("items", element)))))))),
		ArchetypeID: rm.ArchetypeID{Value: " openEHR-EHR-OBSERVATION.padded.v1 "},
		TermDefinitions: []aom14.ArchetypeTerm{
			{Code: " at0002 ", Items: map[string]string{" text ": " Padded text ", "text": " Exact text ", "description": " Padded description ", " ": "no key"}},
			{Code: " ", Items: map[string]string{"text": "no code"}},
			{Code: "at0009", Items: map[string]string{" ": "no item key"}},
		},
		TermBindings: []aom14.TermBindingSet{{Terminology: " SNOMED-CT ", Items: []aom14.TermBindingItem{
			{Code: " at0002 ", Value: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: " SNOMED-CT "}, CodeString: " 12345 "}},
			{Code: "  ", Value: rm.CodePhrase{CodeString: "blank code"}},
		}}},
	}))
	model.TemplateID.Value = " model "
	model.Concept = " concept "
	model.Language.CodeString = " en "
	model.UID = &rm.HierObjectID{Value: " 8b3c-uid "}

	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := [4]string{c.TemplateID(), c.Concept(), c.Language(), c.UID()}; got != [4]string{"model", "concept", "en", "8b3c-uid"} {
		t.Errorf("TemplateID, Concept, Language, UID = %q, want them trimmed", got)
	}
	const rootPath = "/content[openEHR-EHR-OBSERVATION.padded.v1]"
	const elementPath = rootPath + "/data/events[at0002]/data/items[at0001,'Padded name']"
	root, err := c.NodeAt(rootPath)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", rootPath, err)
	}
	if root.RMTypeName() != "OBSERVATION" || root.NodeID() != "at0000" || root.ArchetypeID() != "openEHR-EHR-OBSERVATION.padded.v1" {
		t.Errorf("archetype root = %q %q %q, want its identifiers trimmed", root.RMTypeName(), root.NodeID(), root.ArchetypeID())
	}
	el, err := c.NodeAt(elementPath)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", elementPath, err)
	}
	if el.NodeName() != "Padded name" {
		t.Errorf("NodeName() = %q, want %q", el.NodeName(), "Padded name")
	}
	var names []string
	for _, a := range el.Attributes() {
		if !a.Implicit() {
			names = append(names, a.Name())
		}
	}
	if want := []string{"name", "null_flavour", "value"}; !slices.Equal(names, want) {
		t.Errorf("declared attributes of the ELEMENT = %q, want %q: trimmed, null_flavor read as null_flavour", names, want)
	}
	code, err := c.NodeAt(elementPath + "/value/defining_code")
	if err != nil {
		t.Fatalf("NodeAt(defining_code): %v", err)
	}
	if got, want := code.PrimitiveConstraint(), (constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0002"}}); !equalCodePhrase(got, want) {
		t.Errorf("PrimitiveConstraint() = %#v, want %#v", got, want)
	}

	term, ok := el.Term("at0002", "")
	if !ok {
		t.Fatal(`Term("at0002") not found; want the padded code trimmed`)
	}
	if want := map[string]string{"text": "Exact text", "description": "Padded description"}; term.Code != "at0002" || !maps.Equal(term.Items, want) {
		t.Errorf("Term(at0002) = %q %v, want %q %v: the exact key kept over its padded spelling, texts trimmed, the blank key left out", term.Code, term.Items, "at0002", want)
	}
	if _, ok := el.Term("at0009", ""); ok {
		t.Error(`Term("at0009") found; want a term with no item left out`)
	}
	wantBindings := []template.TermBinding{{Terminology: "SNOMED-CT", NodeOrPath: "at0002", Target: template.CodedTermRef{TerminologyID: "SNOMED-CT", CodeString: "12345"}}}
	if got := c.TermBindings(); !slices.Equal(got, wantBindings) {
		t.Errorf("TermBindings() = %+v, want %+v: trimmed, the blank code left out", got, wantBindings)
	}
}

// equalCodePhrase reports whether the primitive constraint got is the
// CodePhrase want.
func equalCodePhrase(got constraints.PrimitiveConstraint, want constraints.CodePhrase) bool {
	cp, ok := got.(constraints.CodePhrase)
	return ok && cp.Terminology == want.Terminology && slices.Equal(cp.CodeList, want.CodeList)
}

// TestREQ111_CompileReadsZeroIntervalAsAbsent pins how Compile reads an
// interval the model leaves at its zero value, which is how the model holds
// an interval the template leaves out: as no interval, as the deprecated
// parser reads one the template leaves out.
func TestREQ111_CompileReadsZeroIntervalAsAbsent(t *testing.T) {
	section := &aom14.CComplexObject{RMTypeName: "SECTION", NodeID: "at0001"}
	model := modelOf(&aom14.CMultipleAttribute{RMAttributeName: "content", Children: []aom14.CObject{section}})
	c, err := templatecompile.Compile(model)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	n, err := c.NodeAt("/content[at0001]")
	if err != nil {
		t.Fatalf("NodeAt: %v", err)
	}
	if n.Occurrences() != nil {
		t.Errorf("Occurrences() = %v, want nil", n.Occurrences())
	}
	a := c.Root().Attribute("content")
	if a.Existence() != nil || a.ChildMultiplicity() != nil {
		t.Errorf("Existence() = %v, ChildMultiplicity() = %v, want nil and nil", a.Existence(), a.ChildMultiplicity())
	}
}

// TestREQ111_CompileHandlesEveryConstraintObjectKind pins that Compile walks
// every constraint object class the model registers: a zero instance of each,
// under a multiple attribute, compiles or is refused for its content with an
// error wrapping ErrInvalidInput, never as a kind Compile does not handle. A
// class the generator adds to the model fails here until Compile reads it.
func TestREQ111_CompileHandlesEveryConstraintObjectKind(t *testing.T) {
	var kinds []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		obj, ok := ctor().(aom14.CObject)
		if !ok {
			continue
		}
		kinds = append(kinds, name)
		if _, err := templatecompile.Compile(modelOf(multiple("content", obj))); err != nil && !errors.Is(err, templatecompile.ErrInvalidInput) {
			t.Errorf("Compile(a %s child) = %v, want it read", name, err)
		}
	}
	if len(kinds) != 13 {
		t.Errorf("the registry holds %d constraint object classes %q, want the 13 of the AOM 1.4 model", len(kinds), kinds)
	}
}
