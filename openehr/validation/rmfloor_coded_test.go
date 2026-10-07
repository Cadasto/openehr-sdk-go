package validation_test

// rmfloor_coded_test.go: REQ-112 Coded invariants. The RM floor checks every
// coded attribute of the catalogue's table against the openEHR terminology
// group or code set it names and reports a breach as `code_not_in_value_set`
// at the CODE_PHRASE it read. Each test lists the exact findings by path.

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

const codeNotInValueSet = "code_not_in_value_set"

// phrase returns the CODE_PHRASE code_string coded in terminology tid.
func phrase(tid, code string) rm.CodePhrase {
	return rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: tid}, CodeString: code}
}

// codedText returns a DV_CODED_TEXT whose defining_code is code.
func codedText(code rm.CodePhrase) rm.DVCodedText {
	return rm.DVCodedText{Value: "coded", DefiningCode: code}
}

// openEHRCode returns code coded in the openEHR terminology.
func openEHRCode(code string) rm.CodePhrase { return phrase(terminology.ID, code) }

// codedArchetyped returns the archetype_details of an archetype root.
func codedArchetyped(id string) *rm.Archetyped {
	return &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: id}, RMVersion: "1.1.0"}
}

// codedComposition returns a COMPOSITION whose category, language and
// territory hold.
func codedComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID:  "openEHR-EHR-COMPOSITION.encounter.v1",
		ArchetypeDetails: codedArchetyped("openEHR-EHR-COMPOSITION.encounter.v1"),
		Name:             rm.DVText{Value: "Encounter"},
		Category:         codedText(openEHRCode("433")),
		Composer:         rm.PartySelf{},
		Language:         phrase("ISO_639-1", "en"),
		Territory:        phrase("ISO_3166-1", "NL"),
	}
}

// codedContext returns an EVENT_CONTEXT whose setting holds.
func codedContext() *rm.EventContext {
	return &rm.EventContext{
		StartTime: rm.DVDateTime{Value: "2026-10-01T10:00:00Z"},
		Setting:   codedText(openEHRCode("238")),
	}
}

// codedEvaluation returns an EVALUATION whose language and encoding hold.
func codedEvaluation() *rm.Evaluation {
	return &rm.Evaluation{
		ArchetypeNodeID:  "openEHR-EHR-EVALUATION.problem.v1",
		ArchetypeDetails: codedArchetyped("openEHR-EHR-EVALUATION.problem.v1"),
		Name:             rm.DVText{Value: "Problem"},
		Language:         phrase("ISO_639-1", "en"),
		Encoding:         phrase("IANA_character-sets", "UTF-8"),
		Subject:          rm.PartySelf{},
		Data:             &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}},
	}
}

// codedMultimedia returns a DV_MULTIMEDIA whose media_type holds and which
// carries none of its optional coded attributes.
func codedMultimedia() *rm.DVMultimedia {
	return &rm.DVMultimedia{MediaType: phrase("IANA_media-types", "text/plain"), Size: 1}
}

// codedFindings returns the paths of the `code_not_in_value_set` issues in
// issues, sorted.
func codedFindings(issues []validation.Issue) []string {
	var out []string
	for _, i := range issues {
		if i.Code == codeNotInValueSet {
			out = append(out, i.Path)
		}
	}
	slices.Sort(out)
	return out
}

// codedRow is one row of the REQ-112 Coded invariants table, applied to one
// class: root builds a root whose coded attribute is code, and path is where
// that CODE_PHRASE sits.
type codedRow struct {
	name      string
	invariant string
	group     *terminology.Group
	codeSet   *terminology.CodeSet
	root      func(code rm.CodePhrase) any
	path      string
	valid     rm.CodePhrase
	breach    rm.CodePhrase
}

// valueSetName is the display name of the row's group or code set.
func (r codedRow) valueSetName() string {
	if r.group != nil {
		return r.group.Name()
	}
	return r.codeSet.Name()
}

// namedID is the identifier the row's own rule names: the openEHR
// terminology id for a group, the external id for a code set. A diagnostic
// that names it carries no caller data.
func (r codedRow) namedID() string {
	if r.group != nil {
		return terminology.ID
	}
	return r.codeSet.ExternalID()
}

