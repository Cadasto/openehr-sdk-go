package templatecompile

// primitives_from_model_test.go: the compile step's primitive constraints,
// built from the operational template model, held equal to the deprecated
// parser's over every corpus node, and the readings and refusals that part
// the two.

import (
	json "encoding/json/v2"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// corpusTemplateFloor is the number of operational templates the corpus
// vendors today. A walk that finds fewer fails, so a corpus that silently
// empties, or a walk that stops finding a folder, cannot pass as clean.
const corpusTemplateFloor = 74

// corpusOperationalTemplates returns every operational template under the
// corpus root, as PROBE-111 walks them: each regular *.opt file, named by its
// slash-separated path relative to the root, sorted. It walks the corpus
// itself because an in-package test cannot import testkit/probes/template,
// which imports this package through the public compile step.
func corpusOperationalTemplates(t *testing.T) []string {
	t.Helper()
	root := fixtures.CorpusRoot()
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || filepath.Ext(path) != ".opt" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk the corpus at %s: %v", root, err)
	}
	if len(names) < corpusTemplateFloor {
		t.Fatalf("the corpus at %s holds %d operational templates, want at least %d", root, len(names), corpusTemplateFloor)
	}
	slices.Sort(names)
	return names
}

// modelNodePair is one node of a corpus operational template as the parser
// reads it and as the model's decode reads it, paired by its place: where
// names the template and the attribute steps that lead to the node, each
// with the child's index.
type modelNodePair struct {
	where  string
	parsed template.Node
	model  aom14.CObject
}

