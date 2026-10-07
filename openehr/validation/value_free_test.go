package validation_test

// value_free_test.go: REQ-168 on validation.Issue. Each instance validator
// keeps the value it read out of Path, Code, Detail and Severity and hands it
// over only through Value, which never prints, encodes or logs it. Every row
// plants a marker that no Detail text contains, so a Detail that quotes the
// value again, a Value that is dropped, or a Value that reaches JSON fails a
// named test here.

import (
	"bytes"
	"encoding/gob"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// systolicValuePath is where vital_signs.opt constrains the systolic
// DV_QUANTITY that validVitalSignsComposition carries.
const systolicValuePath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

// dateMarker is a DV_DATE value that is not ISO 8601 and that no Detail
// text contains. The rendering test reads it back through every output.
const dateMarker = "MARKER-1a2b"

// valueCase is one row of REQ-168's table of what Value holds on an Issue.
type valueCase struct {
	name string
	// run validates the instance that carries the marker.
	run func(t *testing.T) validation.Result
	// code and path locate the one issue the row is about.
	code, path string
	// markers are the renderings of the planted value, each of which must
	// appear in no value-free field of any issue the run reports.
	markers []string
	// want is what Value.Reveal() must return, with its exact Go type.
	want any
}

// valueCases returns the rows of REQ-168's table that hold a comparable
// value: the RM-floor rows and a primitive through the template walker.
// The decode-error row is TestREQ168_DecodeErrorInValue.
//
// The temporal row has one case for each of the four types, held by
// pointer inside an ELEMENT, and cases where the type is held by value as
// an interval's bounds, so every branch that reads the value string is
// pinned.
func valueCases() []valueCase {
	precision := func(n int) *rm.Integer {
		v := rm.Integer(n)
		return &v
	}
	// inElement validates an ELEMENT whose value is v.
	inElement := func(v rm.DataValue) func(*testing.T) validation.Result {
		return func(*testing.T) validation.Result {
			el := validElement()
			el.Value = v
			return validation.ValidateRM(el)
		}
	}
	// boundCases returns the rows for an interval whose two bounds are
	// temporal values held by value, each carrying its own marker.
	boundCases := func(rmType string, interval any, lower, upper string) []valueCase {
		run := func(*testing.T) validation.Result { return validation.ValidateRM(interval) }
		return []valueCase{
			{name: "rm_invariant " + rmType + " by value, lower bound", run: run, code: "rm_invariant", path: "/lower", markers: []string{lower, upper}, want: lower},
			{name: "rm_invariant " + rmType + " by value, upper bound", run: run, code: "rm_invariant", path: "/upper", markers: []string{lower, upper}, want: upper},
		}
	}
	temporal := slices.Concat(
		[]valueCase{
			{
				name:    "rm_invariant DV_DATE_TIME value (Value_valid)",
				run:     inElement(&rm.DVDateTime{Value: "MARKER-dt-3c4d"}),
				code:    "rm_invariant",
				path:    "/value",
				markers: []string{"MARKER-dt-3c4d"},
				want:    "MARKER-dt-3c4d",
			},
			{
				name:    "rm_invariant DV_DATE value (Value_valid)",
				run:     inElement(&rm.DVDate{Value: dateMarker}),
				code:    "rm_invariant",
				path:    "/value",
				markers: []string{dateMarker},
				want:    dateMarker,
			},
			{
				name:    "rm_invariant DV_TIME value (Value_valid)",
				run:     inElement(&rm.DVTime{Value: "MARKER-t-5e6f"}),
				code:    "rm_invariant",
				path:    "/value",
				markers: []string{"MARKER-t-5e6f"},
				want:    "MARKER-t-5e6f",
			},
			{
				name:    "rm_invariant DV_DURATION value (Value_valid)",
				run:     inElement(&rm.DVDuration{Value: "MARKER-du-7a8b"}),
				code:    "rm_invariant",
				path:    "/value",
				markers: []string{"MARKER-du-7a8b"},
				want:    "MARKER-du-7a8b",
			},
		},
		boundCases("DV_DATE_TIME", &rm.DVInterval[rm.DVDateTime]{
			Lower: rm.DVDateTime{Value: "MARKER-dt-lo"}, Upper: rm.DVDateTime{Value: "MARKER-dt-hi"}, LowerIncluded: true, UpperIncluded: true,
		}, "MARKER-dt-lo", "MARKER-dt-hi"),
		boundCases("DV_DATE", &rm.DVInterval[rm.DVDate]{
			Lower: rm.DVDate{Value: "MARKER-date-lo"}, Upper: rm.DVDate{Value: "MARKER-date-hi"}, LowerIncluded: true, UpperIncluded: true,
		}, "MARKER-date-lo", "MARKER-date-hi"),
		boundCases("DV_TIME", &rm.DVInterval[rm.DVTime]{
			Lower: rm.DVTime{Value: "MARKER-time-lo"}, Upper: rm.DVTime{Value: "MARKER-time-hi"}, LowerIncluded: true, UpperIncluded: true,
		}, "MARKER-time-lo", "MARKER-time-hi"),
		boundCases("DV_DURATION", &rm.DVInterval[rm.DVDuration]{
			Lower: rm.DVDuration{Value: "MARKER-du-lo"}, Upper: rm.DVDuration{Value: "MARKER-du-hi"}, LowerIncluded: true, UpperIncluded: true,
		}, "MARKER-du-lo", "MARKER-du-hi"),
	)
	return slices.Concat(temporal, []valueCase{
		{
			name: "rm_invariant DV_QUANTITY precision below -1",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRM(&rm.DVQuantity{Magnitude: 1, Units: "mg", Precision: precision(-987)})
			},
			code:    "rm_invariant",
			path:    "/",
			markers: []string{"-987"},
			want:    rm.Integer(-987),
		},
		{
			name: "rm_invariant DV_PROPORTION precision below -1",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRM(&rm.DVProportion{Numerator: 1, Denominator: 2, Precision: precision(-9876)})
			},
			code:    "rm_invariant",
			path:    "/",
			markers: []string{"-9876"},
			want:    rm.Integer(-9876),
		},
		{
			name: "rm_invariant DV_PROPORTION precision 0 with a fractional operand",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRM(&rm.DVProportion{Numerator: 12345.678, Denominator: 24680, Precision: precision(0)})
			},
			code:    "rm_invariant",
			path:    "/",
			markers: []string{"12345.678", "24680"},
			want:    [2]float64{12345.678, 24680},
		},
		{
			name: "rm_invariant DV_INTERVAL lower above upper",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRM(&rm.DVInterval[rm.DVCount]{
					Lower:         rm.DVCount{Magnitude: 98765},
					Upper:         rm.DVCount{Magnitude: 4321},
					LowerIncluded: true,
					UpperIncluded: true,
				})
			},
			code:    "rm_invariant",
			path:    "/",
			markers: []string{"98765", "4321"},
			want:    [2]float64{98765, 4321},
		},
		{
			name: "term_mapping_match",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRM(&rm.DVText{
					Value: "text",
					Mappings: []rm.TermMapping{{
						Match: "Ω",
						Target: rm.CodePhrase{
							TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
							CodeString:    "73211009",
						},
					}},
				})
			},
			code:    "term_mapping_match",
			path:    "/mappings[0]/match",
			markers: []string{"Ω"},
			want:    rm.Character("Ω"),
		},
		{
			name: "primitive_out_of_range through the template walker",
			run: func(t *testing.T) validation.Result {
				comp := validVitalSignsComposition()
				systolicElement(t, comp).Value = &rm.DVQuantity{Magnitude: 98765.25, Units: "mm[Hg]"}
				return validation.ValidateComposition(comp, mustCompile(t, "vital_signs"))
			},
			code:    "primitive_out_of_range",
			path:    systolicValuePath,
			markers: []string{"98765.25"},
			want:    98765.25,
		},
	})
}

