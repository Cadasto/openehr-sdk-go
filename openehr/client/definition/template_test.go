package definition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readCassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", "definition", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestUploadTemplate(t *testing.T) {
	var captured *http.Request
	var capturedBody []byte
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/body_weight.v1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(readCassette(t, "template_metadata.json"))
	}))
	defer srv.Close()

	meta, transportMeta, err := definition.UploadTemplate(context.Background(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/definition/template/adl1.4" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if captured.Header.Get("Content-Type") != "application/xml" {
		t.Errorf("Content-Type = %q", captured.Header.Get("Content-Type"))
	}
	if !bytes.Equal(capturedBody, opt) {
		t.Error("upload body bytes mismatch")
	}
	if meta.TemplateID != "body_weight.v1" {
		t.Errorf("TemplateID = %q", meta.TemplateID)
	}
	if meta.ArchetypeID != "openEHR-EHR-COMPOSITION.encounter.v1" {
		t.Errorf("ArchetypeID = %q", meta.ArchetypeID)
	}
	// `uri` is a deployment-specific field; should land in Extras.
	if _, ok := meta.Extras["uri"]; !ok {
		t.Errorf("expected Extras[uri], have keys %v", maskKeys(meta.Extras))
	}
	if transportMeta == nil || transportMeta.Location == "" {
		t.Error("Location not captured")
	}
}

func TestUploadTemplateLocationFallback(t *testing.T) {
	// 204 response with no body but a Location header — surface a
	// minimal TemplateMetadata with TemplateID from the Location tail.
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/body_weight.v1")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	meta, _, err := definition.UploadTemplate(context.Background(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		t.Fatal(err)
	}
	if meta.TemplateID != "body_weight.v1" {
		t.Errorf("fallback TemplateID = %q (want body_weight.v1)", meta.TemplateID)
	}
}

func TestUploadRejectsInvalidInputs(t *testing.T) {
	_, _, err := definition.UploadTemplate(context.Background(), nil, "unknown", bytes.NewReader([]byte("x")))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("invalid format: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = definition.UploadTemplate(context.Background(), nil, definition.FormatADL14, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("nil body: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = definition.UploadTemplate(context.Background(), nil, definition.FormatADL14, bytes.NewReader(nil))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("empty body: expected ErrInvalidConfig, got %v", err)
	}
}

func TestUploadTemplateWithVersion(t *testing.T) {
	var captured *http.Request
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(readCassette(t, "template_metadata.json"))
	}))
	defer srv.Close()
	if _, _, err := definition.UploadTemplate(
		context.Background(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt),
		definition.WithUploadVersion("2"),
	); err != nil {
		t.Fatal(err)
	}
	if got := captured.URL.Query().Get("version"); got != "2" {
		t.Errorf("?version = %q, want 2", got)
	}
}

func TestGetTemplate(t *testing.T) {
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/xml" {
			t.Errorf("Accept = %q, want application/xml", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(opt)
	}))
	defer srv.Close()
	got, _, err := definition.GetTemplate(context.Background(), newClient(t, srv), "body_weight.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, opt) {
		t.Error("GET OPT bytes mismatch")
	}
}

func TestGetTemplateRejectsEmpty(t *testing.T) {
	_, _, err := definition.GetTemplate(context.Background(), nil, "", definition.FormatADL14)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestListTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "template_list.json"))
	}))
	defer srv.Close()
	list, _, err := definition.ListTemplates(context.Background(), newClient(t, srv), definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].TemplateID != "body_weight.v1" {
		t.Errorf("list[0].TemplateID = %q", list[0].TemplateID)
	}
	if list[1].TemplateID != "vital_signs.v1" {
		t.Errorf("list[1].TemplateID = %q", list[1].TemplateID)
	}
}

func TestListTemplatesEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	list, _, err := definition.ListTemplates(context.Background(), newClient(t, srv), definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list on 204, got %d items", len(list))
	}
}

func TestDeleteTemplate(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if _, err := definition.DeleteTemplate(context.Background(), newClient(t, srv), "body_weight.v1", definition.FormatADL14); err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodDelete {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/definition/template/adl1.4/body_weight.v1" {
		t.Errorf("path = %q", captured.URL.Path)
	}
}

func TestDeleteTemplateMethodNotAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"template delete disabled","code":"FORBIDDEN"}`))
	}))
	defer srv.Close()
	_, err := definition.DeleteTemplate(context.Background(), newClient(t, srv), "x", definition.FormatADL14)
	// Server returns 405 (no SDK sentinel for that — surfaces as WireError).
	var we *transport.WireError
	if !errors.As(err, &we) || we.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected WireError with 405, got %v", err)
	}
}

func TestExampleComposition(t *testing.T) {
	var captured *http.Request
	// Reuse the canonical-JSON body_weight cassette as the example.
	composPath := fixtures.CompositionJSON("body_weight")
	body, err := os.ReadFile(composPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	comp, _, err := definition.ExampleComposition(context.Background(), newClient(t, srv), "body_weight.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if comp == nil {
		t.Fatal("nil Composition")
	}
	if captured.URL.Path != "/openehr/v1/definition/template/adl1.4/body_weight.v1/example" {
		t.Errorf("path = %q", captured.URL.Path)
	}
}

func TestExampleCompositionWithParams(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNotImplemented) // we only assert the request shape here
	}))
	defer srv.Close()
	_, _, _ = definition.ExampleComposition(
		context.Background(), newClient(t, srv), "x", definition.FormatADL14,
		definition.WithExampleType(definition.ExampleTypeOutput),
		definition.WithExampleDetailLevel(definition.ExampleDetailComplete),
	)
	if got := captured.URL.Query().Get("type"); got != "output" {
		t.Errorf("?type = %q, want output", got)
	}
	if got := captured.URL.Query().Get("detail_level"); got != "complete" {
		t.Errorf("?detail_level = %q, want complete", got)
	}
	if captured.URL.Path != "/openehr/v1/definition/template/adl1.4/x/example" {
		t.Errorf("path = %q, want …/example", captured.URL.Path)
	}
}

func TestTemplateMetadataRoundTrip(t *testing.T) {
	body := readCassette(t, "template_metadata.json")
	var meta definition.TemplateMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped definition.TemplateMetadata
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if roundTripped.TemplateID != meta.TemplateID {
		t.Errorf("TemplateID drifted: %q vs %q", roundTripped.TemplateID, meta.TemplateID)
	}
	if len(roundTripped.Extras) != len(meta.Extras) {
		t.Errorf("Extras count drifted: %d vs %d", len(roundTripped.Extras), len(meta.Extras))
	}
}

func TestRepository(t *testing.T) {
	opt := readCassette(t, "body_weight.opt")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write(readCassette(t, "template_metadata.json"))
		case http.MethodGet:
			_, _ = w.Write(opt)
		}
	}))
	defer srv.Close()
	repo := definition.NewRepository(newClient(t, srv))
	if _, _, err := repo.UploadTemplate(context.Background(), definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		t.Fatal(err)
	}
	got, _, err := repo.GetTemplate(context.Background(), "body_weight.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, opt) {
		t.Error("Repository.GetTemplate bytes mismatch")
	}
}

func maskKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
