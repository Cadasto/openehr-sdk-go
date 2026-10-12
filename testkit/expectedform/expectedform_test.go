package expectedform_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/expectedform"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// buildCase is one input and the expected form Build must return for it,
// compared by wire equivalence.
type buildCase struct {
	name string
	root string
	in   string
	want string
}

// runBuild runs each case through Build and compares the result with want.
func runBuild(t *testing.T, cases []buildCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expectedform.Build([]byte(tc.in), tc.root)
			if err != nil {
				t.Fatalf("Build(%s, %q) error: %v", tc.in, tc.root, err)
			}
			if ok, diff := wireequiv.Equivalent([]byte(tc.want), got); !ok {
				t.Errorf("Build(%s, %q) = %s, want %s: %s", tc.in, tc.root, got, tc.want, diff)
			}
		})
	}
}

// codePhrase is a complete CODE_PHRASE used where a case needs one.
const codePhrase = `{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}`

// TestREQ052_BuildRemovesMembersTheRMDoesNotDefine pins the first change: a
// member that names no attribute of its object's RM type is removed, at the
// root and below it, with whatever it holds, a "_type" included. An object
// that elides "_type" is filtered by the declared type of its slot.
func TestREQ052_BuildRemovesMembersTheRMDoesNotDefine(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "root member",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","unit":"kg"}`,
			want: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`,
		},
		{
			name: "member holding a typed object",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","note":{"_type":"DV_TEXT","value":"x"}}`,
			want: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`,
		},
		{
			name: "nested object with an elided _type",
			root: "ELEMENT",
			in:   `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"value":"n","label":"l"}}`,
			want: `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"_type":"DV_TEXT","value":"n"}}`,
		},
		{
			name: "container element",
			root: "DV_TEXT",
			in:   `{"_type":"DV_TEXT","value":"v","mappings":[{"_type":"TERM_MAPPING","match":"=","target":` + codePhrase + `,"weight":2}]}`,
			want: `{"_type":"DV_TEXT","value":"v","mappings":[{"_type":"TERM_MAPPING","match":"=","target":` + codePhrase + `}]}`,
		},
		{
			name: "a computed attribute is not an attribute",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","is_integral":true}`,
			want: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`,
		},
	})
}

// TestREQ052_BuildRemovesNullMembers pins that a null member reads as absent.
func TestREQ052_BuildRemovesNullMembers(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "optional primitive",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","precision":null}`,
			want: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`,
		},
		{
			name: "optional object",
			root: "DV_TEXT",
			in:   `{"_type":"DV_TEXT","value":"v","language":null}`,
			want: `{"_type":"DV_TEXT","value":"v"}`,
		},
		{
			name: "null _type reads as elided",
			root: "DV_TEXT",
			in:   `{"_type":null,"value":"v"}`,
			want: `{"_type":"DV_TEXT","value":"v"}`,
		},
	})
}

// TestREQ052_BuildWritesANullMandatoryStringAsEmpty pins that a null member
// the RM declares mandatory with a String type is expected as the empty string
// the encoder writes for it, since a Go string holds no null. The rule is read
// from rminfo, so it reaches every mandatory String attribute, at the root and
// nested. A null optional String, a null mandatory Character, whose empty value
// the encoder refuses, and a null mandatory RM object still read as absent.
func TestREQ052_BuildWritesANullMandatoryStringAsEmpty(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "mandatory String at the root",
			root: "DV_DATE_TIME",
			in:   `{"_type":"DV_DATE_TIME","value":null}`,
			want: `{"_type":"DV_DATE_TIME","value":""}`,
		},
		{
			name: "mandatory String nested",
			root: "DV_MULTIMEDIA",
			in:   `{"_type":"DV_MULTIMEDIA","size":3,"media_type":{"terminology_id":{"value":"openEHR"},"code_string":null}}`,
			want: `{"_type":"DV_MULTIMEDIA","size":3,"media_type":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openEHR"},"code_string":""}}`,
		},
		{
			name: "optional String stays absent",
			root: "DV_TEXT",
			in:   `{"_type":"DV_TEXT","value":"v","formatting":null}`,
			want: `{"_type":"DV_TEXT","value":"v"}`,
		},
		{
			name: "mandatory Character stays absent",
			root: "TERM_MAPPING",
			in:   `{"_type":"TERM_MAPPING","match":null,"target":` + codePhrase + `}`,
			want: `{"_type":"TERM_MAPPING","target":` + codePhrase + `}`,
		},
		{
			name: "mandatory object stays absent",
			root: "CODE_PHRASE",
			in:   `{"_type":"CODE_PHRASE","code_string":"433","terminology_id":null}`,
			want: `{"_type":"CODE_PHRASE","code_string":"433"}`,
		},
	})
}

