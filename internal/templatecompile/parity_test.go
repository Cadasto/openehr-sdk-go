package templatecompile

// parity_test.go: REQ-111 § Parity with the deprecated parser. Every
// operational template that template.ParseOPT reads, of the corpus, of the
// parity fixtures and of the model reader's fixtures, is read by
// aom14.ReadOperationalTemplate and compiled by Compile, both in their
// default mode, with on each node the primitive constraint the parser reads
// there. The exceptions the clause allows are told apart by their errors.
// TestREQ111_ParityOverFolder runs the same check over a folder the
// repository does not hold (make template-parity DIR=<folder>).

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// parityDirEnv names the folder TestREQ111_ParityOverFolder walks.
const parityDirEnv = "TEMPLATE_PARITY_DIR"

// readerFixtureDir holds the model reader's fixtures, OPTs the SDK writes
// for the shapes and values the reader tolerates.
const readerFixtureDir = "../../openehr/aom/aom14/testdata/reader"

// parityExceptionDir holds OPTs the SDK writes that the parser reads and the
// new path refuses for a reason the rule allows, one for each reason.
const parityExceptionDir = "testdata/parity-exceptions"

// parityIntegerBoundDir holds the parity fixtures whose integer bounds the
// reader reads otherwise than the parser, admitting the same integers. They
// sit apart from TestREQ111_ParityFixtures' list, which pins each fixture's
// strict-mode refusals, so they are checked here in the default mode only.
const parityIntegerBoundDir = "testdata/parity/integer-bounds"

// parityKind is what the parity check makes of one operational template.
type parityKind int

const (
	// parityNotRead: the parser does not read it, so the rule asks
	// nothing of the new path.
	parityNotRead parityKind = iota
	// parityHolds: the new path reads and compiles it, with on each node
	// the parser's primitive constraint.
	parityHolds
	// parityLargeInteger: the reader refuses an integer the model's 32-bit
	// Integer cannot hold, which the parser reads.
	parityLargeInteger
	// parityMatchNegated: the reader refuses a match_negated of true, which
	// the parser reads with its meaning inverted.
	parityMatchNegated
	// parityInfiniteBound: the reader refuses an integer bound of positive
	// infinity on a lower side or negative infinity on an upper one, which
	// the parser reads as a float and which admits no integer.
	parityInfiniteBound
	// paritySharedPath: the compile step refuses two nodes under different
	// attributes that share an AQL path.
	paritySharedPath
	// parityRefused: the new path refuses what the parser reads, for none
	// of the reasons the rule allows.
	parityRefused
	// parityMismatch: the new path compiles it, but a node carries another
	// primitive constraint than the parser reads there, or the compiled
	// tree has another shape than the parser's.
	parityMismatch
)

// fails reports whether the kind breaks the rule.
func (k parityKind) fails() bool {
	return k == parityRefused || k == parityMismatch
}

// allowed reports whether the kind is a refusal the rule allows.
func (k parityKind) allowed() bool {
	return k == parityLargeInteger || k == parityMatchNegated || k == parityInfiniteBound || k == paritySharedPath
}

// String names the kind as the summary of TestREQ111_ParityOverFolder
// prints it.
func (k parityKind) String() string {
	switch k {
	case parityNotRead:
		return "the parser does not read it"
	case parityHolds:
		return "holds"
	case parityLargeInteger:
		return "allowed exception, an integer the model cannot hold"
	case parityMatchNegated:
		return "allowed exception, a match_negated of true"
	case parityInfiniteBound:
		return "allowed exception, an infinite integer bound that admits no integer"
	case paritySharedPath:
		return "allowed exception, two nodes under different attributes share an AQL path"
	case parityRefused:
		return "the new path refuses it"
	case parityMismatch:
		return "constraint mismatch"
	}
	return "parityKind(" + strconv.Itoa(int(k)) + ")"
}

// parityResult is what the parity check makes of one operational template.
type parityResult struct {
	kind parityKind
	// err is the error behind the kind: the parser's, the reader's or the
	// compile step's refusal, or the first mismatch.
	err error
	// mismatches names each node whose compiled constraint differs from
	// the parser's.
	mismatches []string
	// seen counts the primitive constraints compiled, by Go type.
	seen map[string]int
	// bareCodeReferences names each C_CODE_REFERENCE with no reference set
	// URI.
	bareCodeReferences []string
}

