package template_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103 — buildPrimitive maps each closed-set xsi:type onto the
// matching constraint Go type during ParseOPT. Uses synthetic OPT
// XML so coverage stays tight regardless of which primitives appear
// in vendored fixtures.

const primitiveOPTTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>primitive_test</value></template_id>
  <concept>primitive_test</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      %s
    </attributes>
  </definition>
</template>`

func parseOPTWithChild(t *testing.T, childXML string) *template.OperationalTemplate {
	t.Helper()
	body := strings.Replace(primitiveOPTTemplate, "%s", childXML, 1)
	tmpl, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	return tmpl
}

func firstChildPrimitive(t *testing.T, tmpl *template.OperationalTemplate) constraints.PrimitiveConstraint {
	t.Helper()
	root, ok := tmpl.Root().(*template.ComplexObject)
	if !ok {
		t.Fatalf("Root not *ComplexObject: %T", tmpl.Root())
	}
	attrs := root.Attributes()
	if len(attrs) != 1 {
		t.Fatalf("want 1 attribute, got %d", len(attrs))
	}
	children := attrs[0].Children()
	if len(children) != 1 {
		t.Fatalf("want 1 child, got %d", len(children))
	}
	co, ok := children[0].(*template.ComplexObject)
	if !ok {
		t.Fatalf("child not *ComplexObject: %T", children[0])
	}
	return co.PrimitiveConstraint()
}

func TestParse_CBoolean(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_BOOLEAN">
		<rm_type_name>BOOLEAN</rm_type_name>
		<node_id />
		<true_valid>true</true_valid>
		<false_valid>false</false_valid>
		<assumed_value>true</assumed_value>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CBoolean)
	if !ok {
		t.Fatalf("want CBoolean, got %T", p)
	}
	if !c.TrueValid || c.FalseValid {
		t.Errorf("flags = (%v, %v), want (true, false)", c.TrueValid, c.FalseValid)
	}
	if c.Default == nil || *c.Default != true {
		t.Errorf("Default = %v, want &true", c.Default)
	}
}

// REQ-103 — single-element omission on C_BOOLEAN: AOM 1.4 declares
// both true_valid and false_valid as mandatory with the invariant
// "Both attributes cannot be set to False" (an unsatisfiable
// constraint). When the OPT XML omits one element, buildBoolean
// defaults each flag *independently* to true so the resulting
// constraint never synthesises the BMM-forbidden {false,false} case.
func TestParse_CBoolean_DefaultsAreIndependent(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantTrue   bool
		wantFalse  bool
		wantReject string // non-empty when the value must NOT be reported as valid
	}{
		{
			// Only <false_valid>false</false_valid> on the wire — the
			// classic bug case. Pre-fix this returned (false, false);
			// fixed it returns (true, false) — "true is allowed,
			// false is forbidden", a satisfiable constraint.
			name: "only_false_valid_false",
			body: `<children xsi:type="C_BOOLEAN">
				<rm_type_name>BOOLEAN</rm_type_name>
				<node_id />
				<false_valid>false</false_valid>
			</children>`,
			wantTrue:  true,
			wantFalse: false,
		},
		{
			// Only <true_valid>true</true_valid> on the wire — implies
			// "no constraint on the false side, so it defaults to
			// allowed".
			name: "only_true_valid_true",
			body: `<children xsi:type="C_BOOLEAN">
				<rm_type_name>BOOLEAN</rm_type_name>
				<node_id />
				<true_valid>true</true_valid>
			</children>`,
			wantTrue:  true,
			wantFalse: true,
		},
		{
			// Both omitted — unconstrained boolean, both allowed.
			// Existing pre-fix behaviour, preserved.
			name: "both_omitted",
			body: `<children xsi:type="C_BOOLEAN">
				<rm_type_name>BOOLEAN</rm_type_name>
				<node_id />
			</children>`,
			wantTrue:  true,
			wantFalse: true,
		},
		{
			// Explicit {false, true} — "true is forbidden, false is
			// allowed". The other satisfiable single-value
			// constraint. Asserts the explicit false on true_valid
			// still propagates.
			name: "explicit_false_true",
			body: `<children xsi:type="C_BOOLEAN">
				<rm_type_name>BOOLEAN</rm_type_name>
				<node_id />
				<true_valid>false</true_valid>
				<false_valid>true</false_valid>
			</children>`,
			wantTrue:  false,
			wantFalse: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := parseOPTWithChild(t, tc.body)
			p := firstChildPrimitive(t, tmpl)
			c, ok := p.(constraints.CBoolean)
			if !ok {
				t.Fatalf("want CBoolean, got %T", p)
			}
			if c.TrueValid != tc.wantTrue || c.FalseValid != tc.wantFalse {
				t.Errorf("flags = (%v, %v), want (%v, %v)",
					c.TrueValid, c.FalseValid, tc.wantTrue, tc.wantFalse)
			}
			// AOM invariant: not both false.
			if !c.TrueValid && !c.FalseValid {
				t.Errorf("unsatisfiable {false,false} synthesised — violates AOM 1.4 C_BOOLEAN invariant")
			}
		})
	}
}

func TestParse_CInteger(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_INTEGER">
		<rm_type_name>INTEGER</rm_type_name>
		<node_id />
		<range>
			<lower_included>true</lower_included>
			<upper_included>true</upper_included>
			<lower_unbounded>false</lower_unbounded>
			<upper_unbounded>false</upper_unbounded>
			<lower>0</lower>
			<upper>100</upper>
		</range>
		<list>1</list>
		<list>5</list>
		<list>10</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CInteger)
	if !ok {
		t.Fatalf("want CInteger, got %T", p)
	}
	if len(c.List) != 3 || c.List[2] != 10 {
		t.Errorf("List = %v, want [1 5 10]", c.List)
	}
	if !c.Range.IsBounded() || c.Range.Upper != 100 {
		t.Errorf("Range = %s, want [0..100]", c.Range)
	}
}

// REQ-103 — malformed list entries on C_INTEGER / C_REAL are silently
// dropped in lenient mode (forward-compat: a partially-broken OPT
// still yields a usable, weaker constraint) but surfaced as
// ErrInvalidOPT under [ParseOPTStrict] / [ParseFileStrict] so
// validators see the parse failure instead of an undocumented
// constraint loosening. Mirrors the strict/lenient split that already
// applies to unknown xsi:type values with nested attributes.
func TestParse_PrimitiveList_StrictRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "CInteger_malformed_list",
			body: `<children xsi:type="C_INTEGER">
				<rm_type_name>INTEGER</rm_type_name>
				<node_id />
				<list>1</list>
				<list>not-a-number</list>
				<list>3</list>
			</children>`,
		},
		{
			name: "CReal_malformed_list",
			body: `<children xsi:type="C_REAL">
				<rm_type_name>REAL</rm_type_name>
				<node_id />
				<list>1.0</list>
				<list>banana</list>
			</children>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"_lenient_skips", func(t *testing.T) {
			// ParseOPT (lenient) MUST drop the bad entry and parse.
			body := strings.Replace(primitiveOPTTemplate, "%s", tc.body, 1)
			tmpl, err := template.ParseOPT(strings.NewReader(body))
			if err != nil {
				t.Fatalf("ParseOPT (lenient) returned %v, want success with weakened constraint", err)
			}
			// Sanity: the well-formed entries still made it through.
			p := firstChildPrimitive(t, tmpl)
			switch c := p.(type) {
			case constraints.CInteger:
				if len(c.List) == 0 {
					t.Errorf("CInteger lenient: list lost ALL entries; expected the well-formed ones to survive")
				}
			case constraints.CReal:
				if len(c.List) == 0 {
					t.Errorf("CReal lenient: list lost ALL entries; expected the well-formed ones to survive")
				}
			default:
				t.Fatalf("unexpected primitive type %T", p)
			}
		})
		t.Run(tc.name+"_strict_rejects", func(t *testing.T) {
			body := strings.Replace(primitiveOPTTemplate, "%s", tc.body, 1)
			_, err := template.ParseOPTStrict(strings.NewReader(body))
			if !errors.Is(err, template.ErrInvalidOPT) {
				t.Errorf("ParseOPTStrict err = %v, want errors.Is(err, ErrInvalidOPT)", err)
			}
		})
	}
}

