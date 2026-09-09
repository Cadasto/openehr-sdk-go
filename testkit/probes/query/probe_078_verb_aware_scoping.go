package queryprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// scopeAQL is a trivial valid statement; the probe asserts request shape, not
// results, so any statement that validates will do.
const scopeAQL = "SELECT e/ehr_id/value FROM EHR e"

// Probe078VerbAwareScoping implements PROBE-078: an EHR-scoped AQL execution
// carries the scope on the mechanism the ITS-REST OAS defines for the verb —
// the `openehr-ehr-id` request header on the POST operations, the `ehr_id`
// query parameter on the GET operations — and never the other one (REQ-055).
//
// A server that only honoured the header would run a POST query that lacked it
// population-wide, so the probe catches the SDK regression that scoped a POST
// via the query parameter (or a GET via the header) instead. It drives both
// query endpoints — ad-hoc `/query/aql` and stored `/query/{qualified_query_name}`
// — over both verbs: four calls in all, each asserted for the verb-appropriate
// scope channel and the absence of the other.
//
// captured returns every request the backend has received, in order; the probe
// reads the newest entry after each call. The caller wires it to a sandbox
// scripted route.
func Probe078VerbAwareScoping(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID string) (Result, error) {
	r := Result{Probe: "PROBE-078"}
	if c == nil {
		return r, errors.New("PROBE-078: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-078: nil captured-request recorder")
	}
	if ehrID == "" {
		return r, errors.New("PROBE-078: empty ehr id to scope on")
	}

	q := aql.NewQuery(scopeAQL)
	const storedName = "org.openehr.probe::scope_test"

	arms := []struct {
		name       string
		wantMethod string
		call       func() error
	}{
		{"ad-hoc POST /query/aql", http.MethodPost, func() error {
			_, _, err := query.Execute(ctx, c, q, query.WithEHRID(ehrID))
			return err
		}},
		{"ad-hoc GET /query/aql", http.MethodGet, func() error {
			_, _, err := query.Execute(ctx, c, q, query.WithEHRID(ehrID), query.WithGET())
			return err
		}},
		{"stored POST /query/{name}", http.MethodPost, func() error {
			_, _, err := query.RunStored(ctx, c, storedName, nil, query.WithEHRID(ehrID), query.WithFetch(10))
			return err
		}},
		{"stored GET /query/{name}", http.MethodGet, func() error {
			_, _, err := query.RunStored(ctx, c, storedName, nil, query.WithEHRID(ehrID), query.WithGET(), query.WithFetch(10))
			return err
		}},
	}

	for _, arm := range arms {
		before := len(captured())
		if err := arm.call(); err != nil {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("%s: execution failed: %v", arm.name, err)
			return r, nil
		}
		reqs := captured()
		if n := len(reqs) - before; n != 1 {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("%s: issued %d requests, want exactly 1", arm.name, n)
			return r, nil
		}
		if msg := assertVerbScope(reqs[len(reqs)-1], arm.wantMethod, ehrID); msg != "" {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("%s: %s", arm.name, msg)
			return r, nil
		}
	}

	r.Status = "pass"
	r.Detail = "POST scopes via the openehr-ehr-id header, GET via the ehr_id query parameter, on both /query/aql and /query/{name}"
	return r, nil
}

// assertVerbScope checks that req scopes the EHR on the verb-appropriate
// mechanism and on no other. Returns a failure detail, or "" on success.
func assertVerbScope(req *http.Request, wantMethod, ehrID string) string {
	if req.Method != wantMethod {
		return fmt.Sprintf("method %s, want %s", req.Method, wantMethod)
	}
	header := req.Header.Get("openehr-ehr-id")
	param := req.URL.Query().Get("ehr_id")
	switch wantMethod {
	case http.MethodPost:
		if header != ehrID {
			return fmt.Sprintf("openehr-ehr-id header = %q, want %q", header, ehrID)
		}
		if param != "" {
			return fmt.Sprintf("carried an ehr_id query parameter %q; a POST scopes via the header only", param)
		}
	case http.MethodGet:
		if param != ehrID {
			return fmt.Sprintf("ehr_id query parameter = %q, want %q", param, ehrID)
		}
		if header != "" {
			return fmt.Sprintf("carried an openehr-ehr-id header %q; a GET scopes via the query parameter only", header)
		}
	}
	return ""
}
