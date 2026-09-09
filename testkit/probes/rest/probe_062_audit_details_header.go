package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// auditHeaderTokens are the dotted-attribute keys the openehr-audit-details
// grammar requires (REQ-059), asserted independently of the encoder so a switch
// to a JSON body or a dropped attribute is caught, not merely a diff against the
// same encoder the SDK used.
var auditHeaderTokens = []string{`change_type.code_string="`, `committer.name="`, `system_id="`}

// Probe062AuditDetailsHeader implements PROBE-062: a write carrying audit
// details emits the `openehr-audit-details` request header in the openEHR
// dotted-attribute grammar (not JSON), and the resulting Contribution's audit
// envelope reflects the same fields on read-back (REQ-059).
//
// captured returns the requests the backend received; the probe reads the write
// request to check the header grammar, then reads the Contribution back to check
// the audit round-trip.
func Probe062AuditDetailsHeader(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID openehrclient.EHRID, contributionUID string, audit *rm.AuditDetails, comp *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-062"}
	if c == nil || ehrID == "" || contributionUID == "" || audit == nil || comp == nil {
		return r, errors.New("PROBE-062: missing required inputs (client/ehr/contributionUID/audit/comp)")
	}
	if captured == nil {
		return r, errors.New("PROBE-062: nil captured-request recorder")
	}

	// Write arm — the audit details ride the openehr-audit-details header.
	before := len(captured())
	if _, _, err := composition.Save(ctx, c, ehrID, comp, composition.WithAuditDetails(audit)); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("audit-carrying write failed: %v", err)
		return r, nil
	}
	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("write issued %d requests, want exactly 1", n)
		return r, nil
	}
	got := reqs[len(reqs)-1].Header.Get("openehr-audit-details")
	if got == "" {
		r.Status = "fail"
		r.Detail = "write carried no openehr-audit-details header"
		return r, nil
	}
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("openehr-audit-details is JSON (%q); REQ-059 requires the dotted-attribute grammar", got)
		return r, nil
	}
	for _, tok := range auditHeaderTokens {
		if !strings.Contains(got, tok) {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("openehr-audit-details %q is missing the dotted attribute %s…", got, tok)
			return r, nil
		}
	}
	want, err := openehrclient.MarshalAuditDetails(audit)
	if err != nil {
		return r, fmt.Errorf("PROBE-062: encode expected header: %w", err)
	}
	if got != want {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("openehr-audit-details on the wire = %q, want the canonical grammar %q", got, want)
		return r, nil
	}

	// Read-back arm — the Contribution's audit reflects the committed fields.
	contrib, _, err := contribution.Get(ctx, c, ehrID, contributionUID)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Contribution read-back failed: %v", err)
		return r, nil
	}
	if contrib == nil || contrib.Audit == nil {
		r.Status = "fail"
		r.Detail = "Contribution read-back carried no audit envelope"
		return r, nil
	}
	if code := contrib.Audit.GetChangeType().DefiningCode.CodeString; code != audit.ChangeType.DefiningCode.CodeString {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back change_type code = %q, want %q", code, audit.ChangeType.DefiningCode.CodeString)
		return r, nil
	}
	if sysID := contrib.Audit.GetSystemID(); sysID != audit.SystemID {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back system_id = %q, want %q", sysID, audit.SystemID)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "openehr-audit-details emitted in dotted grammar; Contribution audit reflects change_type + system_id"
	return r, nil
}
