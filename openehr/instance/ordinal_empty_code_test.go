package instance_test

import (
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"slices"
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

// inLocal is code in local.
func inLocal(code string) rm.CodePhrase {
	return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}}
}

// listedSymbol is the symbol the generator writes for a pair a
// C_DV_ORDINAL lists with code in local: the code, with the code as its
// text, since the OPTs here define no term for it.
func listedSymbol(code string) rm.DVCodedText {
	return rm.DVCodedText{Value: code, DefiningCode: inLocal(code)}
}

// isPlaceholder reports whether symbol is a placeholder symbol: the code
// at0000 with the text example.
func isPlaceholder(symbol rm.DVCodedText) bool {
	return symbol.Value == "example" && symbol.DefiningCode.CodeString == "at0000"
}

// ordinalCase is a C_DV_ORDINAL that lists entries, with the symbol the
// generator writes for each value it may write (want) and the value
// ExampleFill writes (example).
type ordinalCase struct {
	name    string
	entries []string
	want    map[rm.Integer]rm.DVCodedText
	example rm.Integer
}

// checkOrdinalCases generates an ELEMENT root from each case under every
// policy, both value fills and both compile modes, and checks it as
// checkOrdinalSymbol does, and that ExampleFill writes the case's example.
func checkOrdinalCases(t *testing.T, cases []ordinalCase) {
	t.Helper()
	for _, tc := range cases {
		xml := optTemplate("ELEMENT", optSingle("value", optOrdinalOf(tc.entries...)))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, xml, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					opts.Composer = testComposer()
					got := checkOrdinalSymbol(t, c, opts, tc.want)
					if opts.ValueFill == instance.ExampleFill && got != tc.example {
						t.Errorf("ExampleFill DV_ORDINAL value = %d, want %d", got, tc.example)
					}
				})
			}
		}
	}
}

// checkOrdinalDraws generates an ELEMENT root from tc under RandomFill with
// 24 seeds, checks each as checkOrdinalSymbol does, and reports a value of
// tc.want that no seed draws.
func checkOrdinalDraws(t *testing.T, tc ordinalCase) {
	t.Helper()
	c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", optOrdinalOf(tc.entries...))), true)
	seen := map[rm.Integer]bool{}
	for seed := range uint64(24) {
		opts := instance.Options{
			Policy:      instance.Large,
			ValueFill:   instance.RandomFill,
			ValueSource: mrand.NewPCG(seed, 7),
			Now:         defaultsNow,
			Composer:    testComposer(),
		}
		seen[checkOrdinalSymbol(t, c, opts, tc.want)] = true
	}
	for value := range tc.want {
		if !seen[value] {
			t.Errorf("RandomFill never drew the ordinal value %d", value)
		}
	}
}

// TestREQ107_OrdinalPairWithNoCodeTakesThePlaceholderSymbol is the REQ-107
// Placeholders and Precedence 2 check on a C_DV_ORDINAL every pair of which
// is listed with an empty code_string: the DV_ORDINAL the generator writes
// keeps a listed pair's value and gets the placeholder symbol, the code
// at0000 in the pair's terminology with the text example, since the RM
// needs the symbol's code and text present. ExampleFill writes the first
// pair, and RandomFill draws each. The RM floor then finds nothing, and the
// template validator reports the one break REQ-107 § Exceptions and known
// gaps records, the pair not in the list, at the ordinal. It holds under
// every policy, both value fills and both compile modes.
func TestREQ107_OrdinalPairWithNoCodeTakesThePlaceholderSymbol(t *testing.T) {
	cases := []ordinalCase{
		{
			name:    "one pair in local",
			entries: []string{ordinalEntry(1, "local", "")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("local")},
			example: 1,
		},
		{
			name:    "one pair in another terminology",
			entries: []string{ordinalEntry(1, "SNOMED-CT", "")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("SNOMED-CT")},
			example: 1,
		},
		{
			name:    "two pairs",
			entries: []string{ordinalEntry(1, "local", ""), ordinalEntry(2, "local", "")},
			want:    map[rm.Integer]rm.DVCodedText{1: placeholderSymbol("local"), 2: placeholderSymbol("local")},
			example: 1,
		},
	}
	checkOrdinalCases(t, cases)
	t.Run("RandomFill draws each pair", func(t *testing.T) { checkOrdinalDraws(t, cases[2]) })
}

