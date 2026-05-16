package versionedprobes

import (
	"context"
	"errors"
	"fmt"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe010PutWithoutIfMatch implements PROBE-010: a PUT against a
// versioned resource without an If-Match header is rejected with 428
// Precondition Required and surfaces as
// [transport.ErrPreconditionRequired].
//
// The probe uses [ehrstatus.Put] with an empty ifMatch and asserts
// that the SDK refuses to issue the request — short-circuiting with
// [transport.ErrInvalidConfig] BEFORE any network call. The wire-
// level 428 path is asserted by [Probe011PutStaleIfMatch] via a fake
// server, since the SDK guards correct usage at compile/runtime time.
func Probe010PutWithoutIfMatch(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID) (Result, error) {
	r := Result{Probe: "PROBE-010"}
	if c == nil {
		return r, fmt.Errorf("PROBE-010: nil transport.Client")
	}
	if ehrID == "" {
		return r, fmt.Errorf("PROBE-010: empty EHRID")
	}
	_, _, err := ehrstatus.Put(ctx, c, ehrID, "", nil)
	if err == nil {
		r.Status = "fail"
		r.Detail = "Put(ifMatch=\"\") returned nil error; expected guard rejection"
		return r, nil
	}
	if !errors.Is(err, transport.ErrInvalidConfig) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("expected transport.ErrInvalidConfig, got %v", err)
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}