// checkParity reads raw with the deprecated parser and, when the parser
// reads it, with the model's reader and the compile step, each in its
// default mode, and compares the primitive constraint of each node of the
// parser's tree with that of the compiled node that stands where it stands.
func checkParity(raw []byte) parityResult {
	parsed, err := template.ParseOPT(bytes.NewReader(raw))
	if err != nil {
		return parityResult{kind: parityNotRead, err: err}
	}
	model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return parityResult{kind: readerRefusal(err), err: err}
	}
	c, err := Compile(model)
	if err != nil {
		return parityResult{kind: compileRefusal(err), err: err}
	}
	r := parityResult{kind: parityHolds, seen: map[string]int{}}
	if err := r.compare(parsed.Root(), &model.Definition, c.root); err != nil {
		r.kind, r.err = parityMismatch, err
		return r
	}
	if len(r.mismatches) > 0 {
		r.kind = parityMismatch
		r.err = fmt.Errorf("the compiled primitive constraint differs from the parser's on %d node(s); the first, %s", len(r.mismatches), r.mismatches[0])
	}
	return r
}

// readerRefusal tells which reader refusal err is: an integer the model's
// 32-bit Integer cannot hold, the reader's ParseInt range refusal; a
// match_negated of true; an infinite integer bound that admits no integer,
// the reader's ParseInt syntax refusal of a lower or upper bound whose text
// strconv.ParseFloat reads as the infinity no integer is above or below; or
// another.
func readerRefusal(err error) parityKind {
	if !errors.Is(err, canxml.ErrInvalidShape) {
		return parityRefused
	}
	if ne, ok := errors.AsType[*strconv.NumError](err); ok && ne != nil && ne.Func == "ParseInt" {
		switch {
		case errors.Is(ne, strconv.ErrRange):
			return parityLargeInteger
		case errors.Is(ne, strconv.ErrSyntax) && admitsNoInteger(innermostDecodePath(err), ne.Num):
			return parityInfiniteBound
		}
	}
	if innermostDecodePath(err) == "/match_negated" {
		return parityMatchNegated
	}
	return parityRefused
}

// admitsNoInteger reports whether text, the bound at path, is an infinity
// that admits no integer there: positive infinity on a lower side, path
// "/lower", or negative infinity on an upper one, path "/upper", as
// strconv.ParseFloat reads it, as the parser does.
func admitsNoInteger(path, text string) bool {
	f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return false
	}
	switch path {
	case "/lower":
		return math.IsInf(f, 1)
	case "/upper":
		return math.IsInf(f, -1)
	}
	return false
}

// innermostDecodePath returns the path of the innermost decode error in err's
// chain, the element the decode failed on, or "" when err holds none.
func innermostDecodePath(err error) string {
	var path string
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return path
		}
		path, err = de.Path, de.Inner
	}
}

// compileRefusal tells which compile refusal err is: two nodes under
// different attributes that share an AQL path, or another.
func compileRefusal(err error) parityKind {
	if errors.Is(err, ErrInvalidInput) && strings.Contains(err.Error(), ": duplicate AQL path ") {
		return paritySharedPath
	}
	return parityRefused
}

