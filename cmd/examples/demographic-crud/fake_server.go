package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// The fake hands out fixed identifiers, so every run prints the same lines.
// A real server picks its own.
const (
	systemID = "sandbox.local"
	personID = "3e5d7a9c-4b1f-4e2a-8c6d-0f9b8a7e6d5c"
	ownerID  = "7d44b88c-4199-4bad-97dc-d78268e01398"
)

// fakeServer stands in for an openEHR demographic server. It keeps the
// versions of one PERSON and the item tags of its versioned party, and
// answers the routes the example calls with the status codes and headers of
// the openEHR REST API. It runs inside the *http.Client as its transport,
// through sandbox, so no listener is opened.
type fakeServer struct {
	base string // the REST base URL, for Location headers

	mu       sync.Mutex
	versions []*rm.Person // version n is versions[n-1]
	tags     []openehrclient.StoredItemTag
}

// httpClient returns an *http.Client whose transport is the fake. sandbox
// matches a route with or without the REST base in front, and a path ending
// in "/" matches that whole subtree. The first registered route that matches
// wins, so the tags route comes before the update route it extends. Anything
// else gets 404.
func (f *fakeServer) httpClient() *http.Client {
	b := sandbox.New()
	person := "/demographic/person"
	b.HandleFunc(http.MethodPost, person, f.write)
	b.HandleFunc(http.MethodPut, person+"/"+personID+"/tags", f.setTags)
	b.HandleFunc(http.MethodPut, person+"/"+personID, f.write)
	b.HandleFunc(http.MethodGet, person+"/", f.get)
	b.HandleFunc(http.MethodGet, "/demographic/tags", f.listTags)
	b.HandleFunc(http.MethodGet, "/demographic/versioned_party/"+personID+"/version/", f.getVersion)
	return b.HTTPClient()
}

func versionUID(n int) string {
	return personID + "::" + systemID + "::" + strconv.Itoa(n)
}

// write answers a create (POST, 201 Created) and an update (PUT, 204 No
// Content). ETag and Location name the new version, and there is no body:
// the example sends the default Prefer, return=minimal. An update whose
// If-Match does not name the latest version is refused with 412.
func (f *fakeServer) write(w http.ResponseWriter, r *http.Request) {
	person := new(rm.Person)
	body, err := io.ReadAll(r.Body)
	if err == nil {
		err = canjson.Unmarshal(body, person)
	}
	if err != nil || len(body) == 0 {
		http.Error(w, "the request carries no PERSON", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	status := http.StatusCreated
	if r.Method == http.MethodPut {
		status = http.StatusNoContent
		if ifMatch := strings.Trim(r.Header.Get("If-Match"), `"`); ifMatch != versionUID(len(f.versions)) {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
	} else if len(f.versions) != 0 {
		http.Error(w, "this fake keeps a single PERSON", http.StatusBadRequest)
		return
	}
	f.versions = append(f.versions, person)
	uid := versionUID(len(f.versions))
	w.Header().Set("ETag", `"`+uid+`"`)
	w.Header().Set("Location", f.base+"/demographic/person/"+uid)
	w.WriteHeader(status)
}

// get answers GET /demographic/person/{uid_based_id}, in canonical XML when
// Accept asks for it and in canonical JSON otherwise.
func (f *fakeServer) get(w http.ResponseWriter, r *http.Request) {
	_, ref, _ := strings.Cut(r.URL.Path, "/demographic/person/")
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.find(ref)
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", `"`+versionUID(n)+`"`)
	if r.Header.Get("Accept") == "application/xml" {
		body, err := canxml.Marshal(f.versions[n-1])
		send(w, "application/xml", body, err)
		return
	}
	body, err := canjson.Marshal(f.versions[n-1])
	send(w, "application/json", body, err)
}

// getVersion answers GET /demographic/versioned_party/{vo_uid}/version/{version_uid}
// with the ORIGINAL_VERSION that holds that version.
func (f *fakeServer) getVersion(w http.ResponseWriter, r *http.Request) {
	_, uid, _ := strings.Cut(r.URL.Path, "/version/")
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.find(uid)
	if n == 0 || uid == personID {
		http.NotFound(w, r)
		return
	}
	audit := &rm.AuditDetails{
		SystemID:      systemID,
		ChangeType:    openehrCode("249", "creation"),
		Committer:     &rm.PartyIdentified{Name: new("registration desk")},
		TimeCommitted: rm.DVDateTime{Value: time.Now().UTC().Format(time.RFC3339)},
	}
	var party rm.Party = f.versions[n-1]
	version := rm.OriginalVersion[rm.Party]{
		CommitAudit:    audit,
		UID:            rm.ObjectVersionID{Value: uid},
		LifecycleState: openehrCode("532", "complete"),
		Data:           &party,
	}
	if n > 1 {
		audit.ChangeType = openehrCode("251", "modification")
		version.PrecedingVersionUID = &rm.ObjectVersionID{Value: versionUID(n - 1)}
	}
	body, err := canjson.Marshal(version)
	send(w, "application/json", body, err)
}

// setTags answers PUT /demographic/person/{versioned_object_uid}/tags. The
// body replaces the whole list. With return=representation the answer is
// the stored list, where each tag names its target and its owner.
func (f *fakeServer) setTags(w http.ResponseWriter, r *http.Request) {
	var tags []openehrclient.StoredItemTag
	body, err := io.ReadAll(r.Body)
	if err == nil {
		err = canjson.Unmarshal(body, &tags)
	}
	if err != nil {
		http.Error(w, "the body is not a list of tags", http.StatusBadRequest)
		return
	}
	for i := range tags {
		tags[i].Target = rm.ObjectRef{ID: &rm.HierObjectID{Value: personID}, Namespace: "local", Type: "PERSON"}
		tags[i].OwnerID = rm.ObjectRef{ID: &rm.HierObjectID{Value: ownerID}, Namespace: "local", Type: "SYSTEM"}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags = tags
	if r.Header.Get("Prefer") != "return=representation" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err = canjson.Marshal(f.tags)
	send(w, "application/json", body, err)
}

// listTags answers GET /demographic/tags. Of the three filters it reads
// tag_key only, the one the example sends.
func (f *fakeServer) listTags(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("tag_key")
	f.mu.Lock()
	defer f.mu.Unlock()
	found := []openehrclient.StoredItemTag{}
	for _, tag := range f.tags {
		if key == "" || tag.Key == key {
			found = append(found, tag)
		}
	}
	body, err := canjson.Marshal(found)
	send(w, "application/json", body, err)
}

// find returns the number of the version ref names: the latest for the
// versioned object id, that version for a version uid, and 0 for none.
func (f *fakeServer) find(ref string) int {
	if ref == personID {
		return len(f.versions)
	}
	for n := range len(f.versions) {
		if versionUID(n+1) == ref {
			return n + 1
		}
	}
	return 0
}

// send writes a 200 response with body, or a 500 when encoding it failed.
func send(w http.ResponseWriter, contentType string, body []byte, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

// openehrCode is a coded text from the openEHR terminology.
func openehrCode(code, rubric string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        rubric,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
	}
}
