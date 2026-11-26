package canjson_test

// direct_decode_test.go: every generated UnmarshalJSONFrom reads its class's
// members itself, token by token, each into the receiver's own field, unless
// the decoder carries an option under which json/v2's struct decode would
// read a member otherwise; it then decodes the wrapper struct it always had
// (typereg.DirectDecode). The REQ-052 decode contract is unchanged by it:
// these tests decode every input both ways, as a whole buffer and as a
// stream, and hold the two to the same value and the same error chain, link
// by link.
//
// forceDecodeWrapper sends every generated decoder to its wrapper:
// json.StringifyNumbers is an option DirectDecode leaves to json/v2, and it
// changes nothing these inputs decode, since every number the model holds
// decodes through a method of its own.

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// forceDecodeWrapper is an option typereg.DirectDecode refuses the direct
// read under, which changes nothing the inputs below decode.
var forceDecodeWrapper = json.StringifyNumbers(true)

// decodeResult is one decode's outcome: the value it left, and its failure.
type decodeResult struct {
	v   any
	err error
}

// decodeOnce decodes data into a fresh value from newV, with the SDK's hooks
// and opts, from a whole buffer or, when stream is set, from a reader.
func decodeOnce(data []byte, newV func() any, stream bool, opts ...json.Options) decodeResult {
	v := newV()
	all := append([]json.Options{typereg.Unmarshalers()}, opts...)
	var err error
	if stream {
		err = json.UnmarshalDecode(jsontext.NewDecoder(bytes.NewReader(data)), v, all...)
	} else {
		err = json.Unmarshal(data, v, all...)
	}
	return decodeResult{v: v, err: err}
}

// failureChain describes each link of err's chain with what a caller reads
// from it: its Go type and text, and the offset, JSON Pointer, JSON kind,
// value and Go type of a json/v2 error, or the path and class of a
// typereg.DecodeError. A link with several causes lists each, one level in.
func failureChain(err error) []string {
	var out []string
	var walk func(e error, depth int)
	walk = func(e error, depth int) {
		for e != nil {
			line := fmt.Sprintf("%d %T %q", depth, e, e.Error())
			switch x := e.(type) { //nolint:errorlint // each link of the chain in turn
			case *json.SemanticError:
				line += fmt.Sprintf(" offset=%d pointer=%q kind=%q value=%q gotype=%v", x.ByteOffset, x.JSONPointer, x.JSONKind, x.JSONValue, x.GoType)
			case *jsontext.SyntacticError:
				line += fmt.Sprintf(" offset=%d pointer=%q", x.ByteOffset, x.JSONPointer)
			case *typereg.DecodeError:
				line += fmt.Sprintf(" path=%q type=%q", x.Path, x.Type)
			}
			out = append(out, line)
			switch u := e.(type) { //nolint:errorlint // each link of the chain in turn
			case interface{ Unwrap() error }:
				e = u.Unwrap()
			case interface{ Unwrap() []error }:
				for _, cause := range u.Unwrap() {
					walk(cause, depth+1)
				}
				return
			default:
				return
			}
		}
	}
	walk(err, 0)
	return out
}

// decodeSentinels are the errors a caller matches a decode failure against.
var decodeSentinels = []struct {
	name string
	err  error
}{
	{"canjson.ErrInvalidShape", canjson.ErrInvalidShape},
	{"canjson.ErrInvalidValue", canjson.ErrInvalidValue},
	{"typereg.ErrTypeMismatch", typereg.ErrTypeMismatch},
	{"typereg.ErrMissingType", typereg.ErrMissingType},
	{"typereg.ErrUnknownType", typereg.ErrUnknownType},
	{"typereg.ErrMaxDepthExceeded", typereg.ErrMaxDepthExceeded},
	{"typereg.ErrNilReceiver", typereg.ErrNilReceiver},
	{"jsontext.ErrDuplicateName", jsontext.ErrDuplicateName},
	{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF},
	{"io.EOF", io.EOF},
}

// sentinelsOf lists the [decodeSentinels] err wraps.
func sentinelsOf(err error) []string {
	var out []string
	for _, s := range decodeSentinels {
		if errors.Is(err, s.err) {
			out = append(out, s.name)
		}
	}
	return out
}

// decodeTally counts the comparisons a test makes, and those of a failure.
type decodeTally struct{ compared, failed int }