// TestREQ168_InstanceIssuesKeepTheValueInValue plants a marker for each row
// of REQ-168's table and checks that the issue carries it in Value, with the
// exact Go type, and that no issue of the run carries it in a value-free
// field.
func TestREQ168_InstanceIssuesKeepTheValueInValue(t *testing.T) {
	for _, tc := range valueCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.run(t)
			issue := oneIssue(t, r, tc.code, tc.path)
			if got := issue.Value.Reveal(); got != tc.want {
				t.Errorf("%s at %q: Value.Reveal() = %T %v, want %T %v", tc.code, tc.path, got, got, tc.want, tc.want)
			}
			assertValueFree(t, r, tc.markers...)
		})
	}
}

// TestREQ168_DecodeErrorInValue covers the invalid_shape row: input that
// fails either decode of ValidateRMEHRStatusBytes reports one invalid_shape
// at "/" whose Value is the decode error. When the EHR_STATUS decode fails
// on a nested DV_QUANTITY that quotes its magnitude, the error's text quotes
// the literal, which stays out of every value-free field.
func TestREQ168_DecodeErrorInValue(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		// marker, when set, is a literal the decode error quotes.
		marker string
	}{
		{name: "malformed JSON", data: []byte(`{"_type": "EHR_STATUS",`)},
		{name: "a JSON array, not an object", data: []byte(`["EHR_STATUS"]`)},
		{name: "EHR_STATUS decode fails on a quoted magnitude", data: ehrStatusWithQuotedMagnitude("MARKER98765"), marker: "MARKER98765"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRMEHRStatusBytes(tc.data)
			issue := oneIssue(t, r, "invalid_shape", "/")
			if len(r.Issues) != 1 {
				t.Errorf("ValidateRMEHRStatusBytes(%s) reported %d issues, want 1; issues=%+v", tc.data, len(r.Issues), r.Issues)
			}
			if tc.marker != "" {
				assertValueFree(t, r, tc.marker)
			}
			err, ok := issue.Value.Reveal().(error)
			if !ok {
				t.Fatalf("ValidateRMEHRStatusBytes(%s): invalid_shape Value.Reveal() = %T, want an error", tc.data, issue.Value.Reveal())
			}
			if !strings.Contains(err.Error(), tc.marker) {
				t.Errorf("invalid_shape Value.Reveal().Error() = %q, want it to contain the decode literal %q", err.Error(), tc.marker)
			}
		})
	}
}