// corpusModelNodePairs reads every corpus operational template, walked as
// PROBE-111 walks them, with the parser and with the model's decode, and
// returns their nodes paired in document order. A template the two read into
// trees of different shapes fails the test.
func corpusModelNodePairs(t *testing.T) []modelNodePair {
	t.Helper()
	root := fixtures.CorpusRoot()
	var pairs []modelNodePair
	for _, name := range corpusOperationalTemplates(t) {
		path := filepath.Join(root, filepath.FromSlash(name))
		parsed, err := template.ParseFile(path)
		if err != nil {
			t.Fatalf("template.ParseFile(%s) = %v", name, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var model aom14.OperationalTemplate
		if err := canxml.Unmarshal(raw, &model); err != nil {
			t.Fatalf("canxml.Unmarshal(%s) = %v", name, err)
		}
		pairs = pairModelNodes(t, name, parsed.Root(), &model.Definition, pairs)
	}
	return pairs
}

// pairModelNodes appends the pair of parsed and model, then the pairs of
// their descendants in document order.
func pairModelNodes(t *testing.T, where string, parsed template.Node, model aom14.CObject, pairs []modelNodePair) []modelNodePair {
	t.Helper()
	pairs = append(pairs, modelNodePair{where: where, parsed: parsed, model: model})
	var parsedAttrs []*template.Attribute
	if on, ok := parsed.(template.ObjectNode); ok {
		parsedAttrs = on.Attributes()
	}
	modelAttrs := modelAttributes(model)
	if len(parsedAttrs) != len(modelAttrs) {
		t.Fatalf("%s: the parser reads %d attributes and the model %d", where, len(parsedAttrs), len(modelAttrs))
	}
	for i, pa := range parsedAttrs {
		name, children := modelChildren(modelAttrs[i])
		if len(pa.Children()) != len(children) {
			t.Fatalf("%s/%s: the parser reads %d children and the model %d", where, name, len(pa.Children()), len(children))
		}
		for j, pc := range pa.Children() {
			pairs = pairModelNodes(t, fmt.Sprintf("%s/%s[%d]", where, name, j), pc, children[j], pairs)
		}
	}
	return pairs
}

// modelAttributes returns the attributes of a model node that has any.
func modelAttributes(o aom14.CObject) []aom14.CAttribute {
	switch n := o.(type) {
	case *aom14.CComplexObject:
		return n.Attributes
	case *aom14.CArchetypeRoot:
		return n.Attributes
	case *aom14.TComplexObject:
		return n.Attributes
	}
	return nil
}

// modelChildren returns the name and the children of a model attribute.
func modelChildren(a aom14.CAttribute) (string, []aom14.CObject) {
	switch x := a.(type) {
	case *aom14.CSingleAttribute:
		return x.RMAttributeName, x.Children
	case *aom14.CMultipleAttribute:
		return x.RMAttributeName, x.Children
	}
	return fmt.Sprintf("<%T>", a), nil
}

// renderPrimitive renders a primitive constraint as its Go type and its
// exported fields, which is all two equal constraints share: a CString's
// compiled pattern is derived from its Pattern.
func renderPrimitive(pc constraints.PrimitiveConstraint) string {
	if pc == nil {
		return "nil"
	}
	b, err := json.Marshal(pc, json.Deterministic(true))
	if err != nil {
		return fmt.Sprintf("%T<%v>", pc, err)
	}
	return fmt.Sprintf("%T%s", pc, b)
}

func TestREQ103_REQ111_PrimitiveFromModelMatchesParserOverCorpus(t *testing.T) {
	seen := map[string]int{}
	for _, p := range corpusModelNodePairs(t) {
		var want constraints.PrimitiveConstraint
		if on, ok := p.parsed.(interface {
			PrimitiveConstraint() constraints.PrimitiveConstraint
		}); ok {
			want = on.PrimitiveConstraint()
		}
		got, err := primitiveFromModel(p.model, false)
		if err != nil {
			t.Errorf("%s: primitiveFromModel(%T) = %v; want the parser's %s", p.where, p.model, err, renderPrimitive(want))
			continue
		}
		if ref, ok := p.model.(*aom14.CCodeReference); ok {
			// The parser reads no constraint on a C_CODE_REFERENCE; the
			// compile step reads its code phrase as a C_CODE_PHRASE's, with
			// the reference set's URI beside it.
			if want != nil {
				t.Errorf("%s: the parser reads %s on a C_CODE_REFERENCE; want none", p.where, renderPrimitive(want))
			}
			want = constraints.CodeReference{
				CodePhrase:      codePhraseFromModel(&ref.CCodePhrase),
				ReferenceSetURI: strings.TrimSpace(ref.ReferenceSetURI),
			}
			if want.(constraints.CodeReference).ReferenceSetURI == "" {
				t.Errorf("%s: a corpus C_CODE_REFERENCE with no reference set URI", p.where)
			}
		}
		if g, w := renderPrimitive(got), renderPrimitive(want); g != w {
			t.Errorf("%s: primitiveFromModel(%T) =\n  %s\nwant the parser's\n  %s", p.where, p.model, g, w)
		}
		if got != nil {
			seen[fmt.Sprintf("%T", got)]++
		}
	}
	// The corpus holds every constraint type, so a walk that paired no
	// primitive node, or the wrong ones, cannot pass unseen.
	for _, want := range []constraints.PrimitiveConstraint{
		constraints.CBoolean{},
		constraints.CInteger{},
		constraints.CReal{},
		constraints.CString{},
		constraints.CDate{},
		constraints.CTime{},
		constraints.CDateTime{},
		constraints.CDuration{},
		constraints.CodePhrase{},
		constraints.CodeReference{},
		constraints.DvQuantity{},
		constraints.CDvOrdinal{},
		constraints.ConstraintRef{},
	} {
		if seen[fmt.Sprintf("%T", want)] == 0 {
			t.Errorf("no corpus node built a %T; the parity walk pairs no such node", want)
		}
	}
}

// TestREQ103_PrimitiveFromModelReadings pins the readings the compile step
// gives the model's primitive classes beyond what the corpus shows, in both
// of its modes: white space trimmed, blank entries left out, an absent
// list_open read as a closed list, the timezone_validity codes, the quantity
// property units, both forms of an ordinal symbol, and the nodes that carry
// no constraint.
func TestREQ103_PrimitiveFromModelReadings(t *testing.T) {
	tz := func(k aom14.TimezoneValidityKind) *aom14.TimezoneValidityKind { return &k }
	trueValue := true
	cases := []struct {
		name  string
		model aom14.CObject
		want  constraints.PrimitiveConstraint
	}{
		{
			name:  "C_STRING: pattern, entries and default trimmed, a blank entry left out, no list_open a closed list",
			model: &aom14.CPrimitiveObject{RMTypeName: "STRING", Item: &aom14.CString{Pattern: new(" [a-z]+ "), List: []string{" a ", " ", "b"}, AssumedValue: new(" a ")}},
			want:  constraints.NewCString("[a-z]+", []string{"a", "b"}, "a"),
		},
		{
			name:  "C_STRING: list_open true opens the list",
			model: &aom14.CPrimitiveObject{RMTypeName: "STRING", Item: &aom14.CString{List: []string{"a"}, ListOpen: &trueValue}},
			want:  openCString([]string{"a"}),
		},
		{
			name:  "C_BOOLEAN: flags as the model holds them, the assumed value as default",
			model: &aom14.CPrimitiveObject{RMTypeName: "BOOLEAN", Item: &aom14.CBoolean{TrueValid: true, AssumedValue: &trueValue}},
			want:  constraints.CBoolean{TrueValid: true, Default: &trueValue},
		},
		{
			name:  "C_INTEGER: no range is unbounded on both sides",
			model: &aom14.CPrimitiveObject{RMTypeName: "INTEGER", Item: &aom14.CInteger{List: []aom14.Integer{1, 2}, AssumedValue: new(aom14.Integer(2))}},
			want:  constraints.CInteger{Range: constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}, List: []int64{1, 2}, Default: new(int64(2))},
		},
		{
			name:  "C_REAL: a bound on an unbounded side is not kept",
			model: &aom14.CPrimitiveObject{RMTypeName: "REAL", Item: &aom14.CReal{Range: &rm.Interval[aom14.Real]{Lower: 0.5, LowerIncluded: true, Upper: 9, UpperUnbounded: true}}},
			want:  constraints.CReal{Range: constraints.NumericRange{Lower: 0.5, LowerInclusive: true, UpperUnbounded: true}},
		},
		{
			name:  "C_TIME: timezone_validity mandatory, bounds trimmed",
			model: &aom14.CPrimitiveObject{RMTypeName: "TIME", Item: &aom14.CTime{Pattern: new(" HH:MM:SS "), Range: &rm.Interval[string]{Lower: " 09:00:00 ", LowerIncluded: true, UpperUnbounded: true}, TimezoneValidity: tz(aom14.TimezoneValidityKindMandatory)}},
			want:  constraints.CTime{Pattern: "HH:MM:SS", Range: constraints.TemporalRange{Lower: "09:00:00", LowerInclusive: true, UpperUnbounded: true}, TimezoneValidity: constraints.ValidityMandatory},
		},
		{
			name:  "C_DATE_TIME: timezone_validity disallowed",
			model: &aom14.CPrimitiveObject{RMTypeName: "DATE_TIME", Item: &aom14.CDateTime{TimezoneValidity: tz(aom14.TimezoneValidityKindDisallowed)}},
			want:  constraints.CDateTime{TimezoneValidity: constraints.ValidityDisallowed},
		},
		{
			name:  "C_DATE: timezone_validity optional",
			model: &aom14.CPrimitiveObject{RMTypeName: "DATE", Item: &aom14.CDate{TimezoneValidity: tz(aom14.TimezoneValidityKindOptional)}},
			want:  constraints.CDate{TimezoneValidity: constraints.ValidityOptional},
		},
		{
			name:  "C_DURATION: no range is the zero range",
			model: &aom14.CPrimitiveObject{RMTypeName: "DURATION", Item: &aom14.CDuration{Pattern: new("PTHMS")}},
			want:  constraints.CDuration{Pattern: "PTHMS"},
		},
		{
			name: "C_CODE_PHRASE: terminology and codes trimmed, a blank code left out",
			model: &aom14.CCodePhrase{
				RMTypeName:    "CODE_PHRASE",
				TerminologyID: &rm.TerminologyID{Value: " local "},
				CodeList:      &[]string{" at0001 ", "", "at0002"},
			},
			want: constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001", "at0002"}},
		},
		{
			name: "C_CODE_REFERENCE: its code phrase read as a C_CODE_PHRASE's, the reference set URI trimmed",
			model: &aom14.CCodeReference{
				RMTypeName:      "CODE_PHRASE",
				TerminologyID:   &rm.TerminologyID{Value: " SNOMED-CT "},
				CodeList:        &[]string{" 22298006 ", " "},
				ReferenceSetURI: " terminology:SNOMED-CT ",
			},
			want: constraints.CodeReference{
				CodePhrase:      constraints.CodePhrase{Terminology: "SNOMED-CT", CodeList: []string{"22298006"}},
				ReferenceSetURI: "terminology:SNOMED-CT",
			},
		},
		{
			name:  "C_CODE_REFERENCE with only a reference set",
			model: &aom14.CCodeReference{RMTypeName: "CODE_PHRASE", ReferenceSetURI: "terminology:code24?subset=marital_status"},
			want:  constraints.CodeReference{ReferenceSetURI: "terminology:code24?subset=marital_status"},
		},
		{
			name: "C_DV_QUANTITY: an openehr property fills its units, an empty item is left out",
			model: &aom14.CDVQuantity{
				RMTypeName: "DV_QUANTITY",
				Property:   &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: " openehr "}, CodeString: " 122 "},
				List: []aom14.CQuantityItem{
					{Units: " cm ", Magnitude: &rm.Interval[aom14.Real]{Lower: 0, LowerIncluded: true, Upper: 300, UpperIncluded: true}},
					{Units: " "},
				},
			},
			want: constraints.DvQuantity{
				Units: []constraints.QuantityUnit{{
					Units:     "cm",
					Magnitude: constraints.NumericRange{Lower: 0, Upper: 300, LowerInclusive: true, UpperInclusive: true},
					Precision: constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true},
				}},
				Property:      &constraints.CodedTermRef{Terminology: "openehr", CodeString: "122"},
				PropertyUnits: propertyUnits("122"),
			},
		},
		{
			name: "C_DV_QUANTITY: a property of another terminology fills no units",
			model: &aom14.CDVQuantity{
				RMTypeName: "DV_QUANTITY",
				Property:   &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "122"},
			},
			want: constraints.DvQuantity{Property: &constraints.CodedTermRef{Terminology: "local", CodeString: "122"}},
		},
		{
			name: "C_DV_ORDINAL: a symbol as a DV_CODED_TEXT or a CODE_PHRASE, by pointer or by value, and one with no symbol left out",
			model: &aom14.CDVOrdinal{
				RMTypeName: "DV_ORDINAL",
				List: []aom14.Ordinal{
					{Value: 1, Symbol: &rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: " local "}, CodeString: " at0001 "}}},
					{Value: 2, Symbol: rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0002"}}},
					{Value: 3, Symbol: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0003"}},
					{Value: 4, Symbol: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0004"}},
					{Value: 5},
					{Value: 6, Symbol: (*rm.DVCodedText)(nil)},
				},
			},
			want: constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
				{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0001"}},
				{Value: 2, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0002"}},
				{Value: 3, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0003"}},
				{Value: 4, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0004"}},
			}},
		},
		{
			name:  "CONSTRAINT_REF: the reference trimmed",
			model: &aom14.ConstraintRef{RMTypeName: "CODE_PHRASE", Reference: " ac0001 "},
			want:  constraints.ConstraintRef{Reference: "ac0001"},
		},
		{
			name:  "C_COMPLEX_OBJECT carries no constraint",
			model: &aom14.CComplexObject{RMTypeName: "ELEMENT"},
			want:  nil,
		},
		{
			name:  "ARCHETYPE_SLOT carries no constraint",
			model: &aom14.ArchetypeSlot{RMTypeName: "CLUSTER"},
			want:  nil,
		},
	}
	for _, tc := range cases {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
				got, err := primitiveFromModel(tc.model, strict)
				if err != nil {
					t.Fatalf("primitiveFromModel = %v; want %s", err, renderPrimitive(tc.want))
				}
				if g, w := renderPrimitive(got), renderPrimitive(tc.want); g != w {
					t.Errorf("primitiveFromModel =\n  %s\nwant\n  %s", g, w)
				}
			})
		}
	}
}