// compare compares the primitive constraint of parsed with that of compiled,
// model being the node of the model compiled reads, then their descendants.
// The compiled tree lists the attributes the template declares first, in
// document order, and those the compile step injects after them, which hold
// no child. It returns an error when the trees have different shapes.
func (r *parityResult) compare(parsed template.Node, model aom14.CObject, compiled *CompiledNode) error {
	where := fmt.Sprintf("%s (%s)", compiled.aqlPath, compiled.rmTypeName)
	var want constraints.PrimitiveConstraint
	if on, ok := parsed.(interface {
		PrimitiveConstraint() constraints.PrimitiveConstraint
	}); ok {
		want = on.PrimitiveConstraint()
	}
	if ref, ok := model.(*aom14.CCodeReference); ok {
		// The parser reads no constraint on a C_CODE_REFERENCE; the
		// compile step reads its code phrase as a C_CODE_PHRASE's, with
		// the reference set's URI beside it.
		if want != nil {
			r.mismatches = append(r.mismatches, fmt.Sprintf("%s: the parser reads %s on a C_CODE_REFERENCE; want none", where, renderPrimitive(want)))
		}
		uri := strings.TrimSpace(ref.ReferenceSetURI)
		want = constraints.CodeReference{CodePhrase: codePhraseFromModel(&ref.CCodePhrase), ReferenceSetURI: uri}
		if uri == "" {
			r.bareCodeReferences = append(r.bareCodeReferences, where)
		}
	}
	got := compiled.primitive
	if m, differs := primitiveMismatch(got, want); differs {
		r.mismatches = append(r.mismatches, where+": "+m)
	}
	if got != nil {
		r.seen[fmt.Sprintf("%T", got)]++
	}

	var parsedAttrs []*template.Attribute
	if on, ok := parsed.(template.ObjectNode); ok {
		parsedAttrs = on.Attributes()
	}
	modelAttrs := modelAttributes(model)
	if len(modelAttrs) != len(parsedAttrs) || len(compiled.attributes) < len(parsedAttrs) {
		return fmt.Errorf("at %s the parser reads %d attributes, the model holds %d and the compiled tree %d", where, len(parsedAttrs), len(modelAttrs), len(compiled.attributes))
	}
	for i, pa := range parsedAttrs {
		_, modelKids := modelChildren(modelAttrs[i])
		ca := compiled.attributes[i]
		parsedKids := pa.Children()
		if ca.name != pa.Name() || len(ca.children) != len(parsedKids) || len(modelKids) != len(parsedKids) {
			return fmt.Errorf("at %s attribute %d: the parser reads %s with %d children, the model holds %d and the compiled tree reads %s with %d",
				where, i+1, pa.Name(), len(parsedKids), len(modelKids), ca.name, len(ca.children))
		}
		for j, pc := range parsedKids {
			if err := r.compare(pc, modelKids[j], ca.children[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

// primitiveMismatch reports whether compiled, the compile step's primitive
// constraint on a node, admits other values than want, the parser's there,
// and says how. It compares the two as written once want's integer ranges are
// in the integer form the reader reads them in (integerForm), which admits the
// same integers.
func primitiveMismatch(compiled, want constraints.PrimitiveConstraint) (string, bool) {
	g, w := renderPrimitive(compiled), renderPrimitive(integerForm(want))
	return fmt.Sprintf("compiled %s; the parser reads %s", g, w), g != w
}

// integerForm returns pc, the primitive constraint the parser reads, with each
// integer range, of a C_INTEGER or of a quantity's precision, in the integer
// form the reader reads it in (integerSide). The parser reads such a bound as
// a float, so its range can write a bound no integer or no JSON number holds;
// the two forms admit the same integers.
func integerForm(pc constraints.PrimitiveConstraint) constraints.PrimitiveConstraint {
	integer := func(r constraints.NumericRange) constraints.NumericRange {
		r.LowerUnbounded, r.Lower, r.LowerInclusive = integerSide(r.LowerUnbounded, r.Lower, r.LowerInclusive, true)
		r.UpperUnbounded, r.Upper, r.UpperInclusive = integerSide(r.UpperUnbounded, r.Upper, r.UpperInclusive, false)
		return r
	}
	switch c := pc.(type) {
	case constraints.CInteger:
		c.Range = integer(c.Range)
		return c
	case constraints.DvQuantity:
		c.Units = slices.Clone(c.Units)
		for i := range c.Units {
			c.Units[i].Precision = integer(c.Units[i].Precision)
		}
		return c
	}
	return pc
}

// integerSide returns one side of an integer range the parser reads, the
// lower one where lower, in the integer form that admits the same integers:
// a bound that is no whole number rounded up on the lower side and down on the
// upper one, its side included; a NaN bound, which the parser compares no
// integer with, or the infinity that admits every integer on its side,
// negative on the lower one or positive on the upper one, as an unbounded
// side, which keeps its included flag and no bound. An unbounded side, and an
// infinity that admits no integer, which the reader refuses, stay as they
// are.
func integerSide(unbounded bool, bound float64, included, lower bool) (bool, float64, bool) {
	switch {
	case unbounded:
		return unbounded, bound, included
	case math.IsNaN(bound), math.IsInf(bound, -1) && lower, math.IsInf(bound, 1) && !lower:
		return true, 0, included
	case math.IsInf(bound, 0):
		return unbounded, bound, included
	}
	n := math.Floor(bound)
	if lower {
		n = math.Ceil(bound)
	}
	if n == 0 {
		// math.Ceil(-0.5) is the float negative zero, which renders as -0;
		// the reader's bound is the integer zero.
		n = 0
	}
	return false, n, included || n != bound
}

// TestREQ111_ParityWithDeprecatedParser holds the model's reader and the
// compile step to the deprecated parser over every operational template of
// the corpus, of the parity fixtures and of the reader's fixtures: where the
// parser reads one, the new path reads and compiles it with on each node the
// parser's primitive constraint, or refuses it for a reason the rule allows,
// told by its error. The parser reads every corpus template and every parity
// fixture, the corpus compiles each kind of primitive constraint, and each
// fixture of testdata/parity-exceptions is refused for one of the reasons the
// rule allows, each reason by one fixture at least.
func TestREQ111_ParityWithDeprecatedParser(t *testing.T) {
	glob := func(dir string) []string {
		files, err := filepath.Glob(filepath.Join(dir, "*.opt"))
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	var corpus []string
	for _, name := range corpusOperationalTemplates(t) {
		corpus = append(corpus, filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(name)))
	}
	sources := []struct {
		name  string
		files []string
		// parserReadsAll is true when the parser must read every file,
		// so that a parser refusal cannot shrink the check unseen.
		parserReadsAll bool
		// exceptions is true when the new path must refuse every file
		// for a reason the rule allows.
		exceptions bool
	}{
		{name: "corpus", files: corpus, parserReadsAll: true},
		{name: "parity", files: append(glob(parityFixtureDir), glob(parityIntegerBoundDir)...), parserReadsAll: true},
		{name: "reader", files: glob(readerFixtureDir)},
		{name: "exception", files: glob(parityExceptionDir), parserReadsAll: true, exceptions: true},
	}

	// The files are walked in one test, not one subtest each, so that the
	// floors below always see the whole walk.
	corpusSeen := map[string]int{}
	allowed := map[parityKind]int{}
	for _, src := range sources {
		if len(src.files) == 0 {
			t.Errorf("the %s source holds no operational template", src.name)
		}
		var held int
		for _, file := range src.files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			r := checkParity(raw)
			switch {
			case r.kind == parityNotRead && src.parserReadsAll:
				t.Errorf("%s: template.ParseOPT = %v; want it read, so that the parity check covers it", file, r.err)
			case r.kind == parityNotRead:
				t.Logf("%s: the parser does not read it (%v), so the rule asks nothing", file, r.err)
			case r.kind.fails():
				t.Errorf("%s: %s: %v", file, r.kind, r.err)
				for _, m := range r.mismatches[min(1, len(r.mismatches)):] {
					t.Errorf("%s: also %s", file, m)
				}
			case r.kind.allowed():
				allowed[r.kind]++
				t.Logf("%s: %s: %v", file, r.kind, r.err)
			case src.exceptions:
				t.Errorf("%s: the new path reads and compiles it; want a refusal the rule allows", file)
			default:
				held++
			}
			if src.name != "corpus" {
				continue
			}
			for k, n := range r.seen {
				corpusSeen[k] += n
			}
			for _, where := range r.bareCodeReferences {
				t.Errorf("%s: %s: a corpus C_CODE_REFERENCE with no reference set URI", file, where)
			}
		}
		if !src.exceptions && held == 0 {
			t.Errorf("no operational template of the %s source holds the rule; want at least one", src.name)
		}
	}
	for _, k := range []parityKind{parityLargeInteger, parityMatchNegated, parityInfiniteBound, paritySharedPath} {
		if allowed[k] == 0 {
			t.Errorf("no operational template is refused as %q; want a fixture for each exception the rule allows", k)
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
		if corpusSeen[fmt.Sprintf("%T", want)] == 0 {
			t.Errorf("no corpus node compiled a %T; the parity walk pairs no such node", want)
		}
	}
}

// TestREQ111_ParityRefusalsByError pins how the parity check tells a refusal
// the rule allows from one it does not, by the error alone: an integer past
// the 32-bit range is the reader's ParseInt range refusal, a match_negated of
// true is the reader's refusal of that element, an infinite integer bound
// that admits no integer is the reader's ParseInt syntax refusal of a lower
// bound of positive infinity or an upper one of negative infinity, and a
// shared AQL path is the compile step's duplicate-path refusal. A refusal
// that shares part of one of these, such as a real out of range, another
// element's refusal or an infinity on the side it admits every integer,
// breaks the rule.
func TestREQ111_ParityRefusalsByError(t *testing.T) {
	shape := func(path string, inner error) error {
		return &canxml.DecodeError{Path: "/attributes", Type: "C_SINGLE_ATTRIBUTE", Inner: &canxml.DecodeError{Path: path, Inner: inner}}
	}
	rangeErr := func(fn string) error {
		return fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: fn, Num: "5000000000", Err: strconv.ErrRange})
	}
	syntaxErr := func(num string) error {
		return fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: "ParseInt", Num: num, Err: strconv.ErrSyntax})
	}
	readerCases := []struct {
		name string
		err  error
		want parityKind
	}{
		{name: "an integer past the 32-bit range", err: shape("/upper", rangeErr("ParseInt")), want: parityLargeInteger},
		{name: "a match_negated of true", err: shape("/match_negated", fmt.Errorf("%w: the element holds a value other than the one it may hold", canxml.ErrInvalidShape)), want: parityMatchNegated},
		{name: "a real out of range", err: shape("/upper", rangeErr("ParseFloat")), want: parityRefused},
		{name: "an integer that is no number", err: shape("/upper", fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: "ParseInt", Num: "x", Err: strconv.ErrSyntax})), want: parityRefused},
		{name: "a range refusal that is no shape refusal", err: &strconv.NumError{Func: "ParseInt", Num: "5000000000", Err: strconv.ErrRange}, want: parityRefused},
		{name: "another element's refusal", err: shape("/is_ordered", canxml.ErrInvalidShape), want: parityRefused},
		{name: "a refusal inside a match_negated's parent", err: &canxml.DecodeError{Path: "/match_negated", Inner: &canxml.DecodeError{Path: "/value", Inner: canxml.ErrInvalidShape}}, want: parityRefused},
		{name: "a match_negated that is no shape refusal", err: &canxml.DecodeError{Path: "/match_negated", Inner: errors.New("read failed")}, want: parityRefused},
		{name: "a lower bound of positive infinity", err: shape("/lower", syntaxErr("+Inf")), want: parityInfiniteBound},
		{name: "an upper bound of negative infinity", err: shape("/upper", syntaxErr("-Infinity")), want: parityInfiniteBound},
		{name: "a lower bound of negative infinity, which admits every integer", err: shape("/lower", syntaxErr("-Inf")), want: parityRefused},
		{name: "an upper bound of positive infinity, which admits every integer", err: shape("/upper", syntaxErr("INF")), want: parityRefused},
		{name: "a list entry of positive infinity", err: shape("/list", syntaxErr("+Inf")), want: parityRefused},
		{name: "a lower bound past the range of a float", err: shape("/lower", syntaxErr("1e400")), want: parityRefused},
		{name: "a real lower bound of positive infinity", err: shape("/lower", fmt.Errorf("%w: %w", canxml.ErrInvalidShape, &strconv.NumError{Func: "ParseFloat", Num: "+Inf", Err: strconv.ErrSyntax})), want: parityRefused},
		{name: "an infinite bound that is no shape refusal", err: &canxml.DecodeError{Path: "/lower", Inner: &strconv.NumError{Func: "ParseInt", Num: "+Inf", Err: strconv.ErrSyntax}}, want: parityRefused},
	}
	for _, tc := range readerCases {
		if got := readerRefusal(tc.err); got != tc.want {
			t.Errorf("readerRefusal(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}

	compileCases := []struct {
		name string
		err  error
		want parityKind
	}{
		{name: "a shared AQL path", err: fmt.Errorf(`%w: duplicate AQL path "/category" (existing DV_CODED_TEXT, new DV_CODED_TEXT)`, ErrInvalidInput), want: paritySharedPath},
		{name: "another invalid input", err: fmt.Errorf("%w: template has no template_id", ErrInvalidInput), want: parityRefused},
		{name: "a shared AQL path that is no invalid input", err: errors.New(`templatecompile: duplicate AQL path "/category"`), want: parityRefused},
	}
	for _, tc := range compileCases {
		if got := compileRefusal(tc.err); got != tc.want {
			t.Errorf("compileRefusal(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}
}

// TestREQ111_ParityComparesAdmittedIntegers pins how the parity check compares
// a node's primitive constraint with the parser's: an integer range, of a
// C_INTEGER or of a quantity's precision, by the integers it admits, so that
// the parser's bound that is no whole number, its NaN bound and its infinite
// bound that admits every integer match the compile step's integer form, and
// every other constraint as written. A range that admits other integers, or a
// whole bound whose side is excluded on one side only, still differs.
func TestREQ111_ParityComparesAdmittedIntegers(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	closed := func(lower, upper float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lower, Upper: upper, LowerInclusive: true, UpperInclusive: true}
	}
	open := func(lower, upper float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lower, Upper: upper}
	}
	upTo := func(upper float64) constraints.NumericRange {
		return constraints.NumericRange{LowerUnbounded: true, LowerInclusive: true, Upper: upper, UpperInclusive: true}
	}
	from := func(lower float64) constraints.NumericRange {
		return constraints.NumericRange{Lower: lower, LowerInclusive: true, UpperUnbounded: true, UpperInclusive: true}
	}
	integer := func(r constraints.NumericRange) constraints.PrimitiveConstraint {
		return constraints.CInteger{Range: r}
	}
	precision := func(r constraints.NumericRange) constraints.PrimitiveConstraint {
		return constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "kg", Magnitude: closed(0, 100), Precision: r}}}
	}
	magnitude := func(r constraints.NumericRange) constraints.PrimitiveConstraint {
		return constraints.DvQuantity{Units: []constraints.QuantityUnit{{Units: "kg", Magnitude: r, Precision: closed(0, 2)}}}
	}
	cases := []struct {
		name     string
		parsed   constraints.PrimitiveConstraint
		compiled constraints.PrimitiveConstraint
		differs  bool
	}{
		{name: "C_INTEGER 0.5..9.5 and 1..9", parsed: integer(closed(0.5, 9.5)), compiled: integer(closed(1, 9))},
		{name: "C_INTEGER 0.5..9.5 and 1..10", parsed: integer(closed(0.5, 9.5)), compiled: integer(closed(1, 10)), differs: true},
		{name: "C_INTEGER 0.5..9.5 and 0..9", parsed: integer(closed(0.5, 9.5)), compiled: integer(closed(0, 9)), differs: true},
		{name: "C_INTEGER >-0.5..<3.5 and 0..3", parsed: integer(open(-0.5, 3.5)), compiled: integer(closed(0, 3))},
		{name: "C_INTEGER >-0.5..<3.5 and >0..<3", parsed: integer(open(-0.5, 3.5)), compiled: integer(open(0, 3)), differs: true},
		{name: "C_INTEGER >0..<3 and >0..<3", parsed: integer(open(0, 3)), compiled: integer(open(0, 3))},
		{name: "C_INTEGER >0..<3 and 0..3", parsed: integer(open(0, 3)), compiled: integer(closed(0, 3)), differs: true},
		{name: "C_INTEGER NaN..10 and ..10", parsed: integer(closed(nan, 10)), compiled: integer(upTo(10))},
		{name: "C_INTEGER NaN..10 and 0..10", parsed: integer(closed(nan, 10)), compiled: integer(closed(0, 10)), differs: true},
		{name: "C_INTEGER 1..NaN and 1..", parsed: integer(closed(1, nan)), compiled: integer(from(1))},
		{name: "C_INTEGER -Inf..+Inf and unbounded", parsed: integer(closed(-inf, inf)), compiled: integer(constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true, LowerInclusive: true, UpperInclusive: true})},
		{name: "C_INTEGER -Inf..10 and 0..10", parsed: integer(closed(-inf, 10)), compiled: integer(closed(0, 10)), differs: true},
		{name: "precision 0.5..2.5 and 1..2", parsed: precision(closed(0.5, 2.5)), compiled: precision(closed(1, 2))},
		{name: "precision 0.5..2.5 and 1..3", parsed: precision(closed(0.5, 2.5)), compiled: precision(closed(1, 3)), differs: true},
		{name: "precision NaN..2 and ..2", parsed: precision(closed(nan, 2)), compiled: precision(upTo(2))},
		{name: "magnitude, a real, 0.5..9.5 and 1..9", parsed: magnitude(closed(0.5, 9.5)), compiled: magnitude(closed(1, 9)), differs: true},
		{name: "C_REAL 0.5..9.5 and 1..9", parsed: constraints.CReal{Range: closed(0.5, 9.5)}, compiled: constraints.CReal{Range: closed(1, 9)}, differs: true},
	}
	for _, tc := range cases {
		if got, differs := primitiveMismatch(tc.compiled, tc.parsed); differs != tc.differs {
			t.Errorf("primitiveMismatch(%s) = %q, %t; want differs %t", tc.name, got, differs, tc.differs)
		}
	}
}

