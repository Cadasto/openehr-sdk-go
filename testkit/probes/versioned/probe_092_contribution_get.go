package versionedprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe092ContributionGet implements PROBE-092: the contribution read
// leaf issues `GET /ehr/{ehr_id}/contribution/{contribution_uid}` and
// decodes the 200 body as the persisted CONTRIBUTION (REQ-142).
//
// Version metadata is deliberately not asserted: the vendored pin
// defines only `Content-Type` on `200_CONTRIBUTION` (`ETag` / `Location`
// belong to `201_CONTRIBUTION`), so a conformant server may send none.
//
// Per REQ-080 the fail-closed legs assert behaviour, not sentinel
// identity — an empty id issues no request and a 404 returns a non-nil
// error. That `errors.Is` holds for transport.ErrInvalidConfig and
// transport.ErrNotFound is pinned by the contribution package's unit
// tests, which own the sentinel contract.
//
// Inputs:
//   - captured accumulates every request the backend receives, in order.
//     The caller wires it up (an httptest handler in Sandbox mode); the
//     probe reads length deltas to count requests, so it MUST NOT be
//     reset between legs.
//   - presentUID is a uid the backend answers 200 for with a canonical
//     contribution body; missingUID is one it answers 404 for.
func Probe092ContributionGet(ctx context.Context, c *transport.Client, captured *[]*http.Request, ehrID openehrclient.EHRID, presentUID, missingUID string) (Result, error) {
	r := Result{Probe: "PROBE-092"}
	if c == nil || ehrID == "" || presentUID == "" || missingUID == "" {
		return r, errors.New("PROBE-092: missing required inputs (client/ehr/present uid/missing uid)")
	}
	if captured == nil {
		return r, errors.New("PROBE-092: nil captured-request recorder")
	}

	// Leg 1 — a 200 read: request shape matches the ITS-REST template
	// and the body decodes as the persisted CONTRIBUTION.
	before := len(*captured)
	out, _, err := contribution.Get(ctx, c, ehrID, presentUID)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read of a present contribution failed: %v", err)
		return r, nil
	}
	if len(*captured) != before+1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read issued %d request(s), want exactly 1", len(*captured)-before)
		return r, nil
	}
	req := (*captured)[len(*captured)-1]
	if req.Method != http.MethodGet {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("method = %q, want GET (contribution_get)", req.Method)
		return r, nil
	}
	if want := "/ehr/" + string(ehrID) + "/contribution/" + presentUID; !strings.HasSuffix(req.URL.Path, want) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("path = %q, want suffix %q (ITS-REST contribution_get template)", req.URL.Path, want)
		return r, nil
	}
	if out == nil {
		r.Status = "fail"
		r.Detail = "200 body did not decode into a contribution"
		return r, nil
	}
	if out.UID.Value == "" {
		r.Status = "fail"
		r.Detail = "decoded contribution carries no uid — the persisted CONTRIBUTION envelope did not decode"
		return r, nil
	}

	// Leg 2 — empty ids fail before any request reaches the wire.
	before = len(*captured)
	if _, _, err := contribution.Get(ctx, c, "", presentUID); err == nil {
		r.Status = "fail"
		r.Detail = "empty ehr_id was accepted; want a refusal before any request"
		return r, nil
	}
	if _, _, err := contribution.Get(ctx, c, ehrID, ""); err == nil {
		r.Status = "fail"
		r.Detail = "empty contribution_uid was accepted; want a refusal before any request"
		return r, nil
	}
	if n := len(*captured) - before; n != 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("empty ids issued %d request(s), want 0 (fail closed)", n)
		return r, nil
	}

	// Leg 3 — an unknown uid surfaces as an error, not a zero value.
	got, _, err := contribution.Get(ctx, c, ehrID, missingUID)
	if err == nil {
		r.Status = "fail"
		r.Detail = "404 did not surface as an error"
		return r, nil
	}
	if got != nil {
		r.Status = "fail"
		r.Detail = "404 returned a non-nil contribution alongside the error"
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "GET contribution_get: 200 decodes as persisted CONTRIBUTION; empty ids issue no request; 404 errors"
	return r, nil
}