// TestREQ052_BuildWritesANullMandatoryNumberOrBooleanAsItsZero pins that a
// null member the RM declares mandatory with an Integer, Integer64, Real or
// Boolean type is expected as the zero the encoder writes for it, 0 or false,
// since a Go value of those types holds no null. Each kind has its own case,
// so dropping any one from the rule reddens that case. The Double kind, which
// no mandatory RM attribute has, is pinned on a synthetic class in
// walker_internal_test.go. An interval's boundary flag is a mandatory Boolean
// the decoder reads as absent when it is null, so it takes its REQ-169
// default, true on a closed side, rather than false.
func TestREQ052_BuildWritesANullMandatoryNumberOrBooleanAsItsZero(t *testing.T) {
	const symbol = `{"_type":"DV_CODED_TEXT","value":"s","defining_code":` + codePhrase + `}`
	runBuild(t, []buildCase{
		{
			name: "Integer",
			root: "DV_ORDINAL",
			in:   `{"_type":"DV_ORDINAL","value":null,"symbol":` + symbol + `}`,
			want: `{"_type":"DV_ORDINAL","value":0,"symbol":` + symbol + `}`,
		},
		{
			name: "Integer64",
			root: "DV_COUNT",
			in:   `{"_type":"DV_COUNT","magnitude":null}`,
			want: `{"_type":"DV_COUNT","magnitude":0}`,
		},
		{
			name: "Real",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":null,"units":"kg"}`,
			want: `{"_type":"DV_QUANTITY","magnitude":0,"units":"kg"}`,
		},
		{
			name: "Boolean",
			root: "DV_BOOLEAN",
			in:   `{"_type":"DV_BOOLEAN","value":null}`,
			want: `{"_type":"DV_BOOLEAN","value":false}`,
		},
		{
			name: "Boolean nested",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_BOOLEAN","value":null}`),
			want: element(`{"_type":"DV_BOOLEAN","value":false}`),
		},
		{
			name: "optional Integer stays absent",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","precision":null}`,
			want: `{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"}`,
		},
		{
			name: "interval flags take their defaults",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"upper":` + dvQuantity + `,"lower_unbounded":null,"upper_unbounded":null,"lower_included":null,"upper_included":null}`),
			want: element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"upper":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}`),
		},
	})
}

// TestREQ052_BuildWritesAnElidedType pins that an elided "_type" is written:
// the root takes the decode target's type, a nested object the declared type
// of its slot, and an interval bound the type its slot binds the parameter to.
func TestREQ052_BuildWritesAnElidedType(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "root",
			root: "DV_CODED_TEXT",
			in:   `{"value":"event","defining_code":` + codePhrase + `}`,
			want: `{"_type":"DV_CODED_TEXT","value":"event","defining_code":` + codePhrase + `}`,
		},
		{
			name: "nested",
			root: "CODE_PHRASE",
			in:   `{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"value":"openehr"}}`,
			want: codePhrase,
		},
		{
			name: "bound of a concrete binding",
			root: "DV_COUNT",
			in:   `{"_type":"DV_COUNT","magnitude":3,"normal_range":{"_type":"DV_INTERVAL","lower":{"magnitude":1},"upper":{"magnitude":5},"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}}`,
			want: `{"_type":"DV_COUNT","magnitude":3,"normal_range":{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":1},"upper":{"_type":"DV_COUNT","magnitude":5},"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}}`,
		},
		{
			name: "interval in a substitutable slot",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg","normal_range":{"lower":{"magnitude":1,"units":"kg"},"lower_unbounded":false,"upper_unbounded":true,"lower_included":true,"upper_included":false}}`,
			want: `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg","normal_range":{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},"lower_unbounded":false,"upper_unbounded":true,"lower_included":true,"upper_included":false}}`,
		},
	})
}