// TestREQ111_ParityOverFolder runs the parity check of
// TestREQ111_ParityWithDeprecatedParser over every *.opt file under the
// folder TEMPLATE_PARITY_DIR names, a folder the repository does not hold,
// and is skipped when it names none (make template-parity DIR=<folder>). It
// skips, with a note, a file it cannot read or that holds no XML, prints how
// many files fall in each kind and a line for each file that breaks the rule,
// and fails when one does.
func TestREQ111_ParityOverFolder(t *testing.T) {
	dir := os.Getenv(parityDirEnv)
	if dir == "" {
		t.Skipf("%s names no folder; make template-parity DIR=<folder> runs this check", parityDirEnv)
	}
	say := func(format string, args ...any) {
		t.Helper()
		if _, err := fmt.Fprintf(t.Output(), format+"\n", args...); err != nil {
			t.Fatal(err)
		}
	}
	var files, skipped int
	counts := map[parityKind]int{}
	var failed []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			say("skipped %s: %v", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".opt") {
			return nil
		}
		files++
		raw, err := os.ReadFile(path)
		if err != nil {
			skipped++
			say("skipped %s: %v", path, err)
			return nil
		}
		if !looksLikeXML(raw) {
			skipped++
			say("skipped %s: not XML", path)
			return nil
		}
		r := checkParity(raw)
		counts[r.kind]++
		if r.kind.fails() {
			failed = append(failed, fmt.Sprintf("%s: %s: %v", path, r.kind, r.err))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if files == 0 {
		t.Fatalf("%s holds no *.opt file", dir)
	}

	say("template parity over %s", dir)
	say("  *.opt files: %d, skipped: %d", files, skipped)
	say("  parser reads: %d", files-skipped-counts[parityNotRead])
	say("  new path compiles: %d", counts[parityHolds]+counts[parityMismatch])
	say("  constraint mismatches: %d", counts[parityMismatch])
	for _, k := range []parityKind{parityLargeInteger, parityMatchNegated, parityInfiniteBound, paritySharedPath} {
		say("  %s: %d", k, counts[k])
	}
	say("  new path refuses, for no allowed reason: %d", counts[parityRefused])
	say("  files that break the rule: %d", len(failed))
	for _, f := range failed {
		say("FAIL %s", f)
	}
	if len(failed) > 0 {
		t.Fail()
	}
}

// looksLikeXML reports whether raw, after a UTF-8 byte-order mark and white
// space, opens with a markup character, as an XML document does.
func looksLikeXML(raw []byte) bool {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	raw = bytes.TrimLeft(raw, " \t\r\n")
	return len(raw) > 0 && raw[0] == '<'
}
