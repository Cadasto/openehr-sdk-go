package expectedform_test

import (
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
// nested. A null optional String, and a null mandatory member of another type,
// still read as absent.
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
// among them.
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
	})
}

// TestREQ052_BuildRemovesAnEmptyBoundBesideItsOpenFlag pins that an empty
// bound on a side marked unbounded is removed, and that a non-empty one, or an
// empty one on a closed side, is kept. Where the slot binds the bound to a
// concrete class, a bound carrying only zero values is empty; behind an
// unbound parameter an object is never empty.
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
	})
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
// a type or a shape, so an input the decoder would refuse is reported rather
// than turned into an expected form.
func TestREQ052_BuildRefusesInputItCannotRead(t *testing.T) {
	cases := []struct {
		name string
		root string
		in   string
		want string
	}{
		{"not JSON", "DV_TEXT", `{`, "unexpected EOF"},
		{"not an object", "DV_TEXT", `[1]`, "not a JSON object"},
		{"trailing data", "DV_TEXT", `{"value":"v"} {}`, "after top-level"},
		{"empty root type", "", `{"_type":"DV_TEXT","value":"v"}`, "root type is empty"},
		{"unknown _type", "DV_TEXT", `{"_type":"DV_TEXTUAL","value":"v"}`, "names no RM class"},
		{"_type not a string", "DV_TEXT", `{"_type":1,"value":"v"}`, "not a string"},
		{"elided _type on an abstract slot", "ELEMENT", element(`{"value":"v"}`), "abstract"},
		{"elided _type behind an unbound parameter", "ELEMENT", element(`{"_type":"DV_INTERVAL","lower":{"magnitude":1,"units":"kg"}}`), "type parameter with no binding"},
		{"container not an array", "DV_TEXT", `{"_type":"DV_TEXT","value":"v","mappings":{}}`, "want a JSON array"},
		{"class value not an object", "DV_TEXT", `{"_type":"DV_TEXT","value":"v","language":"en"}`, "want a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expectedform.Build([]byte(tc.in), tc.root)
			if err == nil {
				t.Fatalf("Build(%s, %q) = %s, want an error containing %q", tc.in, tc.root, got, tc.want)
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
// stay as written.
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
