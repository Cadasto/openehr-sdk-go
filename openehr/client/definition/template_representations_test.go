package definition_test

// template_representations_test.go — REQ-177. One template route serves two
// representations, chosen by Accept: the operational template as XML and the
// Web Template as JSON. Each has its own call, and both hand the body back
// exactly as the server sent it.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// templateGetter is the shape the template reads share.
type templateGetter func(ctx context.Context, c *transport.Client, templateID string, format definition.TemplateFormat) ([]byte, *transport.Metadata, error)

const (
	acceptOperationalTemplate = "application/xml"
	acceptWebTemplate         = "application/openehr.wt+json"
	wantTemplatePath          = "/openehr/v1/definition/template/adl1.4/body_weight.v1"
)

// Bodies a decode and re-encode would change: leading whitespace, odd
// spacing inside the tag and around the colons, and a trailing newline. Only
// a call that hands the bytes back untouched returns them unchanged.
var (
	rawOperationalTemplate = []byte("\n  <?xml version=\"1.0\"  encoding=\"UTF-8\"?>\n<template   xmlns=\"http://schemas.openehr.org/v1\"  ><template_id><value>body_weight.v1</value></template_id></template>\n")
	rawWebTemplate         = []byte("  {\n  \"templateId\" :  \"body_weight.v1\",\"version\":\"2.3\" ,\n \"tree\" : {\"id\":\"body_weight\"}}\n")
)

// representationCalls lists each template read with the Accept it must send
// and a body in its own representation.
var representationCalls = []struct {
	name   string
	call   templateGetter
	accept string
	body   []byte
}{
	{name: "GetOperationalTemplate", call: definition.GetOperationalTemplate, accept: acceptOperationalTemplate, body: rawOperationalTemplate},
	{name: "GetWebTemplate", call: definition.GetWebTemplate, accept: acceptWebTemplate, body: rawWebTemplate},
}

// capturedRequest is what a test server records of each request it receives.
type capturedRequest struct {
	method, path, accept string
}

// recordingServer answers every request with status, an ETag and body. The
// function it returns lists the requests received so far.
func recordingServer(t *testing.T, status int, body []byte) (*transport.Client, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, capturedRequest{method: r.Method, path: r.URL.Path, accept: r.Header.Get("Accept")})
		mu.Unlock()
		w.Header().Set("ETag", `"body_weight.v1::1"`)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	requests := func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
	return newClient(t, srv), requests
}

// TestTemplateRepresentationRequest pins the request each call sends and the
// body it returns: one GET on the template route with the call's own Accept,
// and the 200 body byte for byte beside the response metadata (REQ-177).
func TestTemplateRepresentationRequest(t *testing.T) {
	// REQ-177
	for _, tc := range representationCalls {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := recordingServer(t, http.StatusOK, tc.body)

			got, meta, err := tc.call(t.Context(), c, "body_weight.v1", definition.FormatADL14)
			if err != nil {
				t.Fatalf("%s(body_weight.v1) = %v, want nil error", tc.name, err)
			}
			seen := requests()
			if len(seen) != 1 {
				t.Fatalf("%s sent %d request(s), want 1", tc.name, len(seen))
			}
			want := capturedRequest{method: http.MethodGet, path: wantTemplatePath, accept: tc.accept}
			if seen[0] != want {
				t.Errorf("%s sent %+v, want %+v", tc.name, seen[0], want)
			}
			if !bytes.Equal(got, tc.body) {
				t.Errorf("%s returned %q, want the body byte for byte: %q", tc.name, got, tc.body)
			}
			if meta == nil {
				t.Fatalf("%s returned nil metadata beside a 200", tc.name)
			}
			if meta.ETag != "body_weight.v1::1" {
				t.Errorf("%s: Metadata.ETag = %q, want %q", tc.name, meta.ETag, "body_weight.v1::1")
			}
		})
	}
}

