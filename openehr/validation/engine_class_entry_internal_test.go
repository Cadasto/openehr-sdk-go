package validation

// engine_class_entry_internal_test.go: REQ-171 § One walk — what the walk
// reads of the class of a node, its RM rules and the table of its
// attributes, is found in one lookup per node, and is what a lookup of each
// in its own table finds; each attribute the table walks is built once per
// class, as the walk read it per node; and at a node a template binds, an
// attribute the template declares is visited in the template's order and not
// again in the class's.

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// walkedNamed returns the attribute name the table t walks, as the walk
// reads it at [classAttributeTable.position], nil when t walks none by that
// name or t is nil.
func walkedNamed(t *classAttributeTable, name string) *engineAttr {
	i, ok := t.position(name)
	if !ok {
		return nil
	}
	return &t.walked[i]
}

// REQ-171 § One walk — for every class that has RM rules or an attribute
// table, for a generic instantiation and for a type no class has, the one
// lookup the walk makes per node finds the rules [floorRules] holds for the
// bare class and the table [classAttributes] holds for it, each nil where its
// own table holds none.
func TestREQ171_ClassEntryHoldsTheRulesAndTheTableOfTheClass(t *testing.T) {
	t.Parallel()
	if r, tb := floorRules.byClass["LOCATABLE"], classAttributes["LOCATABLE"]; r != nil || tb == nil {
		t.Fatalf("precondition: LOCATABLE has rules %v and table %v, want a table and no rules", r, tb)
	}
	types := slices.Concat(slices.Sorted(maps.Keys(floorRules.byClass)), slices.Sorted(maps.Keys(classAttributes)))
	types = append(types, "DV_INTERVAL<DV_QUANTITY>", "NO_SUCH_CLASS")
	for _, rmType := range types {
		entry, class := classOf(rmType)
		want := rmType
		if rmType == "DV_INTERVAL<DV_QUANTITY>" {
			want = "DV_INTERVAL"
		}
		if class != want || entry.rules != floorRules.byClass[want] || entry.attrs != classAttributes[want] {
			t.Errorf("classOf(%s) = rules %p, table %p, class %s; want rules %p, table %p, class %s",
				rmType, entry.rules, entry.attrs, class, floorRules.byClass[want], classAttributes[want], want)
		}
	}
}

// REQ-171 § One walk — the attributes the table of each class walks, built
// once per class, are the attributes it lists that a serialization carries
// and the metadata types, in the class's order, and find returns each by its
// name and no other. Read on a node in each role, each is the attribute the
// walk built per node before: its name, declared type and shape as the
// metadata gives them; mandatory when the metadata marks it mandatory for
// the class, no rule of the class reports its absence itself and, for a
// single-valued one, the role does not let it be blank; with no template
// declaration. Each records whether its declared type has RM rules.
func TestREQ171_ClassTableWalksEachAttributeAsTheWalkReadsIt(t *testing.T) {
	t.Parallel()
	code := walkedNamed(classAttributes["CODE_PHRASE"], "code_string")
	if code == nil || !code.inRole(roleNone).mandatory || code.inRole(roleScaleSymbolCode).mandatory || !code.noRules {
		t.Fatalf("precondition: CODE_PHRASE code_string reads as %+v; want a mandatory String, not mandatory in a DV_SCALE symbol's code", code)
	}
	if lang := walkedNamed(classAttributes["COMPOSITION"], "language"); lang == nil || lang.noRules {
		t.Fatalf("precondition: COMPOSITION language reads as %+v; want a CODE_PHRASE, which has RM rules", lang)
	}
	roles := []floorRole{roleNone, roleScaleSymbol, roleScaleSymbolCode, roleArchetypedID}
	for _, class := range slices.Sorted(maps.Keys(classAttributes)) {
		table, rules := classAttributes[class], floorRules.byClass[class]
		var walked []attributeFacts
		for _, f := range table.listed {
			if f.NonStorable || !f.Typed {
				if got := walkedNamed(table, f.Name); got != nil {
					t.Errorf("%s table finds %s, which it does not walk, as %+v; want none", class, f.Name, got)
				}
				continue
			}
			walked = append(walked, f)
		}
		if len(table.walked) != len(walked) {
			t.Errorf("%s table walks %d attributes, want %d", class, len(table.walked), len(walked))
			continue
		}
		for i, f := range walked {
			if got := walkedNamed(table, f.Name); got != &table.walked[i] {
				t.Errorf("%s table finds %s as %p, want the attribute at position %d, %p", class, f.Name, got, i, &table.walked[i])
			}
			shape := shapeFrom(f.Container, f.PrimitiveItems, f.Hash)
			declared, _ := rulesFor(f.RMType)
			for _, role := range roles {
				want := engineAttr{
					name:      f.Name,
					rmType:    f.RMType,
					shape:     shape,
					mandatory: f.Mandatory && !rules.leavesOut(f.Name) && (shape != shapeSingle || !mayBeBlank(role, f.Name)),
					noRules:   declared == nil,
				}
				if got := table.walked[i].inRole(role); got != want {
					t.Errorf("%s %s in role %d reads as %+v, want %+v", class, f.Name, role, got, want)
				}
			}
		}
	}
	if got := walkedNamed(nil, "value"); got != nil {
		t.Errorf("a nil table finds value as %+v, want none", got)
	}
}