// sameDecode decodes data into a fresh value from newV both ways, under
// extra, from a whole buffer and from a stream, and reports each difference
// between the direct read and the wrapper: in the value left, in whether the
// decode failed, in the failure's chain, or in the sentinels it wraps.
func sameDecode(t *testing.T, name string, data []byte, newV func() any, tally *decodeTally, extra ...json.Options) {
	t.Helper()
	for _, stream := range []bool{false, true} {
		label := name
		if stream {
			label += " (stream)"
		}
		direct := decodeOnce(data, newV, stream, extra...)
		wrapped := decodeOnce(data, newV, stream, append(slices.Clone(extra), forceDecodeWrapper)...)
		tally.compared++
		if (direct.err == nil) != (wrapped.err == nil) {
			t.Errorf("%s: direct failure %v, wrapper failure %v", label, direct.err, wrapped.err)
			continue
		}
		if direct.err != nil {
			tally.failed++
			if d, w := failureChain(direct.err), failureChain(wrapped.err); !slices.Equal(d, w) {
				t.Errorf("%s: failure chains differ\ndirect:\n  %s\nwrapper:\n  %s", label, strings.Join(d, "\n  "), strings.Join(w, "\n  "))
			}
			if d, w := sentinelsOf(direct.err), sentinelsOf(wrapped.err); !slices.Equal(d, w) {
				t.Errorf("%s: direct failure wraps %v, wrapper failure %v", label, d, w)
			}
		}
		if !reflect.DeepEqual(direct.v, wrapped.v) {
			t.Errorf("%s: values differ\ndirect:  %+v\nwrapper: %+v", label, direct.v, wrapped.v)
		}
	}
}

// registeredClass returns a constructor for the class data's `_type` names,
// or nil when data names none the type registry holds.
func registeredClass(data []byte) func() any {
	var head struct {
		Type string `json:"_type"`
	}
	if json.Unmarshal(data, &head) != nil || head.Type == "" {
		return nil
	}
	ctor, ok := typereg.Default.Lookup(head.Type)
	if !ok {
		return nil
	}
	return ctor
}

// minDirectDecodeDocuments is a floor on the corpus documents the corpus test
// compares: a skip rule that stopped matching would otherwise empty it.
const minDirectDecodeDocuments = 90

// TestREQ052DirectDecodeCorpus decodes every canonical-JSON document of the
// corpus whose `_type` the type registry holds, every operational template of
// the corpus, written as canonical JSON, and the benchmark inputs, both ways,
// and holds the direct read to the wrapper's value and failure.
func TestREQ052DirectDecodeCorpus(t *testing.T) {
	t.Parallel()
	var tally decodeTally
	root := fixtures.CorpusRoot()
	var documents int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		newV := registeredClass(data)
		if newV == nil {
			return nil
		}
		sameDecode(t, relativeToRoot(root, path), data, newV, &tally)
		documents++
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if documents < minDirectDecodeDocuments {
		t.Errorf("compared %d corpus documents, want at least %d", documents, minDirectDecodeDocuments)
	}

	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	var templates int
	for _, ref := range refs {
		raw, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("read %s: %v", ref.Name, err)
		}
		opt, err := fixtures.ReadOperationalTemplate(raw)
		if err != nil {
			continue
		}
		data, err := canjson.Marshal(opt)
		if err != nil {
			t.Errorf("%s: canjson.Marshal: %v", ref.Name, err)
			continue
		}
		sameDecode(t, ref.Name, data, func() any { return new(aom14.OperationalTemplate) }, &tally)
		templates++
	}
	if templates < minDirectEncodeTemplates {
		t.Errorf("compared %d operational templates, want at least %d", templates, minDirectEncodeTemplates)
	}

	fixture, err := os.ReadFile(fixtures.CompositionJSON(benchFixture))
	if err != nil {
		t.Fatalf("read %s: %v", benchFixture, err)
	}
	newComposition := func() any { return new(rm.Composition) }
	sameDecode(t, "benchmark fixture", fixture, newComposition, &tally)
	sameDecode(t, "benchmark width 400", benchCompositionPayload(t, 400), newComposition, &tally)
	t.Logf("%d comparisons, %d of them failures both ways", tally.compared, tally.failed)
}

