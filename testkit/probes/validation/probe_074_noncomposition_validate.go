package validationprobes

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// RootCase describes one fixture tuple for the non-COMPOSITION
// validation probe (PROBE-074): an OPT body, an in-memory RM root from
// outside the COMPOSITION content set — the demographic PARTY hierarchy
// (PERSON / ORGANISATION / GROUP / AGENT / ROLE and the archetypeable
// sub-components ADDRESS / CONTACT / PARTY_IDENTITY / PARTY_RELATIONSHIP
// / CAPABILITY) or the EHR-IM roots FOLDER / EHR_STATUS — and the
// expected issue-code multiset.
type RootCase struct {
	// Name labels the case for diagnostic output.
	Name string

	// OPT is the operational-template XML body to compile against.
	OPT []byte

	// Root is the in-memory RM graph under test (passed to
	// [validation.Validate]).
	Root any

	// WantCodes is the multiset of [validation.Issue.Code] values the
	// probe expects. Order is irrelevant; duplicates count. nil means
	// "no issues" (positive case).
	WantCodes []string
}

// Probe074NonCompositionValidate runs each [RootCase] through
// [validation.Validate] and asserts the resulting issue-code multiset
// matches WantCodes — REQ-110. Any conformant implementation that
// extends template-driven validation beyond COMPOSITION to the
// demographic PARTY hierarchy and the EHR-IM roots (FOLDER /
// EHR_STATUS) MUST produce the same multiset for the same OPT + root
// shape.
func Probe074NonCompositionValidate(cases []RootCase) (Result, error) {
	r := Result{Probe: "PROBE-074"}
	if len(cases) == 0 {
		return r, errors.New("PROBE-074: at least one case required")
	}
	var failures []string
	for _, tc := range cases {
		if msg := runRootCase(tc); msg != "" {
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

func runRootCase(tc RootCase) string {
	opt, err := template.ParseOPT(bytes.NewReader(tc.OPT))
	if err != nil {
		return fmt.Sprintf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Sprintf("Compile: %v", err)
	}
	res := validation.Validate(tc.Root, c)
	got := make([]string, 0, len(res.Issues))
	for _, i := range res.Issues {
		got = append(got, i.Code)
	}
	if !codesMatch(got, append([]string(nil), tc.WantCodes...)) {
		return fmt.Sprintf("codes mismatch: got %v want %v", sortedCopy(got), sortedCopy(tc.WantCodes))
	}
	return ""
}
