package main

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// scenario is a named capture routine. It drives real SDK calls through the
// recorder-wrapped client so the resulting HAR witnesses the exchanges a probe
// asserts on. A recording is written as <name>.har.
type scenario struct {
	name    string
	desc    string
	capture func(ctx context.Context, c *transport.Client) error
}

// scenarios is the capture registry, keyed by recording name. Add a scenario
// here and it becomes selectable with -scenario and captured by -scenario all.
var scenarios = map[string]scenario{
	"ehr-lifecycle": {
		name:    "ehr-lifecycle",
		desc:    "POST /ehr, then GET and HEAD the created EHR",
		capture: captureEHRLifecycle,
	},
}

func scenarioNames() []string {
	names := make([]string, 0, len(scenarios))
	for name := range scenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// captureEHRLifecycle records the create-then-read path: POST /ehr, then GET
// and HEAD the EHR the server assigned. The GET and HEAD reuse the returned
// id, so the recording carries the same three exchanges a create-and-confirm
// probe drives — and, unlike the single-exchange ehr-create recording, it
// exercises the reader against a resource the same run created.
func captureEHRLifecycle(ctx context.Context, c *transport.Client) error {
	rec, _, err := ehr.Create(ctx, c)
	if err != nil {
		return fmt.Errorf("create EHR: %w", err)
	}
	if rec == nil || rec.EHRID.Value == "" {
		return errors.New("create EHR returned no ehr_id")
	}
	id := ehr.EHRID(rec.EHRID.Value)
	if _, _, err := ehr.Get(ctx, c, id); err != nil {
		return fmt.Errorf("get EHR: %w", err)
	}
	if _, err := ehr.Exists(ctx, c, id); err != nil {
		return fmt.Errorf("head EHR: %w", err)
	}
	return nil
}