// TestREQ168_EmptyValue pins the issues that read no value, one case per
// kind the package emits: something absent, a count, a type or identity
// mismatch, a guard, and a ValidateAQL issue. Each listed path must carry an
// issue with the code; a case with no paths takes every issue with the code.
// The AQL issue's Detail may quote the query by design, so only its Value is
// checked.
//
// primitive_wrong_type has no case: the walker checks the RM type before it
// runs a primitive constraint, so no ordinary composition reaches it.
func TestREQ168_EmptyValue(t *testing.T) {
	vitalSigns := func(edit func(*rm.Composition)) func(t *testing.T) validation.Result {
		return func(t *testing.T) validation.Result {
			comp := validVitalSignsComposition()
			edit(comp)
			return validation.ValidateComposition(comp, mustCompile(t, "vital_signs"))
		}
	}
	itemList := func(comp *rm.Composition) *rm.ItemList {
		return comp.Content[0].(*rm.Observation).Data.Events[0].(*rm.PointEvent[rm.ItemStructure]).Data.(*rm.ItemList)
	}
	floor := func(root any) func(*testing.T) validation.Result {
		return func(*testing.T) validation.Result { return validation.ValidateRM(root) }
	}
	const bloodPressure = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	type unknownRoot struct{ X int }

	cases := []struct {
		name  string
		run   func(t *testing.T) validation.Result
		code  string
		paths []string
	}{
		{
			name:  "required through the template walker",
			run:   vitalSigns(func(c *rm.Composition) { itemList(c).Items = nil }),
			code:  "required",
			paths: []string{bloodPressure + "/data/events[at0006]/data/items"},
		},
		{
			name: "required on the floor",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRMEHRStatusBytes([]byte(`{
					"_type": "EHR_STATUS",
					"name": {"_type": "DV_TEXT", "value": "EHR Status"},
					"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
					"is_modifiable": true,
					"is_queryable": true
				}`))
			},
			code:  "required",
			paths: []string{"/subject"},
		},
		{
			name:  "rm_invariant ELEMENT with neither value nor null_flavour",
			run:   floor(validElement()),
			code:  "rm_invariant",
			paths: []string{"/"},
		},
		{
			name:  "rm_invariant CODE_PHRASE with an empty code_string",
			run:   floor(&rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}),
			code:  "rm_invariant",
			paths: []string{"/"},
		},
		{
			name: "rm_invariant OBJECT_REF without id, type and namespace",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRMFolder(&rm.Folder{
					ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
					Name:            rm.DVText{Value: "root"},
					Items:           []rm.ObjectRefLike{rm.ObjectRef{}},
				})
			},
			code:  "rm_invariant",
			paths: []string{"/items[0]/id", "/items[0]/type", "/items[0]/namespace"},
		},
		{
			name:  "mappings_valid",
			run:   floor(&rm.DVText{Value: "text", Mappings: []rm.TermMapping{}}),
			code:  "mappings_valid",
			paths: []string{"/mappings"},
		},
		{
			name: "is_archetype_root",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRMEHRStatusBytes(ehrStatusWithArchetypeDetails(""))
			},
			code:  "is_archetype_root",
			paths: []string{"/archetype_details"},
		},
		{
			name:  "rm_version_valid",
			run:   floor(&rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-EHR_STATUS.generic.v1"}}),
			code:  "rm_version_valid",
			paths: []string{"/rm_version"},
		},
		{
			name:  "cardinality through the template walker",
			run:   vitalSigns(func(c *rm.Composition) { c.Content[0].(*rm.Observation).Data.Events = nil }),
			code:  "cardinality",
			paths: []string{bloodPressure + "/data/events"},
		},
		{
			name: "rm_type_mismatch",
			run: vitalSigns(func(c *rm.Composition) {
				c.Content = []rm.ContentItem{&rm.Evaluation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1"}}
			}),
			code: "rm_type_mismatch",
		},
		{
			name: "slot_fill",
			run: vitalSigns(func(c *rm.Composition) {
				c.Content = []rm.ContentItem{&rm.Observation{ArchetypeNodeID: "openEHR-EHR-OBSERVATION.no_such_archetype.v1"}}
			}),
			code: "slot_fill",
		},
		{
			name: "node_id_mismatch",
			run:  vitalSigns(func(c *rm.Composition) { itemList(c).ArchetypeNodeID = "at9999" }),
			code: "node_id_mismatch",
		},
		{
			name:  "nil_root guard",
			run:   floor(nil),
			code:  "nil_root",
			paths: []string{"/"},
		},
		{
			name: "nil_composition guard",
			run: func(t *testing.T) validation.Result {
				return validation.ValidateComposition(nil, mustCompile(t, "vital_signs"))
			},
			code: "nil_composition",
		},
		{
			name:  "rm_type_unknown",
			run:   floor(&unknownRoot{X: 1}),
			code:  "rm_type_unknown",
			paths: []string{"/"},
		},
		{
			name: "invalid_shape on null input, which raises no decode error",
			run: func(*testing.T) validation.Result {
				return validation.ValidateRMEHRStatusBytes([]byte("null"))
			},
			code:  "invalid_shape",
			paths: []string{"/"},
		},
		{
			name: "aql_syntax from ValidateAQL",
			run: func(*testing.T) validation.Result {
				return validation.ValidateAQL(aql.NewQuery("SELECT 'MARKER-aql' FROM"), nil)
			},
			code: "aql_syntax",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.run(t)
			for _, path := range tc.paths {
				if !containsIssue(r.Issues, path, tc.code) {
					t.Errorf("no %s issue at %q; issues=%+v", tc.code, path, r.Issues)
				}
			}
			var found bool
			for _, issue := range r.Issues {
				if issue.Code != tc.code {
					continue
				}
				found = true
				if got := issue.Value.Reveal(); got != nil {
					t.Errorf("%s at %q: Value.Reveal() = %T %v, want nil", issue.Code, issue.Path, got, got)
				}
				if got := issue.Value.String(); got != "" {
					t.Errorf("%s at %q: Value.String() = %q, want empty", issue.Code, issue.Path, got)
				}
			}
			if !found {
				t.Errorf("no %s issue; issues=%+v", tc.code, r.Issues)
			}
		})
	}
}

