package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"
)

const systemID = "sandbox.local"

// openEHRBase is the fixed tail every openEHR REST base URL ends with.
// Deployments prepend whatever they like ("/ehrbase/rest", "/api",
// nothing at all), so the sandbox cuts the request path through this
// segment instead of matching a closed list of known prefixes.
const openEHRBase = "/openehr/v1"

// Backend is an in-memory openEHR REST backend. It is safe for
// concurrent use (REQ-026). The zero value is ready to use, like
// [bytes.Buffer] — ehrs allocates lazily under mu on first write, so
// a caller never needs [New].
type Backend struct {
	mu      sync.Mutex
	ehrs    map[string][]byte
	scripts []scripted
}

// New returns an empty Backend. Equivalent to the zero value
// (var b Backend); New exists for callers who prefer a constructor.
func New() *Backend {
	return &Backend{}
}

// ensureEHRs lazily allocates b.ehrs. Callers MUST hold b.mu.
func (b *Backend) ensureEHRs() {
	if b.ehrs == nil {
		b.ehrs = make(map[string][]byte)
	}
}

// HTTPClient returns an *http.Client whose Transport is b, so
// transport.New can inject it with no listener (REQ-021, REQ-082).
func (b *Backend) HTTPClient() *http.Client {
	return &http.Client{Transport: b}
}

// RoundTrip serves one in-memory openEHR REST exchange.
//
// A request whose context is already done is refused with that
// context's error rather than served: [http.RoundTripper] implementors
// must honour cancellation, and a real transport would fail the same
// way. Without this a probe that cancels before dispatch would see a
// successful 201 in Sandbox mode and a cancellation error against a
// CDR — the cross-mode disagreement REQ-082 exists to prevent.
func (b *Backend) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("sandbox: nil request")
	}
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	if err := req.Context().Err(); err != nil {
		return nil, fmt.Errorf("sandbox: request context: %w", err)
	}
	if h := b.matchScript(req); h != nil {
		return serveScript(h, req), nil
	}
	path := resourcePath(requestPath(req))
	switch {
	case req.Method == http.MethodPost && path == "/ehr":
		return b.createEHR(req, "")
	case req.Method == http.MethodPut && strings.HasPrefix(path, "/ehr/"):
		rest := strings.TrimPrefix(path, "/ehr/")
		if rest != "" && !strings.Contains(rest, "/") {
			return b.createEHR(req, rest)
		}
	case (req.Method == http.MethodGet || req.Method == http.MethodHead) && strings.HasPrefix(path, "/ehr/"):
		rest := strings.TrimPrefix(path, "/ehr/")
		if rest != "" && !strings.Contains(rest, "/") {
			return b.getEHR(req, rest)
		}
	}
	return jsonResponse(req, http.StatusNotFound, []byte(`{"message":"not found"}`)), nil
}