// dvQuantity is a complete DV_QUANTITY bound.
const dvQuantity = `{"_type":"DV_QUANTITY","magnitude":5,"units":"kg"}`

// element wraps an interval value in an ELEMENT, whose value slot does not bind
// the bound type.
func element(interval string) string {
	return `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"_type":"DV_TEXT","value":"n"},"value":` + interval + `}`
}

// TestREQ169_BuildWritesDefaultedIntervalFlags pins that every boundary flag
// the input leaves out is written with the default REQ-169 reads it with, and
// that a flag the input carries is kept as written, an included open side
// among them. An omitted "*_unbounded" is false beside a bound and true where
// the side's bound is omitted or null, and a null bound is dropped.
//
// Can-fail control: write an omitted "*_unbounded" as false whatever the
// bound, as before, and the cases with a side that has no bound expect it
// closed.
func TestREQ169_BuildWritesDefaultedIntervalFlags(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "no flags: closed on both sides",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"upper":` + dvQuantity + `}`),
			want: element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"upper":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}`),
		},
		{
			name: "unbounded flags only",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":true}`),
			want: element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":true,"lower_included":true,"upper_included":false}`),
		},
		{
			name: "present flags kept, an included open side among them",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `,"lower_unbounded":true,"lower_included":true,"upper_included":false}`),
			want: element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `,"lower_unbounded":true,"upper_unbounded":false,"lower_included":true,"upper_included":false}`),
		},
		{
			name: "only an upper bound: open below",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `}`),
			want: element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `,"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}`),
		},
		{
			name: "only a lower bound: open above",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `}`),
			want: element(`{"_type":"DV_INTERVAL","lower":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":true,"lower_included":true,"upper_included":false}`),
		},
		{
			name: "a null bound is dropped and its side is open",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":null,"upper":` + dvQuantity + `}`),
			want: element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `,"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}`),
		},
		{
			name: "no bound on either side: open on both",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL"}`),
			want: element(`{"_type":"DV_INTERVAL","lower_unbounded":true,"upper_unbounded":true,"lower_included":false,"upper_included":false}`),
		},
		{
			// Held by reference, as here, the side decodes as written with no
			// bound; held by value, the decode refuses it before any
			// comparison.
			name: "a side written bounded with no bound is kept as written",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower_unbounded":false,"upper":` + dvQuantity + `}`),
			want: element(`{"_type":"DV_INTERVAL","upper":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}`),
		},
	})
}

