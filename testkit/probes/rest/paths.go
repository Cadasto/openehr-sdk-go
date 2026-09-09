package restprobes

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// servicePath returns the exact URL path the transport emits for the
// service-relative route rel — the openEHR REST service base path from the
// client's catalog, joined to rel the way transport joins them. Probes that
// assert a route compare against this rather than a suffix or a substring: a
// suffix match cannot tell `/openehr/v1/admin/ehr/all` from
// `/wrong/prefix/admin/ehr/all`, nor a correct route from one with an extra
// segment appended.
//
// Fails with an error (not a probe verdict) when the client carries no
// openEHR REST service entry: that is a harness misconfiguration, not a
// deployment finding.
func servicePath(c *transport.Client, rel string) (string, error) {
	if c == nil {
		return "", errors.New("servicePath: nil transport.Client")
	}
	cat := c.Catalog()
	if cat == nil {
		return "", errors.New("servicePath: client carries no service catalog")
	}
	entry, ok := cat.Service(discovery.ServiceIDOpenEHRRest)
	if !ok {
		return "", fmt.Errorf("servicePath: catalog declares no %q service", discovery.ServiceIDOpenEHRRest)
	}
	if entry.BaseURL == nil {
		return "", fmt.Errorf("servicePath: the %q service entry carries no base URL", discovery.ServiceIDOpenEHRRest)
	}
	return joinServicePath(entry.BaseURL.Path, rel), nil
}

// joinServicePath mirrors the transport's own base-plus-path join so a probe's
// expected route is built by the same rule the request took: base
// `/openehr/v1` with rel `/` gives `/openehr/v1/`, and with `/admin/ehr/all`
// gives `/openehr/v1/admin/ehr/all`. Kept here rather than exported from
// transport so the probe's oracle stays a restatement of the rule, not a call
// into the code under test.
func joinServicePath(base, rel string) string {
	switch {
	case base == "":
		return rel
	case rel == "":
		return base
	}
	endsSlash := base[len(base)-1] == '/'
	startsSlash := rel[0] == '/'
	switch {
	case endsSlash && startsSlash:
		return base + rel[1:]
	case !endsSlash && !startsSlash:
		return base + "/" + rel
	default:
		return base + rel
	}
}
