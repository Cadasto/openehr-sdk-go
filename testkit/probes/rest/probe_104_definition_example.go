package restprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe104DefinitionExample implements PROBE-104: the Definition example
// endpoint is `GET /definition/template/{format}/{template_id}/example`, and its
// response decodes into a full COMPOSITION for the named template (REQ-095).
//
// captured returns the requests the backend received; the probe reads the
// newest to confirm the verb and the example route.
func Probe104DefinitionExample(ctx context.Context, c *transport.Client, captured func() []*http.Request, templateID string, format definition.TemplateFormat) (Result, error) {
	r := Result{Probe: "PROBE-104"}
	if c == nil {
		return r, errors.New("PROBE-104: nil transport.Client")
	}
	if captured == nil {
		return r, errors.New("PROBE-104: nil captured-request recorder")
	}
	if templateID == "" {
		return r, errors.New("PROBE-104: empty templateID")
	}

	before := len(captured())
	comp, _, err := definition.ExampleComposition(ctx, c, templateID, format)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("ExampleComposition failed: %v", err)
		return r, nil
	}
	if comp == nil {
		r.Status = "fail"
		r.Detail = "ExampleComposition returned a nil Composition"
		return r, nil
	}
	if comp.ArchetypeNodeID == "" {
		r.Status = "fail"
		r.Detail = "example Composition has empty archetype_node_id; body is not a full COMPOSITION"
		return r, nil
	}

	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("issued %d requests, want exactly 1", n)
		return r, nil
	}
	req := reqs[len(reqs)-1]
	if req.Method != http.MethodGet {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("example fetch used %s, want GET", req.Method)
		return r, nil
	}
	if !strings.Contains(req.URL.Path, "/definition/template/") || !strings.HasSuffix(req.URL.Path, "/example") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("example path %q, want …/definition/template/{format}/{template_id}/example", req.URL.Path)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("GET …/%s/example → full COMPOSITION", templateID)
	return r, nil
}