// TestREQ052_BuildRemovesAnEmptyBoundBesideItsOpenFlag pins that an empty
// bound on a side marked unbounded is removed, and that a non-empty one, or an
// empty one on a closed side, is kept. Where the slot binds the bound to a
// concrete class, a bound is empty when every member it carries is a mandatory
// attribute holding a zero literal: a non-zero mandatory member, or any
// optional member (held behind a pointer, so present even when it holds 0),
// keeps it. Behind an unbound parameter an object is never empty, and a
// primitive bound is empty when it is a zero literal.
func TestREQ052_BuildRemovesAnEmptyBoundBesideItsOpenFlag(t *testing.T) {
	normalRange := func(lower string) string {
		return `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg","normal_range":{"_type":"DV_INTERVAL",` + lower + `"upper":` + dvQuantity + `,"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}}`
	}
	runBuild(t, []buildCase{
		{
			name: "null bound",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":null,`),
			want: normalRange(``),
		},
		{
			name: "type-only bound of a concrete binding",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY"},`),
			want: normalRange(``),
		},
		{
			name: "zero-valued bound of a concrete binding",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":0,"units":""},`),
			want: normalRange(``),
		},
		{
			name: "non-empty bound beside its open flag is kept",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},`),
			want: normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},`),
		},
		{
			name: "non-zero magnitude keeps the bound",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":5,"units":""},`),
			want: normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":5,"units":""},`),
		},
		{
			name: "non-empty units keep the bound",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":0,"units":"kg"},`),
			want: normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":0,"units":"kg"},`),
		},
		{
			name: "an optional member holding zero keeps the bound",
			root: "DV_QUANTITY",
			in:   normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":0,"units":"","precision":0},`),
			want: normalRange(`"lower":{"_type":"DV_QUANTITY","magnitude":0,"units":"","precision":0},`),
		},
		{
			name: "type-only bound behind an unbound parameter is kept",
			root: "ELEMENT",
			in:   element(`{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY"},"upper":` + dvQuantity + `,"lower_unbounded":true}`),
			want: element(`{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY"},"upper":` + dvQuantity + `,"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}`),
		},
		{
			name: "empty bound on a closed side is kept",
			root: "DV_QUANTITY",
			in:   `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg","normal_range":{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY"},"upper":` + dvQuantity + `}}`,
			want: `{"_type":"DV_QUANTITY","magnitude":3,"units":"kg","normal_range":{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY"},"upper":` + dvQuantity + `,"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}}`,
		},
		{name: "primitive zero number bound", root: "Proper_interval", in: primitiveBound(`0`), want: primitiveBound(``)},
		{name: "primitive empty string bound", root: "Proper_interval", in: primitiveBound(`""`), want: primitiveBound(``)},
		{name: "primitive false bound", root: "Proper_interval", in: primitiveBound(`false`), want: primitiveBound(``)},
		{name: "primitive non-zero number bound kept", root: "Proper_interval", in: primitiveBound(`5`), want: primitiveBound(`5`)},
		{name: "primitive non-empty string bound kept", root: "Proper_interval", in: primitiveBound(`"a"`), want: primitiveBound(`"a"`)},
		{name: "primitive true bound kept", root: "Proper_interval", in: primitiveBound(`true`), want: primitiveBound(`true`)},
	})
}

// primitiveBound builds a BASE Proper_interval whose open lower side carries
// the given primitive bound, or none when lower is empty.
func primitiveBound(lower string) string {
	member := ""
	if lower != "" {
		member = `"lower":` + lower + `,`
	}
	return `{"_type":"Proper_interval",` + member + `"upper":9,"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}`
}

// multimedia builds a DV_MULTIMEDIA with extra members appended.
func multimedia(extra string) string {
	return `{"_type":"DV_MULTIMEDIA","media_type":{"_type":"CODE_PHRASE","code_string":"image/png","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_media-types"}},"size":3` + extra + `}`
}

// TestREQ052_BuildRemovesEmptyMappingsAndMultimediaBytes pins that an empty
// DV_TEXT mappings, DV_MULTIMEDIA data or DV_MULTIMEDIA integrity_check is
// removed, while a non-empty one, and an empty container the section does not
// name, is kept.
func TestREQ052_BuildRemovesEmptyMappingsAndMultimediaBytes(t *testing.T) {
	runBuild(t, []buildCase{
		{
			name: "empty mappings",
			root: "DV_TEXT",
			in:   `{"_type":"DV_TEXT","value":"v","mappings":[]}`,
			want: `{"_type":"DV_TEXT","value":"v"}`,
		},
		{
			name: "empty mappings on a DV_CODED_TEXT",
			root: "DV_CODED_TEXT",
			in:   `{"_type":"DV_CODED_TEXT","value":"event","defining_code":` + codePhrase + `,"mappings":[]}`,
			want: `{"_type":"DV_CODED_TEXT","value":"event","defining_code":` + codePhrase + `}`,
		},
		{
			name: "empty data and integrity_check",
			root: "DV_MULTIMEDIA",
			in:   multimedia(`,"data":"","integrity_check":""`),
			want: multimedia(``),
		},
		{
			name: "empty data written as an array",
			root: "DV_MULTIMEDIA",
			in:   multimedia(`,"data":[]`),
			want: multimedia(``),
		},
		{
			name: "non-empty data kept",
			root: "DV_MULTIMEDIA",
			in:   multimedia(`,"data":"AQID"`),
			want: multimedia(`,"data":"AQID"`),
		},
		{
			name: "an empty container the section does not name is kept",
			root: "ELEMENT",
			in:   `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"_type":"DV_TEXT","value":"n"},"links":[]}`,
			want: `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"_type":"DV_TEXT","value":"n"},"links":[]}`,
		},
	})
}

