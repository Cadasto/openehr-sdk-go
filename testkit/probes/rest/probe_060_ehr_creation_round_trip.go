package restprobes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe060EHRCreationRoundTrip implements PROBE-060: `POST /ehr` with an initial
// EHR_STATUS body names the created EHR — through both the decoded body and the
// `Location` header — and a follow-up GET of the EHR_STATUS returns the status
// that was committed (REQ-095).
//
// The create is server-assigned (no client id), so the SDK must recover the id
// from the response — a create that names nothing leaves the caller unable to
// read the EHR it just made. status carries a distinctive archetype_node_id and
// queryable/modifiable flags so the read-back is checked for identity, not mere
// presence, and the submitted body is decoded back out of the captured request
// so a create that dropped or rewrote the initial status is caught at the
// source rather than only in the read-back.
//
// Both routes are asserted exactly: the create must land on `/ehr` and the
// read-back on `/ehr/{ehr_id}/ehr_status` for the id the create returned, so a
// read-back aimed at some other EHR cannot pass by returning a matching status.
//
// The probe does not assert the numeric status code (201): [openehrclient.Create]
// does not surface it — the leaf returns the decoded EHR and the version
// metadata, and the code itself never reaches the caller. Asserting it here
// would mean asserting against the fixture rather than against the SDK.
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
	rec, meta, err := openehrclient.Create(ctx, c, openehrclient.WithInitialStatus(status))
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
	wantCreatePath, err := servicePath(c, "/ehr")
	if err != nil {
		return r, fmt.Errorf("PROBE-060: %w", err)
	}
	if postReq.URL.Path != wantCreatePath {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("create path %q, want the exact /ehr path %q", postReq.URL.Path, wantCreatePath)
		return r, nil
	}
	if postReq.ContentLength == 0 {
		r.Status = "fail"
		r.Detail = "create carried no body; PROBE-060 requires an initial EHR_STATUS"
		return r, nil
	}
	if fault := submittedStatusFault(postReq, status); fault != "" {
		r.Status = "fail"
		r.Detail = fault
		return r, nil
	}

	// The create must name the new EHR in the Location header as well as in
	// the body — a caller that reads only the header must be able to find it.
	if meta == nil || meta.Location == "" {
		r.Status = "fail"
		r.Detail = "create returned no Location header; the created EHR is named only by the body"
		return r, nil
	}
	wantLocationTail := "/ehr/" + rec.EHRID.Value
	if !strings.HasSuffix(strings.TrimSuffix(meta.Location, "/"), wantLocationTail) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("create Location %q does not name the created EHR (want a %q tail)", meta.Location, wantLocationTail)
		return r, nil
	}

	// Read-back arm — the committed EHR_STATUS reads back with the same fields.
	ehrID := openehrclient.EHRID(rec.EHRID.Value)
	beforeRead := len(captured())
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
	readReqs := captured()
	if n := len(readReqs) - beforeRead; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("EHR_STATUS read-back issued %d requests, want exactly 1", n)
		return r, nil
	}
	getReq := readReqs[len(readReqs)-1]
	if getReq.Method != http.MethodGet {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("EHR_STATUS read-back used %s, want GET", getReq.Method)
		return r, nil
	}
	wantReadPath, err := servicePath(c, "/ehr/"+string(ehrID)+"/ehr_status")
	if err != nil {
		return r, fmt.Errorf("PROBE-060: %w", err)
	}
	if getReq.URL.Path != wantReadPath {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back path %q, want the created EHR's ehr_status route %q", getReq.URL.Path, wantReadPath)
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
	r.Detail = fmt.Sprintf("POST /ehr → ehr_id %s (named in body and Location); GET ehr_status round-trips the committed status", rec.EHRID.Value)
	return r, nil
}

// submittedStatusFault decodes the captured create body back into an
// EHR_STATUS and compares the identifying fields with what the caller asked
// for. It returns "" when the submitted body is the status that was handed in,
// and otherwise the probe detail describing the discrepancy.
func submittedStatusFault(postReq *http.Request, want *rm.EHRStatus) string {
	if postReq.Body == nil {
		return "the submitted EHR_STATUS was not captured; the create body is unavailable"
	}
	body, err := io.ReadAll(postReq.Body)
	if err != nil {
		return fmt.Sprintf("reading the submitted EHR_STATUS body: %v", err)
	}
	var got rm.EHRStatus
	if err := canjson.Unmarshal(body, &got); err != nil {
		return fmt.Sprintf("the submitted EHR_STATUS does not decode as an EHR_STATUS: %v", err)
	}
	if got.ArchetypeNodeID != want.ArchetypeNodeID {
		return fmt.Sprintf("submitted EHR_STATUS archetype_node_id = %q, want %q", got.ArchetypeNodeID, want.ArchetypeNodeID)
	}
	if got.IsQueryable != want.IsQueryable || got.IsModifiable != want.IsModifiable {
		return fmt.Sprintf("submitted EHR_STATUS queryable/modifiable = %v/%v, want %v/%v", got.IsQueryable, got.IsModifiable, want.IsQueryable, want.IsModifiable)
	}
	return ""
}
