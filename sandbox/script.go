package sandbox

import (
	"bytes"
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
// built-in EHR surface. First match wins. An empty method or path
// matches any request, so Handle("", "", h) is a catch-all — the
// planted-backend shape probe tests use instead of httptest.NewServer
// (REQ-082).
func (b *Backend) Handle(method, path string, h http.Handler) {
	if h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scripts = append(b.scripts, scripted{method: method, path: path, h: h})
}

// HandleFunc registers a scripted route (see [Handle]).
func (b *Backend) HandleFunc(method, path string, fn func(http.ResponseWriter, *http.Request)) {
	if fn == nil {
		return
	}
	b.Handle(method, path, http.HandlerFunc(fn))
}

// Scripted returns a Backend whose only behaviour is fn. Probe tests
// use this in place of httptest.NewServer so planted and hostile
// backends stay listener-free (REQ-082).
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