// TestREQ052_BuildLeavesAGenericPayloadAlone pins that inside a generic
// payload only numbers change: the registry's ORIGINAL_VERSION holds its
// "data" as plain JSON, which the decoder reads numbers in as float64 and the
// encoder writes back from them. So a bare number, at any depth, is expected
// as the literal the encoder writes for its float64, while a member the RM
// does not define, a null member, an empty list and a quoted number stay as
// written. A member of the ORIGINAL_VERSION itself is still removed.
func TestREQ052_BuildLeavesAGenericPayloadAlone(t *testing.T) {
	payload := func(magnitude, list string) string {
		return `{"_type":"DV_QUANTITY","magnitude":` + magnitude + `,"units":"kg","precision":"2.0","x-a":1,` +
			`"normal_range":null,"mappings":[],"x-list":` + list + `}`
	}
	runBuild(t, []buildCase{
		{
			name: "ORIGINAL_VERSION data",
			root: "ORIGINAL_VERSION",
			in:   `{"_type":"ORIGINAL_VERSION","data":` + payload(`1.50`, `[1e2,{"n":0.10},"3.0",null,true]`) + `,"x-root":1}`,
			want: `{"_type":"ORIGINAL_VERSION","data":` + payload(`1.5`, `[100,{"n":0.1},"3.0",null,true]`) + `}`,
		},
	})
}

// TestREQ052_BuildWalksAHashOfRMObjects pins that the RM objects a Hash
// attribute holds are read as their class: RESOURCE_DESCRIPTION.details holds
// RESOURCE_DESCRIPTION_ITEM values, so a member the class does not define is
// removed from each entry and every change applies inside it, while a Hash of
// strings (original_author) is kept as written.
func TestREQ052_BuildWalksAHashOfRMObjects(t *testing.T) {
	const language = `{"terminology_id":{"value":"ISO_639-1"},"code_string":"en"}`
	runBuild(t, []buildCase{
		{
			name: "RESOURCE_DESCRIPTION details",
			root: "RESOURCE_DESCRIPTION",
			in: `{"_type":"RESOURCE_DESCRIPTION","lifecycle_state":"published","original_author":{"name":"a","x-b":"c"},` +
				`"details":{"en":{"language":` + language + `,"purpose":"p","x-in-hash":1,"use":null}}}`,
			want: `{"_type":"RESOURCE_DESCRIPTION","lifecycle_state":"published","original_author":{"name":"a","x-b":"c"},` +
				`"details":{"en":{"_type":"RESOURCE_DESCRIPTION_ITEM","language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},"purpose":"p"}}}`,
		},
	})
	got, err := expectedform.Prune([]byte(`{"_type":"RESOURCE_DESCRIPTION","details":{"en":{"purpose":"p","x-in-hash":1,"use":null}}}`), "RESOURCE_DESCRIPTION")
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if want := `{"_type":"RESOURCE_DESCRIPTION","details":{"en":{"purpose":"p","use":null}}}`; !wireEqual(t, want, got) {
		t.Errorf("Prune = %s, want %s", got, want)
	}
}

