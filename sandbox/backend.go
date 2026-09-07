package sandbox

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const systemID = "sandbox.local"

// Backend is an in-memory openEHR REST backend. It is safe for
// concurrent use (REQ-026).
type Backend struct {
	mu      sync.Mutex
	ehrs    map[string][]byte
	scripts []scripted
}

// New returns an empty Backend.
func New() *Backend {
	return &Backend{ehrs: make(map[string][]byte)}
}

// HTTPClient returns an *http.Client whose Transport is b, so
// transport.New can inject it with no listener (REQ-021, REQ-082).
func (b *Backend) HTTPClient() *http.Client {
	return &http.Client{Transport: b}
}

// RoundTrip serves one in-memory openEHR REST exchange.
func (b *Backend) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("sandbox: nil request")
	}
	if req.Body != nil {
		defer req.Body.Close()
	}
	if h := b.matchScript(req); h != nil {
		return serveScript(h, req), nil
	}
	path := resourcePath(req.URL.Path)
	switch {
	case req.Method == http.MethodPost && path == "/ehr":
		return b.createEHR("")
	case req.Method == http.MethodPut && strings.HasPrefix(path, "/ehr/"):
		rest := strings.TrimPrefix(path, "/ehr/")
		if rest != "" && !strings.Contains(rest, "/") {
			return b.createEHR(rest)
		}
	case (req.Method == http.MethodGet || req.Method == http.MethodHead) && strings.HasPrefix(path, "/ehr/"):
		rest := strings.TrimPrefix(path, "/ehr/")
		if rest != "" && !strings.Contains(rest, "/") {
			return b.getEHR(req.Method, rest)
		}
	}
	return jsonResponse(http.StatusNotFound, []byte(`{"message":"not found"}`)), nil
}

func (b *Backend) createEHR(id string) (*http.Response, error) {
	if id == "" {
		id = newID()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statusID := newID() + "::" + systemID + "::1"
	body := fmt.Appendf(nil, `{`+
		`"_type":"EHR",`+
		`"system_id":{"_type":"HIER_OBJECT_ID","value":%q},`+
		`"ehr_id":{"_type":"HIER_OBJECT_ID","value":%q},`+
		`"ehr_status":{"_type":"OBJECT_REF","namespace":"local","type":"EHR_STATUS",`+
		`"id":{"_type":"OBJECT_VERSION_ID","value":%q}},`+
		`"time_created":{"_type":"DV_DATE_TIME","value":%q}`+
		`}`, systemID, id, statusID, now)

	b.mu.Lock()
	if _, exists := b.ehrs[id]; exists {
		b.mu.Unlock()
		return jsonResponse(http.StatusConflict, []byte(`{"message":"ehr exists"}`)), nil
	}
	b.ehrs[id] = body
	b.mu.Unlock()

	resp := jsonResponse(http.StatusCreated, body)
	resp.Header.Set("Location", "/ehr/"+id)
	resp.Header.Set("ETag", `"`+id+`"`)
	return resp, nil
}

func (b *Backend) getEHR(method, id string) (*http.Response, error) {
	b.mu.Lock()
	body, ok := b.ehrs[id]
	b.mu.Unlock()
	if !ok {
		return jsonResponse(http.StatusNotFound, []byte(`{"message":"ehr not found"}`)), nil
	}
	var payload []byte
	if method == http.MethodGet {
		payload = body
	}
	resp := jsonResponse(http.StatusOK, payload)
	resp.Header.Set("ETag", `"`+id+`"`)
	resp.Header.Set("Location", "/ehr/"+id)
	return resp, nil
}

func resourcePath(p string) string {
	for _, prefix := range []string{
		"/openehr/v1",
		"/ehrbase/rest/openehr/v1",
		"/ferroehr/rest/openehr/v1",
		"/rest/openehr/v1",
	} {
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			if rest == "" {
				return "/"
			}
			return rest
		}
	}
	return p
}

func jsonResponse(status int, body []byte) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