// TestREQ168_RenderingKeepsTheValueOut prints, encodes and logs an Issue and
// a Result from a real floor run whose Value holds a string marker, and fails
// when the marker appears in any output or when an encoding fails.
func TestREQ168_RenderingKeepsTheValueOut(t *testing.T) {
	el := validElement()
	el.Value = &rm.DVDate{Value: dateMarker}
	result := validation.ValidateRM(el)
	issue := oneIssue(t, result, "rm_invariant", "/value")
	if issue.Value.Reveal() != dateMarker {
		t.Fatalf("rm_invariant at /value: Value.Reveal() = %v, want the marker; the rendering checks below would prove nothing", issue.Value.Reveal())
	}

	type issueRecord struct{ issue validation.Issue }
	type resultRecord struct{ result validation.Result }
	subjects := []struct {
		name string
		v    any
	}{
		{name: "Issue", v: issue},
		{name: "Result", v: result},
		{name: "Issue in an unexported field", v: issueRecord{issue: issue}},
		{name: "Result in an unexported field", v: resultRecord{result: result}},
	}

	t.Run("fmt", func(t *testing.T) {
		for _, s := range subjects {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%d", "%p"} {
				if out := fmt.Sprintf(verb, s.v); strings.Contains(out, dateMarker) {
					t.Errorf("fmt.Sprintf(%q, %s) = %q, which contains the marker", verb, s.name, out)
				}
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		encoders := []struct {
			name    string
			marshal func(any) ([]byte, error)
		}{
			{name: "encoding/json", marshal: jsonv1.Marshal},
			{name: "encoding/json/v2", marshal: func(v any) ([]byte, error) { return jsonv2.Marshal(v) }},
		}
		for _, enc := range encoders {
			out, err := enc.marshal(issue)
			if err != nil {
				t.Fatalf("%s Marshal(Issue): %v", enc.name, err)
			}
			assertNoMarker(t, enc.name+" Issue", out)
			var asIssue map[string]any
			if err := jsonv1.Unmarshal(out, &asIssue); err != nil {
				t.Fatalf("%s Marshal(Issue) = %s, which does not read back: %v", enc.name, out, err)
			}
			if _, ok := asIssue["Value"]; ok {
				t.Errorf("%s Marshal(Issue) = %s, want no Value member", enc.name, out)
			}

			out, err = enc.marshal(result)
			if err != nil {
				t.Fatalf("%s Marshal(Result): %v", enc.name, err)
			}
			assertNoMarker(t, enc.name+" Result", out)
			var asResult struct{ Issues []map[string]any }
			if err := jsonv1.Unmarshal(out, &asResult); err != nil {
				t.Fatalf("%s Marshal(Result) = %s, which does not read back: %v", enc.name, out, err)
			}
			for _, i := range asResult.Issues {
				if _, ok := i["Value"]; ok {
					t.Errorf("%s Marshal(Result) = %s, want no Value member on its issues", enc.name, out)
				}
			}
		}
	})

	t.Run("gob", func(t *testing.T) {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(issue); err != nil {
			t.Fatalf("gob Encode(Issue): %v", err)
		}
		if bytes.Contains(buf.Bytes(), []byte(dateMarker)) {
			t.Errorf("gob Encode(Issue) carries the marker")
		}
		var gotIssue validation.Issue
		if err := gob.NewDecoder(&buf).Decode(&gotIssue); err != nil {
			t.Fatalf("gob Decode(Issue): %v", err)
		}
		assertSameButEmptyValue(t, "gob round trip of Issue", gotIssue, issue)

		buf.Reset()
		if err := gob.NewEncoder(&buf).Encode(result); err != nil {
			t.Fatalf("gob Encode(Result): %v", err)
		}
		if bytes.Contains(buf.Bytes(), []byte(dateMarker)) {
			t.Errorf("gob Encode(Result) carries the marker")
		}
		var gotResult validation.Result
		if err := gob.NewDecoder(&buf).Decode(&gotResult); err != nil {
			t.Fatalf("gob Decode(Result): %v", err)
		}
		if gotResult.OK != result.OK || len(gotResult.Issues) != len(result.Issues) {
			t.Fatalf("gob round trip of Result = OK %v with %d issues, want OK %v with %d", gotResult.OK, len(gotResult.Issues), result.OK, len(result.Issues))
		}
		for k := range result.Issues {
			assertSameButEmptyValue(t, fmt.Sprintf("gob round trip of Result.Issues[%d]", k), gotResult.Issues[k], result.Issues[k])
		}
	})

	t.Run("slog", func(t *testing.T) {
		handlers := []struct {
			name string
			new  func(*bytes.Buffer) slog.Handler
		}{
			{name: "text", new: func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) }},
			{name: "JSON", new: func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) }},
		}
		for _, h := range handlers {
			var buf bytes.Buffer
			logger := slog.New(h.new(&buf))
			logger.Info("validated", slog.Any("issue", issue), slog.Any("result", result))
			if strings.Contains(buf.String(), dateMarker) {
				t.Errorf("slog %s handler wrote %q, which contains the marker", h.name, buf.String())
			}
		}
	})
}

