package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe060EHRCreationRoundTrip implements PROBE-060: `POST /ehr` with an initial
// EHR_STATUS body returns 201 and surfaces the server-assigned ehr_id, and a
// follow-up GET of the EHR_STATUS returns the status that was committed
// (REQ-095).
//
// The create is server-assigned (no client id), so the SDK must recover the id
// from the response — a create that names nothing leaves the caller unable to
// read the EHR it just made. status carries a distinctive archetype_node_id and
// queryable/modifiable flags so the read-back is checked for identity, not mere
// presence.
func Probe060EHRCreationRoundTrip(ctx context.Context, c *transport.Client, captured func() []*http.Request, status *rm.EHRStatus) (Result, error) {
	r := Result{Probe: "PROBE-060"}
	if c == nil || status == nil {
		return r, errors.New("PROBE-060: nil transport.Client or status")
	}
	if captured == nil {
		return r, errors.New("PROBE-060: nil captured-request recorder")
	}

	// Create arm — server-assigned id, initial EHR_STATUS in the body.
	before := len(captured())
	rec, _, err := openehrclient.Create(ctx, c, openehrclient.WithInitialStatus(status))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Create failed: %v", err)
		return r, nil
	}
	if rec == nil || rec.EHRID.Value == "" {
		r.Status = "fail"
		r.Detail = "Create surfaced no ehr_id"
		return r, nil
	}
	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Create issued %d requests, want exactly 1", n)
		return r, nil
	}
	postReq := reqs[len(reqs)-1]
	if postReq.Method != http.MethodPost {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("create used %s, want POST (server-assigned id)", postReq.Method)
		return r, nil
	}
	if !strings.HasSuffix(postReq.URL.Path, "/ehr") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("create path %q, want a trailing /ehr", postReq.URL.Path)
		return r, nil
	}
	if postReq.ContentLength == 0 {
		r.Status = "fail"
		r.Detail = "create carried no body; PROBE-060 requires an initial EHR_STATUS"
		return r, nil
	}

	// Read-back arm — the committed EHR_STATUS reads back with the same fields.
	ehrID := openehrclient.EHRID(rec.EHRID.Value)
	st, _, err := ehrstatus.Get(ctx, c, ehrID)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("EHR_STATUS read-back failed: %v", err)
		return r, nil
	}
	if st == nil {
		r.Status = "fail"
		r.Detail = "EHR_STATUS read-back returned nil"
		return r, nil
	}
	if st.ArchetypeNodeID != status.ArchetypeNodeID {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back archetype_node_id = %q, want %q", st.ArchetypeNodeID, status.ArchetypeNodeID)
		return r, nil
	}
	if st.IsQueryable != status.IsQueryable || st.IsModifiable != status.IsModifiable {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back queryable/modifiable = %v/%v, want %v/%v", st.IsQueryable, st.IsModifiable, status.IsQueryable, status.IsModifiable)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("POST /ehr → ehr_id %s; GET ehr_status round-trips the committed status", rec.EHRID.Value)
	return r, nil
}