// TestREQ052_BuildForReadsTheRootFromTheTarget pins that BuildFor and
// PruneFor read the root the way canjson.WithUnmatched does (REQ-170): a
// generic target's type argument binds the class's type parameter, so the
// data of an rm.OriginalVersion[rm.DVText] is a DV_TEXT, not a generic
// payload, and the bounds of an rm.DVInterval[rm.DVQuantity] may elide their
// "_type". Build and Prune with the bare class name bind nothing, which is
// the contrast each case checks.
func TestREQ052_BuildForReadsTheRootFromTheTarget(t *testing.T) {
	const version = `{"_type":"ORIGINAL_VERSION","data":{"value":"a","x-a":1}}`
	t.Run("ORIGINAL_VERSION bound to DV_TEXT", func(t *testing.T) {
		got, err := expectedform.BuildFor([]byte(version), new(rm.OriginalVersion[rm.DVText]))
		if err != nil {
			t.Fatalf("BuildFor error: %v", err)
		}
		if want := `{"_type":"ORIGINAL_VERSION","data":{"_type":"DV_TEXT","value":"a"}}`; !wireEqual(t, want, got) {
			t.Errorf("BuildFor(%s, *rm.OriginalVersion[rm.DVText]) = %s, want %s", version, got, want)
		}
		got, err = expectedform.PruneFor([]byte(version), new(rm.OriginalVersion[rm.DVText]))
		if err != nil {
			t.Fatalf("PruneFor error: %v", err)
		}
		if want := `{"_type":"ORIGINAL_VERSION","data":{"value":"a"}}`; !wireEqual(t, want, got) {
			t.Errorf("PruneFor(%s, *rm.OriginalVersion[rm.DVText]) = %s, want %s", version, got, want)
		}
		got, err = expectedform.Prune([]byte(version), "ORIGINAL_VERSION")
		if err != nil {
			t.Fatalf("Prune error: %v", err)
		}
		if !wireEqual(t, version, got) {
			t.Errorf("Prune(%s, ORIGINAL_VERSION) = %s, want the generic payload as written", version, got)
		}
	})
	t.Run("DV_INTERVAL bound to DV_QUANTITY", func(t *testing.T) {
		const interval = `{"lower":{"magnitude":1,"units":"kg"},"upper":{"magnitude":2,"units":"kg","x-a":1}}`
		got, err := expectedform.BuildFor([]byte(interval), new(rm.DVInterval[rm.DVQuantity]))
		if err != nil {
			t.Fatalf("BuildFor error: %v", err)
		}
		want := `{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},"upper":{"_type":"DV_QUANTITY","magnitude":2,"units":"kg"},` +
			`"lower_unbounded":false,"upper_unbounded":false,"lower_included":true,"upper_included":true}`
		if !wireEqual(t, want, got) {
			t.Errorf("BuildFor(%s, *rm.DVInterval[rm.DVQuantity]) = %s, want %s", interval, got, want)
		}
		if _, err := expectedform.Build([]byte(interval), "DV_INTERVAL"); err == nil || !strings.Contains(err.Error(), "type parameter with no binding") {
			t.Errorf("Build(%s, DV_INTERVAL) error = %v, want the elided bound refused: the class name binds nothing", interval, err)
		}
	})
	t.Run("DV_INTERVAL bound to an abstract class keeps a zero bound beside its open flag", func(t *testing.T) {
		// Bound to DV_ORDERED the SDK holds a bound by reference, so the
		// encoder keeps a zero one; bound to DV_COUNT it holds the bound by
		// value, and the encoder drops it as empty.
		const in = `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":0},"upper":{"_type":"DV_COUNT","magnitude":5},"lower_unbounded":true,"upper_unbounded":false}`
		const flags = `"lower_unbounded":true,"upper_unbounded":false,"lower_included":false,"upper_included":true}`
		for _, tc := range []struct {
			name   string
			target any
			want   string
		}{
			{name: "DV_ORDERED", target: new(rm.DVInterval[rm.DVOrdered]), want: `{"_type":"DV_INTERVAL","lower":{"_type":"DV_COUNT","magnitude":0},"upper":{"_type":"DV_COUNT","magnitude":5},` + flags},
			{name: "DV_COUNT", target: new(rm.DVInterval[rm.DVCount]), want: `{"_type":"DV_INTERVAL","upper":{"_type":"DV_COUNT","magnitude":5},` + flags},
		} {
			got, err := expectedform.BuildFor([]byte(in), tc.target)
			if err != nil {
				t.Fatalf("BuildFor(%s) error: %v", tc.name, err)
			}
			if !wireEqual(t, tc.want, got) {
				t.Errorf("BuildFor(%s, *rm.DVInterval[%s]) = %s, want %s", in, tc.name, got, tc.want)
			}
		}
	})
	t.Run("targets with no single RM value at the root", func(t *testing.T) {
		for name, target := range map[string]any{
			"slice": new([]rm.DVText),
			"any":   new(any),
			"map":   new(map[string]any),
		} {
			if got, err := expectedform.BuildFor([]byte(`{"value":"a"}`), target); err == nil || !strings.Contains(err.Error(), "no single RM value") {
				t.Errorf("BuildFor(%s target) = %s, %v, want an error naming the target", name, got, err)
			}
		}
	})
}

