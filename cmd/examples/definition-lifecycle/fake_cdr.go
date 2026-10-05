package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// templateRoute is the ADL 1.4 template collection, relative to the REST base.
const templateRoute = "/definition/template/adl1.4"

// createdTimestamp is the upload time the fake reports for every template, so
// the listing is identical on every run.
const createdTimestamp = "2026-01-01T00:00:00Z"

// templateMetadata is one entry of the template listing. The xml tags pick
// its fields out of an uploaded OPT; the json tags are the listing's wire
// names.
type templateMetadata struct {
	TemplateID       string `xml:"template_id>value" json:"template_id"`
	Concept          string `xml:"concept" json:"concept"`
	ArchetypeID      string `xml:"definition>archetype_id>value" json:"archetype_id"`
	CreatedTimestamp string `xml:"-" json:"created_timestamp"`
}

// storedTemplate is what the fake keeps for one uploaded OPT.
type storedTemplate struct {
	meta templateMetadata
	opt  []byte
}

// fakeCDR stands in for a clinical data repository. It answers only the four
// ADL 1.4 template routes the example calls and keeps every uploaded OPT in
// memory. It is safe for concurrent use.
type fakeCDR struct {
	example []byte // canonical JSON composition served as every example

	mu        sync.Mutex
	templates map[string]storedTemplate
}

// newFakeCDR returns an empty fake that serves the vendored body_weight
// composition as its example.
func newFakeCDR() (*fakeCDR, error) {
	example, err := os.ReadFile(fixtures.CompositionJSON(fixtureID))
	if err != nil {
		return nil, fmt.Errorf("read example composition: %w", err)
	}
	return &fakeCDR{example: example, templates: map[string]storedTemplate{}}, nil
}

// backend registers the fake's routes on a sandbox backend, an in-process
// http.RoundTripper. A path ending in "/" matches the whole subtree under it.
func (f *fakeCDR) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, templateRoute, f.upload)
	b.HandleFunc(http.MethodGet, templateRoute, f.list)
	b.HandleFunc(http.MethodGet, templateRoute+"/", f.get)
	return b
}

// upload answers POST /definition/template/adl1.4. UploadTemplate sends no
// Prefer header, so the server's return=minimal default applies: 201 Created,
// a Location naming the new template, and an empty body. A server may also
// send a JSON body with the template_id; UploadTemplate accepts both.
func (f *fakeCDR) upload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read the request body", http.StatusBadRequest)
		return
	}
	meta := templateMetadata{CreatedTimestamp: createdTimestamp}
	if err := xml.Unmarshal(body, &meta); err != nil || meta.TemplateID == "" {
		http.Error(w, "the body is not an OPT with a template_id", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.templates[meta.TemplateID]; exists {
		http.Error(w, "a template with this template_id already exists", http.StatusConflict)
		return
	}
	f.templates[meta.TemplateID] = storedTemplate{meta: meta, opt: body}
	w.Header().Set("Location", baseURL+templateRoute+"/"+url.PathEscape(meta.TemplateID))
	w.WriteHeader(http.StatusCreated)
}

// list answers GET /definition/template/adl1.4 with the metadata of every
// stored template, sorted by id. A real server reads the template_id query
// parameter as a pattern that may hold * wildcards; the fake matches it
// exactly.
func (f *fakeCDR) list(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("template_id")
	out := []templateMetadata{}
	f.mu.Lock()
	for _, id := range slices.Sorted(maps.Keys(f.templates)) {
		if filter != "" && id != filter {
			continue
		}
		out = append(out, f.templates[id].meta)
	}
	f.mu.Unlock()
	body, err := json.Marshal(out)
	if err != nil {
		http.Error(w, "cannot encode the listing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// A failed write surfaces on the client side as a decode error.
	_, _ = w.Write(body)
}

// get answers the two routes under one template:
// GET /definition/template/adl1.4/{template_id} returns the stored OPT, and
// GET /definition/template/adl1.4/{template_id}/example returns an example
// composition of it.
func (f *fakeCDR) get(w http.ResponseWriter, r *http.Request) {
	_, rest, _ := strings.Cut(r.URL.Path, templateRoute+"/")
	id, isExample := strings.CutSuffix(rest, "/example")
	f.mu.Lock()
	stored, ok := f.templates[id]
	f.mu.Unlock()
	if !ok {
		http.Error(w, "unknown template_id", http.StatusNotFound)
		return
	}
	if isExample {
		// A real server generates the example from the template and honours
		// the type and detail_level query parameters. The fake ignores both
		// and returns a composition recorded against the same template.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.example)
		return
	}
	// GetTemplate asks for application/xml, the OPT itself. A server may also
	// serve the Web Template at this path, as application/openehr.wt+json.
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(stored.opt)
}