// TestREQ107_OrdinalPrefersAPairWithACode is the REQ-107 Precedence 3 check
// on a C_DV_ORDINAL that lists a pair with an empty code_string beside pairs
// with a code: the generator writes a pair with a code, whose symbol keeps
// Code_string_valid, never the pair with no code. ExampleFill writes the
// first pair with a code, RandomFill draws each pair with a code, and where
// two pairs share a value the generator writes the one with a code. The RM
// floor and the template validator find nothing. It holds under every
// policy, both value fills and both compile modes.
func TestREQ107_OrdinalPrefersAPairWithACode(t *testing.T) {
	cases := []ordinalCase{
		{
			name:    "a pair with no code before one with a code",
			entries: []string{ordinalEntry(1, "local", ""), ordinalEntry(2, "local", "at0002")},
			want:    map[rm.Integer]rm.DVCodedText{2: listedSymbol("at0002")},
			example: 2,
		},
		{
			name:    "a pair with no code between two with a code",
			entries: []string{ordinalEntry(2, "local", "at0002"), ordinalEntry(1, "local", ""), ordinalEntry(3, "local", "at0003")},
			want:    map[rm.Integer]rm.DVCodedText{2: listedSymbol("at0002"), 3: listedSymbol("at0003")},
			example: 2,
		},
		{
			name:    "a pair with no code before one with a code of the same value",
			entries: []string{ordinalEntry(1, "local", ""), ordinalEntry(1, "local", "at0001")},
			want:    map[rm.Integer]rm.DVCodedText{1: listedSymbol("at0001")},
			example: 1,
		},
	}
	checkOrdinalCases(t, cases)
	t.Run("RandomFill draws each pair with a code", func(t *testing.T) { checkOrdinalDraws(t, cases[1]) })
}