// TestREQ052DirectDecodeEveryClass encodes a value of every class the type
// registry holds, with every member set, with every member zero, with every
// member typed by an interface holding a nil pointer, with every list and map
// empty but present and every string empty, and, for an interval, with both
// sides open, and decodes each both ways. The corpus leaves classes and
// members unset; this reaches each member of each class, so a member the
// direct read does not know, or reads into another field, is caught.
func TestREQ052DirectDecodeEveryClass(t *testing.T) {
	t.Parallel()
	var tally decodeTally
	f := newClassFiller()
	var decoded int
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
			data, err := canjson.Marshal(v.Interface())
			if err != nil {
				if variant != "zero" {
					t.Errorf("%s, %s: the value does not encode, so its members go unchecked: %v", name, variant, err)
				}
				continue
			}
			sameDecode(t, name+", "+variant, data, ctor, &tally)
			if variant == "filled" {
				back := ctor()
				if err := canjson.Unmarshal(data, back); err == nil {
					decoded++
				}
			}
		}
	}
	if tally.compared < 2*4*100 {
		t.Errorf("made %d comparisons, want at least one per way and variant of each of 100 registered classes", tally.compared)
	}
	if decoded < 100 {
		t.Errorf("%d filled values decode, want the registry's classes: a value that fails compares one failure, not its members", decoded)
	}
	t.Logf("%d comparisons, %d of them failures both ways", tally.compared, tally.failed)
}

// directDecodeDoc is a small composition that reaches the alias, flat and
// interval shapes and the polymorphic slots: the document the failure cases
// below each change in one place.
const directDecodeDoc = `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.encounter.v1",` +
	`"name":{"_type":"DV_TEXT","value":"bench"},` +
	`"language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},` +
	`"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},` +
	`"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},` +
	`"composer":{"_type":"PARTY_SELF"},` +
	`"content":[{"_type":"ADMIN_ENTRY","archetype_node_id":"openEHR-EHR-ADMIN_ENTRY.bench.v1","name":{"_type":"DV_TEXT","value":"item"},` +
	`"language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},` +
	`"encoding":{"_type":"CODE_PHRASE","code_string":"UTF-8","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_character-sets"}},` +
	`"subject":{"_type":"PARTY_SELF"},` +
	`"data":{"_type":"ITEM_TREE","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"tree"},` +
	`"items":[{"_type":"ELEMENT","archetype_node_id":"at0002","name":{"value":"el"},"value":{"_type":"DV_QUANTITY","magnitude":1.5,"units":"kg"}},` +
	`{"_type":"ELEMENT","archetype_node_id":"at0003","name":{"value":"range"},"value":{"_type":"DV_INTERVAL",` +
	`"lower":{"_type":"DV_COUNT","magnitude":1},"upper":{"_type":"DV_COUNT","magnitude":9},"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false}}]}}]}`

// directDecodeCase is a change to [directDecodeDoc] and what it reaches.
type directDecodeCase struct {
	name     string
	old, new string
}

