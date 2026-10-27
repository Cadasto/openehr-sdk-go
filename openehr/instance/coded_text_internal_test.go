package instance

import (
	"cmp"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// floorGroupName reads the group a code_not_in_value_set diagnostic of the
// RM floor names; REQ-112 has the diagnostic name the group.
var floorGroupName = regexp.MustCompile(`with a code of its group "([^"]+)"`)

// noGroupCode is an openehr code no group of the openEHR terminology has.
const noGroupCode = "no-such-code"

// setCodedText sets the field of the RM value v (a pointer to a struct)
// whose canonical JSON name is attr, looking into embedded structs, to ct,
// in the form the field holds it: by value, by pointer, or in an
// interface. It reports false when v has no such field, or the field
// cannot hold a DV_CODED_TEXT.
func setCodedText(v any, attr string, ct rm.DVCodedText) bool {
	field, ok := jsonField(reflect.ValueOf(v).Elem(), attr)
	if !ok {
		return false
	}
	for _, form := range []reflect.Value{reflect.ValueOf(ct), reflect.ValueOf(&ct)} {
		if form.Type().AssignableTo(field.Type()) {
			field.Set(form)
			return true
		}
	}
	return false
}

// jsonField returns the field of the struct s whose canonical JSON name is
// name, looking into embedded structs.
func jsonField(s reflect.Value, name string) (reflect.Value, bool) {
	for i := range s.NumField() {
		f := s.Type().Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if v, ok := jsonField(s.Field(i), name); ok {
				return v, true
			}
			continue
		}
		if tag, _, _ := strings.Cut(f.Tag.Get("json"), ","); tag == name {
			return s.Field(i), true
		}
	}
	return reflect.Value{}, false
}

// floorGroupRows returns the attributes the template-less RM floor holds to
// a group of the openEHR terminology, with the name of that group, read
// from the floor's own findings: each single attribute the BMM types
// DV_TEXT or DV_CODED_TEXT, on each concrete class the type registry
// builds, is given an openehr code no group has, and the floor's
// code_not_in_value_set at its defining code names the group. It also
// returns the attributes it could probe, so a caller can tell an attribute
// the floor ties to no group from one it never asked about.
func floorGroupRows(t *testing.T) (rows map[classAttribute]string, probed map[classAttribute]bool) {
	t.Helper()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default answers no class hierarchy")
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default lists no attribute names")
	}
	rows, probed = make(map[classAttribute]string), make(map[classAttribute]bool)
	for _, class := range rminfo.Default.KnownRMTypes() {
		if abstract, known := h.IsAbstract(class); !known || abstract {
			continue
		}
		for _, attr := range lister.AttributeNames(class) {
			typ, _ := rminfo.Default.AttributeRMType(class, attr)
			if container, _ := rminfo.Default.IsContainer(class, attr); container || (typ != "DV_TEXT" && typ != "DV_CODED_TEXT") {
				continue
			}
			value, err := rmwrite.NewRM(class)
			if err != nil {
				continue
			}
			coded := rm.DVCodedText{
				Value:        "example",
				DefiningCode: rm.CodePhrase{CodeString: noGroupCode, TerminologyID: rm.TerminologyID{Value: terminology.ID}},
			}
			if !setCodedText(value, attr, coded) {
				continue
			}
			key := classAttribute{class: class, attr: attr}
			probed[key] = true
			for _, iss := range validation.ValidateRM(value).Issues {
				if iss.Code != "code_not_in_value_set" || iss.Path != "/"+attr+"/defining_code" {
					continue
				}
				m := floorGroupName.FindStringSubmatch(iss.Detail)
				if m == nil {
					t.Errorf("%s.%s: the floor reports %q, which names no group", class, attr, iss.Detail)
					continue
				}
				rows[key] = m[1]
			}
		}
	}
	return rows, probed
}

// sortedKeys returns the keys of m by class, then attribute.
func sortedKeys[V any](m map[classAttribute]V) []classAttribute {
	return slices.SortedFunc(maps.Keys(m), func(a, b classAttribute) int {
		return cmp.Or(cmp.Compare(a.class, b.class), cmp.Compare(a.attr, b.attr))
	})
}

