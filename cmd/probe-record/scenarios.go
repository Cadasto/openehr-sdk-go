package main

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
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
	"ehr-status": {
		name:    "ehr-status",
		desc:    "POST /ehr with a fixed initial EHR_STATUS, then GET that status",
		capture: captureEHRStatus,
	},
	"composition-minimal": {
		name:    "composition-minimal",
		desc:    "POST /ehr, upload a fixed OPT, POST a composition, then GET the version the save returned",
		capture: captureCompositionMinimal,
	},
	"stored-query": {
		name:    "stored-query",
		desc:    "POST /ehr, PUT a fixed stored query, then POST its execution",
		capture: captureStoredQuery,
	},
	"error-envelopes": {
		name:    "error-envelopes",
		desc:    "GET an unknown EHR (404), then POST three AQL queries the server refuses (400)",
		capture: captureErrorEnvelopes,
	},
}

// The error-envelopes scenario reads only: one fixed EHR id no deployment
// holds, and three fixed queries a server refuses. Fixed values keep the
// recorded paths and bodies the same on every capture.
const (
	cassetteUnknownEHRID = "7d44b88c-4199-4bad-97dc-d78268e01398"
	// cassetteMalformedAQL stops after FROM, so it does not parse.
	cassetteMalformedAQL = "SELECT e/ehr_id/value FROM"
	// cassetteUnknownAttributeAQL parses, but COMPOSITION has no such
	// attribute.
	cassetteUnknownAttributeAQL = "SELECT c/no_such_attribute FROM EHR e CONTAINS COMPOSITION c"
	// cassetteUnsupportedPathAQL parses, but selects an EHR attribute that
	// does not exist.
	cassetteUnsupportedPathAQL = "SELECT e/no_such_attribute FROM EHR e"
)

// captureErrorEnvelopes records the error responses PROBE-068 and
// PROBE-021 decode: a GET of an EHR id the server does not hold, then three
// ad-hoc queries it refuses. Each call must be refused, or the capture
// fails rather than record a success in their place.
func captureErrorEnvelopes(ctx context.Context, c *transport.Client) error {
	if _, _, err := ehr.Get(ctx, c, ehr.EHRID(cassetteUnknownEHRID)); !errors.Is(err, transport.ErrNotFound) {
		return fmt.Errorf("get an unknown EHR: got %v, want a 404 refusal", err)
	}
	for _, q := range []string{cassetteMalformedAQL, cassetteUnknownAttributeAQL, cassetteUnsupportedPathAQL} {
		_, _, err := query.ExecuteString(ctx, c, q, nil)
		if we, ok := errors.AsType[*transport.WireError](err); !ok || we == nil || we.StatusCode/100 != 4 {
			return fmt.Errorf("execute %q: got %v, want a 4xx refusal", q, err)
		}
	}
	return nil
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
