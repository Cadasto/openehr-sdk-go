package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// ordinalEntry is one C_DV_ORDINAL list entry: value with a symbol whose
// code is code in terminologyID, with no code where code is "".
func ordinalEntry(value int, terminologyID, code string) string {
	return fmt.Sprintf(`<list><value>%d</value><symbol><value></value><defining_code><terminology_id><value>%s</value></terminology_id>`+
		`<code_string>%s</code_string></defining_code></symbol></list>`, value, terminologyID, code)
}

// optOrdinalOf is a C_DV_ORDINAL that lists entries.
func optOrdinalOf(entries ...string) string {
	return `<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id>` +
		strings.Join(entries, "") + `</children>`
}

// placeholderSymbol is the symbol the generator writes for a listed pair
// with no code: the code at0000 in the pair's terminology, with the text
// example.
func placeholderSymbol(terminologyID string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        "example",
		DefiningCode: rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: terminologyID}},
	}
}

// ordinalNotInList counts the errors among issues that report an ordinal's
// pair as not in its C_DV_ORDINAL's list at a path at accepts, and returns
// every other error, for the test to report.
func ordinalNotInList(issues []validation.Issue, at func(path string) bool) (notInList int, others []validation.Issue) {
	for _, iss := range issues {
		if iss.Severity != validation.Error {
			continue
		}
		if iss.Code == "primitive_not_in_list" && iss.Rule == "not_in_list" && at(iss.Path) {
			notInList++
			continue
		}
		others = append(others, iss)
	}
	return notInList, others
}

// TestREQ107_OrdinalPairWithNoCodeTakesThePlaceholderSymbol is the REQ-107
// Placeholders and Precedence 2 check on a C_DV_ORDINAL that lists a pair
// with an empty code_string: the DV_ORDINAL the generator writes for that
// pair keeps the pair's value and gets the placeholder symbol, the code
// at0000 in the pair's terminology with the text example, since the RM
// needs the symbol's code and text present. The RM floor then finds
// nothing, and the template validator reports the one break REQ-107 §
// Exceptions and known gaps records, the pair not in the list, at the
// ordinal. A pair with a code keeps it, and the template validator finds
// nothing. It holds under every policy, both value fills and both compile
// modes.
func TestREQ107_OrdinalPairWithNoCodeTakesThePlaceholderSymbol(t *testing.T) {
	listed := rm.DVCodedText{
		Value:        "at0002",
		DefiningCode: rm.CodePhrase{CodeString: "at0002", TerminologyID: rm.TerminologyID{Value: "local"}},
	}
	cases := []struct {
		name    string
		entries []string
		// want is the symbol for each value the list holds; noCode marks
		// the value whose pair has no code.
		want   map[rm.Integer]rm.DVCodedText
		noCode rm.Integer
	}{
		{
			name:    "one pair in local",
			entries: []string{ordinalEntry(1, "local", "")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("local")},
			noCode:  1,
		},
		{
			name:    "one pair in another terminology",
			entries: []string{ordinalEntry(1, "SNOMED-CT", "")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("SNOMED-CT")},
			noCode:  1,
		},
		{
			name:    "a pair with no code before one with a code",
			entries: []string{ordinalEntry(1, "local", ""), ordinalEntry(2, "local", "at0002")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("local"), 2: listed},
			noCode:  1,
		},
	}
	for _, tc := range cases {
		xml := optTemplate("ELEMENT", optSingle("value", optOrdinalOf(tc.entries...)))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, xml, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					opts.Composer = testComposer()
					checkOrdinalSymbol(t, c, opts, tc.want, tc.noCode)
				})
			}
		}
	}

	// RandomFill draws either pair of the two-pair list, and each takes its
	// own symbol.
	t.Run("RandomFill draws both pairs", func(t *testing.T) {
		want := map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("local"), 2: listed}
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", optOrdinalOf(cases[2].entries...))), true)
		seen := map[rm.Integer]bool{}
		for seed := range uint64(24) {
			opts := instance.Options{
				Policy:      instance.Large,
				ValueFill:   instance.RandomFill,
				ValueSource: mrand.NewPCG(seed, 7),
				Now:         defaultsNow,
				Composer:    testComposer(),
			}
			seen[checkOrdinalSymbol(t, c, opts, want, 1)] = true
		}
		for value := range want {
			if !seen[value] {
				t.Errorf("RandomFill never drew the ordinal value %d", value)
			}
		}
	})
}