// codedRows covers every row of the table, and both classes of the rows
// that name two (DV_TEXT and DV_CODED_TEXT, DV_MULTIMEDIA and DV_PARSABLE).
// Each root is otherwise coded correctly, so the row's attribute is the only
// coded finding it can produce.
func codedRows() []codedRow {
	return []codedRow{
		{
			name: "COMPOSITION category", invariant: "Category_validity", group: terminology.CompositionCategory,
			root: func(c rm.CodePhrase) any {
				comp := codedComposition()
				comp.Category = codedText(c)
				return comp
			},
			path: "/category/defining_code", valid: openEHRCode("433"), breach: openEHRCode("9999"),
		},
		{
			name: "COMPOSITION language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any {
				comp := codedComposition()
				comp.Language = c
				return comp
			},
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "COMPOSITION territory", invariant: "Territory_valid", codeSet: terminology.Countries,
			root: func(c rm.CodePhrase) any {
				comp := codedComposition()
				comp.Territory = c
				return comp
			},
			path: "/territory", valid: phrase("ISO_3166-1", "NL"), breach: phrase("ISO_3166-1", "ZZ"),
		},
		{
			name: "EVENT_CONTEXT setting", invariant: "Setting_valid", group: terminology.Setting,
			root: func(c rm.CodePhrase) any {
				comp := codedComposition()
				comp.Context = codedContext()
				comp.Context.Setting = codedText(c)
				return comp
			},
			path: "/context/setting/defining_code", valid: openEHRCode("238"), breach: openEHRCode("9999"),
		},
		{
			name: "ENTRY language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any {
				ev := codedEvaluation()
				ev.Language = c
				return ev
			},
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "ENTRY encoding", invariant: "Encoding_valid", codeSet: terminology.CharacterSets,
			root: func(c rm.CodePhrase) any {
				ev := codedEvaluation()
				ev.Encoding = c
				return ev
			},
			path: "/encoding", valid: phrase("IANA_character-sets", "UTF-8"), breach: phrase("IANA_character-sets", "UTF-99"),
		},
		{
			name: "ELEMENT null_flavour", invariant: "Inv_null_flavour_valid", group: terminology.NullFlavours,
			root: func(c rm.CodePhrase) any {
				nf := codedText(c)
				return &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "item"}, NullFlavour: &nf}
			},
			path: "/null_flavour/defining_code", valid: openEHRCode("253"), breach: openEHRCode("999"),
		},
		{
			name: "INTERVAL_EVENT math_function", invariant: "Math_function_validity", group: terminology.EventMathFunction,
			root: func(c rm.CodePhrase) any {
				return &rm.IntervalEvent[rm.ItemStructure]{MathFunction: codedText(c)}
			},
			path: "/math_function/defining_code", valid: openEHRCode("146"), breach: openEHRCode("9999"),
		},
		{
			name: "ISM_TRANSITION current_state", invariant: "Current_state_valid", group: terminology.InstructionStates,
			root: func(c rm.CodePhrase) any {
				return &rm.IsmTransition{CurrentState: codedText(c)}
			},
			path: "/current_state/defining_code", valid: openEHRCode("532"), breach: openEHRCode("9999"),
		},
		{
			name: "ISM_TRANSITION transition", invariant: "Transition_valid", group: terminology.InstructionTransitions,
			root: func(c rm.CodePhrase) any {
				tr := codedText(c)
				return &rm.IsmTransition{CurrentState: codedText(openEHRCode("532")), Transition: &tr}
			},
			path: "/transition/defining_code", valid: openEHRCode("541"), breach: openEHRCode("9999"),
		},
		{
			name: "PARTICIPATION function", invariant: "Function_valid", group: terminology.ParticipationFunction,
			root: func(c rm.CodePhrase) any {
				ev := codedEvaluation()
				ev.OtherParticipations = []rm.Participation{{Function: codedText(c), Performer: rm.PartySelf{}}}
				return ev
			},
			path: "/other_participations[0]/function/defining_code", valid: openEHRCode("253"), breach: openEHRCode("999"),
		},
		{
			name: "PARTICIPATION mode", invariant: "Mode_valid", group: terminology.ParticipationMode,
			root: func(c rm.CodePhrase) any {
				mode := codedText(c)
				ev := codedEvaluation()
				ev.OtherParticipations = []rm.Participation{{Function: rm.DVText{Value: "nurse"}, Mode: &mode, Performer: rm.PartySelf{}}}
				return ev
			},
			path: "/other_participations[0]/mode/defining_code", valid: openEHRCode("193"), breach: openEHRCode("9999"),
		},
		{
			name: "PARTY_RELATED relationship", invariant: "Relationship_valid", group: terminology.SubjectRelationship,
			root: func(c rm.CodePhrase) any {
				ev := codedEvaluation()
				ev.Subject = rm.PartyRelated{Relationship: codedText(c)}
				return ev
			},
			path: "/subject/relationship/defining_code", valid: openEHRCode("10"), breach: openEHRCode("9999"),
		},
		{
			name: "TERM_MAPPING purpose", invariant: "Purpose_valid", group: terminology.TermMappingPurpose,
			root: func(c rm.CodePhrase) any {
				purpose := codedText(c)
				return &rm.DVText{Value: "x", Mappings: []rm.TermMapping{{Match: "=", Target: phrase("SNOMED-CT", "123"), Purpose: &purpose}}}
			},
			path: "/mappings[0]/purpose/defining_code", valid: openEHRCode("669"), breach: openEHRCode("9999"),
		},
		{
			name: "DV_TEXT language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any { return &rm.DVText{Value: "x", Language: &c} },
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "DV_CODED_TEXT language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any {
				ct := codedText(phrase("local", "at1"))
				ct.Language = &c
				return &ct
			},
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "DV_TEXT encoding", invariant: "Encoding_valid", codeSet: terminology.CharacterSets,
			root: func(c rm.CodePhrase) any { return &rm.DVText{Value: "x", Encoding: &c} },
			path: "/encoding", valid: phrase("IANA_character-sets", "UTF-8"), breach: phrase("IANA_character-sets", "UTF-99"),
		},
		{
			name: "DV_CODED_TEXT encoding", invariant: "Encoding_valid", codeSet: terminology.CharacterSets,
			root: func(c rm.CodePhrase) any {
				ct := codedText(phrase("local", "at1"))
				ct.Encoding = &c
				return ct
			},
			path: "/encoding", valid: phrase("IANA_character-sets", "UTF-8"), breach: phrase("IANA_character-sets", "UTF-99"),
		},
		{
			name: "DV_ORDERED normal_status", invariant: "Normal_status_validity", codeSet: terminology.NormalStatuses,
			root: func(c rm.CodePhrase) any { return &rm.DVQuantity{Magnitude: 1, Units: "mg", NormalStatus: &c} },
			path: "/normal_status", valid: phrase("openehr_normal_statuses", "N"), breach: phrase("openehr_normal_statuses", "HHHH"),
		},
		{
			name: "DV_MULTIMEDIA language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any {
				mm := codedMultimedia()
				mm.Language = &c
				return mm
			},
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "DV_PARSABLE language", invariant: "Language_valid", codeSet: terminology.Languages,
			root: func(c rm.CodePhrase) any { return &rm.DVParsable{Value: "x", Formalism: "text", Language: &c} },
			path: "/language", valid: phrase("ISO_639-1", "en"), breach: phrase("ISO_639-1", "xx"),
		},
		{
			name: "DV_MULTIMEDIA charset", invariant: "Charset_valid", codeSet: terminology.CharacterSets,
			root: func(c rm.CodePhrase) any {
				mm := codedMultimedia()
				mm.Charset = &c
				return mm
			},
			path: "/charset", valid: phrase("IANA_character-sets", "UTF-8"), breach: phrase("IANA_character-sets", "UTF-99"),
		},
		{
			name: "DV_PARSABLE charset", invariant: "Charset_valid", codeSet: terminology.CharacterSets,
			root: func(c rm.CodePhrase) any { return rm.DVParsable{Value: "x", Formalism: "text", Charset: &c} },
			path: "/charset", valid: phrase("IANA_character-sets", "UTF-8"), breach: phrase("IANA_character-sets", "UTF-99"),
		},
		{
			name: "DV_MULTIMEDIA media_type", invariant: "Media_type_valid", codeSet: terminology.MediaTypes,
			root: func(c rm.CodePhrase) any {
				mm := codedMultimedia()
				mm.MediaType = c
				return mm
			},
			path: "/media_type", valid: phrase("IANA_media-types", "text/plain"), breach: phrase("IANA_media-types", "text/nonsense"),
		},
		{
			name: "DV_MULTIMEDIA compression_algorithm", invariant: "Compression_algorithm_validity", codeSet: terminology.CompressionAlgorithms,
			root: func(c rm.CodePhrase) any {
				mm := codedMultimedia()
				mm.CompressionAlgorithm = &c
				return mm
			},
			path: "/compression_algorithm", valid: phrase("openehr_compression_algorithms", "gzip"), breach: phrase("openehr_compression_algorithms", "rar"),
		},
		{
			name: "DV_MULTIMEDIA integrity_check_algorithm", invariant: "Integrity_check_algorithm_validity", codeSet: terminology.IntegrityCheckAlgorithms,
			root: func(c rm.CodePhrase) any {
				mm := codedMultimedia()
				mm.IntegrityCheckAlgorithm = &c
				return mm
			},
			path: "/integrity_check_algorithm", valid: phrase("openehr_integrity_check_algorithms", "SHA-256"), breach: phrase("openehr_integrity_check_algorithms", "MD5"),
		},
	}
}

