package templatecompile

// parity_test.go: REQ-111 § Parity with the deprecated parser. Every
// operational template that template.ParseOPT reads, of the corpus, of the
// parity fixtures and of the model reader's fixtures, is read by
// aom14.ReadOperationalTemplate and compiled by Compile, both in their
// default mode, each node of the parser's tree compiled with its RM type,
// node id, archetype id, occurrences, slot rules and primitive constraint,
// and each attribute with its name, existence and cardinality and as many
// children. The exceptions the clause allows are told apart by their errors.
// TestREQ111_ParityOverFolder runs the same check over a folder the
// repository does not hold (make template-parity DIR=<folder>).

import (
	"bytes"
	"encoding/xml"
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
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
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
	// parityDefinitionNotKept: the reader refuses a definition the parser
	// reads as a slot or as a leaf that carries a primitive constraint,
	// which the model's definition, an archetype root, cannot hold.
	parityDefinitionNotKept
	// paritySharedPath: the compile step refuses two nodes under different
	// attributes that share an AQL path.
	paritySharedPath
	// parityNoRMType: the compile step refuses a definition that names no
	// RM type.
	parityNoRMType
	// parityRefused: the new path refuses what the parser reads, for none
	// of the reasons the rule allows.
	parityRefused
	// parityMismatch: the new path compiles it, but a node or an attribute
	// differs from the parser's, or the compiled tree has another shape than
	// the parser's.
	parityMismatch
)

// allowedKinds are the refusals the rule allows, in the order the summary of
// TestREQ111_ParityOverFolder prints them.
var allowedKinds = []parityKind{
	parityLargeInteger, parityMatchNegated, parityInfiniteBound, parityDefinitionNotKept,
	paritySharedPath, parityNoRMType,
}

// fails reports whether the kind breaks the rule.
func (k parityKind) fails() bool {
	return k == parityRefused || k == parityMismatch
}

// allowed reports whether the kind is a refusal the rule allows.
func (k parityKind) allowed() bool {
	return slices.Contains(allowedKinds, k)
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
	case parityDefinitionNotKept:
		return "allowed exception, a definition the parser reads as a slot or a primitive constraint"
	case paritySharedPath:
		return "allowed exception, two nodes under different attributes share an AQL path"
	case parityNoRMType:
		return "allowed exception, a definition that names no RM type"
	case parityRefused:
		return "the new path refuses it"
	case parityMismatch:
		return "the compiled tree differs from the parser's"
	}
	return "parityKind(" + strconv.Itoa(int(k)) + ")"
}

// parityResult is what the parity check makes of one operational template.
type parityResult struct {
	kind parityKind
	// err is the error behind the kind: the parser's, the reader's or the
	// compile step's refusal, or the first mismatch.
	err error
	// mismatches names each node or attribute whose compiled form differs
	// from the parser's, and how.
	mismatches []string
	// seen counts the primitive constraints compiled, by Go type.
	seen map[string]int
	// slots counts the slots whose rules were compared.
	slots int
	// untypedCardinalities counts the attributes the parser reads as single
	// and the compile step as multiple because their <attributes> names no
	// type and carries a <cardinality>, the difference the rule allows.
	untypedCardinalities int
	// bareCodeReferences names each C_CODE_REFERENCE with no reference set
	// URI.
	bareCodeReferences []string
}

// checkParity reads raw with the deprecated parser and, when the parser
// reads it, with the model's reader and the compile step, each in its
// default mode, and compares each node of the parser's tree with the
// compiled node that stands where it stands.
func checkParity(raw []byte) parityResult {
	parsed, err := template.ParseOPT(bytes.NewReader(raw))
	if err != nil {
		return parityResult{kind: parityNotRead, err: err}
	}
	model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return parityResult{kind: readerRefusal(err, parsed.Root()), err: err}
	}
	c, err := Compile(model)
	if err != nil {
		return parityResult{kind: compileRefusal(err, parsed.Root(), &model.Definition), err: err}
	}
	src, err := readSource(raw)
	if err != nil {
		return parityResult{kind: parityMismatch, err: fmt.Errorf("the parity check cannot read the source the parser reads: %w", err)}
	}
	r := parityResult{kind: parityHolds, seen: map[string]int{}}
	if err := r.compare(parsed.Root(), &model.Definition, c.root, src); err != nil {
		r.kind, r.err = parityMismatch, err
		return r
	}
	if len(r.mismatches) > 0 {
		r.kind = parityMismatch
		r.err = fmt.Errorf("the compiled tree differs from the parser's in %d place(s); the first, %s", len(r.mismatches), r.mismatches[0])
	}
	return r
}