// TestParse_NumericRange_StrictRejectsMalformedBounds confirms the
// review-driven hardening: a C_INTEGER / C_REAL with bounded XML
// (lower_unbounded=false) but an unparseable numeric bound surfaces
// as ErrInvalidOPT under ParseOPTStrict, while lenient mode falls
// through to the unbounded sentinel. Mirrors the list-item
// strict/lenient split that already applies to C_INTEGER / C_REAL.
func TestParse_NumericRange_StrictRejectsMalformedBounds(t *testing.T) {
	body := `<children xsi:type="C_INTEGER">
		<rm_type_name>INTEGER</rm_type_name>
		<node_id />
		<range>
			<lower_included>true</lower_included>
			<upper_included>true</upper_included>
			<lower_unbounded>false</lower_unbounded>
			<upper_unbounded>false</upper_unbounded>
			<lower>not-a-number</lower>
			<upper>100</upper>
		</range>
	</children>`
	t.Run("lenient_falls_through_to_unbounded", func(t *testing.T) {
		tmpl := parseOPTWithChild(t, body)
		p := firstChildPrimitive(t, tmpl)
		ci, ok := p.(constraints.CInteger)
		if !ok {
			t.Fatalf("want CInteger, got %T", p)
		}
		if !ci.Range.LowerUnbounded {
			t.Errorf("lenient mode should mark malformed lower as Unbounded; Range = %s", ci.Range)
		}
	})
	t.Run("strict_rejects", func(t *testing.T) {
		full := strings.Replace(primitiveOPTTemplate, "%s", body, 1)
		_, err := template.ParseOPTStrict(strings.NewReader(full))
		if !errors.Is(err, template.ErrInvalidOPT) {
			t.Errorf("ParseOPTStrict err = %v, want errors.Is(err, ErrInvalidOPT)", err)
		}
	})
}

