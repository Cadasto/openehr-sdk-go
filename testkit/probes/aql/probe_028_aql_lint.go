package aqlprobes

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// LintCase describes one fixture tuple consumed by PROBE-028. The probe
// asserts the issue-code MULTISET produced by [lint.LintString] — not the
// Detail text, not the path strings — so the conformance assertion stays at
// the observable-behaviour level: any implementation of REQ-109 over the same
// grammar profile and OPT MUST produce the same codes.
type LintCase struct {
	// Name labels the case for diagnostic output.
	Name string

	// OPT is the operational-template XML body to compile and lint against.
	// Nil runs Layers 1–2 only (syntax, shape) with no template.
	OPT []byte

	// Query is the AQL string under test.
	Query string

	// WantCodes is the multiset of [lint.Issue.Code] values the probe
	// expects. Order is irrelevant; duplicates count. Empty / nil means
	// "no issues" (clean case).
	WantCodes []string
}

// Probe028AQLLint runs each case through [lint.LintString] (Layer 1 syntax +
// Layer 2 shape + Layer 3 template when an OPT is supplied) and asserts the
// resulting issue codes match the case's WantCodes multiset. Sandbox-only: no
// transport, no network (REQ-013 building block).
func Probe028AQLLint(cases []LintCase) (Result, error) {
	r := Result{Probe: "PROBE-028"}
	if len(cases) == 0 {
		return r, errors.New("PROBE-028: at least one case required")
	}
	var failures []string
	for _, tc := range cases {
		if msg := runLintCase(tc); msg != "" {
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

func runLintCase(tc LintCase) string {
	var c *templatecompile.Compiled
	if len(tc.OPT) > 0 {
		opt, err := template.ParseOPT(bytes.NewReader(tc.OPT))
		if err != nil {
			return fmt.Sprintf("ParseOPT: %v", err)
		}
		c, err = templatecompile.Compile(opt)
		if err != nil {
			return fmt.Sprintf("Compile: %v", err)
		}
	}
	res := lint.LintString(tc.Query, &lint.Options{Compiled: c})
	got := make([]string, 0, len(res.Issues))
	for _, i := range res.Issues {
		got = append(got, i.Code)
	}
	if !codesMatch(got, tc.WantCodes) {
		return fmt.Sprintf("codes mismatch: got %v want %v", sortedCopy(got), sortedCopy(tc.WantCodes))
	}
	return ""
}

// codesMatch reports multiset (order-irrelevant) equality of got and want.
func codesMatch(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	g, w := sortedCopy(got), sortedCopy(want)
	for i := range g {
		if g[i] != w[i] {
			return false
		}
	}
	return true
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
