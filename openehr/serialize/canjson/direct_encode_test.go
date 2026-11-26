package canjson_test

// direct_encode_test.go: every generated MarshalJSONTo writes its class's
// members itself, as tokens, unless the encoder carries an option under which
// json/v2 would write a member otherwise; it then marshals the wrapper struct
// it always had (ADR 0022 § Encode writes each member itself). The REQ-052
// wire profile is unchanged by it: these tests encode every value both ways
// and hold the two to the same bytes, and a failure to the same words.
//
// forceWrapper sends every generated encoder to its wrapper, since a caller's
// marshalers are an option the direct writer leaves to json/v2, and changes
// no byte: it marshals a type no value holds.

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// heldByNoValue is a type no value of the model holds.
type heldByNoValue struct{}

// forceWrapper is a caller's marshaler for [heldByNoValue]: it changes no
// byte, and sends every generated MarshalJSONTo to its wrapper.
var forceWrapper = json.WithMarshalers(json.MarshalToFunc(func(*jsontext.Encoder, heldByNoValue) error {
	return errors.New("heldByNoValue is never encoded")
}))

// encoded is one encode's result: its bytes, or the text of its failure.
type encoded struct {
	out []byte
	err string
}

// encode marshals v with the canonical-JSON options and opts.
func encode(v any, opts ...json.Options) encoded {
	out, err := json.Marshal(v, append([]json.Options{typereg.EncodeOptions()}, opts...)...)
	if err != nil {
		return encoded{err: err.Error()}
	}
	return encoded{out: out}
}

// sameAsWrapper encodes v as canjson.Marshal does and through the wrappers,
// both under opts, and describes how the two differ, or returns "".
func sameAsWrapper(v any, opts ...json.Options) string {
	direct := encode(v, opts...)
	wrapped := encode(v, append(slices.Clone(opts), forceWrapper)...)
	return describeDifference(direct, wrapped)
}

// describeDifference describes how direct differs from wrapped, or returns "".
func describeDifference(direct, wrapped encoded) string {
	if direct.err != wrapped.err {
		return fmt.Sprintf("failure differs:\n  direct:  %q\n  wrapper: %q", direct.err, wrapped.err)
	}
	if bytes.Equal(direct.out, wrapped.out) {
		return ""
	}
	at := 0
	for at < len(direct.out) && at < len(wrapped.out) && direct.out[at] == wrapped.out[at] {
		at++
	}
	from := max(at-60, 0)
	return fmt.Sprintf("bytes differ at offset %d:\n  direct:  …%s\n  wrapper: …%s",
		at, direct.out[from:min(at+60, len(direct.out))], wrapped.out[from:min(at+60, len(wrapped.out))])
}

// minDirectEncodeDocuments is a floor on the corpus documents the identity test
// compares: a skip rule that stopped matching would otherwise empty it. Over
// 100 documents decode today.
const minDirectEncodeDocuments = 90

// TestREQ052DirectEncodeCorpusBytes encodes every canonical-JSON document of
// the corpus that decodes, the compositions and the other RM values, both
// ways, compact and indented, and holds the direct writer to the wrapper's
// bytes.
func TestREQ052DirectEncodeCorpusBytes(t *testing.T) {
	t.Parallel()
	root := fixtures.CorpusRoot()
	var compared int
	kinds := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var head struct {
			Type string `json:"_type"`
		}
		if json.Unmarshal(raw, &head) != nil || head.Type == "" {
			return nil
		}
		ctor, ok := typereg.Default.Lookup(head.Type)
		if !ok {
			return nil
		}
		v := ctor()
		if canjson.Unmarshal(raw, v) != nil {
			return nil
		}
		rel := relativeToRoot(root, path)
		for _, layout := range []struct {
			name string
			opts []json.Options
		}{
			{"compact", nil},
			{"indented", []json.Options{jsontext.WithIndent("  ")}},
			{"indented with a prefix", []json.Options{jsontext.WithIndentPrefix("\t"), jsontext.WithIndent("\t")}},
		} {
			if diff := sameAsWrapper(v, layout.opts...); diff != "" {
				t.Errorf("%s, %s: %s", rel, layout.name, diff)
			}
		}
		compared++
		kinds[head.Type] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if compared < minDirectEncodeDocuments {
		t.Errorf("compared %d corpus documents, want at least %d", compared, minDirectEncodeDocuments)
	}
	for _, kind := range []string{"COMPOSITION", "EHR_STATUS", "FOLDER"} {
		if !kinds[kind] {
			t.Errorf("no %s document was compared; the corpus walk lost a kind it held", kind)
		}
	}
}

