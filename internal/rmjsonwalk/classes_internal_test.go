package rmjsonwalk

import (
	"bytes"
	"encoding/json/jsontext"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestClassesAnswerAsTheLookup pins that the index a walk reads classes and
// attributes from gives the answers the lookup gives: for every class the RM
// lists, whether it is abstract, and each of its attributes, and a name no
// class has, read in a slot that binds nothing and in one that binds a class.
func TestClassesAnswerAsTheLookup(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	lister := rminfo.Default.(rminfo.AttributeLister)
	hier := rminfo.Default.(rminfo.Hierarchy)
	classes := m.classes()
	var bound int // attributes a binding changes, so the bound reads are a control
	for _, name := range rminfo.Default.KnownRMTypes() {
		c := classes[name]
		abstract, _ := hier.IsAbstract(name)
		if c == nil || c.name != name || c.abstract != abstract {
			t.Errorf("classes()[%s] = %+v, want the class, abstract %t", name, c, abstract)
			continue
		}
		attrs := lister.AttributeNames(name)
		if len(c.attrs) != len(attrs) {
			t.Errorf("classes()[%s] holds %d attributes, want the %d the RM lists", name, len(c.attrs), len(attrs))
		}
		for _, attr := range append(attrs, "x-no-such-attribute") {
			for _, b := range []string{"", "DV_QUANTITY"} {
				got, gotOK := m.Attribute(name, attr, b)
				want, wantOK := m.attribute(name, attr, b)
				if got != want || gotOK != wantOK {
					t.Errorf("Attribute(%s, %s, %q) = %+v, %t; the lookup gives %+v, %t", name, attr, b, got, gotOK, want, wantOK)
				}
			}
			if plain, _ := m.Attribute(name, attr, ""); plain != (Attribute{}) {
				if withBound, _ := m.Attribute(name, attr, "DV_QUANTITY"); withBound != plain {
					bound++
				}
			}
		}
	}
	if bound == 0 {
		t.Error("no attribute reads differently in a slot that binds a class: the bound reads control nothing")
	}
}

// listerless is rminfo.Default without the optional attribute lister, so a
// Model over it holds no attributes in its index and asks the lookup for each.
type listerless struct {
	rminfo.Lookup
	rminfo.Hierarchy
	rminfo.AbsenceReporter
}

// TestWalkWithoutTheAttributeIndexReportsTheSame pins that a walk over a
// lookup that lists no attributes, which reads every member's attribute from
// the lookup, reports what a walk over the full index reports, on a corpus
// composition with a stray member in every object and a member dropped from
// every third.
func TestWalkWithoutTheAttributeIndexReportsTheSame(t *testing.T) {
	full, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	bare, err := New(listerless{
		Lookup:          rminfo.Default,
		Hierarchy:       rminfo.Default.(rminfo.Hierarchy),
		AbsenceReporter: rminfo.Default.(rminfo.AbsenceReporter),
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if n := len(bare.classes()["COMPOSITION"].attrs); n != 0 {
		t.Fatalf("the index over a lookup that lists no attributes holds %d for COMPOSITION, want none", n)
	}
	data, err := os.ReadFile(fixtures.CompositionJSON("BMI"))
	if err != nil {
		t.Fatalf("read BMI: %v", err)
	}
	tree, err := parseNode(jsontext.NewDecoder(bytes.NewReader(data)))
	if err != nil {
		t.Fatalf("parse BMI: %v", err)
	}
	for i, obj := range tree.objects() {
		obj.addStray(false)
		if i%3 == 1 {
			obj.dropMember()
		}
	}
	var b bytes.Buffer
	tree.write(&b)
	root := Root{Type: "COMPOSITION"}
	var want, got []string
	if err := full.Walk(b.Bytes(), root, logHooks(&want)); err != nil {
		t.Fatalf("Walk over the full index: %v", err)
	}
	if err := bare.Walk(b.Bytes(), root, logHooks(&got)); err != nil {
		t.Fatalf("Walk over the bare lookup: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Walk over a lookup that lists no attributes reported\n%q\nwant what the full index reports,\n%q", got, want)
	}
	if len(want) == 0 {
		t.Error("Walk reported nothing: the case cannot tell the two apart")
	}
}

// TestMemberNamesWithEscapesReadAsTheirText pins that a member name spelled
// with escapes is read as the text it stands for, as the decoder reads it: an
// attribute's name is matched and counts as present, and an unmatched name is
// reported by its text.
func TestMemberNamesWithEscapesReadAsTheirText(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	// Each "~" is a backslash: "m~u0061gnitude" spells "magnitude".
	doc := []byte(strings.ReplaceAll(`{"_type":"DV_QUANTITY","m~u0061gnitude":1,"~u0075nits":"kg","x~u002da":1,"precision":null}`, "~", `\`))
	var got []string
	if err := m.Walk(doc, Root{}, logHooks(&got)); err != nil {
		t.Fatalf("Walk(%s): %v", doc, err)
	}
	want := []string{"unmatched /x-a"}
	if !slices.Equal(got, want) {
		t.Errorf("Walk(%s) reported %q, want %q", doc, got, want)
	}
}