func TestParse_CString(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_STRING">
		<rm_type_name>STRING</rm_type_name>
		<node_id />
		<pattern>^[A-Z]+$</pattern>
		<list>YES</list>
		<list>NO</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CString)
	if !ok {
		t.Fatalf("want CString, got %T", p)
	}
	if c.Pattern != `^[A-Z]+$` {
		t.Errorf("Pattern = %q, want ^[A-Z]+$", c.Pattern)
	}
	if len(c.List) != 2 || c.List[0] != "YES" {
		t.Errorf("List = %v, want [YES NO]", c.List)
	}
}

func TestParse_CReal(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_REAL">
		<rm_type_name>REAL</rm_type_name>
		<node_id />
		<range>
			<lower_included>true</lower_included>
			<upper_included>true</upper_included>
			<lower_unbounded>false</lower_unbounded>
			<upper_unbounded>false</upper_unbounded>
			<lower>0.0</lower>
			<upper>100.0</upper>
		</range>
		<list>1.5</list>
		<list>2.5</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CReal)
	if !ok {
		t.Fatalf("want CReal, got %T", p)
	}
	if len(c.List) != 2 || c.List[1] != 2.5 {
		t.Errorf("List = %v, want [1.5 2.5]", c.List)
	}
	if !c.Range.IsBounded() || c.Range.Upper != 100.0 {
		t.Errorf("Range = %s, want [0..100]", c.Range)
	}
}