// minDirectEncodeTemplates is a floor on the operational templates the AOM 1.4
// identity test compares. Over 60 are vendored today.
const minDirectEncodeTemplates = 50

// TestREQ052DirectEncodeOperationalTemplates encodes every operational
// template of the corpus, which the corpus holds as XML only, both ways,
// compact and indented, and holds the direct writer to the wrapper's bytes.
func TestREQ052DirectEncodeOperationalTemplates(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	var compared int
	for _, ref := range refs {
		raw, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("read %s: %v", ref.Name, err)
		}
		opt, err := fixtures.ReadOperationalTemplate(raw)
		if err != nil {
			continue
		}
		for _, layout := range [][]json.Options{nil, {jsontext.WithIndent("  ")}} {
			if diff := sameAsWrapper(opt, layout...); diff != "" {
				t.Errorf("%s: %s", ref.Name, diff)
			}
		}
		compared++
	}
	if compared < minDirectEncodeTemplates {
		t.Errorf("compared %d operational templates, want at least %d", compared, minDirectEncodeTemplates)
	}
}

// TestREQ052DirectEncodeEveryClass builds a value of every class the type
// registry holds, with every member set, with every member zero, with every
// member typed by an interface holding a nil pointer, with every list and map
// empty but present and every string empty, and, for an interval, with both
// sides open and their bounds empty, and holds the direct writer to the
// wrapper's bytes for each. The corpus leaves classes and members unset; this
// reaches each member of each class, so a member written out of order, or left
// out by another rule than json/v2's, is caught.
func TestREQ052DirectEncodeEveryClass(t *testing.T) {
	t.Parallel()
	f := newClassFiller()
	var compared int
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		typ := reflect.TypeOf(ctor()).Elem()
		for _, variant := range []string{"filled", "zero", "typed-nil", "empty", "open interval"} {
			v := reflect.New(typ)
			switch variant {
			case "filled":
				f.fillStruct(v.Elem(), 0, fillValues)
			case "typed-nil":
				f.fillStruct(v.Elem(), 0, fillTypedNil)
			case "empty":
				f.fillStruct(v.Elem(), 0, fillEmpty)
			case "open interval":
				if !openInterval(v.Elem()) {
					continue
				}
			}
			direct := encode(v.Interface())
			if diff := describeDifference(direct, encode(v.Interface(), forceWrapper)); diff != "" {
				t.Errorf("%s, %s: %s", name, variant, diff)
			}
			if diff := sameAsWrapper(v.Interface(), jsontext.WithIndent("  ")); diff != "" {
				t.Errorf("%s, %s, indented: %s", name, variant, diff)
			}
			// A filled value that fails compares one failure, not its
			// members: the filler must build values that encode.
			if variant != "zero" && direct.err != "" {
				t.Errorf("%s, %s: the value does not encode, so its members go unchecked: %s", name, variant, direct.err)
			}
			compared++
		}
	}
	if compared < 5*100 {
		t.Errorf("compared %d values, want at least one per variant of each of 100 registered classes", compared)
	}
}

// fillMode is how [classFiller] sets a member.
type fillMode int

const (
	// fillValues sets every member to a value: a list or map of one
	// element, and an interface holding a class.
	fillValues fillMode = iota
	// fillTypedNil is fillValues, except that an interface holds a nil
	// pointer to a class.
	fillTypedNil
	// fillEmpty sets every list and map to an empty one, every string but
	// a Character to "", every boolean to false and every interface to nil.
	fillEmpty
)

// classFiller builds values with every member set.
type classFiller struct {
	registered []reflect.Type // the struct types the registry constructs, by name
}