// directDecodeCases change directDecodeDoc so that a member, a name or a
// value fails, or so that it decodes another way.
var directDecodeCases = []directDecodeCase{
	// A member of the wrong JSON kind.
	{"kind: code_string number", `"code_string":"en"`, `"code_string":5`},
	{"kind: code_string object", `"code_string":"en"`, `"code_string":{}`},
	{"kind: _type number", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":5,"code_string":"en"`},
	{"kind: _type object at the root", `{"_type":"COMPOSITION"`, `{"_type":{}`},
	{"kind: _type array in the flat shape", `{"_type":"DV_CODED_TEXT"`, `{"_type":[]`},
	{"kind: _type true", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":true,"code_string":"en"`},
	{"kind: slot number", `"name":{"_type":"DV_TEXT","value":"bench"}`, `"name":5`},
	{"kind: archetype_node_id object", `"archetype_node_id":"at0001"`, `"archetype_node_id":{}`},
	{"kind: language array", `"language":{"_type":"CODE_PHRASE","code_string":"en"`, `"language":[],"x":{"code_string":"en"`},
	{"kind: territory string", `"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}}`, `"territory":"GB"`},
	{"kind: terminology_id number", `"terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}`, `"terminology_id":5`},
	{"kind: DV_TEXT value bool", `"value":"bench"`, `"value":true`},
	{"kind: items object", `"items":[`, `"items":{"a":[`},
	{"kind: content object", `"content":[`, `"content":{"a":[`},
	{"kind: subject string", `"subject":{"_type":"PARTY_SELF"}`, `"subject":"x"`},
	{"kind: category null", `"category":{"_type":"DV_CODED_TEXT","value":"event"`, `"category":null,"x":{"value":"event"`},
	{"kind: flat value number", `{"_type":"DV_CODED_TEXT","value":"event"`, `{"_type":"DV_CODED_TEXT","value":7`},
	// A `_type` that names another class.
	{"mismatch: CODE_PHRASE as DV_TEXT", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"DV_TEXT","code_string":"en"`},
	{"mismatch: the root", `{"_type":"COMPOSITION"`, `{"_type":"OBSERVATION"`},
	{"mismatch: TERMINOLOGY_ID", `{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}`, `{"_type":"HIER_OBJECT_ID","value":"ISO_639-1"}`},
	{"mismatch: the flat shape", `{"_type":"DV_CODED_TEXT"`, `{"_type":"DV_TEXT"`},
	{"mismatch: an unknown class", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"NO_SUCH_CLASS","code_string":"en"`},
	{"mismatch before a member failure", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"DV_TEXT","code_string":5`},
	{"mismatch after a member failure", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"code_string":5,"_type":"DV_TEXT"`},
	{"mismatch last", `{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}}`, `{"code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"_type":"DV_TEXT"}`},
	{"mismatch: escaped _type that matches", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"\u0043ODE_PHRASE","code_string":"en"`},
	{"mismatch: escaped _type that differs", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"DV_\u0054EXT","code_string":"en"`},
	{"interval: a member it does not declare", `"value":{"_type":"DV_INTERVAL",`, `"value":{"_type":"DV_INTERVAL","x":1,`},
	// A missing, null or empty `_type`.
	{"missing _type", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"code_string":"en"`},
	{"null _type", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":null,"code_string":"en"`},
	{"empty _type", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"","code_string":"en"`},
	{"missing _type at the root", `{"_type":"COMPOSITION",`, `{`},
	{"missing _type at a narrow slot", `"name":{"_type":"DV_TEXT","value":"bench"}`, `"name":{"value":"bench"}`},
	{"missing _type at an abstract slot", `"subject":{"_type":"PARTY_SELF"}`, `"subject":{}`},
	// A member name carried twice.
	{"dup: code_string", `"code_string":"en"`, `"code_string":"en","code_string":"fr"`},
	{"dup: _type, the same", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"CODE_PHRASE","_type":"CODE_PHRASE","code_string":"en"`},
	{"dup: _type, another class first", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"_type":"DV_TEXT","_type":"CODE_PHRASE","code_string":"en"`},
	{"dup: unknown", `"code_string":"en"`, `"zz":1,"code_string":"en","zz":2`},
	{"dup: slot", `"name":{"_type":"DV_TEXT","value":"bench"}`, `"name":{"_type":"DV_TEXT","value":"bench"},"name":{"_type":"DV_TEXT","value":"again"}`},
	{"dup: flat shape", `{"_type":"DV_CODED_TEXT","value":"event"`, `{"_type":"DV_CODED_TEXT","value":"event","value":"e2"`},
	{"dup: escaped name", `"code_string":"en"`, `"code_string":"en","\u0063ode_string":"fr"`},
	{"dup: in ITEM_TREE", `"archetype_node_id":"at0001"`, `"archetype_node_id":"at0001","archetype_node_id":"at0009"`},
	{"dup: interval flag", `"lower_included":true,`, `"lower_included":true,"lower_included":false,`},
	{"dup: interval bound", `"upper":{"_type":"DV_COUNT","magnitude":9},`, `"upper":{"_type":"DV_COUNT","magnitude":9},"upper":null,`},
	// Invalid UTF-8.
	{"utf8: a value", `"code_string":"en"`, "\"code_string\":\"e\xffn\""},
	{"utf8: a known name", `"code_string":"en"`, "\"code_st\xffring\":\"en\""},
	{"utf8: an unknown name", `"code_string":"en"`, "\"z\xff\":1,\"code_string\":\"en\""},
	{"utf8: the _type value", `{"_type":"CODE_PHRASE","code_string":"en"`, "{\"_type\":\"CODE\xff\",\"code_string\":\"en\""},
	{"utf8: a slot's value", `"value":"bench"`, "\"value\":\"b\xffench\""},
	{"utf8: the flat shape", `{"_type":"DV_CODED_TEXT","value":"event"`, "{\"_type\":\"DV_CODED_TEXT\",\"value\":\"ev\xffent\""},
	// Escapes, letter case and members the class does not declare.
	{"escaped known name", `"code_string":"en"`, `"\u0063ode_string":"en"`},
	{"escaped _type name", `{"_type":"CODE_PHRASE","code_string":"en"`, `{"\u005ftype":"CODE_PHRASE","code_string":"en"`},
	{"another letter case", `"code_string":"en"`, `"Code_String":"en"`},
	{"unknown members of every kind", `"code_string":"en"`, `"u1":{"a":[1,{"b":null}]},"u2":[[]],"u3":true,"u4":null,"u5":-1e3,"code_string":"en"`},
	{"_type on an interval's unknown side", `"lower_included":true,`, `"lower_included":true,"_type":"Interval",`},
	// Malformed input.
	{"bad escape in a value", `"code_string":"en"`, `"code_string":"e\qn"`},
	{"bad token after a name", `"code_string":"en"`, `"code_string":en`},
	{"missing colon", `"code_string":"en"`, `"code_string" "en"`},
	{"trailing comma", `"value":"ISO_639-1"}}`, `"value":"ISO_639-1",}}`},
	{"a number as a name", `"code_string":"en"`, `5:"en"`},
	{"an array closing an object", `"value":"ISO_3166-1"}`, `"value":"ISO_3166-1"]`},
	// null in place of a value.
	{"null: language", `"language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory"`, `"language":null,"territory"`},
	{"null: encoding", `"encoding":{"_type":"CODE_PHRASE","code_string":"UTF-8","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_character-sets"}}`, `"encoding":null`},
	{"null: a slot", `"name":{"_type":"DV_TEXT","value":"bench"}`, `"name":null`},
	{"null: terminology_id", `"terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}`, `"terminology_id":null`},
	{"null: items", `"items":[`, `"items":null,"x":[`},
	{"null: an interval", `"value":{"_type":"DV_INTERVAL",`, `"value":null,"x":{"_type":"DV_INTERVAL",`},
	// The interval's boundary flags and bounds.
	{"interval: every flag absent", `"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false`, `"_x":0`},
	{"interval: only an upper bound", `"lower":{"_type":"DV_COUNT","magnitude":1},"upper":{"_type":"DV_COUNT","magnitude":9},"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false`, `"upper":{"_type":"DV_COUNT","magnitude":9}`},
	{"interval: flags null", `"lower_included":true,"upper_included":false`, `"lower_included":null,"upper_included":null`},
	{"interval: a flag of another kind", `"lower_included":true`, `"lower_included":"yes"`},
	{"interval: a bound null", `"lower":{"_type":"DV_COUNT","magnitude":1}`, `"lower":null`},
	{"interval: a closed side with no bound", `"lower":{"_type":"DV_COUNT","magnitude":1},`, ``},
	{"interval: an open side with a bound", `"lower_unbounded":false`, `"lower_unbounded":true`},
	{"interval: a bound of another kind", `"lower":{"_type":"DV_COUNT","magnitude":1}`, `"lower":[1]`},
	{"interval: a bound that fails", `"lower":{"_type":"DV_COUNT","magnitude":1}`, `"lower":{"_type":"DV_COUNT","magnitude":"x"}`},
	{"interval: flags first", `"lower":{"_type":"DV_COUNT","magnitude":1},"upper":{"_type":"DV_COUNT","magnitude":9},"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false`, `"lower_unbounded":false,"upper_included":false,"lower":{"_type":"DV_COUNT","magnitude":1},"upper":{"_type":"DV_COUNT","magnitude":9}`},
	// Numbers.
	{"a quoted magnitude", `"magnitude":1.5`, `"magnitude":"1.5"`},
	{"a lossy magnitude", `"magnitude":1.5`, `"magnitude":0.10000000000000000555111512312578270211815834045410156250001`},
	{"a magnitude of another kind", `"magnitude":1.5`, `"magnitude":[1.5]`},
}

// directDecodeDocWith returns directDecodeDoc with c's change made, failing
// the test where the document does not hold the text c changes.
func directDecodeDocWith(t *testing.T, c directDecodeCase) []byte {
	t.Helper()
	if !strings.Contains(directDecodeDoc, c.old) {
		t.Fatalf("%s: the document holds no %q", c.name, c.old)
	}
	return []byte(strings.Replace(directDecodeDoc, c.old, c.new, 1))
}

// TestREQ052DirectDecodeFailures decodes a small composition both ways, as it
// stands and with each change of [directDecodeCases]: a member of the wrong
// kind, a `_type` naming another class before and after a member's failure, a
// missing, null or empty `_type`, a duplicate name, invalid UTF-8 in a value,
// a known name, an unknown name and `_type`, malformed input, null in place
// of a value, and an interval's flags and bounds. It also decodes the
// document cut at every byte, nested past the depth bound through slots and
// at a concrete class, and under the options that allow duplicate names and
// invalid UTF-8. Each holds the direct read to the wrapper's failure, link by
// link, and to the value it leaves.
func TestREQ052DirectDecodeFailures(t *testing.T) {
	t.Parallel()
	var tally decodeTally
	newComposition := func() any { return new(rm.Composition) }
	sameDecode(t, "the document", []byte(directDecodeDoc), newComposition, &tally)
	for _, c := range directDecodeCases {
		sameDecode(t, c.name, directDecodeDocWith(t, c), newComposition, &tally)
	}
	for i := range len(directDecodeDoc) {
		sameDecode(t, fmt.Sprintf("cut at byte %d", i), []byte(directDecodeDoc[:i]), newComposition, &tally)
	}
	for _, c := range directDecodeCases {
		if strings.HasPrefix(c.name, "dup: ") {
			sameDecode(t, c.name+", duplicate names allowed", directDecodeDocWith(t, c), newComposition, &tally, jsontext.AllowDuplicateNames(true))
		}
		if strings.HasPrefix(c.name, "utf8: ") {
			sameDecode(t, c.name+", invalid UTF-8 allowed", directDecodeDocWith(t, c), newComposition, &tally, jsontext.AllowInvalidUTF8(true))
		}
	}

	// Depth past the bound through nested slots.
	for _, n := range []int{509, 510, 511, 512, 513, 600} {
		var sb strings.Builder
		sb.WriteString(`{"_type":"ITEM_TREE","archetype_node_id":"at0","name":{"value":"t"},"items":[`)
		for range n {
			sb.WriteString(`{"_type":"CLUSTER","archetype_node_id":"at1","name":{"value":"c"},"items":[`)
		}
		for range n {
			sb.WriteString(`]}`)
		}
		sb.WriteString(`]}`)
		sameDecode(t, fmt.Sprintf("%d clusters deep", n), []byte(sb.String()), func() any { return new(rm.ItemTree) }, &tally)
	}
	// Depth past the bound at a concrete class with no slot above it: a
	// CODE_PHRASE n arrays down, decoded into n nested slices.
	for _, n := range []int{510, 511, 512, 513} {
		typ := reflect.TypeFor[rm.CodePhrase]()
		for range n {
			typ = reflect.SliceOf(typ)
		}
		in := strings.Repeat("[", n) + `{"_type":"CODE_PHRASE","code_string":"x","terminology_id":{"_type":"TERMINOLOGY_ID","value":"t"}}` + strings.Repeat("]", n)
		sameDecode(t, fmt.Sprintf("CODE_PHRASE %d arrays deep", n), []byte(in), func() any { return reflect.New(typ).Interface() }, &tally)
	}
	t.Logf("%d comparisons, %d of them failures both ways", tally.compared, tally.failed)
}

// TestREQ052DirectDecodeRoots decodes values of many kinds straight into the
// classes of each wire shape, and into receivers that already hold a value:
// null, a value of another kind, malformed and empty input, an empty object,
// a value whose `_type` names another class, and one whose member fails. The
// flat shape puts its receiver back as it was on a failure, as its wrapper
// does, and the other shapes decode in place, as theirs do: a populated
// receiver tells the two apart.
func TestREQ052DirectDecodeRoots(t *testing.T) {
	t.Parallel()
	var tally decodeTally
	roots := []struct {
		name string
		newV func() any
	}{
		{"COMPOSITION", func() any { return new(rm.Composition) }},
		{"CODE_PHRASE", func() any { return new(rm.CodePhrase) }},
		{"DV_CODED_TEXT", func() any { return new(rm.DVCodedText) }},
		{"DV_TEXT", func() any { return new(rm.DVText) }},
		{"ITEM_TREE", func() any { return new(rm.ItemTree) }},
		{"ELEMENT", func() any { return new(rm.Element) }},
		{"PARTY_SELF", func() any { return new(rm.PartySelf) }},
		{"DV_INTERVAL of DV_ORDERED", func() any { return new(rm.DVInterval[rm.DVOrdered]) }},
		{"DV_INTERVAL of DV_COUNT", func() any { return new(rm.DVInterval[rm.DVCount]) }},
		{"DV_INTERVAL of *DV_COUNT", func() any { return new(rm.DVInterval[*rm.DVCount]) }},
		{"Interval of Integer", func() any { return new(rm.Interval[rm.Integer]) }},
		{"Interval of string", func() any { return new(rm.Interval[string]) }},
		{"Point_interval of Integer", func() any { return new(rm.PointInterval[rm.Integer]) }},
		{"Proper_interval of any", func() any { return new(rm.ProperInterval[any]) }},
		{"EXPR_LEAF", func() any { return new(aom14.ExprLeaf) }},
		{"ORDINAL", func() any { return new(aom14.Ordinal) }},
		{"C_DV_ORDINAL", func() any { return new(aom14.CDVOrdinal) }},
		{"C_ATTRIBUTE", func() any { return new(aom14.CSingleAttribute) }},
		{"a populated DV_CODED_TEXT", func() any {
			return &rm.DVCodedText{Value: "orig", DefiningCode: rm.CodePhrase{CodeString: "c", TerminologyID: rm.TerminologyID{Value: "local"}}}
		}},
		{"a populated DV_TEXT", func() any { return &rm.DVText{Value: "orig"} }},
		{"a populated CODE_PHRASE", func() any { return &rm.CodePhrase{CodeString: "orig"} }},
		{"a populated DV_INTERVAL", func() any {
			return &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{Magnitude: 3}, LowerIncluded: true}
		}},
		{"a populated Interval", func() any { return &rm.Interval[rm.Integer]{Lower: 3, LowerIncluded: true, UpperUnbounded: true} }},
	}
	inputs := []string{
		`null`, `5`, `"x"`, `[]`, `true`, `{`, ``, ` `, `{}`, `{"x":1}`, `{"_type":"CODE_PHRASE"}`,
		`{"_type":"DV_TEXT","value":"foreign"}`,
		`{"_type":"DV_CODED_TEXT","value":"foreign","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"},"code_string":"at0001"}}`,
		`{"_type":"CODE_PHRASE","code_string":"x","terminology_id":{"value":"t"}}`,
		`{"value":7}`,
		`{"value":"v","value":"w"}`,
		`{"lower":1,"upper":2}`,
		`{"lower":1,"lower_unbounded":false}`,
		`{"lower":"a","upper":"b","lower_included":false}`,
		`{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":1}}`,
		`{"_type":"DV_INTERVAL","upper":{"_type":"DV_COUNT","magnitude":1},"lower_unbounded":false}`,
		`{"_type":"Point_interval","lower":4,"upper":4}`,
		`{"_type":"Proper_interval","lower":4,"upper_unbounded":true}`,
		`{"_type":"Interval","lower":1}`,
		`{"lower_included":1}`,
		`{"_type":"EXPR_LEAF","item":{"_type":"Integer","value":-42},"type":"Integer","reference_type":"constant"}`,
		`{"_type":"EXPR_LEAF","item":"text","type":"String"}`,
		`{"_type":"EXPR_LEAF","item":5,"type":"Integer"}`,
		`{"_type":"EXPR_LEAF","item":{"_type":"C_STRING","pattern":".*"}}`,
		`{"_type":"EXPR_LEAF","item":{"value":1}}`,
		`{"_type":"ORDINAL","value":1,"symbol":{"_type":"CODE_PHRASE","code_string":"at1","terminology_id":{"value":"local"}}}`,
		`{"_type":"ORDINAL","value":1,"symbol":{"_type":"DV_TEXT","value":"x"}}`,
		`{"_type":"ORDINAL","value":1,"symbol":"x"}`,
		`{"_type":"C_DV_ORDINAL","node_id":"at1","rm_type_name":"DV_ORDINAL","occurrences":{"lower":1,"upper":1},"list":[{"value":1,"symbol":{"_type":"CODE_PHRASE","code_string":"at1","terminology_id":{"value":"local"}}}]}`,
		`{"_type":"C_SINGLE_ATTRIBUTE","rm_attribute_name":"data","existence":{"lower":0,"upper":1,"lower_unbounded":false}}`,
		`{"_type":"C_SINGLE_ATTRIBUTE","rm_attribute_name":"data","existence":{"upper_unbounded":false}}`,
	}
	for _, root := range roots {
		for _, in := range inputs {
			sameDecode(t, root.name+" from "+in, []byte(in), root.newV, &tally)
		}
	}
	t.Logf("%d comparisons, %d of them failures both ways", tally.compared, tally.failed)
}

// TestREQ052DirectDecodeGatedOptions holds the direct read to the wrapper's
// value and failure under each decoder option that changes how json/v2's
// struct decode reads a member, each case on an input whose decode the option
// changes. typereg.DirectDecode sends each such decode to the wrapper; a
// check it dropped would let the direct read, which knows nothing of the
// option, read the value its own way, and the case fails. A caller's hooks
// reach the outermost value only through the wrapper, which passes them on to
// every value inside it. v1's merge semantics keep the wrapper's `_type` as it
// was when a repeated `_type` reads null, where the direct read takes the
// last; the case allows duplicate names on both sides of its check.
func TestREQ052DirectDecodeGatedOptions(t *testing.T) {
	t.Parallel()
	upper := json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, s *string) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		*s = strings.ToUpper(tok.String())
		return nil
	}))
	failing := strings.Replace(directDecodeDoc, `"archetype_node_id":"openEHR-EHR-COMPOSITION.encounter.v1"`, `"archetype_node_id":5`, 1)
	newComposition := func() any { return new(rm.Composition) }
	nullType := `{"_type":"DV_CODED_TEXT","value":"x","defining_code":{"_type":"DV_TEXT","_type":null,"code_string":"c","terminology_id":{"value":"local"}}}`
	for _, tc := range []struct {
		name string
		doc  string
		newV func() any
		opt  json.Options
		base []json.Options // options both sides of the with-and-without check share
	}{
		{name: "MatchCaseInsensitiveNames", doc: `{"_type":"CODE_PHRASE","Code_String":"en"}`, newV: func() any { return new(rm.CodePhrase) }, opt: json.MatchCaseInsensitiveNames(true)},
		{name: "RejectUnknownMembers", doc: `{"_type":"CODE_PHRASE","code_string":"en","zz":1}`, newV: func() any { return new(rm.CodePhrase) }, opt: json.RejectUnknownMembers(true)},
		{name: "ReportErrorsWithLegacySemantics", doc: failing, newV: newComposition, opt: jsonv1.ReportErrorsWithLegacySemantics(true)},
		{name: "MergeWithLegacySemantics", doc: nullType, newV: func() any { return new(rm.DVCodedText) }, opt: jsonv1.MergeWithLegacySemantics(true), base: []json.Options{jsontext.AllowDuplicateNames(true)}},
		{name: "encoding/json defaults", doc: failing, newV: newComposition, opt: jsonv1.DefaultOptionsV1()},
		{name: "a caller's hooks", doc: `{"_type":"code_phrase","code_string":"en"}`, newV: func() any { return new(rm.CodePhrase) }, opt: upper},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tally decodeTally
			sameDecode(t, tc.name, []byte(tc.doc), tc.newV, &tally, append(slices.Clone(tc.base), tc.opt)...)
			with := decodeOnce([]byte(tc.doc), tc.newV, false, append(slices.Clone(tc.base), tc.opt, forceDecodeWrapper)...)
			without := decodeOnce([]byte(tc.doc), tc.newV, false, append(slices.Clone(tc.base), forceDecodeWrapper)...)
			if fmt.Sprint(with.err) == fmt.Sprint(without.err) && reflect.DeepEqual(with.v, without.v) {
				t.Errorf("%s changes nothing the wrapper reads from this input, so the case checks nothing", tc.name)
			}
		})
	}
}

// TestREQ052DirectDecodeAllocatesLess holds canjson.Unmarshal to fewer
// allocations than the wrappers make for the same composition, and, which is
// what keeps the comparisons above honest, holds forceDecodeWrapper to taking
// the wrapper at every level: the direct read builds no wrapper struct, so
// were DirectDecode false for canjson's own options, or the forcing option to
// leave the direct read to some nested class, the two counts would meet.
func TestREQ052DirectDecodeAllocatesLess(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector makes allocation counts vary")
	}
	payload := benchCompositionPayload(t, 400)
	direct := testing.AllocsPerRun(10, func() {
		var c rm.Composition
		if err := canjson.Unmarshal(payload, &c); err != nil {
			t.Fatal(err)
		}
	})
	wrapped := testing.AllocsPerRun(10, func() {
		var c rm.Composition
		if err := json.Unmarshal(payload, &c, typereg.Unmarshalers(), forceDecodeWrapper); err != nil {
			t.Fatal(err)
		}
	})
	if direct*2 > wrapped {
		t.Errorf("canjson.Unmarshal of a width-400 composition makes %.0f allocations and the wrappers %.0f; want the direct read under half of the wrappers'", direct, wrapped)
	}

	// A class whose members are tokens and classes, decoded straight into a
	// receiver: the direct read moves nothing to the heap but the strings it
	// decodes.
	code := []byte(`{"_type":"CODE_PHRASE","code_string":"c","terminology_id":{"_type":"TERMINOLOGY_ID","value":"local"}}`)
	var c rm.CodePhrase
	dec := jsontext.NewDecoder(bytes.NewReader(code), typereg.Unmarshalers())
	n := testing.AllocsPerRun(100, func() {
		dec.Reset(bytes.NewReader(code), typereg.Unmarshalers())
		c = rm.CodePhrase{}
		if err := c.UnmarshalJSONFrom(dec); err != nil {
			t.Fatal(err)
		}
	})
	if n > 2 {
		t.Errorf("CODE_PHRASE.UnmarshalJSONFrom makes %.0f allocations, want at most its two strings", n)
	}
}
