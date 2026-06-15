package templateprobes

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ValidateCase is one (path, value, expectations) tuple for
// PROBE-024. The probe parses the OPT body, walks to the node at
// Path, calls [constraints.PrimitiveConstraint.Validate] with Value,
// and checks the returned [constraints.Violation] codes match the
// case's expectations.
//
// ExpectNoConstraint flips the expectation: the addressed node MUST
// NOT carry a primitive constraint (e.g. a non-primitive COMPLEX_OBJECT
// or an ARCHETYPE_SLOT). Used to negatively assert REQ-103's
// "only primitive xsi:type values carry a constraint" contract.
type ValidateCase struct {
	// Path is the openEHR path string addressing a leaf node. Must
	// resolve via OperationalTemplate.NodeAt; otherwise the probe
	// reports the resolution failure as a probe-level fail.
	Path string

	// Value is the input passed to PrimitiveConstraint.Validate.
	// Accepted Go types per constraint kind — see the constraints
	// package docs.
	Value any

	// WantCodes is the multiset of violation codes the case expects.
	// nil / empty asserts Validate returned no violations.
	WantCodes []constraints.ViolationCode

	// ExpectNoConstraint asserts the addressed node carries no
	// primitive constraint (PrimitiveConstraint() returns nil).
	// Value / WantCodes are ignored when this is true.
	ExpectNoConstraint bool
}

// Probe024PrimitiveValidate implements PROBE-024: parse the OPT body
// and exercise its primitive-constraint surface against a
// fixture-supplied list of validate cases. Sandbox-only — no
// transport involvement.
//
// The probe is invariant under any backend / generator that produces
// the same primitive xsi:type values; consumers SHOULD include
// at least one positive (no violations) and one negative
// (CodeOutOfRange / CodeNotInList / …) case per primitive kind they
// rely on.
func Probe024PrimitiveValidate(opt []byte, cases []ValidateCase) (Result, error) {
	r := Result{Probe: "PROBE-024"}
	if len(opt) == 0 {
		return r, errors.New("PROBE-024: empty OPT body")
	}
	if len(cases) == 0 {
		return r, errors.New("PROBE-024: at least one case required")
	}

	tmpl, err := template.ParseOPT(bytes.NewReader(opt))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("parse: %v", err)
		return r, nil
	}

	var failures []string
	for _, c := range cases {
		if msg := checkValidateCase(tmpl, c); msg != "" {
			failures = append(failures, msg)
		}
	}
	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

func checkValidateCase(t *template.OperationalTemplate, c ValidateCase) string {
	p, err := t.ParsePath(c.Path)
	if err != nil {
		return fmt.Sprintf("%s: parse: %v", c.Path, err)
	}
	n, err := t.NodeAt(p)
	if err != nil {
		return fmt.Sprintf("%s: resolve: %v", c.Path, err)
	}
	co, ok := n.(*template.ComplexObject)
	if !ok {
		if c.ExpectNoConstraint {
			return ""
		}
		return fmt.Sprintf("%s: not a *ComplexObject (got %T)", c.Path, n)
	}
	primitive := co.PrimitiveConstraint()
	if c.ExpectNoConstraint {
		if primitive != nil {
			return fmt.Sprintf("%s: PrimitiveConstraint=%T, want nil", c.Path, primitive)
		}
		return ""
	}
	if primitive == nil {
		return fmt.Sprintf("%s: PrimitiveConstraint is nil; expected one with codes %v", c.Path, c.WantCodes)
	}
	got := primitive.Validate(c.Value)
	gotCodes := codesOf(got)
	if !codesMatch(gotCodes, c.WantCodes) {
		return fmt.Sprintf("%s: Validate codes = %v, want %v (detail=%q)", c.Path, gotCodes, c.WantCodes, detailOf(got))
	}
	return ""
}

// codesOf extracts the typed codes from a violation slice, dropping
// detail strings (those vary across runs).
func codesOf(vs []constraints.Violation) []constraints.ViolationCode {
	out := make([]constraints.ViolationCode, len(vs))
	for i, v := range vs {
		out[i] = v.Code
	}
	return out
}

// detailOf joins violation detail strings for diagnostic surfacing
// in failure messages.
func detailOf(vs []constraints.Violation) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Detail
	}
	return strings.Join(parts, " | ")
}

// codesMatch reports whether got and want hold the same multiset of
// codes (order does not matter; duplicates are honoured).
func codesMatch(got, want []constraints.ViolationCode) bool {
	if len(got) != len(want) {
		return false
	}
	gotCount := make(map[constraints.ViolationCode]int, len(got))
	for _, c := range got {
		gotCount[c]++
	}
	for _, c := range want {
		gotCount[c]--
		if gotCount[c] < 0 {
			return false
		}
	}
	return true
}