// newClassFiller returns a filler over the classes typereg.Default holds.
func newClassFiller() *classFiller {
	var f classFiller
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		f.registered = append(f.registered, reflect.TypeOf(ctor()).Elem())
	}
	return &f
}

// maxFillDepth bounds how far down the filler sets a pointer, list, map or
// interface member: the model's classes nest each other through them without
// end.
const maxFillDepth = 4

var (
	characterType   = reflect.TypeFor[rm.Character]()
	textMarshalType = reflect.TypeFor[interface{ MarshalText() ([]byte, error) }]()
)

// fillStruct sets every exported member of v, a struct, as mode says.
func (f *classFiller) fillStruct(v reflect.Value, depth int, mode fillMode) {
	for i := range v.NumField() {
		field := v.Type().Field(i)
		if !field.IsExported() && !field.Anonymous {
			continue
		}
		f.fill(v.Field(i), depth, mode)
	}
}

// fill sets v, a settable value, as mode says.
func (f *classFiller) fill(v reflect.Value, depth int, mode fillMode) {
	t := v.Type()
	switch t.Kind() {
	case reflect.String:
		switch {
		case t == characterType:
			v.SetString("x")
		case mode != fillEmpty:
			v.SetString("s<&>")
		}
	case reflect.Bool:
		v.SetBool(mode != fillEmpty)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(validInt(t))
	case reflect.Uint8:
		v.SetUint(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(0.000000125)
	case reflect.Struct:
		// A struct held by value nests no deeper than its type, so it is
		// filled at any depth, its scalars included.
		f.fillStruct(v, depth+1, mode)
	case reflect.Pointer:
		if depth <= maxFillDepth {
			p := reflect.New(t.Elem())
			f.fill(p.Elem(), depth+1, mode)
			v.Set(p)
		}
	case reflect.Slice:
		switch {
		case mode == fillEmpty:
			v.Set(reflect.MakeSlice(t, 0, 0))
		case depth <= maxFillDepth:
			s := reflect.MakeSlice(t, 1, 1)
			f.fill(s.Index(0), depth+1, mode)
			v.Set(s)
		}
	case reflect.Map:
		switch {
		case mode == fillEmpty:
			v.Set(reflect.MakeMap(t))
		case depth <= maxFillDepth:
			m := reflect.MakeMap(t)
			e := reflect.New(t.Elem()).Elem()
			f.fill(e, depth+1, mode)
			m.SetMapIndex(reflect.ValueOf("k").Convert(t.Key()), e)
			v.Set(m)
		}
	case reflect.Interface:
		if depth > maxFillDepth || mode == fillEmpty {
			return
		}
		if t.NumMethod() == 0 {
			// A member typed Any: a CODE_PHRASE is a value every such
			// member holds, a choice of shapes or not.
			code := rm.CodePhrase{CodeString: "c", TerminologyID: rm.TerminologyID{Value: "local"}}
			v.Set(reflect.ValueOf(code))
			return
		}
		for _, impl := range f.registered {
			switch {
			case mode == fillTypedNil && reflect.PointerTo(impl).Implements(t):
				v.Set(reflect.Zero(reflect.PointerTo(impl)))
				return
			case mode == fillValues && impl.Implements(t):
				e := reflect.New(impl).Elem()
				f.fill(e, depth+1, mode)
				v.Set(e)
				return
			}
		}
	}
}

// validInt returns a value of t, an integer type, that it encodes: the first
// code of an enumeration, which encodes only its codes, or 1.
func validInt(t reflect.Type) int64 {
	if !t.Implements(textMarshalType) {
		return 1
	}
	for code := range int64(5000) {
		if _, err := reflect.ValueOf(code).Convert(t).Interface().(interface{ MarshalText() ([]byte, error) }).MarshalText(); err == nil {
			return code
		}
	}
	return 1
}

// openInterval sets v, a struct, to an interval with both sides open and
// their bounds empty, and reports whether v is an interval at all.
func openInterval(v reflect.Value) bool {
	lower, upper := v.FieldByName("LowerUnbounded"), v.FieldByName("UpperUnbounded")
	if !lower.IsValid() || !upper.IsValid() || !v.FieldByName("Lower").IsValid() {
		return false
	}
	lower.SetBool(true)
	upper.SetBool(true)
	return true
}

// TestREQ052DirectEncodeGatedOptions holds the direct writer to the wrapper's
// bytes, and its failures to the wrapper's words, under each encoder option
// that changes how json/v2 writes a member, each case on a value whose
// encoding the option changes. typereg.DirectEncode sends each such encode to
// the wrapper; a check it dropped would let the direct writer, which knows
// nothing of the option, write the value its own way, and the case fails.
func TestREQ052DirectEncodeGatedOptions(t *testing.T) {
	t.Parallel()
	codedText := &rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}}}
	typedNilValue := &rm.Element{ArchetypeNodeID: "at1", Name: rm.DVText{Value: "n"}, Value: (*rm.DVText)(nil)}
	type taggedString struct {
		Text rm.DVText `json:"text,string"` //nolint:staticcheck // the misplaced option is the case
	}
	type untagged struct {
		Text rm.DVText `json:"text"`
	}
	for _, tc := range []struct {
		name  string
		value any
		opt   json.Options
		// plain is the value without what the option acts on, when the
		// value carries it itself: the wrapper must write the two
		// differently, or the case checks nothing.
		plain any
	}{
		// An empty string the class writes, which the option leaves out.
		{name: "OmitZeroStructFields", value: codedText, opt: json.OmitZeroStructFields(true)},
		// A nil pointer in an interface member, which the legacy omitempty
		// writes as null.
		{name: "OmitEmptyWithLegacySemantics", value: typedNilValue, opt: jsonv1.OmitEmptyWithLegacySemantics(true)},
		// encoding/json's own defaults carry the legacy omitempty.
		{name: "encoding/json defaults", value: typedNilValue, opt: jsonv1.DefaultOptionsV1()},
		// A failure at depth, which the legacy error reporting wraps once
		// for each method on the way out.
		{name: "ReportErrorsWithLegacySemantics", value: nanAtDepth(), opt: jsonv1.ReportErrorsWithLegacySemantics(true)},
		// A class in a field tagged with the `string` option, which json/v2
		// refuses on the class's own encoding: json.GetOption reports the
		// tag as StringifyNumbers, which itself changes nothing a class
		// writes.
		{name: "the string tag option", value: &taggedString{Text: rm.DVText{Value: "x"}}, plain: &untagged{Text: rm.DVText{Value: "x"}}},
		{name: "StringifyNumbers with the string tag option", value: &taggedString{Text: rm.DVText{Value: "x"}}, opt: json.StringifyNumbers(true), plain: &untagged{Text: rm.DVText{Value: "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := sameAsWrapper(tc.value, tc.opt); diff != "" {
				t.Errorf("under %s: %s", tc.name, diff)
			}
			with, without := encode(tc.value, tc.opt, forceWrapper), encode(tc.value, forceWrapper)
			if tc.plain != nil {
				without = encode(tc.plain, tc.opt, forceWrapper)
			}
			if describeDifference(with, without) == "" {
				t.Errorf("%s changes nothing the wrapper writes for this value, so the case checks nothing", tc.name)
			}
		})
	}

	// A caller's marshaler for a class reaches every value of it, at any
	// depth, whichever way the outer classes encode.
	marked := json.WithMarshalers(json.MarshalToFunc(func(enc *jsontext.Encoder, c rm.CodePhrase) error {
		return enc.WriteToken(jsontext.String("marked " + c.CodeString))
	}))
	value := &rm.Element{ArchetypeNodeID: "at1", Name: &rm.DVCodedText{Value: "n", DefiningCode: rm.CodePhrase{CodeString: "c1"}}}
	got := encode(value, marked)
	if !bytes.Contains(got.out, []byte(`"defining_code":"marked c1"`)) {
		t.Errorf("a caller's marshaler for CODE_PHRASE did not reach the one inside an ELEMENT's name: %s %s", got.out, got.err)
	}
}