// parserDefinitionNotKept reports whether root, the definition the parser
// reads, is a slot or a leaf that carries a primitive constraint, which the
// model's definition cannot hold.
func parserDefinitionNotKept(root template.Node) bool {
	if _, ok := root.(*template.Slot); ok {
		return true
	}
	on, ok := root.(interface {
		PrimitiveConstraint() constraints.PrimitiveConstraint
	})
	return ok && on.PrimitiveConstraint() != nil
}

// parserDefinitionNamesNoRMType reports whether root, the definition the
// parser reads, names no RM type and holds as many attributes as def, the
// model's definition, so that the compile step's refusal of def is that of
// the definition the parser reads.
func parserDefinitionNamesNoRMType(root template.Node, def *aom14.CArchetypeRoot) bool {
	on, ok := root.(template.ObjectNode)
	return ok && def != nil && on.RMTypeName() == "" && len(on.Attributes()) == len(def.Attributes)
}

// sourceObject is a <definition> or a <children> of an OPT, read by the
// parity check itself to tell an <attributes> with no type from a typed one:
// the <attributes> it holds, in document order, matched as the parser matches
// them, so that the parser's attributes of a node are these, one for one.
type sourceObject struct {
	Attributes []sourceAttribute `xml:"attributes"`
}

// sourceAttribute is an <attributes> of an OPT: its type in the XSI
// namespace, whether it carries a <cardinality>, and its <children>.
type sourceAttribute struct {
	Type        string         `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
	Cardinality *struct{}      `xml:"cardinality"`
	Children    []sourceObject `xml:"children"`
}

// untypedWithCardinality reports whether a names no type in the XSI
// namespace and carries a <cardinality>: the <attributes> the reader reads
// as C_MULTIPLE_ATTRIBUTE with that cardinality and the parser as
// C_SINGLE_ATTRIBUTE without it.
func (a *sourceAttribute) untypedWithCardinality() bool {
	return a != nil && strings.TrimSpace(a.Type) == "" && a.Cardinality != nil
}

// readSource returns the <definition> of raw as the parity check reads it,
// or nil when raw holds none.
func readSource(raw []byte) (*sourceObject, error) {
	var doc struct {
		Definition *sourceObject `xml:"definition"`
	}
	if err := xml.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &doc); err != nil {
		return nil, err
	}
	return doc.Definition, nil
}

// readerRefusal tells which reader refusal err is, root being the definition
// the parser reads: an integer the model's 32-bit Integer cannot hold, the
// reader's ParseInt range refusal; a match_negated of true; an infinite
// integer bound that admits no integer, the reader's ParseInt syntax refusal
// of a lower or upper bound whose text strconv.ParseFloat reads as the
// infinity no integer is above or below; a definition the parser reads as a
// slot or a primitive constraint, the reader's type mismatch refusal of the
// definition itself where root is one (parserDefinitionNotKept); or another.
func readerRefusal(err error, root template.Node) parityKind {
	if de := innermostDecodeError(err); de != nil && de.Path == "/definition" && errors.Is(de.Inner, typereg.ErrTypeMismatch) {
		if parserDefinitionNotKept(root) {
			return parityDefinitionNotKept
		}
		return parityRefused
	}
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
	if de := innermostDecodeError(err); de != nil {
		return de.Path
	}
	return ""
}

// innermostDecodeError returns the innermost decode error in err's chain, or
// nil when err holds none.
func innermostDecodeError(err error) *canxml.DecodeError {
	var inner *canxml.DecodeError
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return inner
		}
		inner, err = de, de.Inner
	}
}

// compileRefusal tells which compile refusal err is, root being the
// definition the parser reads and def the model's: two nodes under different
// attributes that share an AQL path; a definition that names no RM type, the
// compile step's refusal of a definition that holds something but no RM
// type, or of one that holds nothing, which names none either, where the
// parser reads root so (parserDefinitionNamesNoRMType); or another.
func compileRefusal(err error, root template.Node, def *aom14.CArchetypeRoot) parityKind {
	if !errors.Is(err, ErrInvalidInput) {
		return parityRefused
	}
	if strings.Contains(err.Error(), ": duplicate AQL path ") {
		return paritySharedPath
	}
	msg, ok := strings.CutPrefix(err.Error(), ErrInvalidInput.Error()+": ")
	if ok && (msg == "template definition names no RM type" || msg == "template has no definition") && parserDefinitionNamesNoRMType(root, def) {
		return parityNoRMType
	}
	return parityRefused
}

// compare compares parsed, a node of the parser's tree, with compiled, the
// compiled node that stands where it stands, model being the node of the
// model compiled reads and src the source element both read, then their
// descendants. The compiled tree lists the attributes the template declares
// first, in document order, and those the compile step injects after them,
// which hold no child. It records each difference of a node or an attribute
// in r.mismatches, and returns an error when the trees have different shapes.
func (r *parityResult) compare(parsed template.Node, model aom14.CObject, compiled *CompiledNode, src *sourceObject) error {
	where := fmt.Sprintf("%s (%s)", compiled.aqlPath, compiled.rmTypeName)
	r.compareNode(where, parsed, compiled)
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
	if len(parsedAttrs) > 0 && (src == nil || len(src.Attributes) != len(parsedAttrs)) {
		return fmt.Errorf("at %s the parser reads %d attributes and the parity check %d <attributes> in the source", where, len(parsedAttrs), len(sourceAttributes(src)))
	}
	for i, pa := range parsedAttrs {
		_, modelKids := modelChildren(modelAttrs[i])
		ca := compiled.attributes[i]
		sa := &src.Attributes[i]
		parsedKids := pa.Children()
		if ca.name != pa.Name() || len(ca.children) != len(parsedKids) || len(modelKids) != len(parsedKids) || len(sa.Children) != len(parsedKids) {
			return fmt.Errorf("at %s attribute %d: the parser reads %s with %d children, the model holds %d, the source %d and the compiled tree reads %s with %d",
				where, i+1, pa.Name(), len(parsedKids), len(modelKids), len(sa.Children), ca.name, len(ca.children))
		}
		r.compareAttribute(where+" attribute "+pa.Name(), pa, ca, sa)
		for j, pc := range parsedKids {
			if err := r.compare(pc, modelKids[j], ca.children[j], &sa.Children[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

// sourceAttributes returns the <attributes> of src, or nil for a nil src.
func sourceAttributes(src *sourceObject) []sourceAttribute {
	if src == nil {
		return nil
	}
	return src.Attributes
}

// compareNode records, at where, each of the RM type, node id, archetype id,
// occurrences and slot rules of compiled, and whether it is a slot, that
// differs from that of parsed. An interval is compared by its closed ends,
// an unbounded side by its being unbounded, and slot rules by their RM type,
// the patterns of their includes and excludes and the count of includes
// written. The raw assertion text is not compared: the parser keeps it and
// the compile step does not.
func (r *parityResult) compareNode(where string, parsed template.Node, compiled *CompiledNode) {
	differ := func(what, got, want string) {
		if got != want {
			r.mismatches = append(r.mismatches, fmt.Sprintf("%s: compiled %s %s; the parser reads %s", where, what, got, want))
		}
	}
	differ("RM type", strconv.Quote(compiled.rmTypeName), strconv.Quote(parsed.RMTypeName()))
	differ("node id", strconv.Quote(compiled.nodeID), strconv.Quote(parsed.NodeID()))
	var archetypeID string
	if root, ok := parsed.(*template.ArchetypeRoot); ok {
		archetypeID = root.ArchetypeID()
	}
	differ("archetype id", strconv.Quote(compiled.archetypeID), strconv.Quote(archetypeID))
	var occurrences *template.Multiplicity
	if on, ok := parsed.(template.ObjectNode); ok {
		occurrences = on.Occurrences()
	}
	differ("occurrences", renderMultiplicity(compiled.occurrences), renderMultiplicity(occurrences))
	slot, isSlot := parsed.(*template.Slot)
	differ("slot", strconv.FormatBool(compiled.slot != nil), strconv.FormatBool(isSlot))
	if isSlot && compiled.slot != nil {
		r.slots++
		differ("slot rules", renderSlotRules(compiled.slot.rules), renderSlotRules(slot.SlotRules()))
	}
}

// compareAttribute records, at where, the existence and the cardinality of
// compiled that differ from those of parsed, src being the <attributes> both
// read. The cardinality is compared as single or multiple and the interval
// of a multiple one. An <attributes> that names no type and carries a
// <cardinality>, which the parser reads as single without it and the compile
// step as multiple with it, is the difference the rule allows, and is counted
// in r.untypedCardinalities.
func (r *parityResult) compareAttribute(where string, parsed *template.Attribute, compiled *CompiledAttribute, src *sourceAttribute) {
	if g, w := renderMultiplicity(compiled.existence), renderMultiplicity(parsed.Existence()); g != w {
		r.mismatches = append(r.mismatches, fmt.Sprintf("%s: compiled existence %s; the parser reads %s", where, g, w))
	}
	g := compiled.cardinality.String() + " " + renderMultiplicity(compiled.childMultiplicity)
	w := parsed.Cardinality().String() + " " + renderMultiplicity(parsed.ChildMultiplicity())
	if g == w {
		return
	}
	if src.untypedWithCardinality() && parsed.Cardinality() == template.Single && parsed.ChildMultiplicity() == nil && compiled.cardinality == template.Multiple {
		r.untypedCardinalities++
		return
	}
	r.mismatches = append(r.mismatches, fmt.Sprintf("%s: compiled cardinality %s; the parser reads %s", where, g, w))
}

// renderMultiplicity renders m as its two closed ends, * for an unbounded
// side, or "none" for a nil m.
func renderMultiplicity(m *template.Multiplicity) string {
	if m == nil {
		return "none"
	}
	lower, upper := "*", "*"
	if !m.LowerUnbounded() {
		lower = strconv.Itoa(m.Lower())
	}
	if !m.UpperUnbounded() {
		upper = strconv.Itoa(m.Upper())
	}
	return lower + ".." + upper
}

// renderSlotRules renders slot rules as their RM type, the patterns of their
// includes and excludes, and the count of includes written.
func renderSlotRules(rules constraints.SlotRules) string {
	return fmt.Sprintf("%q includes %q excludes %q of %d written", rules.RMTypeName,
		slotAssertionPatterns(rules.Includes), slotAssertionPatterns(rules.Excludes), rules.RawIncludeCount)
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
// parser reads one, the new path reads and compiles it with each node and
// attribute as the parser reads it, or refuses it for a reason the rule
// allows, told by its error. The parser reads every corpus template and every
// parity fixture, the corpus compiles each kind of primitive constraint and
// some slots, a fixture holds an <attributes> with no type that carries a
// <cardinality>, and each fixture of testdata/parity-exceptions is refused
// for one of the reasons the rule allows, each reason by one fixture at
// least.
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
		{name: "parity", files: glob(parityFixtureDir), parserReadsAll: true},
		{name: "reader", files: glob(readerFixtureDir)},
		{name: "exception", files: glob(parityExceptionDir), parserReadsAll: true, exceptions: true},
	}

	// The files are walked in one test, not one subtest each, so that the
	// floors below always see the whole walk.
	corpusSeen := map[string]int{}
	allowed := map[parityKind]int{}
	var corpusSlots, untypedCardinalities int
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
			untypedCardinalities += r.untypedCardinalities
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
			corpusSlots += r.slots
			for _, where := range r.bareCodeReferences {
				t.Errorf("%s: %s: a corpus C_CODE_REFERENCE with no reference set URI", file, where)
			}
		}
		if !src.exceptions && held == 0 {
			t.Errorf("no operational template of the %s source holds the rule; want at least one", src.name)
		}
	}
	for _, k := range allowedKinds {
		if allowed[k] == 0 {
			t.Errorf("no operational template is refused as %q; want a fixture for each exception the rule allows", k)
		}
	}
	// A walk that compared no slot's rules, or that never met the
	// cardinality the rule allows to differ, cannot pass unseen.
	if corpusSlots == 0 {
		t.Errorf("no corpus slot had its rules compared; the parity walk pairs no slot")
	}
	if untypedCardinalities == 0 {
		t.Errorf("no <attributes> with no type that carries a <cardinality> was compared; want a fixture that holds one")
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
// the rule allows from one it does not, by its error: an integer past
// the 32-bit range is the reader's ParseInt range refusal, a match_negated of
// true is the reader's refusal of that element, an infinite integer bound
// that admits no integer is the reader's ParseInt syntax refusal of a lower
// bound of positive infinity or an upper one of negative infinity, a
// definition the parser reads as a slot or a primitive constraint is the
// reader's type mismatch refusal of the definition, a shared AQL path is the
// compile step's duplicate-path refusal, and a definition that names no RM
// type is the compile step's refusal of it or of a definition that holds
// nothing. A refusal that shares part of one of these, such as a real out of
// range, another element's refusal, an infinity on the side it admits every
// integer or a type mismatch below the definition, breaks the rule. A
// refusal of what the parser reads at the definition counts only where the
// parser reads the definition so: as a slot or a leaf that carries a
// primitive constraint for the reader's, and as a node of no RM type that
// holds as many attributes as the model's definition for the compile step's.
func TestREQ111_ParityRefusalsByError(t *testing.T) {
	const (
		slot          = `<definition xsi:type="ARCHETYPE_SLOT"><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id></definition>`
		untypedSlot   = `<definition xsi:type="ARCHETYPE_SLOT"><node_id>at0000</node_id></definition>`
		stringLeaf    = `<definition xsi:type="C_STRING"><rm_type_name>STRING</rm_type_name><list>a</list></definition>`
		futureLeaf    = `<definition xsi:type="C_FUTURE_OBJECT"><rm_type_name>CLUSTER</rm_type_name></definition>`
		cluster       = `<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id></definition>`
		untypedObject = `<definition><node_id>at0000</node_id><attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name></attributes></definition>`
		empty         = `<definition/>`
	)
	// root is the definition the parser reads of definition, or nil for "".
	root := func(t *testing.T, definition string) template.Node {
		t.Helper()
		if definition == "" {
			return nil
		}
		parsed, err := template.ParseOPT(bytes.NewReader(inlineOPT(definition)))
		if err != nil {
			t.Fatalf("template.ParseOPT(%s) = %v", definition, err)
		}
		return parsed.Root()
	}
	mismatch := func(typ string) error {
		return &canxml.DecodeError{Path: "/definition", Type: typ, Inner: typereg.ErrTypeMismatch}
	}
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
		// definition is the definition the parser reads; "" for none.
		definition string
		want       parityKind
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
		{name: "a definition the parser reads as a slot", err: mismatch("ARCHETYPE_SLOT"), definition: slot, want: parityDefinitionNotKept},
		{name: "a definition the parser reads as a string constraint", err: mismatch("C_STRING"), definition: stringLeaf, want: parityDefinitionNotKept},
		{name: "a definition the parser reads as a complex object", err: mismatch("ARCHETYPE_SLOT"), definition: cluster, want: parityRefused},
		{name: "a definition the parser reads as a leaf of no constraint", err: mismatch("C_FUTURE_OBJECT"), definition: futureLeaf, want: parityRefused},
		{name: "a definition of an unknown type", err: &canxml.DecodeError{Path: "/definition", Type: "C_FUTURE", Inner: typereg.ErrUnknownType}, definition: slot, want: parityRefused},
		{name: "a type mismatch below the definition", err: &canxml.DecodeError{Path: "/definition", Inner: &canxml.DecodeError{Path: "/attributes", Type: "C_STRING", Inner: typereg.ErrTypeMismatch}}, definition: slot, want: parityRefused},
		{name: "a type mismatch of another element", err: &canxml.DecodeError{Path: "/children", Type: "C_STRING", Inner: typereg.ErrTypeMismatch}, definition: slot, want: parityRefused},
		{name: "a type mismatch that is no decode error", err: fmt.Errorf("definition: %w", typereg.ErrTypeMismatch), definition: slot, want: parityRefused},
	}
	for _, tc := range readerCases {
		if got := readerRefusal(tc.err, root(t, tc.definition)); got != tc.want {
			t.Errorf("readerRefusal(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}

	noRMType := fmt.Errorf("%w: template definition names no RM type", ErrInvalidInput)
	compileCases := []struct {
		name string
		err  error
		// definition is the definition the parser reads; "" for none.
		definition string
		// modelAttributes is how many attributes the model's definition
		// holds.
		modelAttributes int
		want            parityKind
	}{
		{name: "a shared AQL path", err: fmt.Errorf(`%w: duplicate AQL path "/category" (existing DV_CODED_TEXT, new DV_CODED_TEXT)`, ErrInvalidInput), want: paritySharedPath},
		{name: "another invalid input", err: fmt.Errorf("%w: template has no template_id", ErrInvalidInput), want: parityRefused},
		{name: "a shared AQL path that is no invalid input", err: errors.New(`templatecompile: duplicate AQL path "/category"`), want: parityRefused},
		{name: "a definition that names no RM type", err: noRMType, definition: untypedObject, modelAttributes: 1, want: parityNoRMType},
		{name: "a definition that holds nothing", err: fmt.Errorf("%w: template has no definition", ErrInvalidInput), definition: empty, want: parityNoRMType},
		{name: "a definition the parser reads with an RM type", err: noRMType, definition: cluster, want: parityRefused},
		{name: "a definition the parser reads with an attribute the model's does not hold", err: noRMType, definition: untypedObject, want: parityRefused},
		{name: "a definition the parser reads as a slot", err: noRMType, definition: untypedSlot, want: parityRefused},
		{name: "a definition that names no RM type that is no invalid input", err: errors.New("templatecompile: invalid input: template definition names no RM type"), definition: empty, want: parityRefused},
		{name: "a node below the definition that names no RM type", err: fmt.Errorf("%w: at /content: template definition names no RM type", ErrInvalidInput), definition: empty, want: parityRefused},
		{name: "a definition refusal wrapped in another", err: fmt.Errorf("definition: %w", noRMType), definition: empty, want: parityRefused},
	}
	for _, tc := range compileCases {
		def := &aom14.CArchetypeRoot{}
		def.Attributes = make([]aom14.CAttribute, tc.modelAttributes)
		if got := compileRefusal(tc.err, root(t, tc.definition), def); got != tc.want {
			t.Errorf("compileRefusal(%s: %v) = %q; want %q", tc.name, tc.err, got, tc.want)
		}
	}
}

// parityInputs reads raw with the deprecated parser, the model's reader and
// the compile step, each in its default mode, and as the parity check reads
// the source itself, and fails the test when one of them refuses it.
func parityInputs(t *testing.T, raw []byte) (template.Node, *aom14.OperationalTemplate, *Compiled, *sourceObject) {
	t.Helper()
	parsed, err := template.ParseOPT(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("template.ParseOPT = %v", err)
	}
	model, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("aom14.ReadOperationalTemplate = %v", err)
	}
	c, err := Compile(model)
	if err != nil {
		t.Fatalf("Compile = %v", err)
	}
	src, err := readSource(raw)
	if err != nil {
		t.Fatalf("readSource = %v", err)
	}
	return parsed.Root(), model, c, src
}

// firstNode returns the first node at or below n, depth first, that match
// holds for, or nil when none does.
func firstNode(n *CompiledNode, match func(*CompiledNode) bool) *CompiledNode {
	if match(n) {
		return n
	}
	for _, a := range n.attributes {
		for _, child := range a.children {
			if found := firstNode(child, match); found != nil {
				return found
			}
		}
	}
	return nil
}

// firstAttribute returns the first attribute the template declares at or
// below n, depth first, that match holds for, or nil when none does.
func firstAttribute(n *CompiledNode, match func(*CompiledAttribute) bool) *CompiledAttribute {
	for _, a := range n.attributes {
		if !a.implicit && match(a) {
			return a
		}
		for _, child := range a.children {
			if found := firstAttribute(child, match); found != nil {
				return found
			}
		}
	}
	return nil
}

// TestREQ111_ParityCompareFindsEachDifference pins that the parity walk
// reports each field it compares: over a corpus template whose compiled tree
// is the parser's, a change to one field of one compiled node or attribute
// is reported as a difference of that field.
func TestREQ111_ParityCompareFindsEachDifference(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "templates", "cluster-slot.ehrbase.org.v0.opt"))
	if err != nil {
		t.Fatal(err)
	}
	node := func(t *testing.T, root *CompiledNode, what string, match func(*CompiledNode) bool) *CompiledNode {
		t.Helper()
		n := firstNode(root, match)
		if n == nil {
			t.Fatalf("the compiled tree holds no %s", what)
		}
		return n
	}
	attribute := func(t *testing.T, root *CompiledNode, what string, match func(*CompiledAttribute) bool) *CompiledAttribute {
		t.Helper()
		a := firstAttribute(root, match)
		if a == nil {
			t.Fatalf("the compiled tree holds no %s", what)
		}
		return a
	}
	cases := []struct {
		name string
		// want is the field the difference names; "" wants none.
		want   string
		change func(t *testing.T, root *CompiledNode)
	}{
		{name: "nothing changed"},
		{name: "an RM type", want: "RM type", change: func(t *testing.T, root *CompiledNode) {
			node(t, root, "node", func(*CompiledNode) bool { return true }).rmTypeName += "_CHANGED"
		}},
		{name: "a node id", want: "node id", change: func(t *testing.T, root *CompiledNode) {
			node(t, root, "node with a node id", func(n *CompiledNode) bool { return n.nodeID != "" }).nodeID = "at9999"
		}},
		{name: "an archetype id", want: "archetype id", change: func(t *testing.T, root *CompiledNode) {
			node(t, root, "archetype root", func(n *CompiledNode) bool { return n.archetypeID != "" }).archetypeID += ".changed"
		}},
		{name: "occurrences", want: "occurrences", change: func(t *testing.T, root *CompiledNode) {
			node(t, root, "node with occurrences", func(n *CompiledNode) bool { return n.occurrences != nil }).occurrences = nil
		}},
		{name: "a slot", want: "slot", change: func(t *testing.T, root *CompiledNode) {
			node(t, root, "slot", func(n *CompiledNode) bool { return n.slot != nil }).slot = nil
		}},
		{name: "slot rules", want: "slot rules", change: func(t *testing.T, root *CompiledNode) {
			n := node(t, root, "slot with includes", func(n *CompiledNode) bool { return n.slot != nil && len(n.slot.rules.Includes) > 0 })
			n.slot.rules.Includes = n.slot.rules.Includes[1:]
		}},
		{name: "an existence", want: "existence", change: func(t *testing.T, root *CompiledNode) {
			attribute(t, root, "attribute with an existence", func(a *CompiledAttribute) bool { return a.existence != nil }).existence = nil
		}},
		{name: "single or multiple", want: "cardinality", change: func(t *testing.T, root *CompiledNode) {
			attribute(t, root, "multiple attribute", func(a *CompiledAttribute) bool { return a.cardinality == template.Multiple }).cardinality = template.Single
		}},
		{name: "a cardinality interval", want: "cardinality", change: func(t *testing.T, root *CompiledNode) {
			attribute(t, root, "attribute with a cardinality interval", func(a *CompiledAttribute) bool { return a.childMultiplicity != nil }).childMultiplicity = nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, model, c, src := parityInputs(t, raw)
			if tc.change != nil {
				tc.change(t, c.root)
			}
			r := parityResult{seen: map[string]int{}}
			if err := r.compare(parsed, &model.Definition, c.root, src); err != nil {
				t.Fatalf("compare = %v; want the trees of one shape", err)
			}
			if tc.want == "" {
				if len(r.mismatches) > 0 {
					t.Errorf("compare found %q; want no difference", r.mismatches)
				}
				return
			}
			if !slices.ContainsFunc(r.mismatches, func(m string) bool { return strings.Contains(m, ": compiled "+tc.want+" ") }) {
				t.Errorf("compare found %q; want a difference of the %s", r.mismatches, tc.want)
			}
		})
	}
}

// inlineOPT returns an operational template whose definition is definition.
func inlineOPT(definition string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <template_id><value>parity.inline.v1</value></template_id>
  <concept>Parity</concept>
  ` + definition + `
</template>`)
}

