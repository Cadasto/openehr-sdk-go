package definition_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The Definition leaves answer an empty 2xx body with a synthesized value —
// an empty catalog for the list operations (§ REQ-144), a metadata record
// rebuilt from the request or the Location header for the upload and
// stored-query surfaces (§ REQ-151 keyed exclusions). "Empty" includes the
// JSON `null` literal: without the raw-bytes check, `null` unmarshals to a
// nil slice (published as JSON null by a caller re-serialising the result) or
// to an all-zero metadata struct with a nil error. Each test below fails on a
// len(body) == 0 guard.

func TestListTemplatesNullBodyIsEmptyCatalog(t *testing.T) { // REQ-144, REQ-151
	list, _, err := definition.ListTemplates(t.Context(), jsonServerClient(t, "null"), definition.FormatADL14)
	if err != nil {
		t.Fatalf("ListTemplates(null body) = %v, want nil error", err)
	}
	if list == nil {
		t.Fatal("ListTemplates(null body) = nil slice, want a non-nil empty slice (a nil slice marshals as JSON null)")
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

func TestListStoredQueriesNullBodyIsEmptyCatalog(t *testing.T) { // REQ-144, REQ-151
	list, _, err := definition.ListStoredQueries(t.Context(), jsonServerClient(t, "\nnull\n"), "")
	if err != nil {
		t.Fatalf("ListStoredQueries(null body) = %v, want nil error", err)
	}
	if list == nil {
		t.Fatal("ListStoredQueries(null body) = nil slice, want a non-nil empty slice")
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}

func TestGetStoredQueryNullBodySynthesizesMetadata(t *testing.T) { // REQ-151 keyed exclusion (REQ-057)
	const wantName, wantVersion = "org.openehr::vitals", "1.0.0"
	meta, _, err := definition.GetStoredQuery(t.Context(), jsonServerClient(t, "null"), wantName, wantVersion)
	if err != nil {
		t.Fatalf("GetStoredQuery(null body) = %v, want nil error", err)
	}
	if meta == nil || meta.Name != wantName || meta.Version != wantVersion {
		t.Fatalf("GetStoredQuery(null body) = %+v, want synthesized {Name:%q Version:%q}", meta, wantName, wantVersion)
	}
}

func TestUploadTemplateNullBodyFallsBackToLocation(t *testing.T) { // REQ-151 keyed exclusion
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/body_weight.v1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	meta, _, err := definition.UploadTemplate(t.Context(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		t.Fatalf("UploadTemplate(null body) = %v, want nil error", err)
	}
	if meta == nil || meta.TemplateID != "body_weight.v1" {
		t.Fatalf("UploadTemplate(null body) = %+v, want TemplateID body_weight.v1 from the Location header", meta)
	}
}

// TestPutStoredQueryNullBodySynthesizesFromArguments is the can-fail control
// for the two stored-query PUT arms § REQ-151 names as synthesized-metadata
// surfaces. Both public entry points share putStoredQuery, so one table covers
// them; each sends a Location the parser must reject (one path segment, where
// two are required) so the arm falls through to the body classification.
//
// It fails on a len(body) == 0 guard: json.Unmarshal("null", &StoredQueryMetadata)
// returns a nil error and an all-zero record, so the caller would get an empty
// Name, Version and Q instead of the values it passed in.
func TestPutStoredQueryNullBodySynthesizesFromArguments(t *testing.T) { // REQ-151 keyed exclusion
	const aql = "SELECT c/uid/value FROM COMPOSITION c"
	for _, tc := range []struct {
		name        string
		call        func(t *testing.T, c *transport.Client) (*definition.StoredQueryMetadata, error)
		wantName    string
		wantVersion string
	}{
		{
			name: "PutStoredQuery",
			call: func(t *testing.T, c *transport.Client) (*definition.StoredQueryMetadata, error) {
				m, _, err := definition.PutStoredQuery(t.Context(), c, "org.openehr::vitals", aql)
				return m, err
			},
			wantName: "org.openehr::vitals",
			// The no-version entry point passes "" through; the synthesized
			// fallback must report that, not invent a version.
			wantVersion: "",
		},
		{
			name: "PutStoredQueryVersion",
			call: func(t *testing.T, c *transport.Client) (*definition.StoredQueryMetadata, error) {
				m, _, err := definition.PutStoredQueryVersion(t.Context(), c, "org.openehr::vitals", "1.2.3", aql)
				return m, err
			},
			wantName:    "org.openehr::vitals",
			wantVersion: "1.2.3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// One trailing segment only: parseStoredQueryLocation needs two.
				w.Header().Set("Location", "https://example.org/something")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("null"))
			}))
			defer srv.Close()

			meta, err := tc.call(t, newClient(t, srv))
			if err != nil {
				t.Fatalf("%s(null body) = %v, want nil error", tc.name, err)
			}
			if meta == nil {
				t.Fatalf("%s(null body) = nil metadata, want the synthesized record", tc.name)
			}
			if meta.Name != tc.wantName || meta.Version != tc.wantVersion {
				t.Errorf("%s(null body) = {Name:%q Version:%q}, want {Name:%q Version:%q} synthesized from the call arguments",
					tc.name, meta.Name, meta.Version, tc.wantName, tc.wantVersion)
			}
			if meta.Q != aql {
				t.Errorf("%s(null body) Q = %q, want the submitted AQL %q", tc.name, meta.Q, aql)
			}
		})
	}
}
