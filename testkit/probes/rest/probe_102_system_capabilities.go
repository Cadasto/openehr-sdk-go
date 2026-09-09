package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe102SystemCapabilities implements PROBE-102: the System API's single
// operation is `OPTIONS /` (ITS-REST *Options and Conformance*), and its
// response decodes into the typed service capabilities with a declared
// `restapi_specs_version` (REQ-095).
//
// The verb is the point: a deployment that answered a `GET` here, or the SDK
// issuing one, would miss the operation the spec defines. The route is the
// other half: the operation is defined on the service root, so a request that
// landed on any resource below it (`/ehr`, say) is not this operation at all.
// captured returns the requests the backend received; the probe reads the
// newest to confirm the verb, the service-root path, and that exactly one
// request was issued.
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
	req := reqs[len(reqs)-1]
	if req.Method != http.MethodOptions {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("System capabilities used %s, want OPTIONS", req.Method)
		return r, nil
	}
	root, err := servicePath(c, "/")
	if err != nil {
		return r, fmt.Errorf("PROBE-102: %w", err)
	}
	// The trailing slash is not significant here: `/openehr/v1` and
	// `/openehr/v1/` name the same resource, and the transport emits the
	// latter for the root path.
	if got, want := strings.TrimSuffix(req.URL.Path, "/"), strings.TrimSuffix(root, "/"); got != want {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("System capabilities path %q is not the service root %q", got, want)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("OPTIONS / → restapi_specs_version %q", caps.RESTAPISpecsVersion)
	return r, nil
}