func TestParse_CDate(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DATE">
		<rm_type_name>DATE</rm_type_name>
		<node_id />
		<pattern>yyyy-mm-dd</pattern>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDate)
	if !ok {
		t.Fatalf("want CDate, got %T", p)
	}
	if c.Pattern != "yyyy-mm-dd" {
		t.Errorf("Pattern = %q, want yyyy-mm-dd", c.Pattern)
	}
}

func TestParse_CTime(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_TIME">
		<rm_type_name>TIME</rm_type_name>
		<node_id />
		<pattern>hh:mm:ss</pattern>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CTime)
	if !ok {
		t.Fatalf("want CTime, got %T", p)
	}
	if c.Pattern != "hh:mm:ss" {
		t.Errorf("Pattern = %q, want hh:mm:ss", c.Pattern)
	}
}

func TestParse_CDateTime(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DATE_TIME">
		<rm_type_name>DATE_TIME</rm_type_name>
		<node_id />
		<pattern>yyyy-mm-ddThh:mm:ss</pattern>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDateTime)
	if !ok {
		t.Fatalf("want CDateTime, got %T", p)
	}
	if c.Pattern != "yyyy-mm-ddThh:mm:ss" {
		t.Errorf("Pattern = %q, want yyyy-mm-ddThh:mm:ss", c.Pattern)
	}
}

func TestParse_CDuration(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DURATION">
		<rm_type_name>DURATION</rm_type_name>
		<node_id />
		<pattern>PYMD</pattern>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDuration)
	if !ok {
		t.Fatalf("want CDuration, got %T", p)
	}
	if c.Pattern != "PYMD" {
		t.Errorf("Pattern = %q, want PYMD", c.Pattern)
	}
}

func TestParse_CCodePhrase(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_CODE_PHRASE">
		<rm_type_name>CODE_PHRASE</rm_type_name>
		<node_id />
		<terminology_id><value>openehr</value></terminology_id>
		<code_list>433</code_list>
		<code_list>434</code_list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CodePhrase)
	if !ok {
		t.Fatalf("want CodePhrase, got %T", p)
	}
	if c.Terminology != "openehr" {
		t.Errorf("Terminology = %q, want openehr", c.Terminology)
	}
	if len(c.CodeList) != 2 {
		t.Errorf("CodeList = %v, want [433 434]", c.CodeList)
	}
	if c.External() {
		t.Errorf("External() = true, want false (closed list)")
	}
}

func TestParse_CDvQuantity(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DV_QUANTITY">
		<rm_type_name>DV_QUANTITY</rm_type_name>
		<node_id />
		<property>
			<terminology_id><value>openehr</value></terminology_id>
			<code_string>125</code_string>
		</property>
		<list>
			<magnitude>
				<lower_included>true</lower_included>
				<upper_included>true</upper_included>
				<lower_unbounded>false</lower_unbounded>
				<upper_unbounded>false</upper_unbounded>
				<lower>0</lower>
				<upper>300</upper>
			</magnitude>
			<precision>
				<lower_included>true</lower_included>
				<upper_included>true</upper_included>
				<lower_unbounded>false</lower_unbounded>
				<upper_unbounded>false</upper_unbounded>
				<lower>0</lower>
				<upper>0</upper>
			</precision>
			<units>mm[Hg]</units>
		</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.DvQuantity)
	if !ok {
		t.Fatalf("want DvQuantity, got %T", p)
	}
	if len(c.Units) != 1 || c.Units[0].Units != "mm[Hg]" {
		t.Errorf("Units = %+v, want one mm[Hg] entry", c.Units)
	}
	if c.Property == nil || c.Property.CodeString != "125" {
		t.Errorf("Property = %+v, want openehr::125", c.Property)
	}
	if !c.Units[0].Magnitude.IsBounded() || c.Units[0].Magnitude.Upper != 300 {
		t.Errorf("Magnitude range = %s, want [0..300]", c.Units[0].Magnitude)
	}
	// Validate against the parsed constraint — end-to-end smoke.
	if v := c.Validate(constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"}); len(v) != 0 {
		t.Errorf("Validate(120) = %v, want nil", v)
	}
	if v := c.Validate(constraints.QuantityValue{Magnitude: 500, Units: "mm[Hg]"}); len(v) != 1 || v[0].Code != constraints.CodeOutOfRange {
		t.Errorf("Validate(500) = %v, want CodeOutOfRange", v)
	}
}