// TestTemplateRepresentationRefusesBeforeRequest pins the two refusals: a
// format other than ADL 1.4 and an empty template id each fail with
// ErrInvalidConfig, under the call's own name, and the server sees no
// request (REQ-177).
func TestTemplateRepresentationRefusesBeforeRequest(t *testing.T) {
	// REQ-177
	inputs := []struct {
		name       string
		templateID string
		format     definition.TemplateFormat
	}{
		{name: "ADL 2 format", templateID: "body_weight.v1", format: "adl2"},
		{name: "empty format", templateID: "body_weight.v1", format: ""},
		{name: "empty template id", templateID: "", format: definition.FormatADL14},
	}
	for _, tc := range representationCalls {
		for _, in := range inputs {
			t.Run(tc.name+"/"+in.name, func(t *testing.T) {
				c, requests := recordingServer(t, http.StatusOK, tc.body)

				got, meta, err := tc.call(t.Context(), c, in.templateID, in.format)
				if !errors.Is(err, transport.ErrInvalidConfig) {
					t.Fatalf("%s(%q, %q) err = %v, want transport.ErrInvalidConfig", tc.name, in.templateID, in.format, err)
				}
				if prefix := "definition." + tc.name + ": "; !strings.HasPrefix(err.Error(), prefix) {
					t.Errorf("%s(%q, %q) err = %q, want it to start with %q", tc.name, in.templateID, in.format, err, prefix)
				}
				if n := len(requests()); n != 0 {
					t.Errorf("%s(%q, %q) sent %d request(s), want none", tc.name, in.templateID, in.format, n)
				}
				if got != nil || meta != nil {
					t.Errorf("%s(%q, %q) = (%q, %+v), want nil body and nil metadata beside a refusal", tc.name, in.templateID, in.format, got, meta)
				}
			})
		}
	}
}

// TestTemplateRepresentationNotAcceptable pins the non-2xx arm on the case a
// server that does not offer a representation answers with: a 406 fails with
// the typed wire error, and the response metadata still comes back beside it
// (REQ-177).
func TestTemplateRepresentationNotAcceptable(t *testing.T) {
	// REQ-177
	for _, tc := range representationCalls {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := recordingServer(t, http.StatusNotAcceptable, nil)

			got, meta, err := tc.call(t.Context(), c, "body_weight.v1", definition.FormatADL14)
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("%s against a 406 err = %v, want a *transport.WireError", tc.name, err)
			}
			if we.StatusCode != http.StatusNotAcceptable {
				t.Errorf("%s: WireError.StatusCode = %d, want %d", tc.name, we.StatusCode, http.StatusNotAcceptable)
			}
			if meta == nil {
				t.Fatalf("%s returned nil metadata beside the 406; the response headers must come back with the error", tc.name)
			}
			if meta.ETag != "body_weight.v1::1" {
				t.Errorf("%s: Metadata.ETag = %q, want %q", tc.name, meta.ETag, "body_weight.v1::1")
			}
			if got != nil {
				t.Errorf("%s returned body %q beside the 406, want nil", tc.name, got)
			}
		})
	}
}

