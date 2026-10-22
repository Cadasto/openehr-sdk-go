package instance

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// floorSetName reads the group or code set a code_not_in_value_set
// diagnostic of the RM floor names; REQ-112 has the diagnostic name it.
var floorSetName = regexp.MustCompile(`with a code of its group "([^"]+)"|code set "([^"]+)"`)

// codedRow is a coded invariant as one side holds it for an attribute: the
// rule's name CLASS.Invariant, the name of its group or code set, and
// whether the rule reads a coded text's defining code.
type codedRow struct {
	rule, set string
	coded     bool
}

// row is r as a codedRow, coded where r keeps a group, whose attribute is
// a DV_CODED_TEXT.
func (r *codedRule) row() codedRow {
	if r.group != nil {
		return codedRow{rule: r.class + "." + r.invariant, set: r.group.Name(), coded: true}
	}
	return codedRow{rule: r.class + "." + r.invariant, set: r.codeSet.Name()}
}

// setField sets the field of the RM value v (a pointer to a struct) whose
// canonical JSON name is attr, looking into embedded structs, to value, by
// pointer where the field takes one, as the generator writes it, and by
// value otherwise. It reports false when v has no such field, or the field
// can hold neither.
func setField[T any](v any, attr string, value T) bool {
	field, ok := jsonField(reflect.ValueOf(v).Elem(), attr)
	if !ok {
		return false
	}
	for _, form := range []reflect.Value{reflect.ValueOf(&value), reflect.ValueOf(value)} {
		if form.Type().AssignableTo(field.Type()) {
			field.Set(form)
			return true
		}
	}
	return false
}

// probeCoded sets attr of a new value of class to code, as a coded text
// where text is set and as a code phrase otherwise, and returns the value,
// what the RM floor reports at the code phrase a rule reads there, and
// whether attr could be set.
func probeCoded(t *testing.T, class, attr string, code rm.CodePhrase, text bool) (value any, floor []codedRow, ok bool) {
	t.Helper()
	value, err := rmwrite.NewRM(class)
	if err != nil {
		return nil, nil, false
	}
	if text {
		ok = setField(value, attr, rm.DVCodedText{Value: "example", DefiningCode: code})
	} else {
		ok = setField(value, attr, code)
	}
	if !ok {
		return nil, nil, false
	}
	for _, iss := range validation.ValidateRM(value).Issues {
		if iss.Code != "code_not_in_value_set" || (iss.Path != "/"+attr && iss.Path != "/"+attr+"/defining_code") {
			continue
		}
		m := floorSetName.FindStringSubmatch(iss.Detail)
		if m == nil {
			t.Errorf("%s.%s: the floor reports %q, which names no group or code set", class, attr, iss.Detail)
			continue
		}
		floor = append(floor, codedRow{rule: iss.Rule, set: m[1] + m[2], coded: iss.Path != "/"+attr})
	}
	return value, floor, true
}

// slotRows returns the rows of the slots codedSlots lists on attr of v.
func slotRows(v any, attr string) []codedRow {
	var out []codedRow
	for _, s := range codedSlots(v) {
		if s.rule.attr == attr {
			out = append(out, s.rule.row())
		}
	}
	return out
}

// member is a code r accepts: the first of its group in openehr, or the
// first of its code set in the code set's external id.
func (r *codedRule) member() rm.CodePhrase {
	if r.group != nil {
		for c := range r.group.All() {
			return rm.CodePhrase{CodeString: c.Code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
		}
	}
	for c := range r.codeSet.All() {
		return rm.CodePhrase{CodeString: c, TerminologyID: rm.TerminologyID{Value: r.codeSet.ExternalID()}}
	}
	return rm.CodePhrase{}
}

// TestREQ107_REQ112_CodedRulesAreTheFloorsCodedInvariants is the check that
// codedRules, the rules the generator keeps a coded attribute to (REQ-107
// Precedence 4), are the rows of the REQ-112 Coded invariants table, read
// from the RM floor that evaluates that table, so the two cannot drift
// apart. Each single attribute the BMM types CODE_PHRASE, DV_TEXT or
// DV_CODED_TEXT, on each concrete class the type registry builds, is given
// a code no group or code set has: the rule the floor reports at it, with
// its group or code set, must be the rule codedSlots lists there, and the
// floor must report none where codedSlots lists none. Each rule must be met
// on some class, and on each attribute it is met on, a member of its set
// is one both the floor and the rule accept, and, for a group, the same
// code in another terminology is one neither accepts.
func TestREQ107_REQ112_CodedRulesAreTheFloorsCodedInvariants(t *testing.T) {
	t.Parallel()
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default answers no class hierarchy")
	}
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default lists no attribute names")
	}
	outside := rm.CodePhrase{CodeString: noGroupCode, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
	met := make(map[*codedRule]bool)
	for _, class := range rminfo.Default.KnownRMTypes() {
		if abstract, known := h.IsAbstract(class); !known || abstract {
			continue
		}
		for _, attr := range lister.AttributeNames(class) {
			typ, _ := rminfo.Default.AttributeRMType(class, attr)
			text := typ == "DV_TEXT" || typ == "DV_CODED_TEXT"
			if container, _ := rminfo.Default.IsContainer(class, attr); container || (!text && typ != "CODE_PHRASE") {
				continue
			}
			value, floor, ok := probeCoded(t, class, attr, outside, text)
			if !ok {
				continue
			}
			slots := slotRows(value, attr)
			if !reflect.DeepEqual(slots, floor) {
				t.Errorf("%s.%s: codedSlots lists %+v, the floor reports %+v", class, attr, slots, floor)
				continue
			}
			for _, s := range codedSlots(value) {
				if s.rule.attr != attr {
					continue
				}
				met[s.rule] = true
				checkMembership(t, class, attr, s.rule, text)
			}
		}
	}
	for _, r := range codedRules {
		if !met[r] {
			t.Errorf("%s.%s: codedRules lists it, but no class the floor was asked about holds it", r.class, r.attr)
		}
	}
}