// rootRows add a row for each class that codedRows reaches only through a
// holder, with the class itself as the root, so that every class of the
// table is a root in some row.
func rootRows() []codedRow {
	return []codedRow{
		{
			name: "EVENT_CONTEXT setting, as the root", invariant: "Setting_valid", group: terminology.Setting,
			root: func(c rm.CodePhrase) any {
				ec := codedContext()
				ec.Setting = codedText(c)
				return ec
			},
			path: "/setting/defining_code", valid: openEHRCode("238"), breach: openEHRCode("9999"),
		},
		{
			name: "PARTICIPATION function, as the root", invariant: "Function_valid", group: terminology.ParticipationFunction,
			root: func(c rm.CodePhrase) any {
				return &rm.Participation{Function: codedText(c), Performer: rm.PartySelf{}}
			},
			path: "/function/defining_code", valid: openEHRCode("253"), breach: openEHRCode("999"),
		},
		{
			name: "PARTY_RELATED relationship, as the root", invariant: "Relationship_valid", group: terminology.SubjectRelationship,
			root: func(c rm.CodePhrase) any { return &rm.PartyRelated{Relationship: codedText(c)} },
			path: "/relationship/defining_code", valid: openEHRCode("10"), breach: openEHRCode("9999"),
		},
		{
			name: "TERM_MAPPING purpose, as the root", invariant: "Purpose_valid", group: terminology.TermMappingPurpose,
			root: func(c rm.CodePhrase) any {
				purpose := codedText(c)
				return &rm.TermMapping{Match: "=", Target: phrase("SNOMED-CT", "123"), Purpose: &purpose}
			},
			path: "/purpose/defining_code", valid: openEHRCode("669"), breach: openEHRCode("9999"),
		},
	}
}

// bothForms returns v by pointer and by value, whichever of the two it is.
// The walk hands the coded pass either form: an interface slot usually holds
// a pointer, a value-typed attribute such as a DV_INTERVAL bound a value.
func bothForms(v any) (ptr, val any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return v, rv.Elem().Interface()
	}
	p := reflect.New(rv.Type())
	p.Elem().Set(rv)
	return p.Interface(), v
}

