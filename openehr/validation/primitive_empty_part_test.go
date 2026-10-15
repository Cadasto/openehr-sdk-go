package validation_test

import (
	"cmp"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-102 (Validation dimensions, Primitive constraints) and REQ-112 — a
// primitive constraint bound to a value with parts does not judge a part the
// RM rules report as absent or empty, so the RM's `required` at that part is
// the one finding for it: a CODE_PHRASE's code_string, whose terminology the
// constraint still judges, a DV_QUANTITY's units, the value of the temporal
// type an AOM 1.4 short name binds, and the code string of a DV_ORDINAL's
// symbol, whose value the constraint still judges alone. A part the RM rules
// let be blank, a DV_SCALE symbol's code_string, and a part the RM does not
// require, an optional String, are judged as before; so are a zero number
// and false, which are values, and a temporal value that is not empty.
// REQ-171 — where the depth or the visit limit stops the walk before the RM
// rules reach the part, they report nothing there, and the constraint judges
// the part as it judges any value, at its place in walk order.

// OPT children for the empty-part cases.
const (
	// quantityChild admits a DV_QUANTITY of 1 to 100 cm.
	quantityChild = `
      <children xsi:type="C_DV_QUANTITY">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id></node_id>
        <list>
          <magnitude>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>1</lower><upper>100</upper>
          </magnitude>
          <units>cm</units>
        </list>
      </children>`
	// scaleChild admits a DV_SCALE whose symbol is coded as local at0001 or
	// at0002.
	scaleChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_SCALE</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>symbol</rm_attribute_name>` + dvCodedTextChild + `
        </attributes>
      </children>`
	// identifierChild admits a DV_IDENTIFIER whose optional issuer starts
	// with XYZ.
	identifierChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_IDENTIFIER</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>issuer</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id></node_id>
            <item xsi:type="C_STRING"><pattern>XYZ.*</pattern></item>
          </children>
        </attributes>
      </children>`
	// ordinalChild admits a DV_ORDINAL of 1 coded as local at0001, or of 2
	// coded as local at0002.
	ordinalChild = `
      <children xsi:type="C_DV_ORDINAL">
        <rm_type_name>DV_ORDINAL</rm_type_name>
        <node_id></node_id>
        <list>
          <value>1</value>
          <symbol><value/><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0001</code_string></defining_code></symbol>
        </list>
        <list>
          <value>2</value>
          <symbol><value/><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0002</code_string></defining_code></symbol>
        </list>
      </children>`
)

// pinnedIssue is the part of an Issue the empty-part rows pin.
type pinnedIssue struct {
	Code, Path string
	Layer      validation.Layer
	Rule       string
}

// pinnedIssues returns what the rows pin of issues, sorted by code, then
// path.
func pinnedIssues(issues []validation.Issue) []pinnedIssue {
	out := walkOrder(issues)
	sortPinned(out)
	return out
}

// walkOrder returns what the rows pin of issues, in the order the walk
// reported them.
func walkOrder(issues []validation.Issue) []pinnedIssue {
	out := make([]pinnedIssue, 0, len(issues))
	for _, iss := range issues {
		out = append(out, pinnedIssue{Code: iss.Code, Path: iss.Path, Layer: iss.Layer, Rule: iss.Rule})
	}
	return out
}

func sortPinned(issues []pinnedIssue) {
	slices.SortFunc(issues, func(a, b pinnedIssue) int {
		return cmp.Or(cmp.Compare(a.Code, b.Code), cmp.Compare(a.Path, b.Path))
	})
}

// rmRequired is the RM's `required` at path, rule mandatory.
func rmRequired(path string) pinnedIssue {
	return pinnedIssue{Code: "required", Path: path, Layer: validation.LayerRM, Rule: "mandatory"}
}

// templateFinding is the template's primitive_<rule> at path.
func templateFinding(rule, path string) pinnedIssue {
	return pinnedIssue{Code: "primitive_" + rule, Path: path, Layer: validation.LayerTemplate, Rule: rule}
}

// codedAs returns a DV_CODED_TEXT whose defining code is code in
// terminology.
func codedAs(terminology, code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        "coded",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: terminology}, CodeString: code},
	}
}

