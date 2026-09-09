package queryprobes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// query parameter on the GET operations — and never any other one (REQ-055).
//
// A server that only honoured the header would run a POST query that lacked it
// population-wide, so the probe catches the SDK regression that scoped a POST
// via the query parameter (or a GET via the header) instead. It drives all
// three query endpoints — ad-hoc `/query/aql`, stored
// `/query/{qualified_query_name}` and stored-versioned
// `/query/{qualified_query_name}/{version}` — over both verbs: six calls in
// all, each asserted for the verb-appropriate scope channel and the absence of
// the others. On a POST that third channel is the request body: the ITS-REST
// POST schemas declare no `ehr_id` field, so a body carrying one would be an
// unspecified second scope a strict server may ignore or reject.
//
// captured returns every request the backend has received, in order; the probe
// reads the newest entry after each call. The caller wires it to a sandbox
// scripted route, and the recorder MUST buffer each request body: the probe
// reads Body on the POST arms, while the sandbox closes the live request body
// once the exchange is served, so a recorder that stored the live request
// would hand back an unreadable one.
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
	const (
		storedName    = "org.openehr.probe::scope_test"
		storedVersion = "1.0.0"
	)

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
		{"stored-versioned POST /query/{name}/{version}", http.MethodPost, func() error {
			_, _, err := query.RunStoredVersion(ctx, c, storedName, storedVersion, nil, query.WithEHRID(ehrID), query.WithFetch(10))
			return err
		}},
		{"stored-versioned GET /query/{name}/{version}", http.MethodGet, func() error {
			_, _, err := query.RunStoredVersion(ctx, c, storedName, storedVersion, nil, query.WithEHRID(ehrID), query.WithGET(), query.WithFetch(10))
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
	r.Detail = "POST scopes via the openehr-ehr-id header alone (no ehr_id query parameter, no ehr_id body field), GET via the ehr_id query parameter alone, on /query/aql, /query/{name} and /query/{name}/{version}"
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
		return assertNoBodyScope(req)
	case http.MethodGet:
		if param != ehrID {
			return fmt.Sprintf("ehr_id query parameter = %q, want %q", param, ehrID)
		}
		if header != "" {
			return fmt.Sprintf("carried an openehr-ehr-id header %q; a GET scopes via the query parameter only", header)
		}
		return ""
	default:
		// The arm table above drives POST and GET only; a verb added there
		// without an assertion arm here must be loud, never silently green.
		return fmt.Sprintf("unsupported verb %s: the probe asserts scoping for POST and GET only", wantMethod)
	}
}

// assertNoBodyScope checks that a captured POST body carries no top-level
// `ehr_id` field. The ITS-REST POST query schemas declare none, so a body that
// carried one would be a second scope channel outside the spec. Returns a
// failure detail, or "" when the body is a clean JSON object without the key.
func assertNoBodyScope(req *http.Request) string {
	if req.Body == nil {
		return "captured POST request carries no readable body"
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Sprintf("captured POST request body could not be read: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Sprintf("POST body is not a JSON object: %v", err)
	}
	if _, ok := body["ehr_id"]; ok {
		return "carried an ehr_id request-body field; a POST scopes via the header only"
	}
	return ""
}
