package authprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Probe003SpecVersionMismatch implements PROBE-003: a discovery document
// advertising an incompatible spec_version MUST be rejected at resolution
// with a typed DiscoveryError(spec_version_mismatch), before any openEHR
// REST request is made (REQ-072).
//
// The probe serves a SMART configuration whose org.openehr.rest entry
// declares spec_version "1.0.3" while the resolver requires the SDK pin
// (1.1.0-development) — and asserts the resolver fails fast with the
// typed reason, never returning a usable catalog.
func Probe003SpecVersionMismatch(ctx context.Context, mismatchedCassette []byte) (Result, error) { // PROBE-003 (REQ-072)
	r := Result{Probe: "PROBE-003"}
	if len(mismatchedCassette) == 0 {
		return r, errors.New("PROBE-003: mismatchedCassette is empty")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mismatchedCassette)
	}))
	defer srv.Close()

	cat, err := resolveCassette(ctx, srv)
	if err == nil {
		r.Status = "fail"
		r.Detail = "Resolve accepted an incompatible spec_version; want spec_version_mismatch"
		return r, nil
	}
	var de *discovery.DiscoveryError
	if !errors.As(err, &de) || de.Reason != discovery.ReasonSpecVersionMismatch {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("error %v is not DiscoveryError(spec_version_mismatch)", err)
		return r, nil
	}
	if cat != nil {
		r.Status = "fail"
		r.Detail = "Resolve returned a non-nil catalog on mismatch"
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("rejected at resolution: got=%s want=%s (no REST request issued)", de.SpecVersionGot, de.SpecVersionWant)
	return r, nil
}
