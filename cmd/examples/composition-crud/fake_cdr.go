package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// The fake hands out fixed identifiers, so every run prints the same lines.
// A real CDR picks its own.
const (
	systemID      = "sandbox.local"
	compositionID = "8f14e45f-ceea-467a-9575-4e1a8b2c3d4e"
)

// fakeCDR stands in for a clinical data repository (CDR). It keeps the
// versions of one composition in one EHR and answers the three composition
// routes the example calls, with the status codes and headers of the openEHR
// REST API. It runs inside the *http.Client as its transport, through
// sandbox, so no listener is opened.
type fakeCDR struct {
	base  string // the REST base URL, for Location headers
	ehrID string

	mu       sync.Mutex
	versions map[string][]byte // canonical JSON, keyed by version uid
	latest   int               // the latest version number; 0 before the first save
}

func newFakeCDR(base, ehrID string) *fakeCDR {
	return &fakeCDR{base: base, ehrID: ehrID, versions: make(map[string][]byte)}
}

// httpClient returns an *http.Client whose transport is the fake. sandbox
// matches a route with or without the REST base in front, so "/ehr/..." here
// answers "https://sandbox.local/openehr/v1/ehr/...". A path ending in "/"
// matches that whole subtree. Besides the routes the fake registers here, the
// sandbox's built-in EHR routes still answer: POST /ehr, PUT /ehr/{id}, and
// GET and HEAD /ehr/{id}. Anything else gets 404.
func (f *fakeCDR) httpClient() *http.Client {
	b := sandbox.New()
	route := "/ehr/" + f.ehrID + "/composition"
	b.HandleFunc(http.MethodPost, route, f.create)
	b.HandleFunc(http.MethodGet, route+"/", f.get)
	b.HandleFunc(http.MethodPut, route+"/"+compositionID, f.update)
	return b.HTTPClient()
}

func versionUID(n int) string {
	return compositionID + "::" + systemID + "::" + strconv.Itoa(n)
}

// create answers POST /ehr/{ehr_id}/composition with 201 Created, an ETag
// holding the new version uid, and a Location for that version.
func (f *fakeCDR) create(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "the request carries no composition", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latest != 0 {
		http.Error(w, "this fake keeps a single composition", http.StatusBadRequest)
		return
	}
	f.latest = 1
	uid := versionUID(f.latest)
	f.versions[uid] = body
	f.answerWrite(w, r, http.StatusCreated, uid, body)
}

// get answers GET /ehr/{ehr_id}/composition/{uid_based_id}. A versioned
// object id names the latest version, and a version uid names that version.
// The ETag holds the version uid of what is returned.
func (f *fakeCDR) get(w http.ResponseWriter, r *http.Request) {
	_, ref, _ := strings.Cut(r.URL.Path, "/composition/")
	f.mu.Lock()
	defer f.mu.Unlock()
	uid := ref
	if ref == compositionID {
		uid = versionUID(f.latest)
	}
	body, ok := f.versions[uid]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", `"`+uid+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// update answers PUT /ehr/{ehr_id}/composition/{versioned_object_uid}.
// If-Match names the version the client last read. When that is not the
// latest version, the update is refused with 412 Precondition Failed and the
// ETag names the latest version, so the client can read it and try again.
func (f *fakeCDR) update(w http.ResponseWriter, r *http.Request) {
	ifMatch := strings.Trim(r.Header.Get("If-Match"), `"`)
	if ifMatch == "" {
		http.Error(w, "If-Match is required", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "the request carries no composition", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latest == 0 {
		http.NotFound(w, r)
		return
	}
	if current := versionUID(f.latest); ifMatch != current {
		w.Header().Set("ETag", `"`+current+`"`)
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	f.latest++
	uid := versionUID(f.latest)
	f.versions[uid] = body
	f.answerWrite(w, r, http.StatusOK, uid, body)
}

// answerWrite sends the answer to a successful write. ETag and Location name
// the new version. The body follows the request's Prefer: the composition for
// return=representation, an Identifier for return=identifier, and nothing for
// return=minimal or no Prefer at all. An update with no body answers
// 204 No Content instead of 200 OK.
func (f *fakeCDR) answerWrite(w http.ResponseWriter, r *http.Request, status int, uid string, comp []byte) {
	w.Header().Set("ETag", `"`+uid+`"`)
	w.Header().Set("Location", f.base+"/ehr/"+f.ehrID+"/composition/"+uid)
	var body []byte
	switch r.Header.Get("Prefer") {
	case "return=representation":
		body = comp
	case "return=identifier":
		body = []byte(`{"uid":"` + uid + `"}`)
	}
	if body == nil {
		if status == http.StatusOK {
			status = http.StatusNoContent
		}
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