func (b *Backend) createEHR(req *http.Request, id string) (*http.Response, error) {
	if id == "" {
		id = newID()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// The ITS-REST EHR schema (resources/its-rest/ehr-validation.openapi.yaml,
	// components.schemas.Ehr) types both ehr_status.id and ehr_access.id as
	// OBJECT_VERSION_ID (ObjectRefOfObjectVersionId); the vendored
	// testkit/cassettes/its_rest/ehr/ehr.json fixture instead uses
	// HIER_OBJECT_ID for both — this backend follows the OpenAPI schema.
	statusID := newID() + "::" + systemID + "::1"
	accessID := newID() + "::" + systemID + "::1"
	body := fmt.Appendf(nil, `{`+
		`"_type":"EHR",`+
		`"system_id":{"_type":"HIER_OBJECT_ID","value":%q},`+
		`"ehr_id":{"_type":"HIER_OBJECT_ID","value":%q},`+
		`"ehr_status":{"_type":"OBJECT_REF","namespace":"local","type":"EHR_STATUS",`+
		`"id":{"_type":"OBJECT_VERSION_ID","value":%q}},`+
		`"ehr_access":{"_type":"OBJECT_REF","namespace":"local","type":"EHR_ACCESS",`+
		`"id":{"_type":"OBJECT_VERSION_ID","value":%q}},`+
		`"time_created":{"_type":"DV_DATE_TIME","value":%q}`+
		`}`, systemID, id, statusID, accessID, now)

	b.mu.Lock()
	b.ensureEHRs()
	if _, exists := b.ehrs[id]; exists {
		b.mu.Unlock()
		return jsonResponse(req, http.StatusConflict, []byte(`{"message":"ehr exists"}`)), nil
	}
	b.ehrs[id] = body
	b.mu.Unlock()

	// ITS-REST response 201_EHR defines exactly these two headers
	// (components.responses.201_EHR): ETag_EHR is "the ehr_id enclosed
	// by double quotes" and Location_EHR is `format: url` — an absolute
	// URL, as its own example shows.
	resp := jsonResponse(req, http.StatusCreated, body)
	resp.Header.Set("Location", ehrLocation(req, id))
	resp.Header.Set("ETag", `"`+id+`"`)
	return resp, nil
}

// getEHR answers GET/HEAD /ehr/{id}. It deliberately emits no ETag and
// no Location: ITS-REST response 200_EHR
// (components.responses.200_EHR) defines only Content-Type, so a
// conformant CDR returns neither. Emitting them here would make
// transport.Metadata.ETag non-empty in Sandbox mode and empty against a
// real CDR — a cross-mode disagreement REQ-082 forbids.
func (b *Backend) getEHR(req *http.Request, id string) (*http.Response, error) {
	b.mu.Lock()
	body, ok := b.ehrs[id]
	b.mu.Unlock()
	if !ok {
		return jsonResponse(req, http.StatusNotFound, []byte(`{"message":"ehr not found"}`)), nil
	}
	var payload []byte
	if req.Method == http.MethodGet {
		payload = body
	}
	return jsonResponse(req, http.StatusOK, payload), nil
}

// requestPath is the request's URL path, or "" when the caller handed
// RoundTrip a request with no URL.
func requestPath(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.Path
}

// ehrLocation builds the absolute URL of the EHR resource for the
// 201 Location header. ITS-REST types Location_EHR as `format: url`
// and its example is absolute, so the value is derived from the
// request: its scheme, its host, and its own path with the id
// appended — except on PUT /ehr/{id}, where the request path already
// names the id and appending it again would produce /ehr/{id}/{id}.
//
// A request with no usable host (RoundTrip called directly with a
// relative URL) falls back to the path alone rather than inventing a
// host.
func ehrLocation(req *http.Request, id string) string {
	path := "/ehr/" + id
	var scheme, host string
	if req != nil && req.URL != nil {
		scheme, host = req.URL.Scheme, req.URL.Host
		if p := req.URL.Path; p != "" {
			if strings.HasSuffix(p, "/"+id) {
				path = p
			} else {
				path = strings.TrimSuffix(p, "/") + "/" + id
			}
		}
	}
	if host == "" && req != nil {
		host = req.Host
	}
	if host == "" {
		return path
	}
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + host + path
}

// resourcePath reduces a request path to the openEHR resource path by
// cutting it through the first "/openehr/v1" segment, wherever the
// deployment's REST base happens to put it — "/openehr/v1/ehr",
// "/ehrbase/rest/openehr/v1/ehr" and "/api/openehr/v1/ehr" all reduce
// to "/ehr". The match respects segment boundaries: what follows must
// be empty (the base URL itself, which reduces to "/") or start with
// "/", so "/openehr/v11/ehr" is not a base. A path with no such
// segment is used as-is, so a bare "/ehr" still reaches the built-in
// routes.
func resourcePath(p string) string {
	for off := 0; off < len(p); {
		i := strings.Index(p[off:], openEHRBase)
		if i < 0 {
			break
		}
		rest := p[off+i+len(openEHRBase):]
		if rest == "" {
			return "/"
		}
		if strings.HasPrefix(rest, "/") {
			return rest
		}
		off += i + 1
	}
	return p
}

// statusLine renders the Status field net/http documents: the status
// line without the HTTP/1.1 prefix, e.g. "201 Created" — not the bare
// reason phrase.
func statusLine(code int) string {
	return strconv.Itoa(code) + " " + http.StatusText(code)
}

// jsonResponse builds a built-in (non-scripted) response. It matches
// serveScript field for field — same status line, same bytes.Reader
// body, same non-nil Request — so a consumer cannot tell a built-in
// route from a scripted one by inspecting the *http.Response.
func jsonResponse(req *http.Request, status int, body []byte) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode:    status,
		Status:        statusLine(status),
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
	}
}

// newID returns a random UUID v4 string. uuid.NewV4 has no error path —
// it draws from crypto/rand internally — so there is nothing to
// propagate here.
func newID() string {
	return uuid.NewV4().String()
}
