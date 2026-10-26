package instance_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// archetypeTerms maps an archetype id to the texts of the terms the OPT
// defines for each code in that archetype: in every archetype root of
// that id, which an OPT can hold more than once.
type archetypeTerms map[string]map[string][]string

// templateTerms returns the term texts each archetype root of an OPT
// defines, by archetype id: the definition and every C_ARCHETYPE_ROOT.
// A term belongs to the root whose element holds its term_definitions.
func templateTerms(t *testing.T, raw []byte) archetypeTerms {
	t.Helper()
	type root struct {
		id    string
		terms map[string][]string
	}
	// stack holds one entry per open element: the root it opens, or nil.
	var stack []*root
	out := make(archetypeTerms)
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("read term definitions: %v", err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			var parent *root
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			switch {
			case parent != nil && tok.Name.Local == "archetype_id":
				var id struct {
					Value string `xml:"value"`
				}
				if err := dec.DecodeElement(&id, &tok); err != nil {
					t.Fatalf("decode archetype id: %v", err)
				}
				parent.id = strings.TrimSpace(id.Value)
			case parent != nil && tok.Name.Local == "term_definitions":
				var def struct {
					Code  string `xml:"code,attr"`
					Items []struct {
						ID   string `xml:"id,attr"`
						Text string `xml:",chardata"`
					} `xml:"items"`
				}
				if err := dec.DecodeElement(&def, &tok); err != nil {
					t.Fatalf("decode term definition: %v", err)
				}
				for _, item := range def.Items {
					if item.ID == "text" {
						code := strings.TrimSpace(def.Code)
						parent.terms[code] = append(parent.terms[code], strings.TrimSpace(item.Text))
					}
				}
			case tok.Name.Local == "definition" || xsiType(tok) == "C_ARCHETYPE_ROOT":
				stack = append(stack, &root{terms: make(map[string][]string)})
			default:
				stack = append(stack, nil)
			}
		case xml.EndElement:
			closed := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if closed == nil {
				continue
			}
			byCode := out[closed.id]
			if byCode == nil {
				byCode = make(map[string][]string)
				out[closed.id] = byCode
			}
			for code, texts := range closed.terms {
				byCode[code] = append(byCode[code], texts...)
			}
		}
	}
}

// xsiType is the xsi:type attribute of an element, or "".
func xsiType(e xml.StartElement) string {
	for _, a := range e.Attr {
		if a.Name.Local == "type" {
			return a.Value
		}
	}
	return ""
}

// localCode reports whether a coded text's defining code is a local code
// that is not the placeholder at0000.
func localCode(c rm.DVCodedText) bool {
	code := c.DefiningCode.CodeString
	return c.DefiningCode.TerminologyID.Value == "local" && code != "" && code != "at0000"
}

// TestREQ107_CorpusLocalCodedTextsTakeTheTermText is the REQ-107 RM
// defaults check, over every vendored template, that each DV_CODED_TEXT
// the generator writes in terminology local, wherever it sits (an
// ELEMENT's value, an ordinal's symbol, a locatable's name, an
// ISM_TRANSITION's careflow step, …), takes the text of the term its own
// archetype, the nearest enclosing archetype root, defines for its code
// rather than the open-string example or the code itself, and that a code
// its own archetype defines no term for takes no other archetype's term
// text for that code, under every policy, both value fills and several
// fixed seeds. One vendored OPT limits such a text with a C_STRING that
// rejects a listed code's term: sdk_crosscheck_values.v1 pins the coded
// name of at0016 to its second code's term, so the generator writes that
// code.
func TestREQ107_CorpusLocalCodedTextsTakeTheTermText(t *testing.T) {
	checked, foreign := 0, 0
	terms := make(map[string]archetypeTerms)
	eachCorpusRun(t, func(run corpusRun) {
		byArchetype, ok := terms[run.name]
		if !ok {
			byArchetype = templateTerms(t, run.raw)
			terms[run.name] = byArchetype
		}
		visitRMIn(run.out, func(path, archetype string, c rm.DVCodedText) {
			if !localCode(c) || archetype == "" {
				return
			}
			code := c.DefiningCode.CodeString
			if texts, ok := byArchetype[archetype][code]; ok {
				checked++
				if !slices.Contains(texts, c.Value) {
					t.Errorf("%s: DV_CODED_TEXT local::%s at %s has text %q, want the term text %s defines, one of %q",
						run, code, path, c.Value, archetype, texts)
				}
				return
			}
			for other, byCode := range byArchetype {
				if texts := byCode[code]; other != archetype && slices.Contains(texts, c.Value) {
					foreign++
					t.Errorf("%s: DV_CODED_TEXT local::%s at %s in %s, which defines no such term, has the text %q that %s defines",
						run, code, path, archetype, c.Value, other)
				}
			}
		})
	})
	if checked == 0 {
		t.Fatal("no local DV_CODED_TEXT with a term of its own archetype generated, so nothing was checked")
	}
	t.Logf("%d local coded texts checked against their own archetype's term, %d took another archetype's", checked, foreign)
}

