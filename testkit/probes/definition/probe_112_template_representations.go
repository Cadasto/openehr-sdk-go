package definitionprobes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe112Inputs names the template [Probe112TemplateRepresentations] reads
// and the bytes each server serves for it.
type Probe112Inputs struct {
	// TemplateID is the ADL 1.4 template both servers hold.
	TemplateID string
	// OperationalTemplate is the body the first server sends for
	// Accept: application/xml, the operational template (OPT XML).
	OperationalTemplate []byte
	// WebTemplate is the body the first server sends for
	// Accept: application/openehr.wt+json, the Web Template (JSON).
	WebTemplate []byte
	// WithoutWebTemplates is a client to a second server that holds
	// TemplateID but offers no Web Templates, so it answers
	// Accept: application/openehr.wt+json with 406 Not Acceptable.
	WithoutWebTemplates *transport.Client
}

// probe112Read is one representation read and the request it must send.
type probe112Read struct {
	name   string
	accept string
	want   []byte
	call   func(context.Context, *transport.Client, string, definition.TemplateFormat) ([]byte, *transport.Metadata, error)
}

// Probe112TemplateRepresentations implements PROBE-112:
// [definition.GetOperationalTemplate] and [definition.GetWebTemplate] read the
// ADL 1.4 template route with their representation's Accept and hand back
// the body the server sent.
//
// Each read must issue exactly one GET to
// /definition/template/adl1.4/{template_id} under the client's base path,
// with the template id in place and its own Accept: application/xml for the
// operational template, application/openehr.wt+json for the Web Template.
// Each must return the served body byte for byte, so a backend that ignores
// Accept, or one that reformats a body, fails the probe.
//
// It then reads the Web Template from the server without Web Templates. That
// read must fail with a non-nil error and non-nil response metadata. Sentinel
// identity is not asserted here; the definition package's unit tests own it.
//
// Inputs:
//   - c is a client to the first server, which serves both representations.
//   - captured returns every request the first server received, in order.
//     The caller wires it up (a `sandbox.Backend` scripted route in Sandbox
//     mode); the probe reads length deltas to count requests, so it must not
//     be reset while the probe runs.
//   - in names the template, its two bodies and the second server; see
//     [Probe112Inputs].
//
// An error means the probe could not run: a nil client or recorder, an empty
// template id or body, or a client with no openEHR REST service to take the
// base path from.
func Probe112TemplateRepresentations(ctx context.Context, c *transport.Client, captured func() []*http.Request, in Probe112Inputs) (Result, error) { // PROBE-112 (REQ-177)
	r := Result{Probe: "PROBE-112"}
	if c == nil {
		return r, errors.New("PROBE-112: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-112: nil captured-request recorder")
	}
	if in.TemplateID == "" {
		return r, errors.New("PROBE-112: empty template id")
	}
	if len(in.OperationalTemplate) == 0 {
		return r, errors.New("PROBE-112: empty operational template body")
	}
	if len(in.WebTemplate) == 0 {
		return r, errors.New("PROBE-112: empty Web Template body")
	}
	if in.WithoutWebTemplates == nil {
		return r, errors.New("PROBE-112: nil client to the server without Web Templates")
	}
	svc, ok := c.Catalog().OpenEHRRest()
	if !ok || svc.BaseURL == nil {
		return r, errors.New("PROBE-112: the client has no openEHR REST service to take the base path from")
	}
	wantPath := strings.TrimSuffix(svc.BaseURL.Path, "/") + "/definition/template/adl1.4/" + in.TemplateID

	for _, rd := range []probe112Read{
		{"GetOperationalTemplate", "application/xml", in.OperationalTemplate, definition.GetOperationalTemplate},
		{"GetWebTemplate", "application/openehr.wt+json", in.WebTemplate, definition.GetWebTemplate},
	} {
		before := len(captured())
		body, _, err := rd.call(ctx, c, in.TemplateID, definition.FormatADL14)
		if err != nil {
			return probe112Fail(r, "%s failed: %v", rd.name, err), nil
		}
		reqs := captured()
		if n := len(reqs) - before; n != 1 {
			return probe112Fail(r, "%s issued %d request(s), want exactly 1", rd.name, n), nil
		}
		req := reqs[before]
		if req.Method != http.MethodGet {
			return probe112Fail(r, "%s used %s, want GET", rd.name, req.Method), nil
		}
		if req.URL.Path != wantPath {
			return probe112Fail(r, "%s path %q, want the template route %q", rd.name, req.URL.Path, wantPath), nil
		}
		if got := req.Header.Get("Accept"); got != rd.accept {
			return probe112Fail(r, "%s sent Accept %q, want %q", rd.name, got, rd.accept), nil
		}
		if !bytes.Equal(body, rd.want) {
			return probe112Fail(r, "%s returned %d bytes that differ from the %d bytes served; the body must come back byte for byte", rd.name, len(body), len(rd.want)), nil
		}
	}

	// The refusal: a server without Web Templates answers 406, and the read
	// reports it with the response metadata beside the error.
	body, md, err := definition.GetWebTemplate(ctx, in.WithoutWebTemplates, in.TemplateID, definition.FormatADL14)
	if err == nil {
		return probe112Fail(r, "GetWebTemplate succeeded (%d bytes) against the server without Web Templates; want the 406 as an error", len(body)), nil
	}
	if md == nil {
		return probe112Fail(r, "GetWebTemplate against the server without Web Templates failed with no response metadata beside the error: %v", err), nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("GET …/adl1.4/%s as application/xml and application/openehr.wt+json, both bodies byte for byte; 406 surfaced with metadata", in.TemplateID)
	return r, nil
}

// probe112Fail stamps r as a failure with the formatted detail.
func probe112Fail(r Result, format string, args ...any) Result {
	r.Status = "fail"
	r.Detail = fmt.Sprintf(format, args...)
	return r
}