// REQ-171 § One walk — a class that has RM rules and no attribute table
// keeps its rules, and one that has a table and no rules keeps its table:
// joining the two tables loses neither side.
func TestREQ171_JoinClassesKeepsAClassOfOneTableOnly(t *testing.T) {
	t.Parallel()
	rules := &classRules{index: 7}
	table := &classAttributeTable{}
	got := joinClasses(map[string]*classRules{"RULES_ONLY": rules}, map[string]*classAttributeTable{"TABLE_ONLY": table})
	want := map[string]classEntry{"RULES_ONLY": {rules: rules}, "TABLE_ONLY": {attrs: table}}
	if !maps.Equal(got, want) {
		t.Errorf("joinClasses(rules of RULES_ONLY, table of TABLE_ONLY) = %v, want %v", got, want)
	}
}

// declaredTwiceOPT is a COMPOSITION template whose root, node id at0000,
// declares, each with no child, language, an attribute COMPOSITION does not
// list, language again, and territory, which it leaves optional.
const declaredTwiceOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>engine-declared-twice</value></template_id>
  <concept>engine-declared-twice</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>language</rm_attribute_name><existence>` + existence1 + `</existence></attributes>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>no_such_attribute</rm_attribute_name><existence>` + existence1 + `</existence></attributes>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>language</rm_attribute_name><existence>` + existence1 + `</existence></attributes>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>territory</rm_attribute_name><existence>` + existence01 + `</existence></attributes>
  </definition>
