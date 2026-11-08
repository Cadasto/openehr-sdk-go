package definitionprobes_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", b.HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readOPT(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "corpus", "its_rest", "definition", "body_weight.opt")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	return b
}

func TestProbe067TemplateUploadRoundTrip(t *testing.T) {
	opt := readOPT(t)
	// Server stores POSTed body in memory and returns it on GET.
	var stored []byte
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			stored, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/body_weight.v1")
			_, _ = w.Write([]byte(`{"template_id":"body_weight.v1"}`))
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(stored)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	})

	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, b), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-067 status = %q (detail: %s)", r.Status, r.Detail)
	}
	if !bytes.Equal(stored, opt) {
		t.Error("upload body did not round-trip server-side")
	}
}

func TestProbe067RejectsServerIDMismatch(t *testing.T) {
	opt := readOPT(t)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Server returns a different template_id than expected.
		_, _ = w.Write([]byte(`{"template_id":"other_template.v1"}`))
	})
	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, b), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on id mismatch, got %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe067RejectsByteDrift(t *testing.T) {
	opt := readOPT(t)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"template_id":"body_weight.v1"}`))
		case http.MethodGet:
			// Server reformatted on storage — returns different bytes.
			_, _ = w.Write([]byte(`<?xml version="1.0"?><template/>`))
		}
	})
	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, b), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on byte drift, got %q (detail: %s)", r.Status, r.Detail)
	}
}

// TestProbe093TemplateListFilters drives PROBE-093 in Sandbox mode: the
// fake records every request's query and answers each list call with a
// template-metadata body (REQ-143).
func TestProbe093TemplateListFilters(t *testing.T) {
	var captured []url.Values
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"template_id":"vital_signs.v1","concept":"Vital Signs"}]`))
	})

	res, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, b), &captured)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-093 = %s: %s", res.Status, res.Detail)
	}
	// Three of the four legs reach the wire; the negative-paging leg must not.
	if len(captured) != 3 {
		t.Errorf("captured %d requests, want 3 (unfiltered, filtered, zero-paging)", len(captured))
	}
}

// TestProbe093EmptyCatalogPasses pins that an empty catalog is a pass, not
// a decode failure: REQ-143 licenses no assertion that a filtered
// deployment holds templates. This test says nothing about the shape of the
// returned slice — the probe discards it entirely and asserts only on the
// error and the captured queries. The non-nil zero-length slice the
// definition metadata decoding rule requires on an empty 2xx body is pinned
// by TestListTemplatesEmpty and TestListStoredQueriesEmpty in
// openehr/client/definition.
func TestProbe093EmptyCatalogPasses(t *testing.T) {
	var captured []url.Values
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, r.URL.Query())
		w.WriteHeader(http.StatusNoContent)
	})

	res, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, b), &captured)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-093 against an empty catalog = %s: %s", res.Status, res.Detail)
	}
}

// TestProbe093RejectsMissingInputs pins the probe's own guard rails.
func TestProbe093RejectsMissingInputs(t *testing.T) {
	var captured []url.Values
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {})

	if _, err := probes.Probe093TemplateListFilters(t.Context(), nil, &captured); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, b), nil); err == nil {
		t.Error("nil recorder: want an error")
	}
}

// probe112TemplateID is the template id of the vendored fixture pair
// constrain_test.opt and constrain_test.webtemplate.json.
const probe112TemplateID = "constrain_test"

// readTemplatePair reads the vendored operational template and the Web
// Template generated from it, the two bodies PROBE-112's backend serves.
func readTemplatePair(t *testing.T) (opt, webTemplate []byte) {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(src), "..", "..", "corpus", "webtemplate")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read fixture %q: %v", name, err)
		}
		return b
	}
	return read("constrain_test.opt"), read("constrain_test.webtemplate.json")
}

// recording returns a backend that records each request it receives and then
// answers it with serve. rewrite, when set, decides what is recorded for a
// request: a changed copy, nothing, or more than one entry. That plants a
// client that sent something else without touching what the backend serves.
func recording(t *testing.T, rewrite func(*http.Request) []*http.Request, serve http.HandlerFunc) (*sandbox.Backend, func() []*http.Request) {
	t.Helper()
	var (
		mu   sync.Mutex
		reqs []*http.Request
	)
	b := sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		seen := []*http.Request{r.Clone(r.Context())}
		if rewrite != nil {
			seen = rewrite(seen[0])
		}
		mu.Lock()
		reqs = append(reqs, seen...)
		mu.Unlock()
		serve(w, r)
	})
	snap := func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(reqs)
	}
	return b, snap
}