// nanAtDepth returns an ITEM_TREE whose one ELEMENT holds a DV_QUANTITY with
// a NaN magnitude, which JSON cannot spell.
func nanAtDepth() *rm.ItemTree {
	return itemTree(element(rm.DVQuantity{Magnitude: rm.Real(math.NaN()), Units: "kg"}))
}

// itemTree returns an ITEM_TREE holding items.
func itemTree(items ...rm.Item) *rm.ItemTree {
	return &rm.ItemTree{ArchetypeNodeID: "at1", Name: rm.DVText{Value: "tree"}, Items: items}
}

// element returns an ELEMENT holding value.
func element(value rm.DataValue) rm.Element {
	return rm.Element{ArchetypeNodeID: "at2", Name: rm.DVText{Value: "element"}, Value: value}
}

// TestREQ052DirectEncodeFailureText holds each failure of the direct writer to
// the words of the wrapper's: the Go type it names, the JSON Pointer it
// points at and the cause. json/v2 names the class whose method failed and
// points at where it failed, so the direct writer, whose methods call each
// other without json/v2 between them, settles each failure itself. The cases
// fail at depth, in a member and in a list element, before and after the
// failing method wrote anything, and in a class that a value outside the
// model sends to its wrapper.
func TestREQ052DirectEncodeFailureText(t *testing.T) {
	t.Parallel()
	cycle := &rm.Cluster{ArchetypeNodeID: "at1", Name: rm.DVText{Value: "c"}}
	cycle.Items = []rm.Item{cycle}
	mapped := &rm.DVCodedText{Value: "v", DefiningCode: rm.CodePhrase{CodeString: "c", TerminologyID: rm.TerminologyID{Value: "l"}}}
	mapped.Mappings = []rm.TermMapping{{Match: "=", Target: rm.CodePhrase{CodeString: "x"}, Purpose: mapped}}
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{
			"invalid UTF-8 in a member at depth", itemTree(rm.Element{ArchetypeNodeID: "at2", Name: rm.DVText{Value: "e\xff"}}),
			`marshal from Go rm.DVText: invalid UTF-8 within "/items/0/name/value"`,
		},
		{
			"invalid UTF-8 in a list element", &rm.ResourceDescriptionItem{Language: rm.CodePhrase{CodeString: "en"}, Keywords: []string{"ok", "\xff"}},
			`marshal from Go rm.ResourceDescriptionItem: invalid UTF-8 within "/keywords/1"`,
		},
		{
			"a NaN at depth", nanAtDepth(),
			`marshal from Go float64 within "/items/0/value/magnitude": unsupported value: NaN`,
		},
		{
			"an infinity in a number the class writes", itemTree(element(&rm.DVProportion{Numerator: rm.Real(math.Inf(-1)), Denominator: 1})),
			`unsupported value: -Inf`,
		},
		{
			"an invalid TERM_MAPPING.match", itemTree(element(rm.DVText{Value: "v", Mappings: []rm.TermMapping{{Match: "ab"}}})),
			`marshal from Go rm.Character within "/items/0/value/mappings/0/match": rm.Character: must be exactly one character, got 2`,
		},
		{
			"an AOM 1.4 Any member refused in a member", &aom14.ArchetypeSlot{NodeID: "at1", RMTypeName: "CLUSTER", Includes: []aom14.Assertion{{Expression: aom14.ExprBinaryOperator{LeftOperand: aom14.ExprLeaf{Item: 1.5}}}}},
			`marshal from Go aom14.ExprLeaf within "/includes/0/expression/left_operand": canjson: EXPR_LEAF.item holds float64`,
		},
		{
			"an AOM 1.4 Any member refused in a list element", &aom14.CDVOrdinal{NodeID: "at1", RMTypeName: "DV_ORDINAL", List: []aom14.Ordinal{{Value: 1}, {Value: 2, Symbol: 3.5}}},
			`marshal from Go aom14.Ordinal within "/list/1": canjson: ORDINAL.symbol holds float64`,
		},
		{
			"an AOM 1.4 Any member refused at the root", &aom14.ExprLeaf{Item: 1.5},
			`marshal from Go aom14.ExprLeaf: canjson: EXPR_LEAF.item holds float64`,
		},
		{
			"a failure in a class sent to its wrapper", itemTree(element(rm.DVInterval[*rm.DVQuantity]{
				Lower: &rm.DVQuantity{Magnitude: rm.Real(math.NaN()), Units: "kg"}, UpperUnbounded: true,
			})),
			`marshal from Go float64 within "/items/0/value/lower/magnitude": unsupported value: NaN`,
		},
		{"a cycle through a list", cycle, `encountered a cycle`},
		{"a cycle through a pointer member", mapped, `encountered a cycle`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			direct := encode(tc.value)
			if !strings.Contains(direct.err, tc.want) {
				t.Errorf("direct failure = %q, want it to contain %q", direct.err, tc.want)
			}
			if diff := describeDifference(direct, encode(tc.value, forceWrapper)); diff != "" {
				t.Error(diff)
			}
			_, err := canjson.Marshal(tc.value)
			if !errors.Is(err, canjson.ErrInvalidValue) {
				t.Errorf("canjson.Marshal = %v, want an error wrapping canjson.ErrInvalidValue", err)
			}
			if _, ok := errors.AsType[*json.SemanticError](err); !ok {
				t.Errorf("canjson.Marshal = %v, want the encoder's *json.SemanticError reachable through unwrapping", err)
			}
		})
	}
}