// checkOrdinalSymbol generates an ELEMENT root from c under opts and checks
// that its DV_ORDINAL holds a value of want with that value's symbol, that
// the RM floor finds nothing, and that the template validator reports one
// not_in_list where the value is noCode and nothing otherwise. It returns
// the value.
func checkOrdinalSymbol(t *testing.T, c *templatecompile.Compiled, opts instance.Options, want map[rm.Integer]rm.DVCodedText, noCode rm.Integer) rm.Integer {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	ord := rootValueAs[*rm.DVOrdinal](t, out)
	wantSymbol, ok := want[ord.Value]
	if !ok {
		t.Fatalf("DV_ORDINAL value = %d, want a value the list holds", ord.Value)
	}
	if ord.Symbol.Value != wantSymbol.Value || ord.Symbol.DefiningCode != wantSymbol.DefiningCode {
		t.Errorf("DV_ORDINAL %d symbol = %q %+v, want %q %+v",
			ord.Value, ord.Symbol.Value, ord.Symbol.DefiningCode, wantSymbol.Value, wantSymbol.DefiningCode)
	}
	noFloorErrors(t, out)
	notInList, others := ordinalNotInList(templateErrors(out, c), func(path string) bool { return path == "/value" })
	for _, iss := range others {
		t.Errorf("template validator: %s @ %s (%s): %s", iss.Code, iss.Path, iss.Rule, iss.Detail)
	}
	wantNotInList := 0
	if ord.Value == noCode {
		wantNotInList = 1
	}
	if notInList != wantNotInList {
		t.Errorf("template validator: %d not_in_list for the ordinal %d, want %d", notInList, ord.Value, wantNotInList)
	}
	return ord.Value
}

// TestREQ107_OrdinalIntervalBoundWithNoCodeTakesThePlaceholderSymbol is the
// REQ-107 Placeholders check on the bounds of a DV_INTERVAL<DV_ORDINAL>
// whose lower side lists 5 with local::at0002 and then 0 with no code, and
// whose upper side lists 1 with local::at0001 and 3 with local::at0002.
// ExampleFill writes the first pairs, 5 and 1, which the generator puts in
// order as the lowest and the highest pair each side lists, 0 and 3;
// RandomFill also draws 0 for the lower bound. A lower bound of 0 has the
// placeholder symbol, as a single DV_ORDINAL does, the bounds are in
// order, the RM floor finds nothing, and the template validator reports one
// not_in_list for each bound with the placeholder and nothing else.
func TestREQ107_OrdinalIntervalBoundWithNoCodeTakesThePlaceholderSymbol(t *testing.T) {
	lower := strings.Replace(instance.OrdinalBound(5, 0),
		"<code_string>at0002</code_string>", "<code_string></code_string>", 1)
	lower = strings.Replace(lower, "<code_string>at0001</code_string>", "<code_string>at0002</code_string>", 1)
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, "DV_ORDINAL", lower, instance.OrdinalBound(1, 3)))
	symbols := map[rm.Integer]rm.CodePhrase{
		5: {CodeString: "at0002", TerminologyID: rm.TerminologyID{Value: "local"}},
		1: {CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}},
		3: {CodeString: "at0002", TerminologyID: rm.TerminologyID{Value: "local"}},
	}
	runs := []instance.Options{intervalOptions(instance.Large)}
	for seed := range uint64(orderSeeds) {
		opts := intervalOptions(instance.Large)
		opts.ValueFill = instance.RandomFill
		opts.ValueSource = mrand.NewPCG(seed, 1)
		runs = append(runs, opts)
	}
	for i, opts := range runs {
		out := generateCompiled(t, c, opts)
		placeholders, intervals := 0, 0
		visitRM(out, func(path string, iv rm.DVInterval[rm.DVOrdinal]) {
			intervals++
			if iv.Lower.Value > iv.Upper.Value {
				t.Errorf("run %d: bounds %d and %d at %s are out of order", i, iv.Lower.Value, iv.Upper.Value, path)
			}
			if i == 0 && (iv.Lower.Value != 0 || iv.Upper.Value != 3) {
				t.Errorf("ExampleFill bounds at %s = %d and %d, want 0 and 3", path, iv.Lower.Value, iv.Upper.Value)
			}
			if iv.Lower.Value == 0 {
				placeholders++
				if want := placeholderSymbol("local"); iv.Lower.Symbol.Value != want.Value || iv.Lower.Symbol.DefiningCode != want.DefiningCode {
					t.Errorf("run %d: lower bound 0 at %s has symbol %q %+v, want %q %+v",
						i, path, iv.Lower.Symbol.Value, iv.Lower.Symbol.DefiningCode, want.Value, want.DefiningCode)
				}
			} else if want := symbols[iv.Lower.Value]; iv.Lower.Symbol.DefiningCode != want {
				t.Errorf("run %d: lower bound %d at %s has symbol %+v, want %+v", i, iv.Lower.Value, path, iv.Lower.Symbol.DefiningCode, want)
			}
			if want := symbols[iv.Upper.Value]; iv.Upper.Symbol.DefiningCode != want {
				t.Errorf("run %d: upper bound %d at %s has symbol %+v, want %+v", i, iv.Upper.Value, path, iv.Upper.Symbol.DefiningCode, want)
			}
		})
		if intervals == 0 {
			t.Fatalf("run %d: no DV_INTERVAL<DV_ORDINAL> generated", i)
		}
		noFloorErrors(t, out)
		notInList, others := ordinalNotInList(templateErrors(out, c), func(path string) bool { return strings.HasSuffix(path, "/value/lower") })
		for _, iss := range others {
			t.Errorf("run %d: template validator: %s @ %s (%s): %s", i, iss.Code, iss.Path, iss.Rule, iss.Detail)
		}
		if notInList != placeholders {
			t.Errorf("run %d: template validator: %d not_in_list, want %d, one for each placeholder bound", i, notInList, placeholders)
		}
	}
}