// serveRepresentations answers the operational template's Accept with opt,
// the Web Template's Accept with webTemplate, and any other Accept with 406.
// A nil webTemplate stands for a server without Web Templates: it answers
// that Accept with 406 too.
func serveRepresentations(opt, webTemplate []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch accept := r.Header.Get("Accept"); {
		case accept == "application/xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(opt)
		case accept == "application/openehr.wt+json" && webTemplate != nil:
			w.Header().Set("Content-Type", "application/openehr.wt+json")
			_, _ = w.Write(webTemplate)
		default:
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}
}

// serveOnly answers every request with body, whatever the Accept.
func serveOnly(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(body)
	}
}

// withoutWebTemplates is a client to a server that holds the template as an
// operational template only: it answers the Web Template's Accept with 406.
func withoutWebTemplates(t *testing.T, opt []byte) *transport.Client {
	t.Helper()
	return newClient(t, sandbox.Scripted(serveRepresentations(opt, nil)))
}

// roundTripFunc adapts a function to an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func probe112Inputs(opt, webTemplate []byte, refusing *transport.Client) probes.Probe112Inputs {
	return probes.Probe112Inputs{
		TemplateID:          probe112TemplateID,
		OperationalTemplate: opt,
		WebTemplate:         webTemplate,
		WithoutWebTemplates: refusing,
	}
}

