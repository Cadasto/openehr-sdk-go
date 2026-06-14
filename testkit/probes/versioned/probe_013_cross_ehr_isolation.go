package versionedprobes

import (
	"context"
	"errors"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe013CrossEHRIsolation implements PROBE-013: a `version_uid`
// belonging to EHR A cannot be read via EHR B's path. The wire
// assertion is `GET /ehr/{ehr_b_id}/composition/{version_uid_from_a}`
// returns `404 Not Found` — never `200`, never the EHR A data.
//
// The probe exercises tenant isolation on versioned reads (REQ-054
// neighbour). A server that returns 200 leaks cross-EHR data; a
// server that returns 403/500 might still hide a leak (e.g. an audit
// 403 followed by a body). Only a hard 404 is acceptable.
//
// Inputs:
//   - ehrAID is the EHR that legitimately owns versionUIDFromA.
//   - ehrBID is the unrelated EHR used as the attacker's tenant.
//     MUST differ from ehrAID — if equal, the probe returns an
//     error (probe-framework misuse, not a wire failure).
//   - versionUIDFromA is a known Composition VersionUID owned by EHR A.
func Probe013CrossEHRIsolation(ctx context.Context, c *transport.Client, ehrAID, ehrBID openehrclient.EHRID, versionUIDFromA openehrclient.VersionUID) (Result, error) {
	r := Result{Probe: "PROBE-013"}
	if c == nil || ehrAID == "" || ehrBID == "" || versionUIDFromA == "" {
		return r, errors.New("PROBE-013: missing required inputs (client/ehrAID/ehrBID/versionUIDFromA)")
	}
	if ehrAID == ehrBID {
		return r, errors.New("PROBE-013: ehrAID and ehrBID MUST differ — probe-framework misuse")
	}
	_, _, err := composition.Get(ctx, c, ehrBID, openehrclient.VersionOf(versionUIDFromA))
	if err == nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("tenant leak: GET /ehr/%q/composition/%q returned 200 — EHR B served EHR A's Composition", ehrBID, versionUIDFromA)
		return r, nil
	}
	if !errors.Is(err, transport.ErrNotFound) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("expected ErrNotFound on cross-EHR read, got %v", err)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("isolation OK: GET /ehr/%q/composition/%q → 404", ehrBID, versionUIDFromA)
	return r, nil
}