// TestREQ052DirectEncodeWriterFailure holds a failing writer's error, which
// both ways pass through unwrapped, to the same words.
func TestREQ052DirectEncodeWriterFailure(t *testing.T) {
	t.Parallel()
	payload, err := os.ReadFile(fixtures.CompositionJSON(benchFixture))
	if err != nil {
		t.Fatalf("read %s: %v", benchFixture, err)
	}
	var c rm.Composition
	if err := canjson.Unmarshal(payload, &c); err != nil {
		t.Fatalf("decode %s: %v", benchFixture, err)
	}
	write := func(opts ...json.Options) error {
		return json.MarshalWrite(failingWriter{}, &c, append([]json.Options{typereg.EncodeOptions()}, opts...)...)
	}
	direct, wrapped := write(), write(forceWrapper)
	if direct == nil || wrapped == nil || direct.Error() != wrapped.Error() {
		t.Fatalf("a failing writer: direct %v, wrapper %v, want the same failure", direct, wrapped)
	}
	if !errors.Is(direct, errWriteRefused) {
		t.Errorf("direct failure %v does not wrap the writer's error", direct)
	}
}

// errWriteRefused is the error [failingWriter] returns.
var errWriteRefused = errors.New("write refused")

// failingWriter refuses every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteRefused }

var _ io.Writer = failingWriter{}

// TestREQ052DirectEncodeAllocatesLess holds canjson.Marshal to fewer
// allocations than the wrappers make for the same composition. The direct
// writer copies no value into a wrapper, and calls the encoder of a value in
// an interface member without json/v2's copy of it; were DirectEncode false
// for canjson's own options, or a direct writer to move its receiver to the
// heap, the two counts would meet.
func TestREQ052DirectEncodeAllocatesLess(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes allocation counts vary")
	}
	payload, err := os.ReadFile(fixtures.CompositionJSON(benchFixture))
	if err != nil {
		t.Fatalf("read %s: %v", benchFixture, err)
	}
	var c rm.Composition
	if err := canjson.Unmarshal(payload, &c); err != nil {
		t.Fatalf("decode %s: %v", benchFixture, err)
	}
	direct := testing.AllocsPerRun(10, func() { _, _ = canjson.Marshal(&c) })
	wrapped := testing.AllocsPerRun(10, func() { _, _ = json.Marshal(&c, typereg.EncodeOptions(), forceWrapper) })
	if direct*3 > wrapped*2 {
		t.Errorf("canjson.Marshal of %s makes %.0f allocations and the wrappers %.0f; want the direct writer under two thirds of the wrappers'", benchFixture, direct, wrapped)
	}

	// A class whose members are tokens and classes, and one whose members
	// hold classes through interfaces, numbers and pointers: the direct
	// writer copies none of them to the heap, so a receiver moved there, or
	// an interface member sent to the wrapper, shows as an allocation.
	code := rm.CodePhrase{CodeString: "c", TerminologyID: rm.TerminologyID{Value: "local"}, PreferredTerm: new("term")}
	elem := rm.Element{
		ArchetypeNodeID: "at1",
		Name:            &rm.DVText{Value: "weight"},
		Value:           rm.DVQuantity{Magnitude: 80.5, Units: "kg", Precision: new(rm.Integer(1))},
		NullFlavour:     &rm.DVCodedText{Value: "unknown", DefiningCode: code},
	}
	enc := jsontext.NewEncoder(io.Discard, typereg.EncodeOptions())
	for name, v := range map[string]json.MarshalerTo{"CODE_PHRASE": code, "ELEMENT": elem} {
		if n := testing.AllocsPerRun(100, func() {
			enc.Reset(io.Discard, typereg.EncodeOptions())
			if err := v.MarshalJSONTo(enc); err != nil {
				t.Fatal(err)
			}
		}); n != 0 {
			t.Errorf("%s.MarshalJSONTo makes %.0f allocations, want 0: the direct writer copies nothing to the heap", name, n)
		}
	}
}

