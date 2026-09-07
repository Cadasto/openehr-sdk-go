package sandbox

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

// scripted is one Handle/HandleFunc registration. An empty method or
// path matches any request; otherwise method is exact and path is a
// suffix on either the full URL path or the stripped resource path.
type scripted struct {
	method string
	path   string
	h      http.Handler
}

// Handle registers a scripted route that takes precedence over the
// built-in EHR surface. Routes are tried in registration order; the
// first match wins.
//
// An empty method matches any method; an empty path matches any
// request path, so Handle("", "", h) is a catch-all — the
// planted-backend shape probe tests use instead of httptest.NewServer
// (REQ-082). A non-empty path matches the request's full URL path or
// its resource-stripped form (see resourcePath) exactly, or as a
// suffix of either; a path ending in "/" additionally matches any
// request whose path falls under that subtree.
//
// h == nil is not silently dropped: it registers a route that fails
// closed, answering every matching request with 500 and a body naming
// the method and path it was registered for. REQ-025 forbids treating
// caller input as a silent no-op — a dropped nil handler would look
// like "this route never fired" to a test asserting on it, rather
// than the caller mistake it is.
func (b *Backend) Handle(method, path string, h http.Handler) {
	if h == nil {
		h = nilHandler(method, path)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scripts = append(b.scripts, scripted{method: method, path: path, h: h})
}

// HandleFunc registers a scripted route (see [Handle]). fn == nil
// behaves as in [Handle]: the route fails closed rather than being
// dropped.
func (b *Backend) HandleFunc(method, path string, fn func(http.ResponseWriter, *http.Request)) {
	if fn == nil {
		b.Handle(method, path, nil)
		return
	}
	b.Handle(method, path, http.HandlerFunc(fn))
}

// nilHandler answers every request with 500 and a body naming the
// nil registration, so a nil Handle/HandleFunc call fails loudly at
// request time instead of vanishing (REQ-025).
func nilHandler(method, path string) http.Handler {
	msg := fmt.Sprintf("sandbox: nil handler registered for %s %s", method, path)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// Scripted returns a Backend whose only behaviour is fn. Probe tests
// use this in place of httptest.NewServer so planted and hostile
// backends stay listener-free (REQ-082). fn == nil is not a caller
// error: every request gets a 500 naming the registration (see
// [Handle]).
func Scripted(fn func(http.ResponseWriter, *http.Request)) *Backend {
	b := New()
	b.HandleFunc("", "", fn)
	return b
}

func (b *Backend) matchScript(req *http.Request) http.Handler {
	b.mu.Lock()
	scripts := slices.Clone(b.scripts)
	b.mu.Unlock()
	for _, s := range scripts {
		if s.match(req) {
			return s.h
		}
	}
	return nil
}

func (s scripted) match(req *http.Request) bool {
	if s.method != "" && s.method != req.Method {
		return false
	}
	if s.path == "" {
		return true
	}
	p := ""
	if req.URL != nil {
		p = req.URL.Path
	}
	rp := resourcePath(p)
	if p == s.path || rp == s.path {
		return true
	}
	if strings.HasSuffix(p, s.path) || strings.HasSuffix(rp, s.path) {
		return true
	}
	// A trailing slash means "this subtree".
	if strings.HasSuffix(s.path, "/") && (strings.Contains(p, s.path) || strings.HasPrefix(rp, s.path)) {
		return true
	}
	return false
}

func serveScript(h http.Handler, req *http.Request) *http.Response {
	rec := &recorder{header: make(http.Header)}
	h.ServeHTTP(rec, req)
	code := rec.code
	if code == 0 {
		code = http.StatusOK
	}
	body := rec.body.Bytes()
	return &http.Response{
		StatusCode:    code,
		Status:        http.StatusText(code),
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
	}
}

// recorder is a listener-free http.ResponseWriter so scripted routes
// can use the same HandlerFunc shape as httptest.
type recorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
	wrote  bool
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) Write(p []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

func (r *recorder) WriteHeader(code int) {
	if r.wrote {
		return
	}
	r.wrote = true
	r.code = code
}
