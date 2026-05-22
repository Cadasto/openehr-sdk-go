package templateprobes

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// PathAssertion describes one expected path → node match for
// PROBE-022. At most one of WantNodeID, WantArchetypeID, or
// WantRMType is non-empty; if multiple are set, all MUST match.
type PathAssertion struct {
	// Path is the openEHR path string (REQ-100 § Path syntax subset).
	Path string

	// WantRMType, when non-empty, MUST equal the resolved node's
	// RMTypeName().
	WantRMType string

	// WantNodeID, when non-empty, MUST equal the resolved node's
	// NodeID().
	WantNodeID string

	// WantArchetypeID, when non-empty, requires the resolved node to
	// be an *ArchetypeRoot whose ArchetypeID() matches exactly.
	WantArchetypeID string

	// ExpectNotFound flips the expectation: NodeAt MUST return
	// ErrPathNotFound (wrapped). Used to negatively assert that
	// unknown attributes or predicates fail cleanly.
	ExpectNotFound bool
}

// Probe022OPTPathResolution implements PROBE-022: parse an OPT body
// and resolve a fixture-defined list of paths, verifying each one
// returns the expected node shape (or ErrPathNotFound for negative
// cases). Sandbox-only — no transport involvement.
//
// The probe is invariant under reformatting that preserves OPT XML
// semantics; backends or generators that reorder children may
// change predicate-less first-match outcomes, in which case the
// assertion list MUST use explicit at-code or archetype-id
// predicates to disambiguate.
func Probe022OPTPathResolution(opt []byte, assertions []PathAssertion) (Result, error) {
	r := Result{Probe: "PROBE-022"}
	if len(opt) == 0 {
		return r, fmt.Errorf("PROBE-022: empty OPT body")
	}
	if len(assertions) == 0 {
		return r, fmt.Errorf("PROBE-022: at least one assertion required")
	}

	tmpl, err := template.ParseOPT(bytes.NewReader(opt))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("parse: %v", err)
		return r, nil
	}

	var failures []string
	for _, a := range assertions {
		if msg := checkAssertion(tmpl, a); msg != "" {
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

func checkAssertion(t *template.OperationalTemplate, a PathAssertion) string {
	p, err := t.ParsePath(a.Path)
	if err != nil {
		return fmt.Sprintf("%s: parse: %v", a.Path, err)
	}
	n, err := t.NodeAt(p)
	if err != nil {
		if a.ExpectNotFound {
			return "" // expected
		}
		return fmt.Sprintf("%s: resolve: %v", a.Path, err)
	}
	if a.ExpectNotFound {
		return fmt.Sprintf("%s: resolved to %s but expected ErrPathNotFound", a.Path, n.RMTypeName())
	}
	if a.WantRMType != "" && n.RMTypeName() != a.WantRMType {
		return fmt.Sprintf("%s: RMTypeName=%q want %q", a.Path, n.RMTypeName(), a.WantRMType)
	}
	if a.WantNodeID != "" && n.NodeID() != a.WantNodeID {
		return fmt.Sprintf("%s: NodeID=%q want %q", a.Path, n.NodeID(), a.WantNodeID)
	}
	if a.WantArchetypeID != "" {
		ar, ok := n.(*template.ArchetypeRoot)
		if !ok {
			return fmt.Sprintf("%s: not an ArchetypeRoot (got %T)", a.Path, n)
		}
		if ar.ArchetypeID() != a.WantArchetypeID {
			return fmt.Sprintf("%s: ArchetypeID=%q want %q", a.Path, ar.ArchetypeID(), a.WantArchetypeID)
		}
	}
	return ""
}