// TestREQ052DirectEncodeLeavesOutEmptyMembersAsTheWrapperDoes holds the two
// encodes to one rule for a mandatory member held by value: each value holds
// such a member empty, which both leave out, or with a part set, which both
// write. It covers a class of the alias shape, one of the flat shape
// (DV_CODED_TEXT) and a member typed by the class's own type parameter, bound
// to a struct and to an interface. Without the wrapper's marshalJSONOmitting
// the wrapper writes the empty member and the bytes differ.
//
// REQ-052 § An absent mandatory member.
func TestREQ052DirectEncodeLeavesOutEmptyMembersAsTheWrapperDoes(t *testing.T) {
	t.Parallel()
	name := rm.DVText{Value: "n"}
	for _, tc := range []struct {
		name   string
		v      any
		absent string // the member both leave out, or "" when both write it
	}{
		{name: "ROLE with an empty performer", v: &rm.Role{ArchetypeNodeID: "at0000", Name: name}, absent: "performer"},
		{name: "ROLE with a performer that has a type", v: &rm.Role{ArchetypeNodeID: "at0000", Name: name, Performer: rm.PartyRef{Type: "PERSON"}}},
		{name: "COMPOSITION with empty category, language and territory", v: &rm.Composition{ArchetypeNodeID: "at0000", Name: name}, absent: "category"},
		{name: "DV_CODED_TEXT with an empty defining code", v: &rm.DVCodedText{Value: "v"}, absent: "defining_code"},
		{name: "DV_CODED_TEXT with a code and no terminology", v: &rm.DVCodedText{Value: "v", DefiningCode: rm.CodePhrase{CodeString: "c"}}},
		{name: "OBSERVATION with an empty data", v: &rm.Observation{ArchetypeNodeID: "at0000", Name: name}, absent: "data"},
		{name: "POINT_EVENT[ItemTree] with an empty data", v: &rm.PointEvent[rm.ItemTree]{ArchetypeNodeID: "at0001", Name: name}, absent: "data"},
		{name: "POINT_EVENT[ItemStructure] holding an empty ITEM_TREE", v: &rm.PointEvent[rm.ItemStructure]{ArchetypeNodeID: "at0001", Name: name, Data: rm.ItemTree{}}},
	} {
		if diff := sameAsWrapper(tc.v); diff != "" {
			t.Errorf("%s: %s", tc.name, diff)
		}
		wrapped := encode(tc.v, forceWrapper)
		if wrapped.err != "" {
			t.Fatalf("%s: wrapper encode: %s", tc.name, wrapped.err)
		}
		var members map[string]jsonv1.RawMessage
		if err := jsonv1.Unmarshal(wrapped.out, &members); err != nil {
			t.Fatalf("%s: read %s: %v", tc.name, wrapped.out, err)
		}
		if tc.absent != "" {
			if raw, ok := members[tc.absent]; ok {
				t.Errorf("%s: the wrapper writes %q as %s, want it left out", tc.name, tc.absent, raw)
			}
		}
	}
}