</template>`

// existence1 and existence01 are the existence intervals 1..1 and 0..1.
const (
	existence1  = `<lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper>`
	existence01 = `<lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper>`
)

// compileOPTText parses and compiles the operational template xml.
func compileOPTText(t *testing.T, xml string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPT(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// REQ-171 § One walk — at a node a template binds, an attribute the
// template declares twice is visited at each declaration, in the template's
// order, and not again in the class's order; an attribute it declares that
// the class does not have is left out, whatever its existence, and its path
// listed in Coverage.NotInRM (§ Coverage); and the attributes the class
// marks mandatory that the template does not declare are visited in the
// class's order. On a root holding none of them each visit reports the
// attribute once, so language twice, by the template's existence; on a root
// holding language and territory the walk enters language twice and
// territory once. The issues and the visit counts were read off the walk
// before the attributes the template declares were marked by their position
// in the class's table, and the required issue no_such_attribute drew then
// was dropped when REQ-171 One walk came to leave such an attribute out.
func TestREQ171_AnAttributeTheTemplateDeclaresTwiceIsVisitedPerDeclaration(t *testing.T) {
	t.Parallel()
	c := compileOPTText(t, declaredTwiceOPT)
	held := &rm.Composition{
		Language:  rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Territory: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}, CodeString: "NL"},
	}
	tests := []struct {
		name   string
		root   *rm.Composition
		visits int
		issues []string
	}{
		{
			name: "none held", root: &rm.Composition{}, visits: 1,
			issues: []string{
				"is_archetype_root /archetype_details rm",
				"code_not_in_value_set /category/defining_code rm",
				"code_not_in_value_set /language rm",
				"code_not_in_value_set /territory rm",
				"required /language template",
				"required /language template",
				"required /territory rm",
				"required /name rm",
				"required /archetype_node_id rm",
				"required /category rm",
				"required /composer rm",
			},
		},
		{
			name: "language and territory held", root: held, visits: 13,
			issues: []string{
				"is_archetype_root /archetype_details rm",
				"code_not_in_value_set /category/defining_code rm",
				"required /name rm",
				"required /archetype_node_id rm",
				"required /category rm",
				"required /composer rm",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, e := engineWalk(tc.root, c, Limits{})
			if e == nil {
				t.Fatal("engineWalk refused the call, want a walk")
			}
			var got []string
			for _, iss := range r.Issues {
				got = append(got, iss.Code+" "+iss.Path+" "+iss.Layer.String())
			}
			if e.visits != tc.visits || !slices.Equal(got, tc.issues) {
				t.Errorf("engineWalk(%s) visits %d values and reports %q; want %d values and %q", tc.name, e.visits, got, tc.visits, tc.issues)
			}
			if want := []string{"/no_such_attribute"}; !slices.Equal(r.Coverage.NotInRM, want) {
				t.Errorf("engineWalk(%s) Coverage.NotInRM = %q, want %q", tc.name, r.Coverage.NotInRM, want)
			}
		})
	}
}

// REQ-171 § One walk — on no node of a template the corpus vendors, nor of
// the template that declares an attribute twice, does the compile inject an
// attribute ([tcimpl.CompiledAttribute.Implicit]) that the template
// declares. The walk marks the attributes a template declares by the
// template's declarations alone, which reads them as the node's first
// attribute by each name reads them only while that holds.
func TestREQ171_CompileInjectsNoAttributeTheTemplateDeclares(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	compiled := []*templatecompile.Compiled{compileOPTText(t, declaredTwiceOPT)}
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			continue
		}
		if c, err := templatecompile.Compile(opt); err == nil {
			compiled = append(compiled, c)
		}
	}
	if len(compiled) < 10 {
		t.Fatalf("precondition: %d corpus templates compile, want at least 10", len(compiled)-1)
	}
	nodes := 0
	var check func(n *tcimpl.CompiledNode)
	check = func(n *tcimpl.CompiledNode) {
		nodes++
		declared := map[string]bool{}
		for _, a := range tcimpl.NodeAttributes(n) {
			if !a.Implicit() {
				declared[a.Name()] = true
			}
		}
		for _, a := range tcimpl.NodeAttributes(n) {
			if a.Implicit() && declared[a.Name()] {
				t.Errorf("node %s of RM type %s holds %s both as the template declares it and as the compile injects it", n.AQLPath(), n.RMTypeName(), a.Name())
			}
			for _, child := range tcimpl.ChildNodes(a) {
				check(child)
			}
		}
	}
	for _, c := range compiled {
		check(c.Root())
	}
	t.Logf("%d templates, %d nodes", len(compiled), nodes)
}

// REQ-171 § One walk — no class's table walks more attributes than the
// mark of the attributes a template declares has bits ([attrMarks]).
func TestREQ171_NoClassTableWalksMoreAttributesThanTheMarkHolds(t *testing.T) {
	t.Parallel()
	held := reflect.TypeFor[attrMarks]().Bits()
	for _, class := range slices.Sorted(maps.Keys(classAttributes)) {
		if n := len(classAttributes[class].walked); n > held {
			t.Errorf("the %s table walks %d attributes, more than the %d an attrMarks holds", class, n, held)
		}
	}
}
