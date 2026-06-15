package definitionprobes

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe067TemplateUploadRoundTrip implements PROBE-067: a POST
// against /definition/template/{format} with an OPT body succeeds;
// a subsequent GET returns the same OPT bytes (modulo backend-side
// reformatting documented per deployment).
//
// The probe accepts the OPT bytes verbatim so the same fixture is
// reused across conformant implementations (REQ-082). Round-trip equality is checked by a
// byte comparison; backends that reformat the OPT on storage (e.g.
// normalising whitespace) will not pass and SHOULD document the
// canonical-form rule for conformance comparison.
//
// Inputs:
//   - opt is the OPT XML body to upload.
//   - templateID is the deployment-assigned id the SDK MUST receive
//     back in the upload metadata. When empty the probe accepts any
//     non-empty id returned by the server.
func Probe067TemplateUploadRoundTrip(ctx context.Context, c *transport.Client, opt []byte, templateID string) (Result, error) {
	r := Result{Probe: "PROBE-067"}
	if c == nil {
		return r, errors.New("PROBE-067: nil transport.Client")
	}
	if len(opt) == 0 {
		return r, errors.New("PROBE-067: empty OPT")
	}

	uploaded, _, err := definition.UploadTemplate(ctx, c, definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("upload failed: %v", err)
		return r, nil
	}
	if uploaded == nil || uploaded.TemplateID == "" {
		r.Status = "fail"
		r.Detail = "upload response carried no template id"
		return r, nil
	}
	if templateID != "" && uploaded.TemplateID != templateID {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("server-assigned id %q does not match expected %q", uploaded.TemplateID, templateID)
		return r, nil
	}

	fetched, _, err := definition.GetTemplate(ctx, c, uploaded.TemplateID, definition.FormatADL14)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("get failed: %v", err)
		return r, nil
	}
	if !bytes.Equal(fetched, opt) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("OPT bytes differ across the round-trip (uploaded=%d bytes, fetched=%d bytes); deployment may reformat — see PROBE-067 docstring", len(opt), len(fetched))
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}
