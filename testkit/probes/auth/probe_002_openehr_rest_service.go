package authprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
)

// Probe002OpenEHRRestService implements PROBE-002: the resolved service
// catalog MUST contain an entry with id "org.openehr.rest", a parseable
// base URL, and a declared spec_version (REQ-070, REQ-072).
//
// Pass conditions:
//  1. Resolve succeeds.
//  2. catalog.OpenEHRRest() returns an entry.
//  3. The entry has a non-nil, absolute BaseURL.
//  4. The entry declares a non-empty SpecVersion.
func Probe002OpenEHRRestService(ctx context.Context, cassetteBody []byte) (Result, error) { // PROBE-002 (REQ-070)
	r := Result{Probe: "PROBE-002"}
	if len(cassetteBody) == 0 {
		return r, errors.New("PROBE-002: cassetteBody is empty")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cassetteBody)
	}))
	defer srv.Close()

	cat, err := resolveCassette(ctx, srv)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Resolve failed: %v", err)
		return r, nil
	}
	entry, ok := cat.OpenEHRRest()
	if !ok {
		r.Status = "fail"
		r.Detail = "service catalog has no org.openehr.rest entry"
		return r, nil
	}
	if entry.BaseURL == nil || !entry.BaseURL.IsAbs() {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("org.openehr.rest base_url is not a parseable absolute URL: %v", entry.BaseURL)
		return r, nil
	}
	if entry.SpecVersion == "" {
		r.Status = "fail"
		r.Detail = "org.openehr.rest entry declares no spec_version"
		return r, nil
	}
	r.Status = "pass"
	r.Detail = fmt.Sprintf("org.openehr.rest advertised: base_url=%s spec_version=%s", entry.BaseURL, entry.SpecVersion)
	return r, nil
}
