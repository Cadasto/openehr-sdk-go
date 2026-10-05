package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// sandboxBase is the REST base the offline captures in this file drive.
// recordedCalls cuts it away, so an expected exchange reads as the
// resource path the scenario names.
const sandboxBase = "https://sandbox.local/openehr/v1"

// ehrStatusServer answers the ehr-status scenario the way a CDR does. A
// create must carry an EHR_STATUS body, which the server keeps for the EHR
// it mints; a read of /ehr/{id}/ehr_status returns that status, and 404
// for an EHR the server never created. The sandbox's own POST /ehr ignores
// the body, so this server answers the create itself.
type ehrStatusServer struct {
	mu       sync.Mutex
	ids      []string          // created EHR ids, in creation order
	statuses map[string][]byte // submitted EHR_STATUS bodies, by EHR id
}

func newEHRStatusServer() *ehrStatusServer {
	return &ehrStatusServer{statuses: make(map[string][]byte)}
}

// backend returns a sandbox serving s's routes.
func (s *ehrStatusServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/ehr", s.createEHR)
	b.HandleFunc(http.MethodGet, "/ehr/", s.getStatus)
	return b
}

// created returns the EHR ids the server minted, in creation order.
func (s *ehrStatusServer) created() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ids)
}

func (s *ehrStatusServer) createEHR(w http.ResponseWriter, r *http.Request) {
	body, err := requestBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var kind struct {
		Type string `json:"_type"`
	}
	var st rm.EHRStatus
	if json.Unmarshal(body, &kind) != nil || kind.Type != "EHR_STATUS" || canjson.Unmarshal(body, &st) != nil {
		http.Error(w, "the create carries no EHR_STATUS", http.StatusBadRequest)
		return
	}
	id := uuid.NewV4().String()
	s.mu.Lock()
	s.ids = append(s.ids, id)
	s.statuses[id] = body
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "https://sandbox.local"+r.URL.Path+"/"+id)
	w.Header().Set("ETag", `"`+id+`"`)
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"_type":"EHR",`+
		`"system_id":{"_type":"HIER_OBJECT_ID","value":"sandbox.local"},`+
		`"ehr_id":{"_type":"HIER_OBJECT_ID","value":%q},`+
		`"ehr_status":{"_type":"OBJECT_REF","namespace":"local","type":"EHR_STATUS",`+
		`"id":{"_type":"OBJECT_VERSION_ID","value":%q}}}`,
		id, id+"::sandbox.local::1")
}

func (s *ehrStatusServer) getStatus(w http.ResponseWriter, r *http.Request) {
	dir, leaf := path.Split(r.URL.Path)
	if leaf != "ehr_status" {
		http.NotFound(w, r)
		return
	}
	id := path.Base(dir)
	s.mu.Lock()
	body, ok := s.statuses[id]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// TestCaptureScenario_EHRStatusRecordsTheCreateAndItsReadBack drives the
// ehr-status scenario through the recorder against an in-memory server,
// with no CDR and no network. A capture that succeeds has already
// validated and replayed; the test then checks what the published
// recording holds: a create carrying the scenario's fixed EHR_STATUS, and
// a read of that status on the EHR the create minted, answered with the
// status the create carried.
func TestCaptureScenario_EHRStatusRecordsTheCreateAndItsReadBack(t *testing.T) {
	t.Parallel()
	srv := newEHRStatusServer()
	recPath, err := captureScenario(t.Context(), scenarios["ehr-status"], sandboxBase,
		srv.backend(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario(ehr-status) against the status server = %v", err)
	}
	if filepath.Base(recPath) != "ehr-status.har" {
		t.Fatalf("recording path = %q, want basename ehr-status.har", recPath)
	}
	har, err := probe.ValidateHAR(recPath)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}

	ids := srv.created()
	if len(ids) != 1 {
		t.Fatalf("the capture created EHRs %q on the server, want exactly one", ids)
	}
	calls, statuses := recordedCalls(t, har)
	wantCalls := []string{"POST /ehr", "GET /ehr/" + ids[0] + "/ehr_status"}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("ehr-status recorded exchanges %q, want %q", calls, wantCalls)
	}
	if want := []int{http.StatusCreated, http.StatusOK}; !slices.Equal(statuses, want) {
		t.Fatalf("ehr-status recorded statuses %v, want %v", statuses, want)
	}

	create, read := har.Log.Entries[0], har.Log.Entries[1]
	if create.Request.PostData == nil {
		t.Fatal("the recorded create carries no request body, want the scenario's EHR_STATUS")
	}
	sent := create.Request.PostData.Text
	if same, err := sameJSON(sent, cassetteEHRStatusJSON); err != nil || !same {
		t.Fatalf("recorded create body = %s (err %v), want the scenario's EHR_STATUS %s", sent, err, cassetteEHRStatusJSON)
	}
	if same, err := sameJSON(read.Response.Content.Text, sent); err != nil || !same {
		t.Fatalf("recorded read-back = %s (err %v), want the status the create carried %s", read.Response.Content.Text, err, sent)
	}
}

// storedQueryServer answers the stored-query scenario on top of the
// sandbox's EHR surface. A store needs query_type=AQL and a non-empty AQL
// body; it answers 200 with no body and a Location naming the version, as
// EHRbase does. An execution names a query the server holds, or gets 404,
// and must bind target_ehr; its one row is the EHR id that parameter
// binds, which is what the scenario's AQL selects.
type storedQueryServer struct {
	mu      sync.Mutex
	queries map[string]string // AQL text, by qualified name
}

func newStoredQueryServer() *storedQueryServer {
	return &storedQueryServer{queries: make(map[string]string)}
}

// backend returns a sandbox serving s's routes. POST /ehr stays with the
// sandbox's built-in EHR surface.
func (s *storedQueryServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPut, "/definition/query/", s.putQuery)
	b.HandleFunc(http.MethodPost, "/query/", s.runQuery)
	return b
}

// stored returns a copy of the queries the server holds.
func (s *storedQueryServer) stored() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.queries)
}

func (s *storedQueryServer) putQuery(w http.ResponseWriter, r *http.Request) {
	_, name, _ := strings.Cut(r.URL.Path, "/definition/query/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("query_type") != "AQL" {
		http.Error(w, "query_type must be AQL", http.StatusBadRequest)
		return
	}
	body, err := requestBody(r)
	if err != nil || strings.TrimSpace(string(body)) == "" {
		http.Error(w, "the store carries no AQL", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.queries[name] = string(body)
	s.mu.Unlock()
	w.Header().Set("Location", sandboxBase+"/definition/query/"+name+"/1.0.0")
	w.WriteHeader(http.StatusOK)
}

func (s *storedQueryServer) runQuery(w http.ResponseWriter, r *http.Request) {
	_, name, _ := strings.Cut(r.URL.Path, "/query/")
	s.mu.Lock()
	aqlText, ok := s.queries[name]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := requestBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var exec struct {
		QueryParameters map[string]any `json:"query_parameters"`
	}
	if err := json.Unmarshal(body, &exec); err != nil {
		http.Error(w, "the execution body is not JSON", http.StatusBadRequest)
		return
	}
	target, _ := exec.QueryParameters["target_ehr"].(string)
	if target == "" {
		http.Error(w, "the execution binds no target_ehr", http.StatusBadRequest)
		return
	}
	result, err := json.Marshal(map[string]any{
		"meta":    map[string]any{"_type": "RESULTSET", "_schema_version": "1.0.3", "resultsize": 1},
		"name":    name + "/1.0.0",
		"q":       aqlText,
		"columns": []map[string]string{{"path": "e/ehr_id/value", "name": "#0"}},
		"rows":    [][]string{{target}},
	})
	if err != nil {
		http.Error(w, "encode result set", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(result)
}

// TestCaptureScenario_StoredQueryRecordsTheStoreAndItsExecution drives the
// stored-query scenario through the recorder against an in-memory server,
// with no CDR and no network. A capture that succeeds has already
// validated and replayed; the test then checks what the server was sent
// and what the published recording holds: the scenario's one fixed query
// stored under its fixed name, a store response that keeps the Location a
// version is recovered from, and an execution bound to the EHR the same
// capture created, whose row is that EHR's id.
func TestCaptureScenario_StoredQueryRecordsTheStoreAndItsExecution(t *testing.T) {
	t.Parallel()
	srv := newStoredQueryServer()
	recPath, err := captureScenario(t.Context(), scenarios["stored-query"], sandboxBase,
		srv.backend(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario(stored-query) against the stored-query server = %v", err)
	}
	if filepath.Base(recPath) != "stored-query.har" {
		t.Fatalf("recording path = %q, want basename stored-query.har", recPath)
	}
	har, err := probe.ValidateHAR(recPath)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}

	if got, want := srv.stored(), map[string]string{cassetteStoredQueryName: cassetteStoredQueryAQL}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the server holds stored queries %q, want %q", got, want)
	}
	calls, statuses := recordedCalls(t, har)
	wantCalls := []string{
		"POST /ehr",
		"PUT /definition/query/" + cassetteStoredQueryName,
		"POST /query/" + cassetteStoredQueryName,
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("stored-query recorded exchanges %q, want %q", calls, wantCalls)
	}
	if want := []int{http.StatusCreated, http.StatusOK, http.StatusOK}; !slices.Equal(statuses, want) {
		t.Fatalf("stored-query recorded statuses %v, want %v", statuses, want)
	}

	create, store, exec := har.Log.Entries[0], har.Log.Entries[1], har.Log.Entries[2]
	var ehrRec struct {
		EHRID struct {
			Value string `json:"value"`
		} `json:"ehr_id"`
	}
	if err := json.Unmarshal([]byte(create.Response.Content.Text), &ehrRec); err != nil || ehrRec.EHRID.Value == "" {
		t.Fatalf("recorded create response %s carries no ehr_id (err %v)", create.Response.Content.Text, err)
	}
	created := ehrRec.EHRID.Value

	wantLocation := sandboxBase + "/definition/query/" + cassetteStoredQueryName + "/1.0.0"
	if got := responseHeader(store, "Location"); got != wantLocation {
		t.Fatalf("recorded store response Location = %q, want %q", got, wantLocation)
	}

	if exec.Request.PostData == nil {
		t.Fatal("the recorded execution carries no request body, want the bound target_ehr")
	}
	var sent struct {
		QueryParameters map[string]any `json:"query_parameters"`
	}
	if err := json.Unmarshal([]byte(exec.Request.PostData.Text), &sent); err != nil {
		t.Fatalf("recorded execution body %s: %v", exec.Request.PostData.Text, err)
	}
	if got := sent.QueryParameters["target_ehr"]; got != created {
		t.Fatalf("recorded execution binds target_ehr = %v, want the EHR the capture created %q", got, created)
	}
	var rs struct {
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(exec.Response.Content.Text), &rs); err != nil {
		t.Fatalf("recorded execution response %s: %v", exec.Response.Content.Text, err)
	}
	if len(rs.Rows) != 1 || len(rs.Rows[0]) != 1 || rs.Rows[0][0] != created {
		t.Fatalf("recorded execution rows = %v, want one row holding the created EHR id %q", rs.Rows, created)
	}
}

// requestBody reads r's body. The sandbox hands a handler the client's own
// request, whose Body is nil when the client sent none (a server request's
// Body never is), so a nil Body reads as empty here.
func requestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(r.Body)
}

// recordedCalls lists har's exchanges in capture order, each as
// "METHOD /resource/path" with the /openehr/v1 base cut away, and the
// response status each one recorded.
func recordedCalls(t *testing.T, har probe.HAR) (calls []string, statuses []int) {
	t.Helper()
	for _, e := range har.Log.Entries {
		u, err := url.Parse(e.Request.URL)
		if err != nil {
			t.Fatalf("recorded URL %q: %v", e.Request.URL, err)
		}
		p, _ := strings.CutPrefix(u.Path, "/openehr/v1")
		calls = append(calls, e.Request.Method+" "+p)
		statuses = append(statuses, e.Response.Status)
	}
	return calls, statuses
}

// responseHeader returns the first value e's response recorded for name,
// matched without regard to case, or "" when there is none.
func responseHeader(e probe.HAREntry, name string) string {
	for _, h := range e.Response.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// sameJSON reports whether a and b decode to the same JSON value, so key
// order and spacing do not matter.
func sameJSON(a, b string) (bool, error) {
	var va, vb any
	if err := json.Unmarshal([]byte(a), &va); err != nil {
		return false, fmt.Errorf("decode %s: %w", a, err)
	}
	if err := json.Unmarshal([]byte(b), &vb); err != nil {
		return false, fmt.Errorf("decode %s: %w", b, err)
	}
	return reflect.DeepEqual(va, vb), nil
}