// TestREQ168_IssuesStayComparable checks that every issue the rows produce
// equals itself under ==, and that two runs over the same input give issues
// that compare equal, so the values the SDK puts in Value are comparable.
// The decode-error row is checked against itself only: two decode errors are
// distinct values.
func TestREQ168_IssuesStayComparable(t *testing.T) {
	for _, tc := range valueCases() {
		t.Run(tc.name, func(t *testing.T) {
			first, second := tc.run(t), tc.run(t)
			if len(first.Issues) != len(second.Issues) {
				t.Fatalf("two runs reported %d and %d issues", len(first.Issues), len(second.Issues))
			}
			for k, issue := range first.Issues {
				if copied := issue; copied != issue {
					t.Errorf("Issues[%d] (%s at %q) != a copy of itself", k, issue.Code, issue.Path)
				}
				if issue != second.Issues[k] {
					t.Errorf("Issues[%d] (%s at %q) differs between two runs over the same input", k, issue.Code, issue.Path)
				}
			}
		})
	}
	t.Run("invalid_shape from a failed decode", func(t *testing.T) {
		r := validation.ValidateRMEHRStatusBytes(ehrStatusWithQuotedMagnitude("MARKER98765"))
		if len(r.Issues) == 0 {
			t.Fatal("ValidateRMEHRStatusBytes(quoted magnitude) reported no issue")
		}
		for k, issue := range r.Issues {
			if copied := issue; copied != issue {
				t.Errorf("Issues[%d] (%s at %q) != a copy of itself", k, issue.Code, issue.Path)
			}
		}
	})
}