// TestREQ107_REQ112_AttributeGroupsAreTheFloorsGroupRows is the check that
// attributeGroups, the attributes whose openehr code the generator gives
// its group rubric (REQ-107 RM defaults), are the attributes the REQ-112
// Coded invariants table pairs with a group, read from the RM floor that
// evaluates that table, each with the same group, so the two cannot drift
// apart. Every attribute attributeGroups lists must be one the floor was
// asked about.
func TestREQ107_REQ112_AttributeGroupsAreTheFloorsGroupRows(t *testing.T) {
	t.Parallel()
	floor, probed := floorGroupRows(t)
	if len(floor) == 0 {
		t.Fatal("the floor held no attribute to a group, so nothing was compared")
	}
	for _, key := range sortedKeys(attributeGroups) {
		group := attributeGroups[key]
		got, ok := floor[key]
		switch {
		case !probed[key]:
			t.Errorf("%s.%s: attributeGroups lists it, but the floor could not be asked about it", key.class, key.attr)
		case !ok:
			t.Errorf("%s.%s: attributeGroups ties it to the group %q, the floor to none", key.class, key.attr, group.Name())
		case got != group.Name():
			t.Errorf("%s.%s: attributeGroups ties it to the group %q, the floor to %q", key.class, key.attr, group.Name(), got)
		}
	}
	for _, key := range sortedKeys(floor) {
		if _, ok := attributeGroups[key]; !ok {
			t.Errorf("%s.%s: the floor holds it to the group %q, attributeGroups leaves it out", key.class, key.attr, floor[key])
		}
	}
}

// foreignCode returns an openEHR code that another group has and group
// does not.
func foreignCode(t *testing.T, group *terminology.Group) string {
	t.Helper()
	for other := range terminology.Groups() {
		for concept := range other.All() {
			if !group.Has(concept.Code) {
				return concept.Code
			}
		}
	}
	t.Fatalf("every openEHR code is in the group %q", group.Name())
	return ""
}

// TestREQ107_EveryGroupTiedAttributeTakesItsGroupRubric is the REQ-107 RM
// defaults check of useCodedText on each attribute attributeGroups lists,
// including those the walk cannot build from an OPT node (a
// PARTICIPATION's function and mode, an ISM_TRANSITION's transition, a
// TERM_MAPPING's purpose): a coded text the OPT builds under that
// attribute, whose OPT node admits any openehr code and any text, takes
// for each code of the attribute's group that code's rubric in the group,
// and keeps the text the walk wrote for an openehr code the group does not
// have. The same coded text under an attribute the RM ties to no group, an
// ELEMENT's value, keeps its text.
func TestREQ107_EveryGroupTiedAttributeTakesItsGroupRubric(t *testing.T) {
	t.Parallel()
	codedNode := func(t *testing.T, class, attr string) *tcimpl.CompiledNode {
		t.Helper()
		_, a := visitAttribute(t, visitRootOPT(class, visitSingle(attr, relationshipCodedText(terminology.ID))), attr)
		children := tcimpl.AllowedChildNodes(a)
		if len(children) != 1 || children[0].RMTypeName() != "DV_CODED_TEXT" {
			t.Fatalf("%s.%s compiled with %d allowed children, want its one DV_CODED_TEXT", class, attr, len(children))
		}
		return children[0]
	}
	coded := func(code string) rm.DVCodedText {
		return rm.DVCodedText{
			Value:        "example",
			DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}},
		}
	}
	for _, key := range sortedKeys(attributeGroups) {
		group := attributeGroups[key]
		t.Run(key.class+"."+key.attr, func(t *testing.T) {
			node := codedNode(t, key.class, key.attr)
			for concept := range group.All() {
				v := coded(concept.Code)
				useCodedText(node, &v)
				if v.Value != concept.Rubric {
					t.Errorf("openehr::%s = %q, want its rubric %q in the group %q", concept.Code, v.Value, concept.Rubric, group.Name())
				}
			}
			foreign := foreignCode(t, group)
			v := coded(foreign)
			if useCodedText(node, &v); v.Value != "example" {
				t.Errorf("openehr::%s, which the group %q does not have, = %q, want the walk's \"example\"", foreign, group.Name(), v.Value)
			}
		})
	}
	t.Run("ELEMENT.value", func(t *testing.T) {
		node := codedNode(t, "ELEMENT", "value")
		v := coded("271")
		if useCodedText(node, &v); v.Value != "example" {
			t.Errorf("ELEMENT value openehr::271 = %q, want the walk's \"example\": the RM ties ELEMENT.value to no group", v.Value)
		}
	})
}