// TestREQ112_CodedInvariantsTable checks every row of the Coded invariants
// table, with the root by pointer and by value: a member of the row's group
// or code set gives no `code_not_in_value_set`, and a code outside it gives
// exactly one, at the CODE_PHRASE the row reads. The diagnostic names the
// invariant and the group or code set, and carries neither the caller's code
// nor its terminology id.
func TestREQ112_CodedInvariantsTable(t *testing.T) {
	for _, row := range append(codedRows(), rootRows()...) {
		t.Run(row.name, func(t *testing.T) {
			validPtr, validVal := bothForms(row.root(row.valid))
			breachPtr, breachVal := bothForms(row.root(row.breach))
			for _, form := range []struct {
				name          string
				valid, breach any
			}{
				{"by pointer", validPtr, breachPtr},
				{"by value", validVal, breachVal},
			} {
				if got := codedFindings(validation.ValidateRM(form.valid).Issues); len(got) != 0 {
					t.Errorf("ValidateRM(%s %s::%s, %s) code_not_in_value_set at %q, want none",
						row.name, row.valid.TerminologyID.Value, row.valid.CodeString, form.name, got)
				}
				r := validation.ValidateRM(form.breach)
				if got, want := codedFindings(r.Issues), []string{row.path}; !slices.Equal(got, want) {
					t.Errorf("ValidateRM(%s %s::%s, %s) code_not_in_value_set at %q, want %q; issues=%+v",
						row.name, row.breach.TerminologyID.Value, row.breach.CodeString, form.name, got, want, r.Issues)
					continue
				}
				assertCodedDetail(t, row, r.Issues, row.breach)
			}
		})
	}
}

// TestREQ112_CodedInvariantsValueFree breaks every row with a code and a
// terminology id no diagnostic could name, and checks that neither reaches
// the Detail: a group breaks on the terminology id, a code set on the code.
func TestREQ112_CodedInvariantsValueFree(t *testing.T) {
	sentinel := phrase("caller-terminology-7f3a", "caller-code-7f3a")
	for _, row := range codedRows() {
		t.Run(row.name, func(t *testing.T) {
			r := validation.ValidateRM(row.root(sentinel))
			if got, want := codedFindings(r.Issues), []string{row.path}; !slices.Equal(got, want) {
				t.Fatalf("ValidateRM(%s with a sentinel code) code_not_in_value_set at %q, want %q", row.name, got, want)
			}
			assertCodedDetail(t, row, r.Issues, sentinel)
		})
	}
}

// TestREQ112_CodedInvariantsEmptyCode breaks every row with an empty
// code_string, once with no terminology id either (an attribute the source
// left out, as a value-typed mandatory one decodes) and once in the row's own
// terminology: an empty code_string is no member, so each gives exactly one
// `code_not_in_value_set`, at the CODE_PHRASE the row reads.
func TestREQ112_CodedInvariantsEmptyCode(t *testing.T) {
	for _, row := range append(codedRows(), rootRows()...) {
		t.Run(row.name, func(t *testing.T) {
			for _, empty := range []rm.CodePhrase{{}, phrase(row.valid.TerminologyID.Value, "")} {
				r := validation.ValidateRM(row.root(empty))
				if got, want := codedFindings(r.Issues), []string{row.path}; !slices.Equal(got, want) {
					t.Errorf("ValidateRM(%s %q::\"\") code_not_in_value_set at %q, want %q; issues=%+v",
						row.name, empty.TerminologyID.Value, got, want, r.Issues)
				}
			}
		})
	}
}

// assertCodedDetail checks the Detail of the row's finding: it names the
// invariant, the group or code set and, for an ISO or IANA code set, its
// external id, and it carries neither the code string nor the terminology id
// of code, unless that id is the one the rule itself names.
func assertCodedDetail(t *testing.T, row codedRow, issues []validation.Issue, code rm.CodePhrase) {
	t.Helper()
	i := slices.IndexFunc(issues, func(i validation.Issue) bool { return i.Code == codeNotInValueSet })
	if i < 0 {
		t.Fatalf("%s: no %s issue in %+v", row.name, codeNotInValueSet, issues)
	}
	detail := issues[i].Detail
	want := []string{row.invariant, strconv.Quote(row.valueSetName())}
	if row.codeSet != nil && row.codeSet.Issuer() != "openehr" {
		want = append(want, row.codeSet.ExternalID())
	}
	for _, w := range want {
		if !strings.Contains(detail, w) {
			t.Errorf("%s: Detail %q does not name %q", row.name, detail, w)
		}
	}
	if strings.Contains(detail, code.CodeString) {
		t.Errorf("%s: Detail %q carries the code string %q", row.name, detail, code.CodeString)
	}
	if tid := code.TerminologyID.Value; tid != row.namedID() && strings.Contains(detail, tid) {
		t.Errorf("%s: Detail %q carries the terminology id %q", row.name, detail, tid)
	}
	if issues[i].Severity != validation.Error {
		t.Errorf("%s: Severity = %v, want Error", row.name, issues[i].Severity)
	}
}

// TestREQ112_CodedInvariantsAcceptance pins the findings the requirement
// names, on one COMPOSITION: a setting outside the setting group, an ELEMENT
// with no value and a null_flavour outside the null flavours, a language and
// a territory outside their code sets, and an ENTRY encoding outside the
// character sets.
func TestREQ112_CodedInvariantsAcceptance(t *testing.T) {
	ev := codedEvaluation()
	ev.Encoding = phrase("IANA_character-sets", "UTF-99")
	nf := codedText(openEHRCode("999"))
	ev.Data = &rm.ItemTree{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "tree"},
		Items:           []rm.Item{&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "item"}, NullFlavour: &nf}},
	}
	comp := codedComposition()
	comp.Language = phrase("ISO_639-1", "xx")
	comp.Territory = phrase("ISO_3166-1", "ZZ")
	comp.Context = codedContext()
	comp.Context.Setting = codedText(openEHRCode("9999"))
	comp.Content = []rm.ContentItem{ev}

	got := codedFindings(validation.ValidateRM(comp).Issues)
	want := []string{
		"/content[0]/data/items[0]/null_flavour/defining_code",
		"/content[0]/encoding",
		"/context/setting/defining_code",
		"/language",
		"/territory",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ValidateRM(COMPOSITION) code_not_in_value_set at %q, want %q", got, want)
	}
}

