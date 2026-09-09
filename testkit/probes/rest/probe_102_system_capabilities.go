package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe102SystemCapabilities implements PROBE-102: the System API's single
// operation is `OPTIONS /` (ITS-REST *Options and Conformance*), and its
// response decodes into the typed service capabilities with a declared
// `restapi_specs_version` (REQ-095).
//
// The verb is the point: a deployment that answered a `GET` here, or the SDK
// issuing one, would miss the operation the spec defines. captured returns the
// requests the backend received; the probe reads the newest to confirm the
// verb and that exactly one request was issued.
func Probe102SystemCapabilities(ctx context.Context, c *transport.Client, captured func() []*http.Request) (Result, error) {
	r := Result{Probe: "PROBE-102"}
	if c == nil {
		return r, errors.New("PROBE-102: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-102: nil captured-request recorder")
	}

	before := len(captured())
	caps, _, err := system.Capabilities(ctx, c)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Capabilities failed: %v", err)
		return r, nil
	}
	if caps == nil {
		r.Status = "fail"
		r.Detail = "Capabilities returned a nil ServiceCapabilities"
		return r, nil
	}
	if caps.RESTAPISpecsVersion == "" {
		r.Status = "fail"
		r.Detail = "capabilities carried no restapi_specs_version"
		return r, nil
	}

	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("issued %d requests, want exactly 1", n)
		return r, nil
	}
	if m := reqs[len(reqs)-1].Method; m != http.MethodOptions {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("System capabilities used %s, want OPTIONS", m)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("OPTIONS / → restapi_specs_version %q", caps.RESTAPISpecsVersion)
	return r, nil
}
