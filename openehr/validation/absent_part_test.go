package validation_test

// absent_part_test.go: REQ-102 § Primitive constraints, REQ-112 § Value-typed
// mandatory presence and REQ-171 § One finding per defect. A part of a value
// that a primitive constraint reads, and that the RM rules report absent, is
// reported by the RM's `required` alone: the constraint does not judge the
// zero value the decode leaves in its place.

import (
	json "encoding/json/v2"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// quantityMagnitudes returns the JSON Pointer of the magnitude of every
// DV_QUANTITY in doc, in document order.
func quantityMagnitudes(t *testing.T, doc []byte) []string {
	t.Helper()
	var tree any
	if err := json.Unmarshal(doc, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	var out []string
	var visit func(v any, pointer string)
	visit = func(v any, pointer string) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["magnitude"]; ok && x["_type"] == "DV_QUANTITY" {
				out = append(out, pointer+"/magnitude")
			}
			for _, k := range slices.Sorted(maps.Keys(x)) {
				visit(x[k], pointer+"/"+k)
			}
		case []any:
			for i, item := range x {
				visit(item, pointer+"/"+strconv.Itoa(i))
			}
		}
	}
	visit(tree, "")
	return out
}

// REQ-102, REQ-112, REQ-171 — the corpus composition whose quantities a
// C_DV_QUANTITY holds to 5 to 10 cm, with the magnitude of every DV_QUANTITY
// left out or sent as null: the decode reports each magnitude missing, so the
// one finding for each is the RM's `required` at the magnitude, and the
// constraint does not judge the zero the decode leaves there. Every other
// finding is the fixture's own.
func TestValidateJSON_REQ102_REQ112_REQ171_CorpusAbsentMagnitudeIsRequiredAlone(t *testing.T) {
	t.Parallel()
	const id = "Test_dv_quantity_property_units_constrained_with_magnitude_range.v0"
	c := mustCompile(t, id)
	doc, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	// The quantities are the ELEMENT at0043 of each of the three events.
	pointers := quantityMagnitudes(t, doc)
	if len(pointers) != 3 {
		t.Fatalf("%s holds %d DV_QUANTITY magnitudes %q, want 3 (fixture drift)", id, len(pointers), pointers)
	}
	var want []pinnedIssue
	for i := range pointers {
		want = append(want, rmRequired("/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events["+strconv.Itoa(i+1)+"]/data/items[at0043]/value/magnitude"))
	}
	baseline := pinnedIssues(validation.ValidateJSON(doc, new(rm.Composition), c).Issues)
	nulled := make(map[string]string, len(pointers))
	for _, p := range pointers {
		nulled[p] = "null"
	}
	for name, edits := range map[string]map[string]string{
		"left out": dropped(pointers...),
		"null":     nulled,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := validation.ValidateJSON(rewriteJSON(t, string(doc), edits), new(rm.Composition), c)
			added, removed := diffPinned(baseline, pinnedIssues(r.Issues))
			if !slices.Equal(added, want) || len(removed) != 0 {
				t.Errorf("ValidateJSON(%s, magnitudes %s) against the fixture's own result:\n  added   %+v\n  removed %+v\nwant added %+v and nothing removed", id, name, added, removed, want)
			}
			for _, w := range want {
				quantity := strings.TrimSuffix(w.Path, "/magnitude")
				if got := atOrBelow(r, quantity); !slices.Equal(got, []string{"required " + w.Path}) {
					t.Errorf("ValidateJSON(%s, magnitudes %s) reports %q at or below %s, want the `required` at its magnitude alone", id, name, got, quantity)
				}
			}
		})
	}
}

// quantityPrecisionChild admits a DV_QUANTITY of 1 to 100 cm with a
// precision of 0 or 1.
const quantityPrecisionChild = `
      <children xsi:type="C_DV_QUANTITY">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id></node_id>
        <list>
          <magnitude>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>1</lower><upper>100</upper>
          </magnitude>
          <precision>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>0</lower><upper>1</upper>
          </precision>
          <units>cm</units>
        </list>
      </children>`

// elementID is the archetype id of the ELEMENT root elementValueOPT pins.
const elementID = "openEHR-EHR-ELEMENT.element_value.v1"

// elementJSON returns an ELEMENT that elementValueOPT's root matches, in
// canonical JSON, whose value member is value.
func elementJSON(value string) string {
	return `{"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "element"}, "archetype_node_id": "` + elementID + `", "archetype_details": ` + mmArchetyped(elementID) + `, "value": ` + value + `}`
}