// checkOrdinalSymbol generates an ELEMENT root from c under opts and checks
// that its DV_ORDINAL holds a value of want with that value's symbol, that
// the RM floor finds nothing, and that the template validator reports one
// not_in_list where that symbol is the placeholder and nothing otherwise.
// It returns the value.
func checkOrdinalSymbol(t *testing.T, c *templatecompile.Compiled, opts instance.Options, want map[rm.Integer]rm.DVCodedText) rm.Integer {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	ord := rootValueAs[*rm.DVOrdinal](t, out)
	wantSymbol, ok := want[ord.Value]
	if !ok {
		t.Fatalf("DV_ORDINAL value = %d with symbol %q %+v, want one of the values %v",
			ord.Value, ord.Symbol.Value, ord.Symbol.DefiningCode, slices.Sorted(maps.Keys(want)))
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
	if isPlaceholder(wantSymbol) {
		wantNotInList = 1
	}
	if notInList != wantNotInList {
		t.Errorf("template validator: %d not_in_list for the ordinal %d, want %d", notInList, ord.Value, wantNotInList)
	}
	return ord.Value
}

// TestREQ107_OrdinalPairTheTemplateNamesWithNoCodeIsNotWritten is the
// REQ-107 Trust model check on a DV_ORDINAL whose template names a pair its
// C_DV_ORDINAL lists with an empty code_string: as the default value of its
// <constraints>, which leaves the symbol's code_string, an attribute the RM
// needs, empty, or as the C_DV_ORDINAL's assumed_value, which the generator
// does not read. Neither is written. Under every policy and both value
// fills the generator writes what it writes for the same OPT without it: a
// pair with a code where the list has one, and the placeholder symbol where
// it has none.
func TestREQ107_OrdinalPairTheTemplateNamesWithNoCodeIsNotWritten(t *testing.T) {
	mixed := optOrdinalOf(ordinalEntry(1, "local", ""), ordinalEntry(2, "local", "at0002"))
	blank := optOrdinalOf(ordinalEntry(1, "local", ""), ordinalEntry(2, "local", ""))
	assumed := strings.Replace(mixed, "<node_id></node_id>", "<node_id></node_id><assumed_value><value>1</value><symbol><value>First</value>"+
		"<defining_code><terminology_id><value>local</value></terminology_id><code_string></code_string></defining_code></symbol></assumed_value>", 1)
	coded := func(got string) bool { return got == "2 at0002 local::at0002" }
	cases := []struct {
		name string
		// opt names the pair with no code; plain is the same OPT without
		// it.
		opt, plain string
		// want reports whether got, the ordinal as commentText renders it,
		// is the one the case asks for, which about names.
		want  func(got string) bool
		about string
	}{
		{
			name: "a default beside a pair with a code", plain: optCommentValue(mixed),
			opt:  withDefault(optCommentValue(mixed), "DV_ORDINAL", ordinalBody("1", "First", "")),
			want: coded, about: "the pair with a code, 2 local::at0002",
		},
		{
			name: "a default in a list with no code", plain: optCommentValue(blank),
			opt:  withDefault(optCommentValue(blank), "DV_ORDINAL", ordinalBody("2", "Second", "")),
			want: func(got string) bool { return strings.HasSuffix(got, " example local::at0000") }, about: "a pair with the placeholder symbol",
		},
		{
			name: "an assumed value beside a pair with a code", plain: optCommentValue(mixed), opt: optCommentValue(assumed),
			want: coded, about: "the pair with a code, 2 local::at0002",
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		plain := compileOPTText(t, tc.plain, true)
		named, without := defaultsOptions(), defaultsOptions()
		for i, opts := range named {
			t.Run(fmt.Sprintf("%s/%v/%v", tc.name, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out := generateCompiled(t, c, opts)
				got, want := commentText(t, out), commentText(t, generateCompiled(t, plain, without[i]))
				if got != want {
					t.Errorf("ELEMENT.value = %q, want %q, what the OPT without the pair it names gives", got, want)
				}
				if !tc.want(got) {
					t.Errorf("ELEMENT.value = %q, want %s", got, tc.about)
				}
				noFloorErrors(t, out)
			})
		}
	}
}

// TestREQ107_OrdinalIntervalBoundWithNoCodeTakesThePlaceholderSymbol is the
// REQ-107 Placeholders check on the bounds of a DV_INTERVAL<DV_ORDINAL>
// whose lower side lists 5 and then 0, each with no code, and whose upper
// side lists 1 with local::at0001 and 3 with local::at0002. ExampleFill
// writes the first pairs, 5 and 1, which the generator puts in order as the
// lowest and the highest pair each side lists, 0 and 3. RandomFill draws 0
// or 5 for the lower bound, and puts 5 in order the same way. The lower
// bound 0 has the placeholder symbol, as a single DV_ORDINAL does, the
// bounds are in order, the RM floor finds nothing, and the template
// validator reports one not_in_list for each lower bound and nothing else.
func TestREQ107_OrdinalIntervalBoundWithNoCodeTakesThePlaceholderSymbol(t *testing.T) {
	lower := strings.Replace(instance.OrdinalBound(5, 0), "<code_string>at0001</code_string>", "<code_string></code_string>", 1)
	lower = strings.Replace(lower, "<code_string>at0002</code_string>", "<code_string></code_string>", 1)
	upper := map[rm.Integer]rm.CodePhrase{1: inLocal("at0001"), 3: inLocal("at0002")}
	checkOrdinalIntervals(t, lower, instance.OrdinalBound(1, 3), func(t *testing.T, i int, path string, iv rm.DVInterval[rm.DVOrdinal]) {
		if iv.Lower.Value != 0 || !isPlaceholder(iv.Lower.Symbol) {
			t.Errorf("run %d: lower bound at %s = %d %q %+v, want 0 with the placeholder symbol",
				i, path, iv.Lower.Value, iv.Lower.Symbol.Value, iv.Lower.Symbol.DefiningCode)
		}
		checkBoundSymbol(t, i, path, "upper", iv.Upper, upper)
	}, 1)
}

// TestREQ107_OrdinalIntervalBoundPrefersAPairWithACode is the REQ-107
// Precedence 3 check on the bounds of a DV_INTERVAL<DV_ORDINAL> whose lower
// side lists a pair with no code beside pairs with a code, and whose upper
// side lists pairs with a code. ExampleFill writes each side's first pair,
// the lower above the upper, which the generator puts in order with pairs
// with a code each side lists, never the lower side's pair with no code,
// and RandomFill never writes that pair either:
//
//   - below the pairs with a code, 0 is not the lowest pair the lower side
//     gives, 1 is;
//   - beside a pair with a code of the same value, 1 is the lower bound
//     with its code, which the two sides swap to.
//
// Each bound has the code its side lists for its value, the bounds are in
// order, and the RM floor and the template validator find nothing.
func TestREQ107_OrdinalIntervalBoundPrefersAPairWithACode(t *testing.T) {
	blank := func(bound, code string) string {
		return strings.Replace(bound, "<code_string>"+code+"</code_string>", "<code_string></code_string>", 1)
	}
	recode := func(bound, from, to string) string {
		return strings.Replace(bound, "<code_string>"+from+"</code_string>", "<code_string>"+to+"</code_string>", 1)
	}
	cases := []struct {
		name         string
		lower, upper string
		// lowerCodes and upperCodes are the code each side lists for each
		// value it may take; example is the pair of bounds ExampleFill
		// writes.
		lowerCodes, upperCodes map[rm.Integer]rm.CodePhrase
		example                [2]rm.Integer
	}{
		{
			// The lower side lists 5 local::at0001, 0 with no code and 1
			// local::at0003; the upper side 1 local::at0001 and 3
			// local::at0002.
			name:       "a pair with no code below the pairs with a code",
			lower:      blank(instance.OrdinalBound(5, 0, 1), "at0002"),
			upper:      instance.OrdinalBound(1, 3),
			lowerCodes: map[rm.Integer]rm.CodePhrase{5: inLocal("at0001"), 1: inLocal("at0003")},
			upperCodes: map[rm.Integer]rm.CodePhrase{1: inLocal("at0001"), 3: inLocal("at0002")},
			example:    [2]rm.Integer{1, 3},
		},
		{
			// The lower side lists 5 local::at0001, 1 with no code and 1
			// local::at0003; the upper side 1 local::at0003 and 5
			// local::at0001.
			name:       "a pair with no code beside one with a code of the same value",
			lower:      blank(instance.OrdinalBound(5, 1, 1), "at0002"),
			upper:      recode(recode(instance.OrdinalBound(1, 5), "at0001", "at0003"), "at0002", "at0001"),
			lowerCodes: map[rm.Integer]rm.CodePhrase{5: inLocal("at0001"), 1: inLocal("at0003")},
			upperCodes: map[rm.Integer]rm.CodePhrase{1: inLocal("at0003"), 5: inLocal("at0001")},
			example:    [2]rm.Integer{1, 5},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkOrdinalIntervals(t, tc.lower, tc.upper, func(t *testing.T, i int, path string, iv rm.DVInterval[rm.DVOrdinal]) {
				if got := [2]rm.Integer{iv.Lower.Value, iv.Upper.Value}; i == 0 && got != tc.example {
					t.Errorf("ExampleFill bounds at %s = %d, want %d", path, got, tc.example)
				}
				checkBoundSymbol(t, i, path, "lower", iv.Lower, tc.lowerCodes)
				checkBoundSymbol(t, i, path, "upper", iv.Upper, tc.upperCodes)
			}, 0)
		})
	}
}

// checkOrdinalIntervals generates the CountIntervalOPT, retargeted to
// DV_INTERVAL<DV_ORDINAL> with the lower and upper bound children, under
// ExampleFill (run 0) and RandomFill with orderSeeds seeds, under Large. It
// checks that each run holds such an interval, in order, and passes each to
// check; that the RM floor finds nothing; and that the template validator
// reports notInList not_in_list at each lower bound and nothing else.
func checkOrdinalIntervals(t *testing.T, lower, upper string, check func(t *testing.T, i int, path string, iv rm.DVInterval[rm.DVOrdinal]), notInList int) {
	t.Helper()
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, "DV_ORDINAL", lower, upper))
	runs := []instance.Options{intervalOptions(instance.Large)}
	for seed := range uint64(orderSeeds) {
		opts := intervalOptions(instance.Large)
		opts.ValueFill = instance.RandomFill
		opts.ValueSource = mrand.NewPCG(seed, 1)
		runs = append(runs, opts)
	}
	for i, opts := range runs {
		out := generateCompiled(t, c, opts)
		intervals := 0
		visitRM(out, func(path string, iv rm.DVInterval[rm.DVOrdinal]) {
			intervals++
			if iv.Lower.Value > iv.Upper.Value {
				t.Errorf("run %d: bounds %d and %d at %s are out of order", i, iv.Lower.Value, iv.Upper.Value, path)
			}
			check(t, i, path, iv)
		})
		if intervals == 0 {
			t.Fatalf("run %d: no DV_INTERVAL<DV_ORDINAL> generated", i)
		}
		noFloorErrors(t, out)
		got, others := ordinalNotInList(templateErrors(out, c), func(path string) bool { return strings.HasSuffix(path, "/value/lower") })
		for _, iss := range others {
			t.Errorf("run %d: template validator: %s @ %s (%s): %s", i, iss.Code, iss.Path, iss.Rule, iss.Detail)
		}
		if want := notInList * intervals; got != want {
			t.Errorf("run %d: template validator: %d not_in_list, want %d", i, got, want)
		}
	}
}

// checkBoundSymbol reports a bound, the side of an interval at path in run
// i, whose value is not one of want or whose symbol's code is not want's
// for it. The symbol's text is the term the OPT's archetype defines for the
// code, which is not checked.
func checkBoundSymbol(t *testing.T, i int, path, side string, bound rm.DVOrdinal, want map[rm.Integer]rm.CodePhrase) {
	t.Helper()
	code, ok := want[bound.Value]
	if !ok {
		t.Errorf("run %d: %s bound at %s = %d, want one of %v", i, side, path, bound.Value, slices.Sorted(maps.Keys(want)))
		return
	}
	if got := bound.Symbol.DefiningCode; got != code {
		t.Errorf("run %d: %s bound %d at %s has symbol code %+v, want %+v", i, side, bound.Value, path, got, code)
	}
}