// TestGetTemplateIsOperationalTemplateAlias pins the deprecated GetTemplate
// to what GetOperationalTemplate returns for the same arguments: the same
// request, the same bytes, the same error and the same metadata, on success,
// on a 406 and on each refusal (REQ-177).
func TestGetTemplateIsOperationalTemplateAlias(t *testing.T) {
	// REQ-177
	cases := []struct {
		name       string
		status     int
		templateID string
		format     definition.TemplateFormat
	}{
		{name: "200", status: http.StatusOK, templateID: "body_weight.v1", format: definition.FormatADL14},
		{name: "406", status: http.StatusNotAcceptable, templateID: "body_weight.v1", format: definition.FormatADL14},
		{name: "ADL 2 format", status: http.StatusOK, templateID: "body_weight.v1", format: "adl2"},
		{name: "empty template id", status: http.StatusOK, templateID: "", format: definition.FormatADL14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := recordingServer(t, tc.status, rawOperationalTemplate)

			wantBody, wantMeta, wantErr := definition.GetOperationalTemplate(t.Context(), c, tc.templateID, tc.format)
			gotBody, gotMeta, gotErr := definition.GetTemplate(t.Context(), c, tc.templateID, tc.format)

			if !bytes.Equal(gotBody, wantBody) {
				t.Errorf("GetTemplate body = %q, want %q (what GetOperationalTemplate returned)", gotBody, wantBody)
			}
			if errText(gotErr) != errText(wantErr) {
				t.Errorf("GetTemplate err = %v, want %v (what GetOperationalTemplate returned)", gotErr, wantErr)
			}
			if (gotMeta == nil) != (wantMeta == nil) {
				t.Errorf("GetTemplate metadata = %+v, want %+v (what GetOperationalTemplate returned)", gotMeta, wantMeta)
			}
			if gotMeta != nil && wantMeta != nil && gotMeta.ETag != wantMeta.ETag {
				t.Errorf("GetTemplate: Metadata.ETag = %q, want %q", gotMeta.ETag, wantMeta.ETag)
			}
			// Both calls reach the server or neither does; when they do, they
			// send the same request.
			switch seen := requests(); len(seen) {
			case 0:
			case 2:
				if seen[1] != seen[0] {
					t.Errorf("GetTemplate sent %+v, want %+v (what GetOperationalTemplate sent)", seen[1], seen[0])
				}
				if seen[1].accept != acceptOperationalTemplate {
					t.Errorf("GetTemplate sent Accept %q, want %q", seen[1].accept, acceptOperationalTemplate)
				}
			default:
				t.Errorf("GetOperationalTemplate and GetTemplate together sent %d request(s), want 0 or 2", len(seen))
			}
		})
	}
}

// errText returns err's text, or "" for a nil error.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestRepositoryTemplateRepresentations pins that the Repository's template
// reads send the request and return the bytes of the package-level call each
// one stands for (REQ-177).
func TestRepositoryTemplateRepresentations(t *testing.T) {
	// REQ-177
	cases := []struct {
		name   string
		repo   func(r definition.Repository) templateGetter
		direct templateGetter
		body   []byte
	}{
		{
			name: "GetOperationalTemplate",
			repo: func(r definition.Repository) templateGetter {
				return func(ctx context.Context, _ *transport.Client, id string, f definition.TemplateFormat) ([]byte, *transport.Metadata, error) {
					return r.GetOperationalTemplate(ctx, id, f)
				}
			},
			direct: definition.GetOperationalTemplate,
			body:   rawOperationalTemplate,
		},
		{
			name: "GetWebTemplate",
			repo: func(r definition.Repository) templateGetter {
				return func(ctx context.Context, _ *transport.Client, id string, f definition.TemplateFormat) ([]byte, *transport.Metadata, error) {
					return r.GetWebTemplate(ctx, id, f)
				}
			},
			direct: definition.GetWebTemplate,
			body:   rawWebTemplate,
		},
		{
			name: "GetTemplate",
			repo: func(r definition.Repository) templateGetter {
				return func(ctx context.Context, _ *transport.Client, id string, f definition.TemplateFormat) ([]byte, *transport.Metadata, error) {
					return r.GetTemplate(ctx, id, f)
				}
			},
			direct: definition.GetOperationalTemplate,
			body:   rawOperationalTemplate,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := recordingServer(t, http.StatusOK, tc.body)
			viaRepo := tc.repo(definition.NewRepository(c))

			want, _, err := tc.direct(t.Context(), c, "body_weight.v1", definition.FormatADL14)
			if err != nil {
				t.Fatalf("package-level call = %v, want nil error", err)
			}
			got, meta, err := viaRepo(t.Context(), nil, "body_weight.v1", definition.FormatADL14)
			if err != nil {
				t.Fatalf("Repository.%s = %v, want nil error", tc.name, err)
			}
			seen := requests()
			if len(seen) != 2 {
				t.Fatalf("sent %d request(s), want 2", len(seen))
			}
			if seen[1] != seen[0] {
				t.Errorf("Repository.%s sent %+v, want %+v (what the package-level call sent)", tc.name, seen[1], seen[0])
			}
			if !bytes.Equal(got, want) || !bytes.Equal(got, tc.body) {
				t.Errorf("Repository.%s returned %q, want %q", tc.name, got, tc.body)
			}
			if meta == nil {
				t.Errorf("Repository.%s returned nil metadata beside a 200", tc.name)
			}
		})
	}
}
