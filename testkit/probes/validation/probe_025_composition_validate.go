package validationprobes

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// ValidateCase describes one fixture tuple consumed by the
// composition-validate probes. The probes assert the issue
// code multiset produced by ValidateComposition (not the exact
// Detail text, not the path strings), so the assertion stays at the
// level of observable behaviour.
type ValidateCase struct {
	// Name labels the case for diagnostic output.
	Name string

	// OPT is the operational-template XML body to compile against.
	OPT []byte

	// Composition is the in-memory RM graph under test.
	Composition *rm.Composition

	// WantCodes is the multiset of [validation.Issue.Code] values
	// the probe expects. Order is irrelevant; duplicates count.
	// An empty / nil slice means "no issues" (positive case).
	WantCodes []string
}

// Probe025CompositionValidate runs each fixture tuple through
// ValidateComposition and asserts the resulting issue codes match
// the case's WantCodes multiset. This is a stable openEHR-conformance
// assertion: any implementation of template-driven validation with the
// same OPT + composition shape must produce the same multiset.
func Probe025CompositionValidate(cases []ValidateCase) (Result, error) { // PROBE-025 (REQ-082, REQ-102, REQ-103)
	r := Result{Probe: "PROBE-025"}
	if len(cases) == 0 {
		return r, errors.New("PROBE-025: at least one case required")
	}
	var failures []string
	for _, tc := range cases {
		if msg := runCase(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("%s: %s", tc.Name, msg))
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

// Probe026MissingNodes implements PROBE-026, the negative-case probe.
// It has the same shape as [Probe025CompositionValidate], but the cases
// focus on structural completion:
// missing required nodes, cardinality violations,
// alternative_mismatch, RM-type mismatch. Code multiset
// expectations are stable across conformant implementations.
func Probe026MissingNodes(cases []ValidateCase) (Result, error) { // PROBE-026 (REQ-082, REQ-102)
	r := Result{Probe: "PROBE-026"}
	if len(cases) == 0 {
		return r, errors.New("PROBE-026: at least one case required")
	}
	var failures []string
	for _, tc := range cases {
		if msg := runCase(tc); msg != "" {
			failures = append(failures, fmt.Sprintf("%s: %s", tc.Name, msg))
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

func runCase(tc ValidateCase) string {
	opt, err := template.ParseOPT(bytes.NewReader(tc.OPT))
	if err != nil {
		return fmt.Sprintf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Sprintf("Compile: %v", err)
	}
	res := validation.ValidateComposition(tc.Composition, c)
	got := make([]string, 0, len(res.Issues))
	for _, i := range res.Issues {
		got = append(got, i.Code)
	}
	if !codesMatch(got, tc.WantCodes) {
		return fmt.Sprintf("codes mismatch: got %v want %v", sortedCopy(got), sortedCopy(tc.WantCodes))
	}
	return ""
}

// codesMatch reports whether `got` and `want` are the same
// multiset (order-irrelevant equality). Implemented via sorted
// comparison so the probe stays allocation-light and deterministic.
func codesMatch(got, want []string) bool {
	return slices.Equal(sortedCopy(got), sortedCopy(want))
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