// TestREQ111_ParityAllowsOnlyAnUntypedCardinality pins the one cardinality
// the parity rule lets differ: an <attributes> that names no type and
// carries a <cardinality>, which the parser reads as single without it and
// the compile step as multiple with it. The same difference at an
// <attributes> typed C_SINGLE_ATTRIBUTE, or at one with no <cardinality>,
// breaks the rule.
func TestREQ111_ParityAllowsOnlyAnUntypedCardinality(t *testing.T) {
	raw := inlineOPT(`<definition>
    <rm_type_name>CLUSTER</rm_type_name>
    <occurrences><lower>1</lower><upper>1</upper></occurrences>
    <node_id>at0000</node_id>
    <attributes>
      <rm_attribute_name>items</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>ELEMENT</rm_type_name>
        <occurrences><lower>0</lower><upper>1</upper></occurrences>
        <node_id>at0001</node_id>
      </children>
      <cardinality>
        <is_ordered>false</is_ordered>
        <is_unique>false</is_unique>
        <interval><lower>0</lower><upper_unbounded>true</upper_unbounded></interval>
      </cardinality>
    </attributes>
    <archetype_id><value>openEHR-EHR-CLUSTER.parity.v1</value></archetype_id>
  </definition>`)
	if r := checkParity(raw); r.kind != parityHolds || r.untypedCardinalities != 1 {
		t.Fatalf("checkParity = %s, %d untyped cardinalities (%v); want it held with 1", r.kind, r.untypedCardinalities, r.err)
	}
	cases := []struct {
		name   string
		change func(*sourceAttribute)
	}{
		{name: "typed C_SINGLE_ATTRIBUTE", change: func(a *sourceAttribute) { a.Type = "C_SINGLE_ATTRIBUTE" }},
		{name: "with no cardinality", change: func(a *sourceAttribute) { a.Cardinality = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, model, c, src := parityInputs(t, raw)
			tc.change(&src.Attributes[0])
			r := parityResult{seen: map[string]int{}}
			if err := r.compare(parsed, &model.Definition, c.root, src); err != nil {
				t.Fatalf("compare = %v; want the trees of one shape", err)
			}
			if r.untypedCardinalities != 0 || !slices.ContainsFunc(r.mismatches, func(m string) bool { return strings.Contains(m, ": compiled cardinality ") }) {
				t.Errorf("compare found %q and %d untyped cardinalities; want a difference of the cardinality and none allowed", r.mismatches, r.untypedCardinalities)
			}
		})
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
	var files, skipped, untypedCardinalities int
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
		untypedCardinalities += r.untypedCardinalities
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
	say("  the compiled tree differs from the parser's: %d", counts[parityMismatch])
	say("  <attributes> with no type and a <cardinality>, read as multiple: %d", untypedCardinalities)
	for _, k := range allowedKinds {
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