// REQ-102, REQ-112 — each row binds one value to ELEMENT.value under one
// OPT child and pins the whole result. The first rows blank a part the RM
// rules report: the RM's `required` at the part is its one finding, and the
// constraint judges only what the part leaves. The other rows are judged as
// before the rule: the part is one the RM lets be blank or does not
// require, or the value is not empty.
func TestValidate_REQ102_REQ112_PrimitiveLeavesTheEmptyPartToTheRM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		child string
		value rm.DataValue
		want  []pinnedIssue
	}{
		{
			name:  "C_CODE_PHRASE list, blank code_string",
			child: dvCodedTextChild,
			value: codedAs("local", ""),
			want:  []pinnedIssue{rmRequired("/value/defining_code/code_string")},
		},
		{
			// The constraint still judges the terminology.
			name:  "C_CODE_PHRASE list, blank code_string in another terminology",
			child: dvCodedTextChild,
			value: codedAs("SNOMED-CT", ""),
			want: []pinnedIssue{
				templateFinding("invalid_value", "/value/defining_code"),
				rmRequired("/value/defining_code/code_string"),
			},
		},
		{
			// The magnitude is outside every range, but the ranges are per
			// unit: nothing is left to judge.
			name:  "C_DV_QUANTITY units list, blank units",
			child: quantityChild,
			value: &rm.DVQuantity{Magnitude: 500},
			want:  []pinnedIssue{rmRequired("/value/units")},
		},
		{
			name:  "C_DV_ORDINAL list, blank symbol code",
			child: ordinalChild,
			value: &rm.DVOrdinal{Value: 1, Symbol: codedAs("local", "")},
			want:  []pinnedIssue{rmRequired("/value/symbol/defining_code/code_string")},
		},
		{
			// The constraint still judges the value alone.
			name:  "C_DV_ORDINAL list, unlisted value with a blank symbol code",
			child: ordinalChild,
			value: &rm.DVOrdinal{Value: 3, Symbol: codedAs("local", "")},
			want: []pinnedIssue{
				templateFinding("not_in_list", "/value"),
				rmRequired("/value/symbol/defining_code/code_string"),
			},
		},
		{
			name:  "DURATION, blank value",
			child: durationChild,
			value: rm.DVDuration{},
			want:  []pinnedIssue{rmRequired("/value/value")},
		},
		{
			name:  "DATE, blank value",
			child: shortNameChild("DATE", "C_DATE", ""),
			value: &rm.DVDate{},
			want:  []pinnedIssue{rmRequired("/value/value")},
		},
		{
			name:  "TIME, blank value",
			child: shortNameChild("TIME", "C_TIME", ""),
			value: &rm.DVTime{},
			want:  []pinnedIssue{rmRequired("/value/value")},
		},
		{
			name:  "DATE_TIME, blank value",
			child: shortNameChild("DATE_TIME", "C_DATE_TIME", ""),
			value: &rm.DVDateTime{},
			want:  []pinnedIssue{rmRequired("/value/value")},
		},
		{
			// The RM lets a DV_SCALE symbol's code_string be blank, so it
			// reports nothing there, and the constraint judges the code.
			name:  "DV_SCALE symbol, blank code_string under a list",
			child: scaleChild,
			value: &rm.DVScale{Value: 0.5, Symbol: codedAs("local", "")},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value/symbol/defining_code")},
		},
		{
			name:  "optional String issuer, blank under a pattern",
			child: identifierChild,
			value: &rm.DVIdentifier{ID: "XYZ1", Issuer: new("")},
			want:  []pinnedIssue{templateFinding("pattern_mismatch", "/value/issuer")},
		},
		{
			name:  "C_DV_QUANTITY units list, zero magnitude",
			child: quantityChild,
			value: &rm.DVQuantity{Magnitude: 0, Units: "cm"},
			want:  []pinnedIssue{templateFinding("out_of_range", "/value")},
		},
		{
			name:  "INTEGER 10 to 20, zero count",
			child: integerChild,
			value: rm.DVCount{Magnitude: 0},
			want:  []pinnedIssue{templateFinding("out_of_range", "/value")},
		},
		{
			name:  "BOOLEAN true only, false",
			child: shortNameChild("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>false</false_valid>"),
			value: rm.DVBoolean{Value: false},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value")},
		},
		{
			name:  "DATE, malformed value",
			child: shortNameChild("DATE", "C_DATE", ""),
			value: &rm.DVDate{Value: "yesterday"},
			want: []pinnedIssue{
				templateFinding("invalid_value", "/value"),
				{Code: "rm_invariant", Path: "/value", Layer: validation.LayerRM, Rule: "DV_DATE.Value_valid"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			got := pinnedIssues(validation.Validate(valueElement(tc.value), c).Issues)
			sortPinned(tc.want)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues =\n  %+v\nwant\n  %+v", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-102, REQ-171 — at the depth limit the walk does not visit the
// attributes of the node it stops at, so the RM rules report no empty part
// there, and the constraint judges the value as before, beside the
// max_depth stop: a blank code_string is not left unreported.
func TestValidator_REQ102_REQ171_EmptyPartAtTheDepthLimit(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, elementValueOPT(dvCodedTextChild))
	v := validation.New(validation.WithLimits(validation.Limits{Depth: 2}))
	got := pinnedIssues(v.Validate(valueElement(codedAs("local", "")), c).Issues)
	want := []pinnedIssue{
		{Code: "max_depth", Path: "/value/defining_code"},
		templateFinding("not_in_list", "/value/defining_code"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("Validate(blank code_string at the depth limit) issues =\n  %+v\nwant\n  %+v", got, want)
	}
}

// REQ-102, REQ-171 — where the walk visits the node's attributes but stops
// before the RM rules reach its empty part, at the visit limit on an earlier
// attribute or at the depth limit below the node, the RM rules report
// nothing there, and the constraint judges the value as it judges any
// value: its finding comes at the node, in walk order, before the stop. The
// last rows stop the walk after the RM rules reported the part, whose
// `required` then stands for the constraint's finding, as with no stop.
// Each row pins the whole result in walk order.
func TestValidator_REQ102_REQ171_EmptyPartWhereTheWalkStops(t *testing.T) {
	t.Parallel()
	const (
		code     = "/value/defining_code"
		symbol   = "/value/symbol"
		ordCode  = symbol + "/defining_code"
		ordBlank = ordCode + "/code_string"
	)
	stop := func(cond, path string) pinnedIssue { return pinnedIssue{Code: cond, Path: path} }
	blankOrdinal := &rm.DVOrdinal{Value: 1, Symbol: codedAs("local", "")}
	tests := []struct {
		name  string
		child string
		value rm.DataValue
		lim   validation.Limits
		want  []pinnedIssue
	}{
		{
			name:  "blank code_string, visit limit at terminology_id",
			child: dvCodedTextChild,
			value: codedAs("local", ""),
			lim:   validation.Limits{Nodes: 3},
			want:  []pinnedIssue{templateFinding("not_in_list", code), stop("max_nodes", code+"/terminology_id")},
		},
		{
			name:  "blank code_string, visit limit below terminology_id",
			child: dvCodedTextChild,
			value: codedAs("local", ""),
			lim:   validation.Limits{Nodes: 4},
			want:  []pinnedIssue{templateFinding("not_in_list", code), stop("max_nodes", code+"/terminology_id/value")},
		},
		{
			// The terminology finding keeps its place before the code's.
			name:  "blank code_string in another terminology, visit limit at terminology_id",
			child: dvCodedTextChild,
			value: codedAs("SNOMED-CT", ""),
			lim:   validation.Limits{Nodes: 3},
			want: []pinnedIssue{
				templateFinding("invalid_value", code),
				templateFinding("not_in_list", code),
				stop("max_nodes", code+"/terminology_id"),
			},
		},
		{
			name:  "blank units, visit limit at magnitude",
			child: quantityChild,
			value: &rm.DVQuantity{Magnitude: 500},
			lim:   validation.Limits{Nodes: 2},
			want:  []pinnedIssue{templateFinding("unit_unknown", "/value"), stop("max_nodes", "/value/magnitude")},
		},
		{
			name:  "blank symbol code, visit limit at symbol",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Nodes: 2},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value"), stop("max_nodes", symbol)},
		},
		{
			name:  "blank symbol code, visit limit below terminology_id",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Nodes: 6},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value"), stop("max_nodes", ordCode+"/terminology_id/value")},
		},
		{
			name:  "blank symbol code, depth limit at symbol",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Depth: 2},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value"), stop("max_depth", symbol)},
		},
		{
			name:  "blank symbol code, depth limit at the defining code",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Depth: 3},
			want:  []pinnedIssue{templateFinding("not_in_list", "/value"), stop("max_depth", ordCode)},
		},
		{
			name:  "blank code_string, visit limit after it",
			child: dvCodedTextChild,
			value: codedAs("local", ""),
			lim:   validation.Limits{Nodes: 5},
			want:  []pinnedIssue{rmRequired(code + "/code_string"), stop("max_nodes", "/value/value")},
		},
		{
			name:  "blank code_string, depth limit at terminology_id",
			child: dvCodedTextChild,
			value: codedAs("local", ""),
			lim:   validation.Limits{Depth: 3},
			want:  []pinnedIssue{stop("max_depth", code+"/terminology_id"), rmRequired(code + "/code_string")},
		},
		{
			name:  "blank units, visit limit after them",
			child: quantityChild,
			value: &rm.DVQuantity{Magnitude: 500},
			lim:   validation.Limits{Nodes: 3},
			want:  []pinnedIssue{rmRequired("/value/units"), stop("max_nodes", "/name")},
		},
		{
			name:  "blank symbol code, visit limit after it",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Nodes: 7},
			want:  []pinnedIssue{rmRequired(ordBlank), stop("max_nodes", "/value/value")},
		},
		{
			name:  "blank symbol code, depth limit at terminology_id",
			child: ordinalChild,
			value: blankOrdinal,
			lim:   validation.Limits{Depth: 4},
			want:  []pinnedIssue{stop("max_depth", ordCode+"/terminology_id"), rmRequired(ordBlank)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			v := validation.New(validation.WithLimits(tc.lim))
			got := walkOrder(v.Validate(valueElement(tc.value), c).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) with limits %+v issues, in walk order =\n  %+v\nwant\n  %+v", tc.name, tc.lim, got, tc.want)
			}
		})
	}
}

