package instanceprobes

import (
	"context"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Probe027GeneratedValidates asserts that the canonical
// generator → validator round-trip passes cleanly for the supplied
// compiled OPT. The probe is sandbox-only (no transport
// dependency); cross-SDK parity means another implementation of
// REQ-107 + REQ-102 v2 against the same fixtures MUST produce the
// same OK outcome.
//
// Currently scoped to COMPOSITION roots — that is what the v2
// validator (ValidateComposition) supports. Non-COMPOSITION roots
// land when validation v3 grows a generic ValidateLocatable.
func Probe027GeneratedValidates(ctx context.Context, c *templatecompile.Compiled, opts instance.Options) (Result, error) {
	r := Result{Probe: "PROBE-027"}
	if c == nil || c.Root() == nil {
		return r, fmt.Errorf("PROBE-027: nil compiled template")
	}
	rootType := c.Root().RMTypeName()
	if rootType != "COMPOSITION" {
		r.Status = "skip"
		r.Detail = fmt.Sprintf("template root %q not COMPOSITION; v1 probe scope is composition-only", rootType)
		return r, nil
	}

	var failures []string
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		opts.Policy = policy
		out, err := instance.Generate(ctx, c, opts)
		if err != nil {
			failures = append(failures, fmt.Sprintf("[%s] Generate: %v", policy, err))
			continue
		}
		comp, err := instance.AsComposition(out)
		if err != nil {
			failures = append(failures, fmt.Sprintf("[%s] AsComposition: %v", policy, err))
			continue
		}
		result := validation.ValidateComposition(comp, c)
		if !result.OK {
			failures = append(failures, fmt.Sprintf("[%s] ValidateComposition produced %d issue(s): %s",
				policy, len(result.Issues), summariseIssues(result.Issues)))
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

// summariseIssues renders the first few validation issues for
// diagnostic context. Full output would bloat the probe Detail
// field; the underlying tree is reproducible from the same fixtures.
func summariseIssues(issues []validation.Issue) string {
	if len(issues) == 0 {
		return "<none>"
	}
	const maxShown = 5
	parts := make([]string, 0, maxShown)
	for i, iss := range issues {
		if i >= maxShown {
			parts = append(parts, fmt.Sprintf("…and %d more", len(issues)-maxShown))
			break
		}
		parts = append(parts, fmt.Sprintf("%s@%s: %s", iss.Code, iss.Path, iss.Detail))
	}
	return strings.Join(parts, " | ")
}