// TestREQ107_BMIMethodTakesTheTermText is the REQ-107 RM defaults check on
// the coded text issue 292 names: BMI.opt's method element, coded
// local::at0007 or local::at0008, takes the text the OPT's terminology
// gives the code, Automatic entry or Direct entry, not the open-string
// example.
func TestREQ107_BMIMethodTakesTheTermText(t *testing.T) {
	want := map[string]string{"at0007": "Automatic entry", "at0008": "Direct entry"}
	c := compileFixture(t, "BMI")
	found := 0
	for _, policy := range everyPolicy {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			for seed := range uint64(8) {
				opts := instance.Options{
					Policy: policy, ValueFill: fill, ValueSource: mrand.NewPCG(seed, corpusStream),
					Territory: "NL", Composer: testComposer(), Now: defaultsNow,
				}
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%v/%v/seed=%d: Generate: %v", policy, fill, seed, err)
				}
				visitRM(out, func(path string, ct rm.DVCodedText) {
					text, ok := want[ct.DefiningCode.CodeString]
					if !ok || ct.DefiningCode.TerminologyID.Value != "local" {
						return
					}
					found++
					if ct.Value != text {
						t.Errorf("%v/%v/seed=%d: local::%s at %s has text %q, want %q",
							policy, fill, seed, ct.DefiningCode.CodeString, path, ct.Value, text)
					}
				})
			}
		}
	}
	if found == 0 {
		t.Fatal("BMI generated no method coded local::at0007 or at0008")
	}
}

// Term texts the synthetic OPTs below define, in their one archetype.
const (
	termOne   = "First term"
	termTwo   = "Second term"
	termThree = "Third term"
	otherText = "Text the OPT lists"
)

// optTerms are the term definitions of the synthetic archetype: at0001 to
// at0003, each with a text and a description. at0099 has none.
var optTerms = func() string {
	var b strings.Builder
	for i, text := range []string{termOne, termTwo, termThree} {
		fmt.Fprintf(&b, `<term_definitions code="at%04d"><items id="text">%s</items><items id="description">*</items></term_definitions>`, i+1, text)
	}
	return b.String()
}()

// optOrdinal is a C_DV_ORDINAL that lists value 0 for local::at0001 and 1
// for local::at0002.
const optOrdinal = `<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id>` +
	`<list><value>0</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol></list>` +
	`<list><value>1</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0002</code_string></defining_code></symbol></list>` +
	`</children>`