// oneIssue returns the one issue of r with code at path, and stops the test
// unless there is exactly one.
func oneIssue(t *testing.T, r validation.Result, code, path string) validation.Issue {
	t.Helper()
	var found []validation.Issue
	for _, issue := range r.Issues {
		if issue.Code == code && issue.Path == path {
			found = append(found, issue)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d %s issues at %q, want 1; issues=%+v", len(found), code, path, r.Issues)
	}
	return found[0]
}

// assertValueFree fails the test when a marker appears in Path, Code, Detail
// or Severity of any issue of r.
func assertValueFree(t *testing.T, r validation.Result, markers ...string) {
	t.Helper()
	for _, issue := range r.Issues {
		fields := []struct{ name, text string }{
			{name: "Path", text: issue.Path},
			{name: "Code", text: issue.Code},
			{name: "Detail", text: issue.Detail},
			{name: "Severity", text: issue.Severity.String()},
		}
		for _, f := range fields {
			for _, m := range markers {
				if strings.Contains(f.text, m) {
					t.Errorf("%s at %q: %s = %q, which contains the submitted value %q", issue.Code, issue.Path, f.name, f.text, m)
				}
			}
		}
	}
}

// assertNoMarker fails the test when encoded JSON contains the marker.
func assertNoMarker(t *testing.T, what string, out []byte) {
	t.Helper()
	if bytes.Contains(out, []byte(dateMarker)) {
		t.Errorf("%s encodes as %s, which contains the marker", what, out)
	}
}

// assertSameButEmptyValue fails the test unless got has want's Path, Code,
// Detail and Severity and an empty Value.
func assertSameButEmptyValue(t *testing.T, what string, got, want validation.Issue) {
	t.Helper()
	if got.Path != want.Path || got.Code != want.Code || got.Detail != want.Detail || got.Severity != want.Severity {
		t.Errorf("%s = {%q %q %q %v}, want {%q %q %q %v}", what, got.Path, got.Code, got.Detail, got.Severity, want.Path, want.Code, want.Detail, want.Severity)
	}
	if v := got.Value.Reveal(); v != nil {
		t.Errorf("%s: Value.Reveal() = %T, want nil", what, v)
	}
}

// ehrStatusWithQuotedMagnitude returns an EHR_STATUS whose other_details
// holds a DV_QUANTITY with its magnitude quoted as the string literal, which
// the decode refuses.
func ehrStatusWithQuotedMagnitude(literal string) []byte {
	return []byte(`{
	"_type": "EHR_STATUS",
	"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
	"name": {"_type": "DV_TEXT", "value": "s"},
	"subject": {"_type": "PARTY_SELF"},
	"is_queryable": true,
	"is_modifiable": true,
	"other_details": {
		"_type": "ITEM_TREE",
		"archetype_node_id": "at0001",
		"name": {"_type": "DV_TEXT", "value": "tree"},
		"items": [{
			"_type": "ELEMENT",
			"archetype_node_id": "at0002",
			"name": {"_type": "DV_TEXT", "value": "element"},
			"value": {"_type": "DV_QUANTITY", "magnitude": "` + literal + `", "units": "mm"}
		}]
	}
}`)
}