// wireEqual reports whether got is wire-equivalent to want.
func wireEqual(t *testing.T, want string, got []byte) bool {
	t.Helper()
	ok, diff := wireequiv.Equivalent([]byte(want), got)
	if !ok {
		t.Log(diff)
	}
	return ok
}

// TestREQ052_BuildWritesNumbersAsTheEncoderDoes pins that a number held by a
// numeric attribute, bare or quoted, becomes the literal the encoder writes
// for its value (ADR 0004), and that a value that is not such a number is
// kept as written.
func TestREQ052_BuildWritesNumbersAsTheEncoderDoes(t *testing.T) {
	quantity := func(magnitude, precision string) string {
		return `{"_type":"DV_QUANTITY","magnitude":` + magnitude + `,"units":"kg","precision":` + precision + `}`
	}
	runBuild(t, []buildCase{
		{name: "quoted Real and Integer", root: "DV_QUANTITY", in: quantity(`"80.50"`, `"2"`), want: quantity(`80.5`, `2`)},
		{name: "trailing zero", root: "DV_QUANTITY", in: quantity(`80.0`, `2`), want: quantity(`80`, `2`)},
		{name: "exponent", root: "DV_QUANTITY", in: quantity(`1e2`, `2`), want: quantity(`100`, `2`)},
		{name: "large exponent", root: "DV_QUANTITY", in: quantity(`1e21`, `2`), want: quantity(`1e+21`, `2`)},
		{name: "small fraction", root: "DV_QUANTITY", in: quantity(`0.0000001`, `2`), want: quantity(`1e-7`, `2`)},
		{name: "not a number kept", root: "DV_QUANTITY", in: quantity(`"heavy"`, `2`), want: quantity(`"heavy"`, `2`)},
		{name: "Integer64", root: "DV_COUNT", in: `{"_type":"DV_COUNT","magnitude":"0012"}`, want: `{"_type":"DV_COUNT","magnitude":12}`},
		{name: "a String member is not a number", root: "DV_TEXT", in: `{"_type":"DV_TEXT","value":"1.50"}`, want: `{"_type":"DV_TEXT","value":"1.50"}`},
	})
}