// TestREQ112_CodedGroupNeedsOpenEHRTerminology checks that a group invariant
// holds only for a code of the openEHR terminology: a setting member coded
// in any other terminology breaks Setting_valid.
func TestREQ112_CodedGroupNeedsOpenEHRTerminology(t *testing.T) {
	cases := []struct {
		name string
		code rm.CodePhrase
		want []string
	}{
		{name: "238 in openehr", code: openEHRCode("238")},
		{name: "238 in local", code: phrase("local", "238"), want: []string{"/context/setting/defining_code"}},
		{name: "238 in openEHR (other letter case)", code: phrase("openEHR", "238"), want: []string{"/context/setting/defining_code"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := codedComposition()
			comp.Context = codedContext()
			comp.Context.Setting = codedText(tc.code)
			if got := codedFindings(validation.ValidateRM(comp).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(setting %s::%s) code_not_in_value_set at %q, want %q",
					tc.code.TerminologyID.Value, tc.code.CodeString, got, tc.want)
			}
		})
	}
}

// TestREQ112_CodeSetIgnoresTerminologyID checks that a code-set invariant
// reads the bare code: a member coded under another terminology id holds.
func TestREQ112_CodeSetIgnoresTerminologyID(t *testing.T) {
	comp := codedComposition()
	comp.Language = phrase("ISO_639-2", "en")
	if got := codedFindings(validation.ValidateRM(comp).Issues); len(got) != 0 {
		t.Errorf("ValidateRM(COMPOSITION language ISO_639-2::en) code_not_in_value_set at %q, want none", got)
	}
	ev := codedEvaluation()
	ev.Encoding = phrase("Unicode", "UTF-8")
	if got := codedFindings(validation.ValidateRM(ev).Issues); len(got) != 0 {
		t.Errorf("ValidateRM(EVALUATION encoding Unicode::UTF-8) code_not_in_value_set at %q, want none", got)
	}
}

