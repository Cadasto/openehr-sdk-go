package definition_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
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