// TestREQ103_REQ111_PrimitiveFromModelReadsWeakValues pins the values the
// compile step reads, in its default mode, in the weaker form the parser's
// default mode reads them, and refuses in its strict mode, as ParseOPTStrict
// does: a temporal range bound that is not ISO 8601 text of its type, read as
// unbounded on its side; a timezone_validity that names no VALIDITY_KIND, and
// a mandatory one on a C_DATE, read as optional; a CONSTRAINT_REF with no or a
// blank reference, read with an empty one; and a C_PRIMITIVE_OBJECT with no
// item, or with a nil pointer for one, read as no constraint.
func TestREQ103_REQ111_PrimitiveFromModelReadsWeakValues(t *testing.T) {
	primitive := func(item aom14.CPrimitive) aom14.CObject {
		return &aom14.CPrimitiveObject{RMTypeName: "ANY", Item: item}
	}
	tz := func(k aom14.TimezoneValidityKind) *aom14.TimezoneValidityKind { return &k }
	cases := []struct {
		name    string
		model   aom14.CObject
		want    constraints.PrimitiveConstraint // the default mode's reading
		wantMsg string                          // what the strict mode's error names
	}{
		{
			name:    "C_DATE lower bound not a date",
			model:   primitive(&aom14.CDate{Range: &rm.Interval[string]{Lower: "2020-13-45", LowerIncluded: true, Upper: "2021-12-31", UpperIncluded: true}}),
			want:    constraints.CDate{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "2021-12-31", UpperInclusive: true}},
			wantMsg: `C_DATE range: lower bound "2020-13-45" is not ISO 8601 text of the type`,
		},
		{
			name:    "C_TIME upper bound not a time",
			model:   primitive(&aom14.CTime{Range: &rm.Interval[string]{Lower: "09:00:00", LowerIncluded: true, Upper: "noon", UpperIncluded: true}}),
			want:    constraints.CTime{Range: constraints.TemporalRange{Lower: "09:00:00", LowerInclusive: true, UpperUnbounded: true}},
			wantMsg: `C_TIME range: upper bound "noon" is not ISO 8601 text of the type`,
		},
		{
			name:    "C_DATE_TIME lower bound a duration",
			model:   primitive(&aom14.CDateTime{Range: &rm.Interval[string]{Lower: "PT1H", LowerIncluded: true, UpperUnbounded: true}}),
			want:    constraints.CDateTime{Range: constraints.TemporalRange{LowerUnbounded: true, UpperUnbounded: true}},
			wantMsg: `C_DATE_TIME range: lower bound "PT1H" is not ISO 8601 text of the type`,
		},
		{
			name:    "C_DURATION upper bound empty",
			model:   primitive(&aom14.CDuration{Range: &rm.Interval[string]{Lower: "PT0S", LowerIncluded: true, Upper: " ", UpperIncluded: true}}),
			want:    constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT0S", LowerInclusive: true, UpperUnbounded: true}},
			wantMsg: `C_DURATION range: upper bound "" is not ISO 8601 text of the type`,
		},
		{
			name:    "C_DURATION bounds P and P",
			model:   primitive(&aom14.CDuration{Range: &rm.Interval[string]{Lower: "P", LowerIncluded: true, Upper: "P", UpperIncluded: true}}),
			want:    constraints.CDuration{Range: constraints.TemporalRange{LowerUnbounded: true, UpperUnbounded: true}},
			wantMsg: `C_DURATION range: lower bound "P" is not ISO 8601 text of the type`,
		},
		{
			name:    "C_DATE timezone_validity mandatory",
			model:   primitive(&aom14.CDate{Pattern: new("yyyy-mm-dd"), TimezoneValidity: tz(aom14.TimezoneValidityKindMandatory)}),
			want:    constraints.CDate{Pattern: "yyyy-mm-dd", TimezoneValidity: constraints.ValidityOptional},
			wantMsg: "C_DATE timezone_validity is mandatory",
		},
		{
			name:    "C_TIME timezone_validity no VALIDITY_KIND",
			model:   primitive(&aom14.CTime{TimezoneValidity: tz(42)}),
			want:    constraints.CTime{TimezoneValidity: constraints.ValidityOptional},
			wantMsg: "C_TIME timezone_validity 42 is no VALIDITY_KIND",
		},
		{
			name:    "C_DATE_TIME timezone_validity no VALIDITY_KIND",
			model:   primitive(&aom14.CDateTime{TimezoneValidity: tz(0)}),
			want:    constraints.CDateTime{TimezoneValidity: constraints.ValidityOptional},
			wantMsg: "C_DATE_TIME timezone_validity 0 is no VALIDITY_KIND",
		},
		{
			name:    "CONSTRAINT_REF no reference",
			model:   &aom14.ConstraintRef{RMTypeName: "CODE_PHRASE"},
			want:    constraints.ConstraintRef{},
			wantMsg: `CONSTRAINT_REF for "CODE_PHRASE" has no reference`,
		},
		{
			name:    "CONSTRAINT_REF blank reference",
			model:   &aom14.ConstraintRef{RMTypeName: "CODE_PHRASE", Reference: " \t"},
			want:    constraints.ConstraintRef{},
			wantMsg: `CONSTRAINT_REF for "CODE_PHRASE" has no reference`,
		},
		{
			name:    "C_PRIMITIVE_OBJECT with no item",
			model:   &aom14.CPrimitiveObject{RMTypeName: " STRING "},
			want:    nil,
			wantMsg: `C_PRIMITIVE_OBJECT for "STRING" has no item`,
		},
		{
			name:    "C_PRIMITIVE_OBJECT with a nil pointer for an item",
			model:   &aom14.CPrimitiveObject{RMTypeName: "DATE", Item: (*aom14.CDate)(nil)},
			want:    nil,
			wantMsg: `C_PRIMITIVE_OBJECT for "DATE" has no item`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := primitiveFromModel(tc.model, false)
			if err != nil {
				t.Fatalf("primitiveFromModel(default) = %v; want %s", err, renderPrimitive(tc.want))
			}
			if g, w := renderPrimitive(got), renderPrimitive(tc.want); g != w {
				t.Errorf("primitiveFromModel(default) =\n  %s\nwant\n  %s", g, w)
			}

			got, err = primitiveFromModel(tc.model, true)
			if err == nil {
				t.Fatalf("primitiveFromModel(strict) = %s, nil; want an error containing %q", renderPrimitive(got), tc.wantMsg)
			}
			if got != nil {
				t.Errorf("primitiveFromModel(strict) returned %s beside its error; want nil", renderPrimitive(got))
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("primitiveFromModel(strict) error = %q; want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

// openCString returns the closed-list CString of list with its list opened.
func openCString(list []string) constraints.CString {
	c := constraints.NewCString("", list, "")
	c.ListOpen = true
	return c
}

// TestREQ103_REQ111_PrimitiveFromModelRefuses pins the refusals of the
// compile step in both of its modes: the model values no reading names, an
// item that is no primitive class held by pointer and an ordinal symbol that
// is neither a DV_CODED_TEXT nor a CODE_PHRASE, which only a model built in
// code can hold.
func TestREQ103_REQ111_PrimitiveFromModelRefuses(t *testing.T) {
	cases := []struct {
		name    string
		model   aom14.CObject
		wantMsg string
	}{
		{
			name:    "an item held by value",
			model:   &aom14.CPrimitiveObject{RMTypeName: "ANY", Item: aom14.CString{}},
			wantMsg: "item aom14.CString of C_PRIMITIVE_OBJECT",
		},
		{
			name: "an ordinal symbol of another type",
			model: &aom14.CDVOrdinal{RMTypeName: "DV_ORDINAL", List: []aom14.Ordinal{
				{Value: 1, Symbol: &rm.DVText{Value: "mild"}},
			}},
			wantMsg: "C_DV_ORDINAL list[0]: symbol *rm.DVText is neither a DV_CODED_TEXT nor a CODE_PHRASE",
		},
	}
	for _, tc := range cases {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
				got, err := primitiveFromModel(tc.model, strict)
				if err == nil {
					t.Fatalf("primitiveFromModel = %s, nil; want an error containing %q", renderPrimitive(got), tc.wantMsg)
				}
				if got != nil {
					t.Errorf("primitiveFromModel returned %s beside its error; want nil", renderPrimitive(got))
				}
				if !strings.Contains(err.Error(), tc.wantMsg) {
					t.Errorf("primitiveFromModel error = %q; want it to contain %q", err, tc.wantMsg)
				}
			})
		}
	}
}

// TestREQ103_REQ111_PrimitiveFromModelStrictBuildsValidValues pins the
// controls of the strict refusals: the shapes they refuse, with a value the
// reading admits, build in the strict mode.
func TestREQ103_REQ111_PrimitiveFromModelStrictBuildsValidValues(t *testing.T) {
	primitive := func(item aom14.CPrimitive) aom14.CObject {
		return &aom14.CPrimitiveObject{RMTypeName: "ANY", Item: item}
	}
	mandatory := aom14.TimezoneValidityKindMandatory
	optional := aom14.TimezoneValidityKindOptional
	for _, model := range []aom14.CObject{
		primitive(&aom14.CDate{Range: &rm.Interval[string]{Lower: "2020-12-31", LowerIncluded: true, UpperUnbounded: true}, TimezoneValidity: &optional}),
		primitive(&aom14.CTime{Range: &rm.Interval[string]{LowerUnbounded: true, Upper: "12:00:00", UpperIncluded: true}, TimezoneValidity: &mandatory}),
		primitive(&aom14.CDateTime{TimezoneValidity: &mandatory}),
		primitive(&aom14.CDuration{Range: &rm.Interval[string]{Lower: "PT0S", LowerIncluded: true, Upper: "PT1H", UpperIncluded: true}}),
		primitive(&aom14.CString{}),
		&aom14.ConstraintRef{RMTypeName: "CODE_PHRASE", Reference: "ac0001"},
	} {
		if _, err := primitiveFromModel(model, true); err != nil {
			t.Errorf("primitiveFromModel(%s, strict) = %v; want no error", describeModel(model), err)
		}
	}
}

// describeModel names a model node for a failure message.
func describeModel(o aom14.CObject) string {
	if p, ok := o.(*aom14.CPrimitiveObject); ok {
		return fmt.Sprintf("C_PRIMITIVE_OBJECT of %T", p.Item)
	}
	return fmt.Sprintf("%T", o)
}