// TestREQ112_CodeSetLetterCase checks the accessor's case rule as the floor
// applies it: the ISO and IANA sets ignore letter case, the openEHR-issued
// sets do not, and an alias the pinned set leaves out is no member.
func TestREQ112_CodeSetLetterCase(t *testing.T) {
	cases := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "ENTRY encoding utf-8",
			root: func() any { ev := codedEvaluation(); ev.Encoding = phrase("IANA_character-sets", "utf-8"); return ev }(),
		},
		{
			name: "ENTRY encoding utf8, no member in any letter case",
			root: func() any { ev := codedEvaluation(); ev.Encoding = phrase("IANA_character-sets", "utf8"); return ev }(),
			want: []string{"/encoding"},
		},
		{
			name: "COMPOSITION language EN",
			root: func() any { c := codedComposition(); c.Language = phrase("ISO_639-1", "EN"); return c }(),
		},
		{
			name: "COMPOSITION territory nl",
			root: func() any { c := codedComposition(); c.Territory = phrase("ISO_3166-1", "nl"); return c }(),
		},
		{
			name: "DV_QUANTITY normal_status h",
			root: func() any {
				ns := phrase("openehr_normal_statuses", "h")
				return &rm.DVQuantity{Magnitude: 1, Units: "mg", NormalStatus: &ns}
			}(),
			want: []string{"/normal_status"},
		},
		{
			name: "DV_MULTIMEDIA charset ISO-8859-1",
			root: func() any {
				mm := codedMultimedia()
				cs := phrase("IANA_character-sets", "ISO-8859-1")
				mm.Charset = &cs
				return mm
			}(),
			want: []string{"/charset"},
		},
		{
			name: "DV_MULTIMEDIA charset ISO_8859-1:1987",
			root: func() any {
				mm := codedMultimedia()
				cs := phrase("IANA_character-sets", "ISO_8859-1:1987")
				mm.Charset = &cs
				return mm
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codedFindings(validation.ValidateRM(tc.root).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) code_not_in_value_set at %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestREQ112_CodedInvariantGuards pins when an invariant applies: an optional
// attribute only when present, ELEMENT's null_flavour only when the ELEMENT
// has no value, PARTICIPATION's function only when it is a DV_CODED_TEXT, a
// PARTICIPATION's performer only when it is a PARTY_RELATED, and a mandatory
// value-typed attribute always, empty or not, a PARTY_RELATED's relationship
// included. Each row lists every finding the root gives, by code and path.
func TestREQ112_CodedInvariantGuards(t *testing.T) {
	badCode := codedText(openEHRCode("999"))
	nurse := rm.DVText{Value: "nurse"}
	elementWithBoth := &rm.Element{
		ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "item"},
		Value: &rm.DVText{Value: "x"}, NullFlavour: &badCode,
	}
	var typedNilValue *rm.DVText
	elementTypedNilValue := &rm.Element{
		ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "item"},
		Value: typedNilValue, NullFlavour: &badCode,
	}
	withParticipations := func(ps ...rm.Participation) any {
		ev := codedEvaluation()
		ev.OtherParticipations = ps
		return ev
	}
	withContextParticipations := func(ps ...rm.Participation) any {
		comp := codedComposition()
		comp.Context = codedContext()
		comp.Context.Participations = ps
		return comp
	}
	emptyLanguage := codedComposition()
	emptyLanguage.Language = rm.CodePhrase{}
	blankLanguage := codedComposition()
	blankLanguage.Language = phrase("ISO_639-1", "")
	noRelationship := codedEvaluation()
	noRelationship.Subject = rm.PartyRelated{}
	noRelationshipCode := codedEvaluation()
	noRelationshipCode.Subject = rm.PartyRelated{Relationship: rm.DVCodedText{Value: "mother"}}
	badRelative := rm.PartyRelated{Relationship: codedText(openEHRCode("9999"))}
	mother := rm.PartyRelated{Relationship: codedText(openEHRCode("10"))}
	performer := func(p rm.PartyProxy) rm.Participation {
		return rm.Participation{Function: nurse, Performer: p}
	}
	carer := "carer"

	cases := []struct {
		name string
		root any
		want []string // "code path", sorted
	}{
		{
			name: "ELEMENT with a value and a null_flavour",
			root: elementWithBoth,
			want: []string{"rm_invariant /"},
		},
		{
			name: "ELEMENT with a typed-nil value and a null_flavour",
			root: elementTypedNilValue,
			want: []string{"code_not_in_value_set /null_flavour/defining_code"},
		},
		{
			name: "ELEMENT with neither value nor null_flavour",
			root: &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "item"}},
			want: []string{"rm_invariant /"},
		},
		{
			name: "PARTICIPATION function as a plain DV_TEXT",
			root: withParticipations(rm.Participation{Function: rm.DVText{Value: "999"}, Performer: rm.PartySelf{}}),
		},
		{
			name: "PARTICIPATION function as a DV_CODED_TEXT outside the group",
			root: withParticipations(rm.Participation{Function: badCode, Performer: rm.PartySelf{}}),
			want: []string{"code_not_in_value_set /other_participations[0]/function/defining_code"},
		},
		{
			name: "PARTICIPATION function as a pointer to a DV_CODED_TEXT outside the group",
			root: withParticipations(rm.Participation{Function: &badCode, Performer: rm.PartySelf{}}),
			want: []string{"code_not_in_value_set /other_participations[0]/function/defining_code"},
		},
		{
			name: "PARTICIPATION with no function",
			root: withParticipations(rm.Participation{Performer: rm.PartySelf{}}),
		},
		{
			name: "PARTICIPATION mode absent",
			root: withParticipations(rm.Participation{Function: nurse, Performer: rm.PartySelf{}}),
		},
		{
			name: "PARTICIPATION mode outside the group, second participation",
			root: withParticipations(
				rm.Participation{Function: nurse, Performer: rm.PartySelf{}},
				rm.Participation{Function: nurse, Mode: &badCode, Performer: rm.PartySelf{}},
			),
			want: []string{"code_not_in_value_set /other_participations[1]/mode/defining_code"},
		},
		{
			name: "EVENT_CONTEXT participation function and mode outside their groups",
			root: withContextParticipations(rm.Participation{Function: badCode, Mode: &badCode, Performer: rm.PartySelf{}}),
			want: []string{
				"code_not_in_value_set /context/participations[0]/function/defining_code",
				"code_not_in_value_set /context/participations[0]/mode/defining_code",
			},
		},
		{
			name: "EVENT_CONTEXT participation with a plain DV_TEXT function and no mode",
			root: withContextParticipations(rm.Participation{Function: nurse, Performer: rm.PartySelf{}}),
		},
		{
			name: "PARTICIPATION root",
			root: &rm.Participation{Function: badCode, Mode: &badCode, Performer: rm.PartySelf{}},
			want: []string{
				"code_not_in_value_set /function/defining_code",
				"code_not_in_value_set /mode/defining_code",
			},
		},
		{
			name: "PARTICIPATION root by value",
			root: rm.Participation{Function: badCode, Performer: rm.PartySelf{}},
			want: []string{"code_not_in_value_set /function/defining_code"},
		},
		{
			name: "ENTRY participation performer, a PARTY_RELATED outside the group",
			root: withParticipations(performer(badRelative)),
			want: []string{"code_not_in_value_set /other_participations[0]/performer/relationship/defining_code"},
		},
		{
			name: "ENTRY participation performer, a pointer to a PARTY_RELATED outside the group",
			root: withParticipations(performer(&badRelative)),
			want: []string{"code_not_in_value_set /other_participations[0]/performer/relationship/defining_code"},
		},
		{
			name: "ENTRY participation performer, a PARTY_RELATED in the group",
			root: withParticipations(performer(mother)),
		},
		{
			name: "ENTRY participation performer, a PARTY_RELATED with no relationship",
			root: withParticipations(performer(rm.PartyRelated{})),
			want: []string{"code_not_in_value_set /other_participations[0]/performer/relationship/defining_code"},
		},
		{
			name: "EVENT_CONTEXT participation performer, a PARTY_RELATED outside the group",
			root: withContextParticipations(performer(badRelative)),
			want: []string{"code_not_in_value_set /context/participations[0]/performer/relationship/defining_code"},
		},
		{
			name: "PARTICIPATION root performer, a PARTY_RELATED outside the group",
			root: &rm.Participation{Function: nurse, Performer: &badRelative},
			want: []string{"code_not_in_value_set /performer/relationship/defining_code"},
		},
		{
			name: "PARTICIPATION root by value, performer a PARTY_RELATED outside the group",
			root: rm.Participation{Function: nurse, Performer: badRelative},
			want: []string{"code_not_in_value_set /performer/relationship/defining_code"},
		},
		{
			name: "ENTRY participation performer, a PARTY_SELF",
			root: withParticipations(performer(rm.PartySelf{})),
		},
		{
			name: "ENTRY participation performer, a PARTY_IDENTIFIED",
			root: withParticipations(performer(&rm.PartyIdentified{Name: &carer})),
		},
		{
			name: "PARTICIPATION root performer, a PARTY_IDENTIFIED",
			root: &rm.Participation{Function: nurse, Performer: rm.PartyIdentified{Name: &carer}},
		},
		{
			name: "PARTY_RELATED with no relationship",
			root: noRelationship,
			want: []string{"code_not_in_value_set /subject/relationship/defining_code"},
		},
		{
			name: "PARTY_RELATED relationship with a value and no code",
			root: noRelationshipCode,
			want: []string{"code_not_in_value_set /subject/relationship/defining_code"},
		},
		{
			name: "COMPOSITION language left out",
			root: emptyLanguage,
			want: []string{"code_not_in_value_set /language", "required /language"},
		},
		{
			name: "COMPOSITION language with an empty code_string",
			root: blankLanguage,
			want: []string{"code_not_in_value_set /language", "required /language/code_string", "rm_invariant /language"},
		},
		{
			name: "DV_TEXT without language or encoding",
			root: &rm.DVText{Value: "x"},
		},
		{
			name: "DV_QUANTITY without normal_status",
			root: &rm.DVQuantity{Magnitude: 1, Units: "mg"},
		},
		{
			name: "DV_MULTIMEDIA without optional coded attributes",
			root: codedMultimedia(),
		},
		{
			name: "DV_PARSABLE without optional coded attributes",
			root: &rm.DVParsable{Value: "x", Formalism: "text"},
		},
		{
			name: "ISM_TRANSITION without transition",
			root: &rm.IsmTransition{CurrentState: codedText(openEHRCode("532"))},
		},
		{
			name: "TERM_MAPPING without purpose",
			root: &rm.TermMapping{Match: "=", Target: phrase("SNOMED-CT", "123")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			var got []string
			for _, i := range r.Issues {
				got = append(got, i.Code+" "+i.Path)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues (code path) = %q, want %q; issues=%+v", tc.name, got, tc.want, r.Issues)
			}
		})
	}
}