// TestProbe112TemplateRepresentations drives PROBE-112 (REQ-177) in Sandbox
// mode: one backend serves both representations of the vendored template by
// Accept, a second holds no Web Templates and answers that Accept with 406.
func TestProbe112TemplateRepresentations(t *testing.T) {
	opt, wt := readTemplatePair(t)
	b, snap := recording(t, nil, serveRepresentations(opt, wt))

	r, err := probes.Probe112TemplateRepresentations(t.Context(), newClient(t, b), snap, probe112Inputs(opt, wt, withoutWebTemplates(t, opt)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Fatalf("PROBE-112 = %s: %s", r.Status, r.Detail)
	}
	if n := len(snap()); n != 2 {
		t.Errorf("the serving backend saw %d requests, want 2 (one per representation)", n)
	}
}

// TestProbe112FlagsBrokenReads pins one can-fail plant per probe check
// (PROBE-112, REQ-177). The backend plants change what a server sends; the
// recorder plants change what the probe sees reach the wire, standing in for
// a client that sent another request.
func TestProbe112FlagsBrokenReads(t *testing.T) {
	opt, wt := readTemplatePair(t)
	reformatted := func(b []byte) []byte { return append(slices.Clone(b), '\n') }
	rewriteIf := func(accept string, change func(*http.Request)) func(*http.Request) []*http.Request {
		return func(r *http.Request) []*http.Request {
			if r.Header.Get("Accept") == accept {
				change(r)
			}
			return []*http.Request{r}
		}
	}
	unreachable := func(t *testing.T) *transport.Client {
		hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("planted: connection refused")
		})}
		c, err := probe.NewClient("https://sandbox.local/openehr/v1", hc, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	cases := []struct {
		name       string
		serve      http.HandlerFunc
		rewrite    func(*http.Request) []*http.Request
		refusing   func(*testing.T) *transport.Client
		wantDetail string
	}{
		{
			name:       "backend ignores Accept and serves the operational template to both reads",
			serve:      serveOnly(opt),
			wantDetail: "GetWebTemplate returned",
		},
		{
			name:       "backend alters the operational template body",
			serve:      serveRepresentations(reformatted(opt), wt),
			wantDetail: "GetOperationalTemplate returned",
		},
		{
			name:       "backend alters the Web Template body",
			serve:      serveRepresentations(opt, reformatted(wt)),
			wantDetail: "GetWebTemplate returned",
		},
		{
			name:  "server without Web Templates serves one instead of 406",
			serve: serveRepresentations(opt, wt),
			refusing: func(t *testing.T) *transport.Client {
				return newClient(t, sandbox.Scripted(serveRepresentations(opt, wt)))
			},
			wantDetail: "succeeded",
		},
		{
			name:       "refusal arrives with no response metadata",
			serve:      serveRepresentations(opt, wt),
			refusing:   unreachable,
			wantDetail: "no response metadata",
		},
		{
			name:       "read reaches the wire zero times",
			serve:      serveRepresentations(opt, wt),
			rewrite:    func(*http.Request) []*http.Request { return nil },
			wantDetail: "issued 0 request(s)",
		},
		{
			name:       "read reaches the wire twice",
			serve:      serveRepresentations(opt, wt),
			rewrite:    func(r *http.Request) []*http.Request { return []*http.Request{r, r} },
			wantDetail: "issued 2 request(s)",
		},
		{
			name:       "read uses another method",
			serve:      serveRepresentations(opt, wt),
			rewrite:    rewriteIf("application/openehr.wt+json", func(r *http.Request) { r.Method = http.MethodPost }),
			wantDetail: "want GET",
		},
		{
			name:  "read lands on another format",
			serve: serveRepresentations(opt, wt),
			rewrite: rewriteIf("application/xml", func(r *http.Request) {
				r.URL.Path = strings.Replace(r.URL.Path, "/adl1.4/", "/adl2/", 1)
			}),
			wantDetail: "template route",
		},
		{
			name:  "read carries another template id",
			serve: serveRepresentations(opt, wt),
			rewrite: rewriteIf("application/openehr.wt+json", func(r *http.Request) {
				r.URL.Path = strings.TrimSuffix(r.URL.Path, probe112TemplateID) + "other.v1"
			}),
			wantDetail: "template route",
		},
		{
			name:       "operational template read asks for JSON",
			serve:      serveRepresentations(opt, wt),
			rewrite:    rewriteIf("application/xml", func(r *http.Request) { r.Header.Set("Accept", "application/json") }),
			wantDetail: `GetOperationalTemplate sent Accept "application/json"`,
		},
		{
			name:       "Web Template read asks for the operational template",
			serve:      serveRepresentations(opt, wt),
			rewrite:    rewriteIf("application/openehr.wt+json", func(r *http.Request) { r.Header.Set("Accept", "application/xml") }),
			wantDetail: `GetWebTemplate sent Accept "application/xml"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, snap := recording(t, tc.rewrite, tc.serve)
			refusing := withoutWebTemplates(t, opt)
			if tc.refusing != nil {
				refusing = tc.refusing(t)
			}
			r, err := probes.Probe112TemplateRepresentations(t.Context(), newClient(t, b), snap, probe112Inputs(opt, wt, refusing))
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "fail" {
				t.Fatalf("PROBE-112 = %s, want fail (detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tc.wantDetail) {
				t.Errorf("PROBE-112 failed for the wrong reason:\n got: %s\nwant it to mention: %s", r.Detail, tc.wantDetail)
			}
			t.Logf("PROBE-112 fail detail: %s", r.Detail)
		})
	}
}

// TestProbe112RejectsMissingInputs pins the probe's own guard rails
// (PROBE-112, REQ-177): each refusal is an error, and none reaches the wire.
func TestProbe112RejectsMissingInputs(t *testing.T) {
	opt, wt := readTemplatePair(t)
	b, snap := recording(t, nil, serveRepresentations(opt, wt))
	c := newClient(t, b)
	refusing := withoutWebTemplates(t, opt)

	if _, err := probes.Probe112TemplateRepresentations(t.Context(), nil, snap, probe112Inputs(opt, wt, refusing)); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe112TemplateRepresentations(t.Context(), c, nil, probe112Inputs(opt, wt, refusing)); err == nil {
		t.Error("nil recorder: want an error")
	}
	blank := map[string]func(*probes.Probe112Inputs){
		"TemplateID":          func(in *probes.Probe112Inputs) { in.TemplateID = "" },
		"OperationalTemplate": func(in *probes.Probe112Inputs) { in.OperationalTemplate = nil },
		"WebTemplate":         func(in *probes.Probe112Inputs) { in.WebTemplate = nil },
		"WithoutWebTemplates": func(in *probes.Probe112Inputs) { in.WithoutWebTemplates = nil },
	}
	for field, blankOut := range blank {
		in := probe112Inputs(opt, wt, refusing)
		blankOut(&in)
		if _, err := probes.Probe112TemplateRepresentations(t.Context(), c, snap, in); err == nil {
			t.Errorf("empty %s: want an error", field)
		}
	}

	// The probe takes the base path from the client's openEHR REST service,
	// so a catalog without that service, or with one that has no base URL,
	// cannot run it.
	noREST, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog rejected an empty services map (setup change?): %v", err)
	}
	noBaseURL := &discovery.ServiceCatalog{
		Issuer:   "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: {ID: discovery.ServiceIDOpenEHRRest}},
	}
	for name, cat := range map[string]*discovery.ServiceCatalog{
		"catalog without the openEHR REST service": noREST,
		"openEHR REST service without a base URL":  noBaseURL,
	} {
		bare, err := transport.New(cat, transport.WithHTTPClient(c.HTTPClient()))
		if err != nil {
			t.Fatalf("%s: transport.New: %v", name, err)
		}
		if _, err := probes.Probe112TemplateRepresentations(t.Context(), bare, snap, probe112Inputs(opt, wt, refusing)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if n := len(snap()); n != 0 {
		t.Errorf("PROBE-112 issued %d request(s) on missing inputs, want 0", n)
	}
}