// demonstrationCluster is the issue path of the cluster at0004 of the first
// event in the Demonstration.v1 fixture.
const demonstrationCluster = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0002]/data/items[at0004]"

// demonstrationElement decodes a fresh Demonstration.v1 composition and
// returns it with the element at index of the first event's cluster at0004,
// which must carry nodeID.
func demonstrationElement(t *testing.T, index int, nodeID string) (*rm.Composition, *rm.Element) {
	t.Helper()
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("Demonstration.v1"), &comp)
	obs := comp.Content[0].(*rm.Observation)
	event := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	cluster := event.Data.(*rm.ItemTree).Items[1].(*rm.Cluster)
	element := cluster.Items[index].(*rm.Element)
	if element.ArchetypeNodeID != nodeID {
		t.Fatalf("element archetype_node_id = %q, want %s (fixture drift)", element.ArchetypeNodeID, nodeID)
	}
	return &comp, element
}

// REQ-102, REQ-112 — in the Demonstration.v1 corpus composition, a blank
// code_string under at0006's closed C_CODE_PHRASE and blank units under
// at0012's C_DV_QUANTITY units list each add one issue to the fixture's own
// result: the RM's `required` at the part.
func TestValidateComposition_REQ102_REQ112_CorpusEmptyPartReportedOnce(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "Demonstration.v1")
	var fixture rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("Demonstration.v1"), &fixture)
	baseline := pinnedIssues(validation.ValidateComposition(&fixture, c).Issues)
	tests := []struct {
		name   string
		index  int
		nodeID string
		blank  func(t *testing.T, value rm.DataValue)
		want   pinnedIssue
	}{
		{
			name:   "at0006 blank code_string",
			index:  1,
			nodeID: "at0006",
			blank: func(t *testing.T, value rm.DataValue) {
				t.Helper()
				coded, ok := value.(*rm.DVCodedText)
				if !ok || coded.DefiningCode.CodeString == "" {
					t.Fatalf("at0006 value = %T, want a coded *rm.DVCodedText (fixture drift)", value)
				}
				coded.DefiningCode.CodeString = ""
			},
			want: rmRequired(demonstrationCluster + "/items[at0006]/value/defining_code/code_string"),
		},
		{
			name:   "at0012 blank units",
			index:  3,
			nodeID: "at0012",
			blank: func(t *testing.T, value rm.DataValue) {
				t.Helper()
				q, ok := value.(*rm.DVQuantity)
				if !ok || q.Units == "" {
					t.Fatalf("at0012 value = %T, want a *rm.DVQuantity with units (fixture drift)", value)
				}
				q.Units = ""
			},
			want: rmRequired(demonstrationCluster + "/items[at0012]/value/units"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp, element := demonstrationElement(t, tc.index, tc.nodeID)
			tc.blank(t, element.Value)
			added, removed := diffPinned(baseline, pinnedIssues(validation.ValidateComposition(comp, c).Issues))
			if want := []pinnedIssue{tc.want}; !slices.Equal(added, want) || len(removed) != 0 {
				t.Errorf("ValidateComposition(Demonstration.v1, %s) against the fixture's own result:\n  added   %+v\n  removed %+v\nwant added %+v and nothing removed", tc.name, added, removed, want)
			}
		})
	}
}

// diffPinned returns the issues of got that base does not hold, and those
// of base that got does not hold, each counted as often as it occurs; both
// come back sorted, as base and got are.
func diffPinned(base, got []pinnedIssue) (added, removed []pinnedIssue) {
	left := make(map[pinnedIssue]int, len(base))
	for _, iss := range base {
		left[iss]++
	}
	for _, iss := range got {
		if left[iss] > 0 {
			left[iss]--
			continue
		}
		added = append(added, iss)
	}
	for _, iss := range base {
		if left[iss] > 0 {
			left[iss]--
			removed = append(removed, iss)
		}
	}
	return added, removed
}
