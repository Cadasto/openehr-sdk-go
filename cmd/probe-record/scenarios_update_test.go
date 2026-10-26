package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// versionedCompositionServer answers the composition-update scenario the way
// the ITS-REST pin describes, on top of the sandbox's EHR surface. It keeps
// every composition's versions; a save makes version 1, and an update must
// carry the latest version id as a quoted If-Match, or it is refused: 400
// when the header is missing or unquoted, staleStatus when it names an older
// version. A write with Prefer: return=representation answers the body.
type versionedCompositionServer struct {
	// staleStatus is the answer to a stale If-Match: 412 as the pin says,
	// or another status for a server that departs from it.
	staleStatus int

	mu       sync.Mutex
	versions map[string]int    // latest version number, by versioned object id
	bodies   map[string][]byte // latest body, by versioned object id
}

func newVersionedCompositionServer(staleStatus int) *versionedCompositionServer {
	return &versionedCompositionServer{
		staleStatus: staleStatus,
		versions:    make(map[string]int),
		bodies:      make(map[string][]byte),
	}
}

func (s *versionedCompositionServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/definition/template/adl1.4", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", sandboxBase+"/definition/template/adl1.4/sdk_cassette_template.v1")
		w.WriteHeader(http.StatusCreated)
	})
	b.HandleFunc(http.MethodPost, "/ehr/", s.save)
	b.HandleFunc(http.MethodGet, "/ehr/", s.get)
	b.HandleFunc(http.MethodPut, "/ehr/", s.update)
	return b
}

func versionUID(vo string, n int) string { return fmt.Sprintf("%s::sandbox.local::%d", vo, n) }

// answer writes a version's metadata and, under return=representation, its
// body; otherwise it answers with no content.
func answer(w http.ResponseWriter, r *http.Request, status int, location, uid string, body []byte) {
	w.Header().Set("Location", location)
	w.Header().Set("ETag", `"`+uid+`"`)
	if r.Header.Get("Prefer") != "return=representation" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *versionedCompositionServer) save(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/composition") {
		http.NotFound(w, r)
		return
	}
	body, err := requestBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	vo := fmt.Sprintf("composition-%d", len(s.versions)+1)
	s.versions[vo] = 1
	s.bodies[vo] = body
	s.mu.Unlock()
	answer(w, r, http.StatusCreated, "https://sandbox.local"+r.URL.Path+"/"+vo, versionUID(vo, 1), body)
}

func (s *versionedCompositionServer) get(w http.ResponseWriter, r *http.Request) {
	dir, vo := path.Split(r.URL.Path)
	s.mu.Lock()
	n, ok := s.versions[vo]
	body := s.bodies[vo]
	s.mu.Unlock()
	if !strings.HasSuffix(dir, "/composition/") || !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", `"`+versionUID(vo, n)+`"`)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *versionedCompositionServer) update(w http.ResponseWriter, r *http.Request) {
	dir, vo := path.Split(r.URL.Path)
	ifMatch := r.Header.Get("If-Match")
	if len(ifMatch) < 2 || !strings.HasPrefix(ifMatch, `"`) || !strings.HasSuffix(ifMatch, `"`) {
		http.Error(w, "If-Match must be a quoted version uid", http.StatusBadRequest)
		return
	}
	body, err := requestBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.versions[vo]
	if !strings.HasSuffix(dir, "/composition/") || !ok {
		http.NotFound(w, r)
		return
	}
	if ifMatch != `"`+versionUID(vo, n)+`"` {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.staleStatus)
		_, _ = io.WriteString(w, `{"message":"If-Match is not the latest version","validationErrors":[]}`)
		return
	}
	s.versions[vo] = n + 1
	s.bodies[vo] = body
	answer(w, r, http.StatusOK, "https://sandbox.local"+r.URL.Path, versionUID(vo, n+1), body)
}

// TestCaptureScenario_CompositionUpdateRecordsTheVersionedWrites drives the
// composition-update scenario through the recorder against an in-memory
// server, with no CDR and no network. The recording holds the save, the
// read and update with the latest ETag, the stale update refused with 412,
// the second read, and the save and update with return=representation, in
// that order, each update on the versioned object the first save created.
func TestCaptureScenario_CompositionUpdateRecordsTheVersionedWrites(t *testing.T) {
	t.Parallel()
	recPath, err := captureScenario(t.Context(), scenarios["composition-update"], sandboxBase,
		newVersionedCompositionServer(http.StatusPreconditionFailed).backend(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario(composition-update) against the versioned server = %v", err)
	}
	if filepath.Base(recPath) != "composition-update.har" {
		t.Fatalf("recording path = %q, want basename composition-update.har", recPath)
	}
	har, err := probe.ValidateHAR(recPath)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}

	calls, statuses := recordedCalls(t, har)
	if len(calls) < 3 {
		t.Fatalf("composition-update recorded exchanges %q, want at least the create, upload and save", calls)
	}
	ehrPath := strings.TrimPrefix(calls[2], "POST ")
	voPath := ehrPath + "/composition-1"
	wantCalls := []string{
		"POST /ehr",
		"POST /definition/template/adl1.4",
		"POST " + ehrPath,
		"GET " + voPath,
		"PUT " + voPath,
		"PUT " + voPath,
		"GET " + voPath,
		"POST " + ehrPath,
		"PUT " + voPath,
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("composition-update recorded exchanges %q, want %q", calls, wantCalls)
	}
	want := []int{
		http.StatusCreated, http.StatusCreated, http.StatusCreated,
		http.StatusOK, http.StatusNoContent, http.StatusPreconditionFailed,
		http.StatusOK, http.StatusCreated, http.StatusOK,
	}
	if !slices.Equal(statuses, want) {
		t.Fatalf("composition-update recorded statuses %v, want %v", statuses, want)
	}
}

// TestCaptureScenario_CompositionUpdateRefusesAConflictForAStaleIfMatch: a
// server that answers the stale update with 409 instead of the 412 the
// pin gives fails the capture, which says so and writes nothing.
func TestCaptureScenario_CompositionUpdateRefusesAConflictForAStaleIfMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := captureScenario(t.Context(), scenarios["composition-update"], sandboxBase,
		newVersionedCompositionServer(http.StatusConflict).backend(), nil, validProvenance(), dir)
	if err == nil || !strings.Contains(err.Error(), "stale If-Match") {
		t.Fatalf("captureScenario(composition-update) against a 409 server = %v, want a refusal naming the stale If-Match", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the refused capture left %d entries in the output directory, want none", len(entries))
	}
}