// TestREQ052_BuildRefusesInputItCannotRead pins the errors: Build never guesses
// a type or a shape, so an input it cannot read against the RM is reported
// rather than turned into an expected form. A case that names wantErr must
// wrap that error; the others must name the reason in the message.
func TestREQ052_BuildRefusesInputItCannotRead(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		in      string
		want    string // a substring of the error message
		wantErr error  // when set, an error the result must wrap
	}{
		{name: "not JSON", root: "DV_TEXT", in: `{`, wantErr: io.ErrUnexpectedEOF},
		{name: "not an object", root: "DV_TEXT", in: `[1]`, want: "not a JSON object"},
		{name: "trailing data", root: "DV_TEXT", in: `{"value":"v"} {}`, want: "after top-level"},
		{name: "empty root type", root: "", in: `{"_type":"DV_TEXT","value":"v"}`, want: "root type is empty"},
		{name: "unknown _type", root: "DV_TEXT", in: `{"_type":"DV_TEXTUAL","value":"v"}`, want: "names no RM class"},
		{name: "empty _type", root: "ELEMENT", in: `{"_type":"ELEMENT","archetype_node_id":"at1","name":{"_type":"","value":"n"}}`, want: `_type "" names no RM class`},
		{name: "_type not a string", root: "DV_TEXT", in: `{"_type":1,"value":"v"}`, want: "not a string"},
		{name: "elided _type on an abstract slot", root: "ELEMENT", in: element(`{"value":"v"}`), want: "abstract"},
		{name: "elided _type behind an unbound parameter", root: "ELEMENT", in: element(`{"_type":"DV_INTERVAL","lower":{"magnitude":1,"units":"kg"}}`), want: "type parameter with no binding"},
		{name: "container not an array", root: "DV_TEXT", in: `{"_type":"DV_TEXT","value":"v","mappings":{}}`, want: "want a JSON array"},
		{name: "class value not an object", root: "DV_TEXT", in: `{"_type":"DV_TEXT","value":"v","language":"en"}`, want: "want a JSON object"},
		{name: "Hash of RM objects not an object", root: "RESOURCE_DESCRIPTION", in: `{"_type":"RESOURCE_DESCRIPTION","details":[]}`, want: "want a JSON object for a Hash attribute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expectedform.Build([]byte(tc.in), tc.root)
			if err == nil {
				t.Fatalf("Build(%s, %q) = %s, want an error", tc.in, tc.root, got)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("Build(%s, %q) error = %v, want it to wrap %v", tc.in, tc.root, err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Build(%s, %q) error = %q, want it to contain %q", tc.in, tc.root, err, tc.want)
			}
		})
	}
}

// TestREQ052_PruneOnlyRemovesMembersTheRMDoesNotDefine pins that Prune applies
// the member filter and nothing else: a null member (a mandatory String among
// them), an elided "_type", an omitted interval flag and a quoted number all
// stay as written, and so does a number inside a generic payload, which Build
// writes as the encoder does.
func TestREQ052_PruneOnlyRemovesMembersTheRMDoesNotDefine(t *testing.T) {
	in := `{"_type":"DV_QUANTITY","magnitude":"80.50","units":null,"precision":null,"note":{"_type":"DV_TEXT","value":"x"},` +
		`"normal_range":{"lower":{"magnitude":1,"units":"kg","extra":{"_type":"DV_BOOLEAN","value":true}},"lower_unbounded":false,"upper_unbounded":true}}`
	want := `{"_type":"DV_QUANTITY","magnitude":"80.50","units":null,"precision":null,` +
		`"normal_range":{"lower":{"magnitude":1,"units":"kg"},"lower_unbounded":false,"upper_unbounded":true}}`
	got, err := expectedform.Prune([]byte(in), "DV_QUANTITY")
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if ok, diff := wireequiv.Equivalent([]byte(want), got); !ok {
		t.Errorf("Prune(%s) = %s, want %s: %s", in, got, want, diff)
	}

	const payload = `{"_type":"ORIGINAL_VERSION","data":{"_type":"DV_QUANTITY","magnitude":1.50,"x-list":[1e2]}}`
	got, err = expectedform.Prune([]byte(payload), "ORIGINAL_VERSION")
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if ok, diff := wireequiv.Equivalent([]byte(payload), got); !ok {
		t.Errorf("Prune(%s) = %s, want the payload's numbers as written: %s", payload, got, diff)
	}
}

// TestRootType pins that the root type is read from the decode target.
func TestRootType(t *testing.T) {
	if got, ok := expectedform.RootType(new(rm.Composition)); !ok || got != "COMPOSITION" {
		t.Errorf("RootType(*rm.Composition) = %q, %v, want COMPOSITION, true", got, ok)
	}
	if got, ok := expectedform.RootType(new(int)); ok {
		t.Errorf("RootType(*int) = %q, true, want false", got)
	}
}