func TestParse_CDvOrdinal(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DV_ORDINAL">
		<rm_type_name>DV_ORDINAL</rm_type_name>
		<node_id />
		<list>
			<value>0</value>
			<symbol>
				<terminology_id><value>local</value></terminology_id>
				<code_string>at0001</code_string>
			</symbol>
		</list>
		<list>
			<value>1</value>
			<symbol>
				<terminology_id><value>local</value></terminology_id>
				<code_string>at0002</code_string>
			</symbol>
		</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDvOrdinal)
	if !ok {
		t.Fatalf("want CDvOrdinal, got %T", p)
	}
	if len(c.Values) != 2 || c.Values[1].Value != 1 || c.Values[1].Symbol.CodeString != "at0002" {
		t.Errorf("Values = %+v, want two ordered entries", c.Values)
	}
}

// REQ-103 — non-primitive xsi:type values (here ARCHETYPE_SLOT) do
// NOT carry a primitive constraint. Guards against the dispatch
// over-attaching.
func TestParse_NonPrimitive_NoConstraint(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="ARCHETYPE_SLOT">
		<rm_type_name>CLUSTER</rm_type_name>
		<node_id>at0001</node_id>
	</children>`)
	root, _ := tmpl.Root().(*template.ComplexObject)
	attrs := root.Attributes()
	if len(attrs) != 1 || len(attrs[0].Children()) != 1 {
		t.Fatal("unexpected tree shape")
	}
	if _, ok := attrs[0].Children()[0].(*template.Slot); !ok {
		t.Errorf("child should be *Slot, got %T", attrs[0].Children()[0])
	}
}

// REQ-103 — a leaf COMPLEX_OBJECT (i.e. one of the non-primitive
// RM types like ELEMENT) returns nil from PrimitiveConstraint().
func TestParse_LeafComplexObject_NoConstraint(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_COMPLEX_OBJECT">
		<rm_type_name>ELEMENT</rm_type_name>
		<node_id>at0001</node_id>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	if p != nil {
		t.Errorf("PrimitiveConstraint() = %T, want nil for C_COMPLEX_OBJECT", p)
	}
}

// TestParse_CPrimitiveObject_Coverage exercises the C_PRIMITIVE_OBJECT
// recursion across every primitive wrapper the AOM 1.4 OPTs in the
// wild use. The wrapper carries the primitive short name on
// rm_type_name; the inner <item xsi:type="C_*"> carries the typed
// constraint that the parser must thread through. See [REQ-100
// wire-parser plan](../../docs/plans/archive/2026-05-26-c-primitive-object-wire-parser.md)
// for the full landed scope.
func TestParse_CPrimitiveObject_Coverage(t *testing.T) {
	cases := []struct {
		name   string
		rmType string
		inner  string
		assert func(t *testing.T, p constraints.PrimitiveConstraint)
	}{
		{
			name:   "DATE/pattern",
			rmType: "DATE",
			inner:  `<item xsi:type="C_DATE"><pattern>YYYY-MM-??</pattern></item>`,
			assert: func(t *testing.T, p constraints.PrimitiveConstraint) {
				cd, ok := p.(constraints.CDate)
				if !ok {
					t.Fatalf("got %T, want CDate", p)
				}
				if cd.Pattern != "YYYY-MM-??" {
					t.Errorf("Pattern = %q, want YYYY-MM-??", cd.Pattern)
				}
			},
		},
		{
			name:   "TIME/pattern",
			rmType: "TIME",
			inner:  `<item xsi:type="C_TIME"><pattern>HH:MM:SS</pattern></item>`,
			assert: func(t *testing.T, p constraints.PrimitiveConstraint) {
				ct, ok := p.(constraints.CTime)
				if !ok {
					t.Fatalf("got %T, want CTime", p)
				}
				if ct.Pattern != "HH:MM:SS" {
					t.Errorf("Pattern = %q, want HH:MM:SS", ct.Pattern)
				}
			},
		},
		{
			name:   "DATE_TIME/pattern",
			rmType: "DATE_TIME",
			inner:  `<item xsi:type="C_DATE_TIME"><pattern>YYYY-MM-DDTHH:MM:SS</pattern></item>`,
			assert: func(t *testing.T, p constraints.PrimitiveConstraint) {
				cdt, ok := p.(constraints.CDateTime)
				if !ok {
					t.Fatalf("got %T, want CDateTime", p)
				}
				if cdt.Pattern != "YYYY-MM-DDTHH:MM:SS" {
					t.Errorf("Pattern = %q, want YYYY-MM-DDTHH:MM:SS", cdt.Pattern)
				}
			},
		},
		{
			name:   "BOOLEAN/true-only",
			rmType: "BOOLEAN",
			inner:  `<item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>false</false_valid></item>`,
			assert: func(t *testing.T, p constraints.PrimitiveConstraint) {
				cb, ok := p.(constraints.CBoolean)
				if !ok {
					t.Fatalf("got %T, want CBoolean", p)
				}
				if !cb.TrueValid || cb.FalseValid {
					t.Errorf("CBoolean = %+v, want {TrueValid:true FalseValid:false}", cb)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := parseOPTWithChild(t, `<children xsi:type="C_PRIMITIVE_OBJECT">
				<rm_type_name>`+tc.rmType+`</rm_type_name>
				<node_id/>
				`+tc.inner+`
			</children>`)
			p := firstChildPrimitive(t, tmpl)
			if p == nil {
				t.Fatalf("PrimitiveConstraint() = nil for %s wrapper", tc.rmType)
			}
			tc.assert(t, p)
		})
	}
}

// TestParse_CPrimitiveObject_StrictMissingItem confirms strict
// parsing surfaces a malformed C_PRIMITIVE_OBJECT (no inner <item>)
// rather than silently dropping it.
func TestParse_CPrimitiveObject_StrictMissingItem(t *testing.T) {
	body := strings.Replace(primitiveOPTTemplate, "%s", `<children xsi:type="C_PRIMITIVE_OBJECT">
		<rm_type_name>DURATION</rm_type_name>
		<node_id/>
	</children>`, 1)
	if _, err := template.ParseOPTStrict(strings.NewReader(body)); err == nil {
		t.Error("ParseOPTStrict on C_PRIMITIVE_OBJECT without <item> should fail")
	}
}

// TestParse_CPrimitiveObject_Duration is the focused phase-0 regression
// gate for the C_PRIMITIVE_OBJECT inner-`<item>` extraction (now
// landed via the [archived wire-parser
// plan](../../docs/plans/archive/2026-05-26-c-primitive-object-wire-parser.md)).
// Pre-fix the parser dropped the inner item, leaving
// PrimitiveConstraint() = nil; the test pins a typed CDuration with
// its inner-`<pattern>` preserved.
func TestParse_CPrimitiveObject_Duration(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_PRIMITIVE_OBJECT">
		<rm_type_name>DURATION</rm_type_name>
		<node_id/>
		<item xsi:type="C_DURATION">
			<pattern>PThhHmmM</pattern>
		</item>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	cd, ok := p.(constraints.CDuration)
	if !ok {
		t.Fatalf("PrimitiveConstraint() = %T, want constraints.CDuration", p)
	}
	if cd.Pattern != "PThhHmmM" {
		t.Errorf("CDuration.Pattern = %q, want PThhHmmM", cd.Pattern)
	}
}
