package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/admin"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe103AdminBulkDelete implements PROBE-103: the Admin bulk-delete surface is
// `DELETE /admin/ehr/all`, and a subset delete restricts to the named EHRs via
// the repeatable `ehr_id` query parameter — not a request body and not a path
// segment per id (REQ-099).
//
// captured returns the requests the backend received; the probe reads the
// newest to confirm the verb, the literal `/all` path, and one `ehr_id`
// parameter per id supplied.
func Probe103AdminBulkDelete(ctx context.Context, c *transport.Client, captured func() []*http.Request, ids []openehrclient.EHRID) (Result, error) {
	r := Result{Probe: "PROBE-103"}
	if c == nil {
		return r, errors.New("PROBE-103: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-103: nil captured-request recorder")
	}
	if len(ids) == 0 {
		return r, errors.New("PROBE-103: at least one ehr id is required to exercise the subset-delete parameters")
	}

	before := len(captured())
	if err := admin.DeleteAllEHRs(ctx, c, ids...); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("DeleteAllEHRs failed: %v", err)
		return r, nil
	}

	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("issued %d requests, want exactly 1", n)
		return r, nil
	}
	req := reqs[len(reqs)-1]
	if req.Method != http.MethodDelete {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("bulk delete used %s, want DELETE", req.Method)
		return r, nil
	}
	if !strings.HasSuffix(req.URL.Path, "/admin/ehr/all") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("bulk delete path %q, want a trailing /admin/ehr/all", req.URL.Path)
		return r, nil
	}
	// One repeatable ehr_id parameter per id, and never a path segment per id.
	want := make([]string, len(ids))
	for i, id := range ids {
		want[i] = string(id)
	}
	got := req.URL.Query()["ehr_id"]
	slices.Sort(want)
	sortedGot := slices.Clone(got)
	slices.Sort(sortedGot)
	if !slices.Equal(sortedGot, want) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("ehr_id query parameters = %v, want one per id %v", got, want)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("DELETE /admin/ehr/all with %d ehr_id parameter(s)", len(ids))
	return r, nil
}
