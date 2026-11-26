package validation_test

import (
	encjson "encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// required returns the paths at which r reports `required`, sorted.
func required(r validation.Result) []string {
	var out []string
	for _, i := range r.Issues {
		if i.Code == "required" {
			out = append(out, i.Path)
		}
	}
	slices.Sort(out)
	return out
}

// TestREQ052_FloorReadsAnEncodedEmptyMemberAsBefore holds the floor to the
// findings it gave before the canonical encoders left out an empty mandatory
// member held by value: for a value that holds such members empty, the floor
// reports `required` at the same paths whether it reads the value itself or
// what canonical JSON or canonical XML writes for it. A member left out is
// one the floor already reported as missing, so leaving it out moves no
// `required` finding.
//
// REQ-052 § An absent mandatory member; REQ-112 § Value-typed mandatory
// presence.
func TestREQ052_FloorReadsAnEncodedEmptyMemberAsBefore(t *testing.T) {
	name := rm.DVText{Value: "n"}
	for _, tc := range []struct {
		name string
		v    any
		root func() any
		want []string // the paths of `required` the floor reports
	}{
		{
			name: "COMPOSITION with empty category, language and territory",
			v:    &rm.Composition{ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1", Name: name},
			root: func() any { return new(rm.Composition) },
			want: []string{"/category", "/composer", "/language", "/territory"},
		},
		{
			name: "PARTY_RELATIONSHIP with an empty source and target",
			v:    &rm.PartyRelationship{ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1", Name: name},
			root: func() any { return new(rm.PartyRelationship) },
			want: []string{"/source", "/target"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := validation.ValidateRM(tc.v)
			if got := required(value); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%T) reports required at %q, want %q", tc.v, got, tc.want)
			}
			j, err := canjson.Marshal(tc.v)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", tc.v, err)
			}
			x, err := canxml.Marshal(tc.v)
			if err != nil {
				t.Fatalf("canxml.Marshal(%T): %v", tc.v, err)
			}
			for format, r := range map[string]validation.Result{
				"canonical JSON": validation.ValidateJSON(j, tc.root(), nil),
				"canonical XML":  validation.ValidateXML(x, tc.root(), nil),
			} {
				if got := required(r); !slices.Equal(got, tc.want) {
					t.Errorf("the floor reads the %s of %T with required at %q, want %q", format, tc.v, got, tc.want)
				}
			}
		})
	}
}

// minFloorCorpusDocuments is a floor on the corpus documents the test below
// reads: a skip rule that stopped matching would otherwise empty it.
const minFloorCorpusDocuments = 90

// extra returns the members of got that want lacks.
func extra(got, want []string) []string {
	var out []string
	for _, g := range got {
		if !slices.Contains(want, g) {
			out = append(out, g)
		}
	}
	return out
}

// emptyValues returns the JSON Pointer of every empty list and every empty
// string in doc, a canonical-JSON document. Canonical XML writes no element
// for an empty list, and an element with no text for an empty string, which
// its decoder reads as carrying no value.
func emptyValues(doc []byte) []string {
	var v any
	if encjson.Unmarshal(doc, &v) != nil {
		return nil
	}
	var out []string
	var walk func(v any, ptr string)
	walk = func(v any, ptr string) {
		switch x := v.(type) {
		case map[string]any:
			for k, m := range x {
				walk(m, ptr+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"))
			}
		case string:
			if x == "" {
				out = append(out, ptr)
			}
		case []any:
			if len(x) == 0 {
				out = append(out, ptr)
			}
			for i, m := range x {
				walk(m, ptr+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(v, "")
	return out
}

// TestREQ052_CorpusReencodeAddsNoMissingMember re-encodes every canonical-JSON
// document of the corpus that decodes, and holds each output to the rule
// that it leaves out only what the floor already reports as missing:
//
//   - the floor reports `required` on the canonical-JSON re-encode at no path
//     it does not report on the input, so a document the floor accepts
//     re-encodes to one it accepts, and a member with a value, or an
//     EHR_STATUS subject that is a PARTY_SELF, is never left out;
//   - canonical XML's decoder reports no member of the canonical-XML
//     re-encode missing that canonical JSON's decoder does not report on the
//     canonical-JSON re-encode, except an empty list or an empty string,
//     which canonical XML writes as no element and as an element with no
//     text, read back as no value.
//
// REQ-052 § An absent mandatory member; REQ-056; REQ-175.
func TestREQ052_CorpusReencodeAddsNoMissingMember(t *testing.T) {
	t.Parallel()
	root := fixtures.CorpusRoot()
	read := 0
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
		if encjson.Unmarshal(raw, &head) != nil || head.Type == "" {
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
		rel, _ := filepath.Rel(root, path)
		j, err := canjson.Marshal(v)
		if err != nil {
			t.Errorf("%s: canjson.Marshal: %v", rel, err)
			return nil
		}
		in := required(validation.ValidateJSON(raw, ctor(), nil))
		if added := extra(required(validation.ValidateJSON(j, ctor(), nil)), in); len(added) > 0 {
			t.Errorf("%s: the floor reports required at %q on the canonical-JSON re-encode and not on the input", rel, added)
		}
		read++

		x, err := canxml.Marshal(v)
		if err != nil {
			return nil // a value canonical XML does not model, such as a Hash of Hashes
		}
		var jsonMissing, xmlMissing []string
		if err := canjson.Unmarshal(j, ctor(), canjson.WithMissing(func(p canjson.Pointer) { jsonMissing = append(jsonMissing, p.String()) })); err != nil {
			t.Errorf("%s: decode the canonical-JSON re-encode: %v", rel, err)
			return nil
		}
		if err := canxml.Unmarshal(x, ctor(), canxml.WithMissing(func(p canxml.Pointer) { xmlMissing = append(xmlMissing, p.String()) })); err != nil {
			t.Errorf("%s: decode the canonical-XML re-encode: %v", rel, err)
			return nil
		}
		if added := extra(extra(xmlMissing, jsonMissing), emptyValues(j)); len(added) > 0 {
			t.Errorf("%s: the canonical-XML re-encode leaves out %q, which the canonical-JSON re-encode writes", rel, added)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if read < minFloorCorpusDocuments {
		t.Errorf("read %d corpus documents, want at least %d", read, minFloorCorpusDocuments)
	}
}