// elementXML returns the ELEMENT of elementJSON in canonical XML, whose
// value element is value.
func elementXML(value string) string {
	return `<element ` + mmNS + ` archetype_node_id="` + elementID + `"><name><value>element</value></name>` + mmArchetypedXML(elementID) + value + `</element>`
}

// Ordinal symbols in canonical JSON and XML: local at0002, which
// ordinalChild lists beside the value 2, and local at0003, which it does not
// list.
const (
	listedSymbolJSON   = `"symbol": {"_type": "DV_CODED_TEXT", "value": "moderate", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "local"}, "code_string": "at0002"}}`
	listedSymbolXML    = `<symbol><value>moderate</value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0002</code_string></defining_code></symbol>`
	unlistedSymbolJSON = `"symbol": {"_type": "DV_CODED_TEXT", "value": "severe", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "local"}, "code_string": "at0003"}}`
	unlistedSymbolXML  = `<symbol><value>severe</value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0003</code_string></defining_code></symbol>`
)

// REQ-102, REQ-112, REQ-171 — each row is an ELEMENT document whose value a
// primitive constraint reads by its parts, with a part the RM makes
// mandatory left out or sent as null: a quantity's magnitude or units, an
// ordinal's value, symbol or symbol's defining code, the magnitude of the
// DV_COUNT the INTEGER short name binds and the value of the DV_BOOLEAN the
// BOOLEAN short name binds. The decode reports the part missing, so the RM's
// `required` at the part is its one finding, and the constraint judges only
// what the absent parts leave: a quantity's units and precision, an
// ordinal's value, or an ordinal's symbol beside the value of a pair the
// constraint lists. ValidateJSON, ValidateXML and Validate given the same
// report by WithMissing each give the row's whole result.
func TestValidateJSON_REQ102_REQ112_REQ171_AbsentPartIsRequiredAlone(t *testing.T) {
	t.Parallel()
	boolean := shortNameChild("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>false</false_valid>")
	tests := []struct {
		name      string
		child     string
		json, xml string
		pointers  []string
		want      []pinnedIssue
	}{
		{
			name:     "C_DV_QUANTITY, magnitude left out",
			child:    quantityChild,
			json:     `{"_type": "DV_QUANTITY", "units": "cm"}`,
			xml:      `<value xsi:type="DV_QUANTITY"><units>cm</units></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{rmRequired("/value/magnitude")},
		},
		{
			name:     "C_DV_QUANTITY, magnitude null",
			child:    quantityChild,
			json:     `{"_type": "DV_QUANTITY", "magnitude": null, "units": "cm"}`,
			xml:      `<value xsi:type="DV_QUANTITY"><magnitude xsi:nil="true"/><units>cm</units></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{rmRequired("/value/magnitude")},
		},
		{
			// The constraint still judges the units.
			name:     "C_DV_QUANTITY, magnitude left out, unlisted units",
			child:    quantityChild,
			json:     `{"_type": "DV_QUANTITY", "units": "m"}`,
			xml:      `<value xsi:type="DV_QUANTITY"><units>m</units></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{templateFinding("unit_unknown", "/value"), rmRequired("/value/magnitude")},
		},
		{
			// The constraint still judges the precision.
			name:     "C_DV_QUANTITY, magnitude left out, precision out of range",
			child:    quantityPrecisionChild,
			json:     `{"_type": "DV_QUANTITY", "units": "cm", "precision": 3}`,
			xml:      `<value xsi:type="DV_QUANTITY"><units>cm</units><precision>3</precision></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{templateFinding("out_of_range", "/value"), rmRequired("/value/magnitude")},
		},
		{
			name:     "C_DV_QUANTITY, magnitude left out, precision in range",
			child:    quantityPrecisionChild,
			json:     `{"_type": "DV_QUANTITY", "units": "cm", "precision": 1}`,
			xml:      `<value xsi:type="DV_QUANTITY"><units>cm</units><precision>1</precision></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{rmRequired("/value/magnitude")},
		},
		{
			name:     "C_DV_QUANTITY, units and magnitude left out",
			child:    quantityChild,
			json:     `{"_type": "DV_QUANTITY"}`,
			xml:      `<value xsi:type="DV_QUANTITY"/>`,
			pointers: []string{"/value/magnitude", "/value/units"},
			want:     []pinnedIssue{rmRequired("/value/magnitude"), rmRequired("/value/units")},
		},
		{
			name:     "C_DV_ORDINAL, value left out, listed symbol",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", ` + listedSymbolJSON + `}`,
			xml:      `<value xsi:type="DV_ORDINAL">` + listedSymbolXML + `</value>`,
			pointers: []string{"/value/value"},
			want:     []pinnedIssue{rmRequired("/value/value")},
		},
		{
			name:     "C_DV_ORDINAL, value null, listed symbol",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", "value": null, ` + listedSymbolJSON + `}`,
			xml:      `<value xsi:type="DV_ORDINAL"><value xsi:nil="true"/>` + listedSymbolXML + `</value>`,
			pointers: []string{"/value/value"},
			want:     []pinnedIssue{rmRequired("/value/value")},
		},
		{
			// The constraint still judges the symbol: no listed pair has it.
			name:     "C_DV_ORDINAL, value left out, unlisted symbol",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", ` + unlistedSymbolJSON + `}`,
			xml:      `<value xsi:type="DV_ORDINAL">` + unlistedSymbolXML + `</value>`,
			pointers: []string{"/value/value"},
			want:     []pinnedIssue{templateFinding("not_in_list", "/value"), rmRequired("/value/value")},
		},
		{
			name:     "C_DV_ORDINAL, symbol left out",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", "value": 1}`,
			xml:      `<value xsi:type="DV_ORDINAL"><value>1</value></value>`,
			pointers: []string{"/value/symbol"},
			want:     []pinnedIssue{rmRequired("/value/symbol")},
		},
		{
			name:     "C_DV_ORDINAL, symbol null",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", "value": 1, "symbol": null}`,
			xml:      `<value xsi:type="DV_ORDINAL"><value>1</value><symbol xsi:nil="true"/></value>`,
			pointers: []string{"/value/symbol"},
			want:     []pinnedIssue{rmRequired("/value/symbol")},
		},
		{
			// The constraint still judges the value alone.
			name:     "C_DV_ORDINAL, symbol left out, unlisted value",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", "value": 3}`,
			xml:      `<value xsi:type="DV_ORDINAL"><value>3</value></value>`,
			pointers: []string{"/value/symbol"},
			want:     []pinnedIssue{templateFinding("not_in_list", "/value"), rmRequired("/value/symbol")},
		},
		{
			name:     "C_DV_ORDINAL, symbol's defining code left out",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL", "value": 1, "symbol": {"_type": "DV_CODED_TEXT", "value": "mild"}}`,
			xml:      `<value xsi:type="DV_ORDINAL"><value>1</value><symbol><value>mild</value></symbol></value>`,
			pointers: []string{"/value/symbol/defining_code"},
			want:     []pinnedIssue{rmRequired("/value/symbol/defining_code")},
		},
		{
			name:     "C_DV_ORDINAL, value and symbol left out",
			child:    ordinalChild,
			json:     `{"_type": "DV_ORDINAL"}`,
			xml:      `<value xsi:type="DV_ORDINAL"/>`,
			pointers: []string{"/value/symbol", "/value/value"},
			want:     []pinnedIssue{rmRequired("/value/symbol"), rmRequired("/value/value")},
		},
		{
			name:     "INTEGER, DV_COUNT magnitude left out",
			child:    integerChild,
			json:     `{"_type": "DV_COUNT"}`,
			xml:      `<value xsi:type="DV_COUNT"/>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{rmRequired("/value/magnitude")},
		},
		{
			name:     "INTEGER, DV_COUNT magnitude null",
			child:    integerChild,
			json:     `{"_type": "DV_COUNT", "magnitude": null}`,
			xml:      `<value xsi:type="DV_COUNT"><magnitude xsi:nil="true"/></value>`,
			pointers: []string{"/value/magnitude"},
			want:     []pinnedIssue{rmRequired("/value/magnitude")},
		},
		{
			name:     "BOOLEAN true only, DV_BOOLEAN value left out",
			child:    boolean,
			json:     `{"_type": "DV_BOOLEAN"}`,
			xml:      `<value xsi:type="DV_BOOLEAN"/>`,
			pointers: []string{"/value/value"},
			want:     []pinnedIssue{rmRequired("/value/value")},
		},
		{
			name:     "BOOLEAN true only, DV_BOOLEAN value null",
			child:    boolean,
			json:     `{"_type": "DV_BOOLEAN", "value": null}`,
			xml:      `<value xsi:type="DV_BOOLEAN"><value xsi:nil="true"/></value>`,
			pointers: []string{"/value/value"},
			want:     []pinnedIssue{rmRequired("/value/value")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			doc := []byte(elementJSON(tc.json))
			decoded := new(rm.Element)
			if err := canjson.Unmarshal(doc, decoded); err != nil {
				t.Fatalf("canjson.Unmarshal(%s): %v", tc.name, err)
			}
			sortPinned(tc.want)
			for route, r := range map[string]validation.Result{
				"ValidateJSON":          validation.ValidateJSON(doc, new(rm.Element), c),
				"ValidateXML":           validation.ValidateXML([]byte(elementXML(tc.xml)), new(rm.Element), c),
				"Validate(WithMissing)": validation.Validate(decoded, c, validation.WithMissing(tc.pointers)),
			} {
				if got := pinnedIssues(r.Issues); !slices.Equal(got, tc.want) {
					t.Errorf("%s(%s) issues =\n  %+v\nwant\n  %+v", route, tc.name, got, tc.want)
				}
			}
		})
	}
}

// REQ-102, REQ-171 — an ordinal whose symbol's code string is blank and
// whose value the call's report names has two absent parts. The constraint's
// judgement of what they leave stands only where the RM rules report each of
// them: where the depth limit stops the walk inside the symbol, before they
// report its code string, or the visit limit stops it after the code string,
// at the code phrase's preferred term, and before the value, the constraint
// judges the whole value, at its place in walk order, so neither part goes
// unreported. Each row pins the whole result in walk order.
func TestValidator_REQ102_REQ171_AbsentPartsWhereTheWalkStops(t *testing.T) {
	t.Parallel()
	const (
		code      = "/value/symbol/defining_code/code_string"
		preferred = "/value/symbol/defining_code/preferred_term"
	)
	stop := func(cond, path string) pinnedIssue { return pinnedIssue{Code: cond, Path: path} }
	tests := []struct {
		name string
		lim  validation.Limits
		want []pinnedIssue
	}{
		{
			name: "no stop",
			want: []pinnedIssue{rmRequired(code), rmRequired("/value/value")},
		},
		{
			name: "depth limit at the symbol",
			lim:  validation.Limits{Depth: 2},
			want: []pinnedIssue{templateFinding("not_in_list", "/value"), stop("max_depth", "/value/symbol"), rmRequired("/value/value")},
		},
		{
			name: "visit limit at the preferred term",
			lim:  validation.Limits{Nodes: 7},
			want: []pinnedIssue{templateFinding("not_in_list", "/value"), rmRequired(code), stop("max_nodes", preferred)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(ordinalChild))
			v := validation.New(validation.WithLimits(tc.lim))
			ordinal := &rm.DVOrdinal{Value: 1, Symbol: codedAs("local", "")}
			ordinal.Symbol.DefiningCode.PreferredTerm = new("mild")
			got := walkOrder(v.Validate(valueElement(ordinal), c, validation.WithMissing([]string{"/value/value"})).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) with limits %+v issues, in walk order =\n  %+v\nwant\n  %+v", tc.name, tc.lim, got, tc.want)
			}
		})
	}
}

// REQ-102, REQ-171 — judging a quantity whose magnitude is absent leaves the
// compiled template as it was: the units and the precision are judged by a
// copy of the constraint with its magnitude ranges set aside, so a later
// call with the same compiled template still holds a magnitude to its range.
// A Validator holds no state between calls, and a compiled template serves
// every call that shares it.
func TestValidate_REQ102_REQ171_AbsentMagnitudeLeavesTheTemplateAsItWas(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, elementValueOPT(quantityChild))
	outOfRange := valueElement(&rm.DVQuantity{Magnitude: 500, Units: "cm"})
	want := []pinnedIssue{templateFinding("out_of_range", "/value")}
	if got := pinnedIssues(validation.Validate(outOfRange, c).Issues); !slices.Equal(got, want) {
		t.Fatalf("precondition: Validate(magnitude 500 cm) issues = %+v, want %+v", got, want)
	}
	absent := validation.ValidateJSON([]byte(elementJSON(`{"_type": "DV_QUANTITY", "units": "cm"}`)), new(rm.Element), c)
	if got, wantAbsent := pinnedIssues(absent.Issues), []pinnedIssue{rmRequired("/value/magnitude")}; !slices.Equal(got, wantAbsent) {
		t.Fatalf("precondition: ValidateJSON(magnitude left out) issues = %+v, want %+v", got, wantAbsent)
	}
	if got := pinnedIssues(validation.Validate(outOfRange, c).Issues); !slices.Equal(got, want) {
		t.Errorf("Validate(magnitude 500 cm) after a quantity with no magnitude, same compiled template, issues = %+v, want %+v: the call changed the template's magnitude range", got, want)
	}
}