// TestREQ107_LocalCodedTextTakesTheTermText is the REQ-107 RM defaults
// check on each place the generator writes a DV_CODED_TEXT whose code the
// OPT gives in terminology local (an ELEMENT's value, a DV_ORDINAL's
// symbol, a locatable's name, an ISM_TRANSITION's careflow step): the
// coded text takes the text of the term its own archetype, the nearest
// enclosing archetype root, defines for the code, in Options.Language as
// REQ-111 resolves it (a language the OPT has no translation for gets the
// document language's term). Where the OPT limits the text with a C_STRING
// that rejects the term text, the text the walk wrote stays (Precedence
// 1), and a code its own archetype defines no term for, even where an
// enclosing archetype does, the placeholder at0000 and a code of another
// terminology keep the walk's text. It holds under every policy and both
// value fills.
func TestREQ107_LocalCodedTextTakesTheTermText(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		coded func(t *testing.T, out any) rm.DVCodedText
		want  []string
	}{
		{
			name:  "ELEMENT value",
			opt:   optTemplate("ELEMENT", optTerms, optSingle("value", optCodedText("local", "at0001"))),
			coded: elementCodedText,
			want:  []string{termOne},
		},
		{
			name:  "ELEMENT value over two codes",
			opt:   optTemplate("ELEMENT", optTerms, optSingle("value", optCodedText("local", "at0001", "at0002"))),
			coded: elementCodedText,
			want:  []string{termOne, termTwo},
		},
		{
			name: "ELEMENT value whose C_STRING lists the term text",
			opt: optTemplate("ELEMENT", optTerms, optSingle("value", optNode("DV_CODED_TEXT", "",
				optStringAttr("value", "<list>"+otherText+"</list><list>"+termOne+"</list>"),
				optSingle("defining_code", optCodePhrase("local", "at0001"))))),
			coded: elementCodedText,
			want:  []string{termOne},
		},
		{
			name: "ELEMENT value whose C_STRING rejects the term text",
			opt: optTemplate("ELEMENT", optTerms, optSingle("value", optNode("DV_CODED_TEXT", "",
				optStringAttr("value", "<list>"+otherText+"</list>"),
				optSingle("defining_code", optCodePhrase("local", "at0001"))))),
			coded: elementCodedText,
			want:  []string{otherText},
		},
		{
			name: "ELEMENT value a C_CODE_PHRASE constrains as a whole",
			opt: optTemplate("ELEMENT", optTerms, optSingle("value",
				`<children xsi:type="C_CODE_PHRASE"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id>`+
					`<terminology_id><value>local</value></terminology_id><code_list>at0002</code_list></children>`)),
			coded: elementCodedText,
			want:  []string{termTwo},
		},
		{
			// An archetype always defines its concept, at0000, but the walk
			// writes at0000 as a placeholder where the OPT lists no code.
			name: "ELEMENT value with the placeholder code",
			opt: optTemplate("ELEMENT", optTerms, `<term_definitions code="at0000"><items id="text">Concept</items></term_definitions>`,
				optSingle("value", optCodedText("local"))),
			coded: elementCodedText,
			want:  []string{walkText},
		},
		{
			// A code of another terminology that reads like an at-code
			// still names no term of the archetype.
			name:  "ELEMENT value coded in another terminology",
			opt:   optTemplate("ELEMENT", optTerms, optSingle("value", optCodedText("ICD10", "at0001"))),
			coded: elementCodedText,
			want:  []string{walkText},
		},
		{
			name:  "ELEMENT value whose code has no term",
			opt:   optTemplate("ELEMENT", optTerms, optSingle("value", optCodedText("local", "at0099"))),
			coded: elementCodedText,
			want:  []string{walkText},
		},
		{
			// The coded text's own archetype, the inner CLUSTER, defines
			// no at0005; the enclosing one does, for another concept.
			name:  "ELEMENT value whose code only an enclosing archetype defines",
			opt:   nestedTermOPT(""),
			coded: codedTextWithCode("at0005"),
			want:  []string{walkText},
		},
		{
			name:  "ELEMENT value whose code its own and an enclosing archetype define",
			opt:   nestedTermOPT(innerTerm),
			coded: codedTextWithCode("at0005"),
			want:  []string{innerTerm},
		},
		{
			name:  "DV_ORDINAL symbol",
			opt:   optTemplate("ELEMENT", optTerms, optSingle("value", optOrdinal)),
			coded: func(t *testing.T, out any) rm.DVCodedText { return rootValueAs[*rm.DVOrdinal](t, out).Symbol },
			want:  []string{termOne, termTwo},
		},
		{
			name: "LOCATABLE name",
			opt:  optTemplate("ELEMENT", optTerms, optSingle("name", optCodedText("local", "at0003"))),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				switch name := out.(*rm.Element).Name.(type) {
				case rm.DVCodedText:
					return name
				case *rm.DVCodedText:
					return *name
				default:
					t.Fatalf("ELEMENT name is %T, want a DV_CODED_TEXT", name)
					return rm.DVCodedText{}
				}
			},
			want: []string{termThree},
		},
		{
			name: "ISM_TRANSITION careflow_step",
			opt: optTemplate("ACTION", optTerms, optSingle("language"), optSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "",
					optSingle("current_state", optCodedText("openehr", "524")),
					optSingle("careflow_step", optCodedText("local", "at0002")))),
				optSingle("description", emptyTree)),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				step := out.(*rm.Action).IsmTransition.CareflowStep
				if step == nil {
					t.Fatal("ISM_TRANSITION careflow_step absent")
				}
				return *step
			},
			want: []string{termTwo},
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, language := range []string{"", "en", "nl"} {
			for _, opts := range defaultsOptions() {
				opts.Language = language
				t.Run(fmt.Sprintf("%s/lang=%q/%v/%v", tc.name, language, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got := tc.coded(t, out)
					if !slices.Contains(tc.want, got.Value) {
						t.Errorf("%s = %q %+v, want text one of %q", tc.name, got.Value, got.DefiningCode, tc.want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// Term texts of the nested archetypes nestedTermOPT builds.
const (
	outerTerm = "Outer term"
	innerTerm = "Inner term"
)

// nestedTermOPT is a CLUSTER template whose archetype defines at0005 as
// outerTerm, holding an inner CLUSTER archetype that defines at0001 and,
// where innerAt0005 is not "", at0005 as innerAt0005. The inner archetype
// holds an ELEMENT at0001 whose value is coded local::at0005.
func nestedTermOPT(innerAt0005 string) string {
	outerTerms := `<term_definitions code="at0005"><items id="text">` + outerTerm + `</items></term_definitions>`
	innerTerms := `<term_definitions code="at0001"><items id="text">Inner element</items></term_definitions>`
	if innerAt0005 != "" {
		innerTerms += `<term_definitions code="at0005"><items id="text">` + innerAt0005 + `</items></term_definitions>`
	}
	element := optNode("ELEMENT", "at0001", optSingle("value", optCodedText("local", "at0005")))
	inner := optArchetypeRoot("CLUSTER", "openEHR-EHR-CLUSTER.inner.v1", innerTerms, optMultiple("items", element))
	return optTemplate("CLUSTER", outerTerms, optMultiple("items", inner))
}

// codedTextWithCode returns the one DV_CODED_TEXT coded local::code in a
// generated root.
func codedTextWithCode(code string) func(t *testing.T, out any) rm.DVCodedText {
	return func(t *testing.T, out any) rm.DVCodedText {
		t.Helper()
		var found []rm.DVCodedText
		visitRM(out, func(_ string, c rm.DVCodedText) {
			if c.DefiningCode.TerminologyID.Value == "local" && c.DefiningCode.CodeString == code {
				found = append(found, c)
			}
		})
		if len(found) != 1 {
			t.Fatalf("%d DV_CODED_TEXT coded local::%s generated, want 1", len(found), code)
		}
		return found[0]
	}
}

// elementCodedText is the DV_CODED_TEXT value of a generated ELEMENT root.
func elementCodedText(t *testing.T, out any) rm.DVCodedText {
	t.Helper()
	return *rootValueAs[*rm.DVCodedText](t, out)
}

// rootValueAs is the value of a generated ELEMENT root, as a V.
func rootValueAs[V any](t *testing.T, out any) V {
	t.Helper()
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("root is %T, want *rm.Element", out)
	}
	v, ok := el.Value.(V)
	if !ok {
		t.Fatalf("ELEMENT value is %T, want %T", el.Value, *new(V))
	}
	return v
}

// TestREQ107_OrdinalIntervalBoundsTakeTheTermText is the REQ-107 RM
// defaults check on the symbols of a DV_INTERVAL<DV_ORDINAL>'s bounds,
// local codes the vendored OPT's OBSERVATION archetype defines terms for
// (at0001 History, at0002 Any event): the symbol of each bound takes its
// code's term text, including where the generator replaces an inverted
// pair with the lowest and the highest pair each side's list admits.
func TestREQ107_OrdinalIntervalBoundsTakeTheTermText(t *testing.T) {
	want := map[string]string{"at0001": "History", "at0002": "Any event"}
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	// Lower 0 (at0001) or 5 (at0002), upper 1 (at0001) or 3 (at0002): a
	// lower 5 is put in order as 0 to 3, which neither side's own pair
	// gives the other.
	c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, "DV_ORDINAL", instance.OrdinalBound(0, 5), instance.OrdinalBound(1, 3)))
	pairs := 0
	for seed := range uint64(orderSeeds) {
		opts := intervalOptions(instance.Large)
		opts.ValueFill = instance.RandomFill
		opts.ValueSource = mrand.NewPCG(seed, 1)
		root, _ := generateWithJSON(t, c, opts)
		visitRM(root, func(path string, iv rm.DVInterval[rm.DVOrdinal]) {
			if iv.Lower.Value == 0 && iv.Upper.Value == 3 {
				pairs++
			}
			for _, bound := range []rm.DVOrdinal{iv.Lower, iv.Upper} {
				code := bound.Symbol.DefiningCode.CodeString
				if text := want[code]; bound.Symbol.Value != text {
					t.Errorf("seed %d: ordinal %d at %s has symbol local::%s %q, want %q", seed, bound.Value, path, code, bound.Symbol.Value, text)
				}
			}
		})
	}
	if pairs == 0 {
		t.Fatal("no seed gave the pair 0 to 3, so the replacement was not checked")
	}
}
