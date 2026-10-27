package instanceprobes

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Probe027GeneratedValidates asserts that the canonical
// generator → validator round-trip passes cleanly for the supplied
// compiled OPT. The probe needs no client or server. For openEHR
// conformance, another implementation of
// the generator and validator against the same fixtures must produce
// the same OK outcome.
//
// It is scoped to COMPOSITION roots, the roots ValidateComposition
// supports. It generates once per policy (Large, Medium and Required) and
// value fill (ExampleFill and RandomFill), and checks each instance with
// ValidateComposition and ValidateRM. RandomFill draws from opts.ValueSource,
// or from a fixed seed when that is nil, so the probe is reproducible. On a
// pass, Detail names each policy/fill pair the instance validated clean
// under, in that order.
func Probe027GeneratedValidates(ctx context.Context, c *templatecompile.Compiled, opts instance.Options) (Result, error) { // PROBE-027 (REQ-107)
	r := Result{Probe: "PROBE-027"}
	if c == nil || c.Root() == nil {
		return r, errors.New("PROBE-027: nil compiled template")
	}
	rootType := c.Root().RMTypeName()
	if rootType != "COMPOSITION" {
		r.Status = "skip"
		r.Detail = fmt.Sprintf("template root %q not COMPOSITION; v1 probe scope is composition-only", rootType)
		return r, nil
	}

	var failures, validated []string
	for _, policy := range []instance.Policy{instance.Large, instance.Medium, instance.Required} {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			o := opts
			o.Policy, o.ValueFill = policy, fill
			if fill == instance.RandomFill && o.ValueSource == nil {
				o.ValueSource = rand.NewPCG(27, 27)
			}
			label := policy.String() + "/" + fill.String()
			out, err := instance.Generate(ctx, c, o)
			if err != nil {
				failures = append(failures, fmt.Sprintf("[%s] Generate: %v", label, err))
				continue
			}
			comp, err := instance.AsComposition(out)
			if err != nil {
				failures = append(failures, fmt.Sprintf("[%s] AsComposition: %v", label, err))
				continue
			}
			if result := validation.ValidateComposition(comp, c); !result.OK {
				failures = append(failures, fmt.Sprintf("[%s] ValidateComposition produced %d issue(s): %s",
					label, len(result.Issues), summariseIssues(result.Issues)))
				continue
			}
			if result := validation.ValidateRM(comp); !result.OK {
				failures = append(failures, fmt.Sprintf("[%s] ValidateRM produced %d issue(s): %s",
					label, len(result.Issues), summariseIssues(result.Issues)))
				continue
			}
			validated = append(validated, label)
		}
	}

	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "validated clean under " + strings.Join(validated, ", ")
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
