package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// fakeCDR stands in for a CDR's openEHR REST API in tests. It answers the
// calls the run makes, and is as strict as the CDRs on the points the run
// handles: it refuses an OPT that starts with a byte-order mark, an EHR
// create that carries a body or a Content-Type, and, when persistent is
// set, a second composition in one EHR.
type fakeCDR struct {
	srv *httptest.Server
	// user and pass, when set, are the Basic credential every request must
	// carry.
	user, pass string
	// persistent refuses a second composition in one EHR, as FerroEHR does
	// for a persistent template.
	persistent bool
	// refuseCompositions answers every composition with a 422 whose text
	// quotes the request's Authorization header.
	refuseCompositions bool
	// serveStored rewrites a stored OPT on the way out; nil serves it as
	// uploaded.
	serveStored func([]byte) []byte
	// example is served for every template the fake holds.
	example []byte

	mu          sync.Mutex
	requests    int
	templates   map[string][]byte
	uploads     [][]byte
	ehrs        map[string]int
	ehrCreates  int
	posted      []postedDoc
	exampleAsks []string
}

type postedDoc struct {
	ehr  string
	body []byte
}

const fakeBase = "/openehr/v1"

var fakeTemplateID = regexp.MustCompile(`(?s)<template_id>\s*<value>(.*?)</value>`)

func newFakeCDR(t *testing.T) *fakeCDR {
	t.Helper()
	f := &fakeCDR{templates: map[string][]byte{}, ehrs: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// url is the base the run is given.
func (f *fakeCDR) url() string { return f.srv.URL + fakeBase }

func (f *fakeCDR) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// fakeState is a copy of what a fakeCDR saw.
type fakeState struct {
	uploads     [][]byte
	ehrCreates  int
	posted      []postedDoc
	exampleAsks []string
}

func (f *fakeCDR) state() fakeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fakeState{
		uploads:     slices.Clone(f.uploads),
		ehrCreates:  f.ehrCreates,
		posted:      slices.Clone(f.posted),
		exampleAsks: slices.Clone(f.exampleAsks),
	}
}

func writeJSONError(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (f *fakeCDR) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.user != "" {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(f.user+":"+f.pass))
		if r.Header.Get("Authorization") != want {
			writeJSONError(w, http.StatusUnauthorized, `{"error":"Unauthorized"}`)
			return
		}
	}
	p, ok := strings.CutPrefix(r.URL.Path, fakeBase)
	if !ok {
		http.NotFound(w, r)
		return
	}
	const tplRoute = "/definition/template/adl1.4"
	switch {
	case r.Method == http.MethodPost && p == tplRoute:
		f.uploads = append(f.uploads, body)
		if bytes.HasPrefix(body, utf8BOM) {
			writeJSONError(w, http.StatusBadRequest, `{"error":"Bad Request","message":"Content is not allowed in prolog."}`)
			return
		}
		m := fakeTemplateID.FindSubmatch(body)
		if m == nil {
			writeJSONError(w, http.StatusBadRequest, `{"message":"no template id"}`)
			return
		}
		id := string(m[1])
		if _, dup := f.templates[id]; dup {
			writeJSONError(w, http.StatusConflict, fmt.Sprintf(`{"message":"Operational template with this template ID %s already exists"}`, id))
			return
		}
		f.templates[id] = body
		w.Header().Set("Location", f.url()+tplRoute+"/"+id)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodGet && strings.HasPrefix(p, tplRoute+"/") && strings.HasSuffix(p, "/example"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, tplRoute+"/"), "/example")
		f.exampleAsks = append(f.exampleAsks, id+"?"+r.URL.RawQuery)
		if _, ok := f.templates[id]; !ok || f.example == nil {
			writeJSONError(w, http.StatusNotFound, `{"message":"not found"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.example)
	case r.Method == http.MethodGet && strings.HasPrefix(p, tplRoute+"/"):
		stored, ok := f.templates[strings.TrimPrefix(p, tplRoute+"/")]
		if !ok {
			writeJSONError(w, http.StatusNotFound, `{"message":"not found"}`)
			return
		}
		if f.serveStored != nil {
			stored = f.serveStored(stored)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(stored)
	case r.Method == http.MethodPost && p == "/ehr":
		if len(body) > 0 || r.Header.Get("Content-Type") != "" {
			writeJSONError(w, http.StatusUnsupportedMediaType, `{"message":"Content-Type is not supported"}`)
			return
		}
		f.ehrCreates++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.ehrCreates)
		f.ehrs[id] = 0
		w.Header().Set("Location", f.url()+"/ehr/"+id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"_type":"EHR","ehr_id":{"_type":"HIER_OBJECT_ID","value":%q},"system_id":{"_type":"HIER_OBJECT_ID","value":"fake"},"time_created":{"_type":"DV_DATE_TIME","value":"2026-01-02T03:04:05Z"}}`, id)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/ehr/") && strings.HasSuffix(p, "/composition"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/ehr/"), "/composition")
		n, ok := f.ehrs[id]
		if !ok {
			writeJSONError(w, http.StatusNotFound, `{"message":"EHR not found"}`)
			return
		}
		if f.persistent && n > 0 {
			writeJSONError(w, http.StatusConflict, `{"message":"a persistent composition already exists in this EHR"}`)
			return
		}
		f.ehrs[id] = n + 1
		f.posted = append(f.posted, postedDoc{ehr: id, body: body})
		if f.refuseCompositions {
			writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf(`{"message":"rejected request with header Authorization: %s"}`, r.Header.Get("Authorization")))
			return
		}
		w.Header().Set("Location", f.url()+"/ehr/"+id+"/composition/11111111-1111-4111-8111-111111111111::fake::1")
		w.WriteHeader(http.StatusCreated)
	default:
		writeJSONError(w, http.StatusNotFound, `{"message":"no such route"}`)
	}
}

// testTemplate is the vendored OPT the pipeline tests run over.
const testTemplate = "minimal_evaluation.en.v1"

// templateDir returns a new directory holding the given OPT files, each
// under the base name of its path, and with prefix prepended to its bytes.
func templateDir(t *testing.T, prefix []byte, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), append(append([]byte(nil), prefix...), b...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// env is a fake process environment.
type env map[string]string

func (e env) get(k string) string { return e[k] }

// runFor runs the tool under e and returns what it printed. The output
// directory is a new one under the test's temp directory unless e names
// one.
func runFor(t *testing.T, e env, args ...string) (stdout, stderr string, out string, err error) {
	t.Helper()
	out = filepath.Join(t.TempDir(), "out")
	if e[envOut] == "" {
		e[envOut] = out
	} else {
		out = e[envOut]
	}
	var so, se bytes.Buffer
	err = run(t.Context(), args, e.get, &so, &se)
	return so.String(), se.String(), out, err
}

// vendoredOPT is the path of the test template.
func vendoredOPT() string { return fixtures.TemplateOptForName(testTemplate) }

// exampleUID is the uid the fake's example carries, which the cross stage
// must remove.
const exampleUID = "22222222-2222-4222-8222-222222222222::cdr::1"

// vendoredExample is the vendored canonical-JSON composition of the test
// template (it carries no uid), given a top-level uid.
func vendoredExample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(fixtures.CompositionJSON(testTemplate))
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.TrimSpace(b)
	if !bytes.HasPrefix(b, []byte("{")) || bytes.Contains(b, []byte(`"uid"`)) {
		t.Fatalf("%s is no longer an object without a uid", fixtures.CompositionJSON(testTemplate))
	}
	return append([]byte(`{"uid":{"_type":"OBJECT_VERSION_ID","value":"`+exampleUID+`"},`), b[1:]...)
}