// TestREQ112_CodedIntervalEventInstantiations checks Math_function_validity on
// every INTERVAL_EVENT instantiation the floor recognises, as a root.
func TestREQ112_CodedIntervalEventInstantiations(t *testing.T) {
	mf := codedText(openEHRCode("9999"))
	for _, root := range []any{
		&rm.IntervalEvent[rm.ItemStructure]{MathFunction: mf},
		rm.IntervalEvent[rm.ItemStructure]{MathFunction: mf},
		&rm.IntervalEvent[rm.ItemList]{MathFunction: mf},
		rm.IntervalEvent[rm.ItemList]{MathFunction: mf},
		&rm.IntervalEvent[rm.ItemSingle]{MathFunction: mf},
		rm.IntervalEvent[rm.ItemSingle]{MathFunction: mf},
		&rm.IntervalEvent[rm.ItemTable]{MathFunction: mf},
		rm.IntervalEvent[rm.ItemTable]{MathFunction: mf},
		&rm.IntervalEvent[rm.ItemTree]{MathFunction: mf},
		rm.IntervalEvent[rm.ItemTree]{MathFunction: mf},
	} {
		want := []string{"/math_function/defining_code"}
		if got := codedFindings(validation.ValidateRM(root).Issues); !slices.Equal(got, want) {
			t.Errorf("ValidateRM(%T with math_function outside the group) code_not_in_value_set at %q, want %q", root, got, want)
		}
	}
}

// codedJSONPhrase is a canonical-JSON CODE_PHRASE.
func codedJSONPhrase(tid, code string) string {
	return `{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"` + tid + `"},"code_string":"` + code + `"}`
}

// assertCodedBothForms validates v by pointer and by value and checks that
// each gives `code_not_in_value_set` exactly at want.
func assertCodedBothForms(t *testing.T, label string, v any, want []string) {
	t.Helper()
	ptr, val := bothForms(v)
	for _, form := range []struct {
		name string
		root any
	}{{"by pointer", ptr}, {"by value", val}} {
		if got := codedFindings(validation.ValidateRM(form.root).Issues); !slices.Equal(got, want) {
			t.Errorf("ValidateRM(%s, %s) code_not_in_value_set at %q, want %q", label, form.name, got, want)
		}
	}
}

// TestREQ112_CodedInvariantsEveryEntry checks Language_valid and
// Encoding_valid on every registered ENTRY concrete, by pointer and by value.
// A concrete added to the registry that the floor's coded pass does not read,
// in either form, fails here.
func TestREQ112_CodedInvariantsEveryEntry(t *testing.T) {
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.Entry); !ok {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			body := `{"_type":"` + name + `",` +
				`"language":` + codedJSONPhrase("ISO_639-1", "xx") + `,` +
				`"encoding":` + codedJSONPhrase("IANA_character-sets", "UTF-99") + `}`
			v, err := typereg.Default.Decode([]byte(body))
			if err != nil {
				t.Fatalf("typereg.Decode(%s with a language and an encoding): %v", name, err)
			}
			assertCodedBothForms(t, name, v, []string{"/encoding", "/language"})
		})
	}
	if checked == 0 {
		t.Fatal("the registry yields no ENTRY concretes; registrations missing?")
	}
}