// checkMembership checks that the floor and r agree on a member of r's set
// on attr of class, and, for a group, on that code in another terminology.
func checkMembership(t *testing.T, class, attr string, r *codedRule, text bool) {
	t.Helper()
	member := r.member()
	if _, floor, _ := probeCoded(t, class, attr, member, text); !r.accepts(member) || len(floor) != 0 {
		t.Errorf("%s.%s: %+v: rule accepts = %t, the floor reports %+v; want both to accept it", class, attr, member, r.accepts(member), floor)
	}
	if r.group == nil {
		return
	}
	local := rm.CodePhrase{CodeString: member.CodeString, TerminologyID: rm.TerminologyID{Value: "local"}}
	if _, floor, _ := probeCoded(t, class, attr, local, text); r.accepts(local) || len(floor) != 1 {
		t.Errorf("%s.%s: %+v: rule accepts = %t, the floor reports %+v; want neither to accept it", class, attr, local, r.accepts(local), floor)
	}
}

// TestREQ107_CodedSettleReadsAProhibitedAttributeOnlyWhereTheRMNeedsIt is the
// REQ-107 Precedence 1 and 2 check of settleCoded on a coded attribute the
// OPT prohibits. Where no RM rule needs it, the OPT admits no value there, so
// the code it holds stays, though the child the OPT names under it lists a
// member of the group. Where an RM rule needs it, an ELEMENT's null flavour
// where the OPT prohibits value too, the child constrains it as if the OPT
// allowed the attribute, so the null flavour takes the member that child
// lists. It calls settleCoded directly, since a value Generate builds holds
// no attribute the OPT prohibits that no RM rule needs.
func TestREQ107_CodedSettleReadsAProhibitedAttributeOnlyWhereTheRMNeedsIt(t *testing.T) {
	t.Parallel()
	outside := func(code string) *rm.DVCodedText {
		return &rm.DVCodedText{Value: "example", DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}}
	}
	t.Run("TERM_MAPPING purpose, prohibited, needed by no rule", func(t *testing.T) {
		t.Parallel()
		opt, _ := visitAttribute(t, visitRootOPT("TERM_MAPPING",
			visitProhibitedSingle("purpose", relationshipCodedText(terminology.ID, "271", "670"))), "purpose")
		tm := &rm.TermMapping{Purpose: outside("271")}
		(&generator{}).settleCoded(opt, tm)
		if got := tm.Purpose.DefiningCode.CodeString; got != "271" {
			t.Errorf("TERM_MAPPING.purpose = %s, want the 271 it held: the OPT prohibits purpose", got)
		}
	})
	t.Run("ELEMENT null_flavour, prohibited with value, needed by Inv_null_flavour_indicated", func(t *testing.T) {
		t.Parallel()
		opt, _ := visitAttribute(t, visitRootOPT("ELEMENT",
			visitProhibitedSingle("null_flavour", relationshipCodedText(terminology.ID, "433", "272"))+
				visitProhibitedSingle("value", "")), "null_flavour")
		e := &rm.Element{NullFlavour: outside("433")}
		(&generator{}).settleCoded(opt, e)
		if got := e.NullFlavour.DefiningCode.CodeString; got != "272" {
			t.Errorf("ELEMENT.null_flavour = %s, want 272, the member its OPT child lists", got)
		}
	})
}