// TestREQ112_CodedInvariantsEveryOrdered checks Normal_status_validity on
// every registered DV_ORDERED concrete, by pointer and by value: the walk
// meets a DV_INTERVAL's bounds by value. A concrete added to the registry
// that the floor's coded pass does not read, in either form, fails here.
func TestREQ112_CodedInvariantsEveryOrdered(t *testing.T) {
	var checked int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); !ok {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				code string
				want []string
			}{
				{code: "N"},
				{code: "HHHH", want: []string{"/normal_status"}},
			} {
				body := `{"_type":"` + name + `","normal_status":` + codedJSONPhrase("openehr_normal_statuses", tc.code) + `}`
				v, err := typereg.Default.Decode([]byte(body))
				if err != nil {
					t.Fatalf("typereg.Decode(%s with normal_status %s): %v", name, tc.code, err)
				}
				assertCodedBothForms(t, name+" normal_status "+tc.code, v, tc.want)
			}
		})
	}
	if checked == 0 {
		t.Fatal("the registry yields no DV_ORDERED concretes; registrations missing?")
	}
}

// TestREQ112_CodedIntervalBound checks Normal_status_validity on an interval
// bound, in the shapes the walk meets one. DV_QUANTITY, DV_COUNT and
// DV_PROPORTION type their normal_range, so its bounds are values; the
// temporal types' normal_range is a bare DV_INTERVAL<DV_ORDERED>, whose
// decoded bounds are pointers; and a typed interval such as the
// DV_INTERVAL<DV_DATE> the generator writes holds its bounds by value.
func TestREQ112_CodedIntervalBound(t *testing.T) {
	const bad = `"normal_status":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr_normal_statuses"},"code_string":"HHHH"}`
	interval := func(lower, upper string) string {
		return `{"_type":"DV_INTERVAL","lower":` + lower + `,"upper":` + upper + `,` +
			`"lower_included":true,"upper_included":true,"lower_unbounded":false,"upper_unbounded":false}`
	}
	decodedElement := func(t *testing.T, value string) any {
		t.Helper()
		body := `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"item"},"value":` + value + `}`
		v, err := typereg.Default.Decode([]byte(body))
		if err != nil {
			t.Fatalf("typereg.Decode(ELEMENT %s): %v", body, err)
		}
		return v
	}
	badStatus := phrase("openehr_normal_statuses", "HHHH")
	cases := []struct {
		name string
		root func(t *testing.T) any
		want []string
	}{
		{
			name: "decoded DV_QUANTITY normal_range, lower bound by value",
			root: func(t *testing.T) any {
				return decodedElement(t, `{"_type":"DV_QUANTITY","magnitude":5,"units":"mg","normal_range":`+interval(
					`{"_type":"DV_QUANTITY","magnitude":1,"units":"mg",`+bad+`}`,
					`{"_type":"DV_QUANTITY","magnitude":9,"units":"mg"}`)+`}`)
			},
			want: []string{"/value/normal_range/lower/normal_status"},
		},
		{
			name: "decoded DV_COUNT normal_range, upper bound by value",
			root: func(t *testing.T) any {
				return decodedElement(t, `{"_type":"DV_COUNT","magnitude":5,"normal_range":`+interval(
					`{"_type":"DV_COUNT","magnitude":1}`,
					`{"_type":"DV_COUNT","magnitude":9,`+bad+`}`)+`}`)
			},
			want: []string{"/value/normal_range/upper/normal_status"},
		},
		{
			name: "decoded DV_DATE_TIME normal_range, upper bound by pointer",
			root: func(t *testing.T) any {
				return decodedElement(t, `{"_type":"DV_DATE_TIME","value":"2026-10-01T10:00:00Z","normal_range":`+interval(
					`{"_type":"DV_DATE_TIME","value":"2026-10-01T09:00:00Z"}`,
					`{"_type":"DV_DATE_TIME","value":"2026-10-01T11:00:00Z",`+bad+`}`)+`}`)
			},
			want: []string{"/value/normal_range/upper/normal_status"},
		},
		{
			name: "DV_INTERVAL<DV_DATE> as an ELEMENT value, lower bound by value",
			root: func(*testing.T) any {
				return &rm.Element{
					ArchetypeNodeID: "at0001",
					Name:            rm.DVText{Value: "item"},
					Value: &rm.DVInterval[rm.DVDate]{
						Lower: rm.DVDate{Value: "2026-10-01", NormalStatus: &badStatus}, LowerIncluded: true,
						Upper: rm.DVDate{Value: "2026-10-02"}, UpperIncluded: true,
					},
				}
			},
			want: []string{"/value/lower/normal_status"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codedFindings(validation.ValidateRM(tc.root(t)).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(ELEMENT, %s outside the normal statuses) code_not_in_value_set at %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestREQ112_CodedInvariantsOutOfScope pins the coded invariants the entry
// leaves out: an AUDIT_DETAILS change_type or an ATTESTATION reason outside
// its group gives no `code_not_in_value_set`, even as the root.
func TestREQ112_CodedInvariantsOutOfScope(t *testing.T) {
	bad := codedText(openEHRCode("9999"))
	for _, root := range []any{
		&rm.AuditDetails{
			SystemID:      "example.org",
			Committer:     rm.PartySelf{},
			TimeCommitted: rm.DVDateTime{Value: "2026-10-01T10:00:00Z"},
			ChangeType:    bad,
		},
		&rm.Attestation{SystemID: "example.org", Committer: rm.PartySelf{}, ChangeType: bad, Reason: bad},
	} {
		if got := codedFindings(validation.ValidateRM(root).Issues); len(got) != 0 {
			t.Errorf("ValidateRM(%T with a code outside its group) code_not_in_value_set at %q, want none", root, got)
		}
	}
}
