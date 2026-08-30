package definition_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// jsonServerClient starts a test server that answers every request with
// body as JSON, and returns a client bound to it. Used by the decode
// tests, which care about the response body and nothing else (REQ-144).
func jsonServerClient(t *testing.T, body string) *transport.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newClient(t, srv)
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

// compactJSON is the fair comparison for a preserved Extras value. Extras
// holds the wire bytes exactly as decoded, but re-encoding runs them
// through encoding/json, which compacts insignificant whitespace — so a
// wire value spelled `["a", "b"]` comes back as `["a","b"]` with nothing
// lost. Comparing raw bytes would fail on the spacing alone.
func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("Compact(%s) = %v, want nil error", raw, err)
	}
	return buf.String()
}

// assertEmittedKeys checks that the encoded object carries each named key
// with the given JSON value. It reads the raw object rather than a decoded
// descriptor because that is the only way to see two keys differing only
// by case: decoding folds them onto one documented field.
func assertEmittedKeys(t *testing.T, out []byte, want map[string]string) {
	t.Helper()
	if len(want) == 0 {
		return
	}
	var emitted map[string]json.RawMessage
	if err := json.Unmarshal(out, &emitted); err != nil {
		t.Fatalf("Unmarshal(%s) into a raw object = %v, want nil error", out, err)
	}
	for k, w := range want {
		raw, ok := emitted[k]
		if !ok {
			t.Errorf("encode dropped key %q: %s", k, out)
			continue
		}
		if w := compactJSON(t, json.RawMessage(w)); string(raw) != w {
			t.Errorf("encoded[%q] = %s, want %s", k, raw, w)
		}
	}
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

	meta, transportMeta, err := definition.UploadTemplate(t.Context(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt))
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
		t.Errorf("expected Extras[uri], have keys %v", slices.Sorted(maps.Keys(meta.Extras)))
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
	meta, _, err := definition.UploadTemplate(t.Context(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt))
	if err != nil {
		t.Fatal(err)
	}
	if meta.TemplateID != "body_weight.v1" {
		t.Errorf("fallback TemplateID = %q (want body_weight.v1)", meta.TemplateID)
	}
}

func TestUploadRejectsInvalidInputs(t *testing.T) {
	_, _, err := definition.UploadTemplate(t.Context(), nil, "unknown", bytes.NewReader([]byte("x")))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("invalid format: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = definition.UploadTemplate(t.Context(), nil, definition.FormatADL14, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("nil body: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = definition.UploadTemplate(t.Context(), nil, definition.FormatADL14, bytes.NewReader(nil))
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
		t.Context(), newClient(t, srv), definition.FormatADL14, bytes.NewReader(opt),
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
	got, _, err := definition.GetTemplate(t.Context(), newClient(t, srv), "body_weight.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, opt) {
		t.Error("GET OPT bytes mismatch")
	}
}

func TestGetTemplateRejectsEmpty(t *testing.T) {
	_, _, err := definition.GetTemplate(t.Context(), nil, "", definition.FormatADL14)
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
	list, _, err := definition.ListTemplates(t.Context(), newClient(t, srv), definition.FormatADL14)
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

// TestListTemplatesEmpty pins the empty-BODY arm of REQ-144: an empty 2xx
// body yields a non-nil zero-length slice and a nil error. The nil check is
// the load-bearing one — a nil slice boxed in a non-nil interface marshals
// as JSON null, so a caller who re-serialises the result would publish null
// for "no templates". Restoring the bare `return nil` fails this test.
func TestListTemplatesEmpty(t *testing.T) {
	// REQ-144
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()
			list, _, err := definition.ListTemplates(t.Context(), newClient(t, srv), definition.FormatADL14)
			if err != nil {
				t.Fatal(err)
			}
			if list == nil {
				t.Errorf("list = nil on %d, want a non-nil empty slice (a nil slice marshals as JSON null)", status)
			}
			if len(list) != 0 {
				t.Errorf("expected empty list on %d, got %d items", status, len(list))
			}
		})
	}
}

func TestDeleteTemplate(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if _, err := definition.DeleteTemplate(t.Context(), newClient(t, srv), "body_weight.v1", definition.FormatADL14); err != nil {
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
	_, err := definition.DeleteTemplate(t.Context(), newClient(t, srv), "x", definition.FormatADL14)
	// Server returns 405 (no SDK sentinel for that — surfaces as WireError).
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we.StatusCode != http.StatusMethodNotAllowed {
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
	comp, _, err := definition.ExampleComposition(t.Context(), newClient(t, srv), "body_weight.v1", definition.FormatADL14)
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
		t.Context(), newClient(t, srv), "x", definition.FormatADL14,
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

func TestExampleCompositionRejectsInvalidParams(t *testing.T) {
	_, _, err := definition.ExampleComposition(t.Context(), nil, "x", definition.FormatADL14,
		definition.WithExampleType(definition.ExampleType("garbage")))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("invalid type: err = %v, want ErrInvalidConfig", err)
	}
	_, _, err = definition.ExampleComposition(t.Context(), nil, "x", definition.FormatADL14,
		definition.WithExampleDetailLevel(definition.ExampleDetailLevel("deep")))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("invalid detail_level: err = %v, want ErrInvalidConfig", err)
	}
}

// TestTemplateMetadataRoundTrip is the template twin of
// TestStoredQueryMetadataExtrasRoundTrip. Each row's invariant is the
// same: the documented fields a value carries before encode come back
// unchanged after a Marshal/Unmarshal round trip, and only the unknown
// keys ride along in Extras. The two collision rows add the caller-
// constructed case — an Extras key naming a documented field is ignored
// on encode — for an emitted key (`template_id`) and for an omitted one
// (`description` is omitempty and `created_timestamp` omitzero, so
// nothing is emitted for the overlay to win with). The last row pins the
// other edge of the rule: collision is decided by exact comparison, so a
// key differing from a documented name only by case is not one (REQ-144).
func TestTemplateMetadataRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		label string
		body  []byte
		// inject is applied to the decoded value before re-encoding;
		// UnmarshalJSON never routes a documented field name into Extras, so
		// a collision can only come from a caller.
		inject map[string]json.RawMessage
		// wantTimestampDecoded asserts the premise that `created_timestamp`
		// reaches CreatedOn rather than leaking into Extras.
		wantTimestampDecoded bool
		// caseVariant, when set, is a wire key differing from a documented
		// field name only by case. It must populate the documented field
		// (encoding/json matches field names case-insensitively) AND be kept
		// in Extras (the known-field set is matched exactly).
		caseVariant string
		// wantEmitted names keys the encoded object itself must carry, read
		// from the raw JSON — the only way to see two keys that differ only
		// by case, since decoding folds them onto one field.
		wantEmitted map[string]string
	}{
		{
			label:                "cassette",
			body:                 readCassette(t, "template_metadata.json"),
			wantTimestampDecoded: true,
		},
		{
			label: "colliding extra on an emitted field is ignored",
			body:  []byte(`{"template_id":"t.v1","uri":"/definition/template/adl1.4/t.v1"}`),
			inject: map[string]json.RawMessage{
				"template_id": json.RawMessage(`"caller-supplied id"`),
			},
		},
		{
			label: "colliding extra on an omitted field is ignored",
			// Only template_id is populated here, so description (omitempty)
			// and created_timestamp (omitzero) emit no key at all — the case
			// a last-wins merge cannot survive.
			body: []byte(`{"template_id":"t.v1","uri":"/definition/template/adl1.4/t.v1"}`),
			inject: map[string]json.RawMessage{
				"description":       json.RawMessage(`"caller-supplied description"`),
				"created_timestamp": json.RawMessage(`"16/07/2017"`),
			},
		},
		{
			label: "case-variant key rides beside the documented field",
			// "Template_ID" differs from the documented `template_id` only by
			// case, so it is not a collision: it populates TemplateID on decode
			// and is preserved in Extras, and encode emits both keys.
			// TemplateID is non-empty, so its omitempty key is emitted too.
			body:        []byte(`{"Template_ID":"t.v1","concept":"Body Weight"}`),
			caseVariant: "Template_ID",
			wantEmitted: map[string]string{"template_id": `"t.v1"`, "Template_ID": `"t.v1"`},
		},
	} {
		t.Run(tc.label, func(t *testing.T) {
			var meta definition.TemplateMetadata
			if err := json.Unmarshal(tc.body, &meta); err != nil {
				t.Fatalf("Unmarshal(wire) = %v, want nil error", err)
			}
			// Snapshot the decoded value before injection: it is the want for
			// every documented field and for the Extras key set. want.Extras
			// is cleared so nothing reads it through the alias the copy keeps
			// on the map injection is about to mutate.
			wantExtras := maps.Clone(meta.Extras)
			want := meta
			want.Extras = nil
			if tc.wantTimestampDecoded {
				// created_timestamp (the spec field) must decode into
				// CreatedOn, not silently land in Extras.
				if meta.CreatedOn.IsZero() {
					t.Errorf("CreatedOn not populated from created_timestamp: %+v", meta)
				}
				if _, leaked := meta.Extras["created_timestamp"]; leaked {
					t.Error("created_timestamp leaked into Extras instead of CreatedOn")
				}
			}
			if tc.caseVariant != "" {
				if meta.TemplateID != "t.v1" {
					t.Errorf("TemplateID = %q after decoding a body whose only template-id key is %q, want t.v1 — encoding/json matches field names case-insensitively",
						meta.TemplateID, tc.caseVariant)
				}
				if raw := meta.Extras[tc.caseVariant]; string(raw) != `"t.v1"` {
					t.Errorf(`Extras[%q] = %s, want "t.v1" preserved verbatim — the known-field set is matched exactly, so a case variant is not a documented name`,
						tc.caseVariant, raw)
				}
			}
			for k, v := range tc.inject {
				if meta.Extras == nil {
					meta.Extras = map[string]json.RawMessage{}
				}
				meta.Extras[k] = v
			}
			out, err := json.Marshal(meta)
			if err != nil {
				t.Fatalf("Marshal = %v, want nil error", err)
			}
			assertEmittedKeys(t, out, tc.wantEmitted)
			var got definition.TemplateMetadata
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("Unmarshal(re-encoded %s) = %v, want nil error — MarshalJSON emitted what this package cannot decode", out, err)
			}
			if got.TemplateID != want.TemplateID {
				t.Errorf("TemplateID = %q, want %q (the documented field is authoritative)", got.TemplateID, want.TemplateID)
			}
			if got.Concept != want.Concept {
				t.Errorf("Concept = %q, want %q", got.Concept, want.Concept)
			}
			if got.ArchetypeID != want.ArchetypeID {
				t.Errorf("ArchetypeID = %q, want %q", got.ArchetypeID, want.ArchetypeID)
			}
			if got.Version != want.Version {
				t.Errorf("Version = %q, want %q", got.Version, want.Version)
			}
			if got.Description != want.Description {
				t.Errorf("Description = %q, want %q (the documented field is authoritative)", got.Description, want.Description)
			}
			if !got.CreatedOn.Equal(want.CreatedOn) {
				t.Errorf("CreatedOn = %s, want %s", got.CreatedOn.Format(time.RFC3339Nano), want.CreatedOn.Format(time.RFC3339Nano))
			}
			if len(got.Extras) != len(wantExtras) {
				t.Errorf("Extras = %v, want exactly the %d unknown key(s) %v", got.Extras, len(wantExtras), wantExtras)
			}
			for k, wantRaw := range wantExtras {
				raw, ok := got.Extras[k]
				if !ok {
					t.Errorf("Extras[%q] dropped on re-encode: %s", k, out)
					continue
				}
				if want := compactJSON(t, wantRaw); string(raw) != want {
					t.Errorf("Extras[%q] = %s, want %s", k, raw, want)
				}
			}
		})
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
	if _, _, err := repo.UploadTemplate(t.Context(), definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		t.Fatal(err)
	}
	got, _, err := repo.GetTemplate(t.Context(), "body_weight.v1", definition.FormatADL14)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, opt) {
		t.Error("Repository.GetTemplate bytes mismatch")
	}
}

// TestListTemplatesFilters pins the ITS-REST list query parameters on the
// wire: each option emits its named key, unset options omit theirs, and an
// explicit zero offset/fetch is sent rather than dropped (REQ-143,
// PROBE-093).
func TestListTemplatesFilters(t *testing.T) {
	cases := []struct {
		name string
		opts []definition.ListOption
		want url.Values
	}{
		{
			name: "no options emits no query",
			want: url.Values{},
		},
		{
			name: "template id",
			opts: []definition.ListOption{definition.WithTemplateID("vital*")},
			want: url.Values{"template_id": {"vital*"}},
		},
		{
			name: "concept",
			opts: []definition.ListOption{definition.WithConcept("*signs*")},
			want: url.Values{"concept": {"*signs*"}},
		},
		{
			name: "version",
			opts: []definition.ListOption{definition.WithVersion("1.2.*")},
			want: url.Values{"version": {"1.2.*"}},
		},
		{
			name: "offset",
			opts: []definition.ListOption{definition.WithOffset(10)},
			want: url.Values{"offset": {"10"}},
		},
		{
			name: "fetch",
			opts: []definition.ListOption{definition.WithFetch(25)},
			want: url.Values{"fetch": {"25"}},
		},
		{
			name: "explicit zero offset and fetch are sent",
			opts: []definition.ListOption{definition.WithOffset(0), definition.WithFetch(0)},
			want: url.Values{"offset": {"0"}, "fetch": {"0"}},
		},
		{
			name: "all five combined",
			opts: []definition.ListOption{
				definition.WithTemplateID("vital*"),
				definition.WithConcept("*signs*"),
				definition.WithVersion("1.2.*"),
				definition.WithOffset(10),
				definition.WithFetch(25),
			},
			want: url.Values{
				"template_id": {"vital*"},
				"concept":     {"*signs*"},
				"version":     {"1.2.*"},
				"offset":      {"10"},
				"fetch":       {"25"},
			},
		},
		{
			name: "last option wins for a repeated key",
			opts: []definition.ListOption{definition.WithOffset(10), definition.WithOffset(20)},
			want: url.Values{"offset": {"20"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(readCassette(t, "template_list.json"))
			}))
			defer srv.Close()

			list, _, err := definition.ListTemplates(t.Context(), newClient(t, srv), definition.FormatADL14, tc.opts...)
			if err != nil {
				t.Fatalf("ListTemplates(%v) = %v, want nil error", tc.name, err)
			}
			if got := captured.URL.Query(); !maps.EqualFunc(got, tc.want, slices.Equal) {
				t.Errorf("query = %v, want %v", got, tc.want)
			}
			if captured.URL.Path != "/openehr/v1/definition/template/adl1.4" {
				t.Errorf("path = %q, want the unfiltered list path", captured.URL.Path)
			}
			// The decode is unchanged by filtering (REQ-143).
			if len(list) != 2 {
				t.Errorf("len(list) = %d, want 2", len(list))
			}
		})
	}
}

// TestListTemplatesRejectsNegativePaging pins the SDK floor: the pin gives
// offset/fetch no negative semantics, so a negative fails closed with no
// request rather than being forwarded (REQ-143, PROBE-093).
func TestListTemplatesRejectsNegativePaging(t *testing.T) {
	cases := []struct {
		name string
		opt  definition.ListOption
	}{
		{"negative offset", definition.WithOffset(-1)},
		{"negative fetch", definition.WithFetch(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
			}))
			defer srv.Close()

			_, _, err := definition.ListTemplates(t.Context(), newClient(t, srv), definition.FormatADL14, tc.opt)
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("err = %v, want ErrInvalidConfig", err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("issued %d requests, want 0", n)
			}
		})
	}
}

// TestRepositoryListTemplatesCarriesOptions pins that the DI seam reaches
// the same filters as the package function (REQ-143).
func TestRepositoryListTemplatesCarriesOptions(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "template_list.json"))
	}))
	defer srv.Close()

	repo := definition.NewRepository(newClient(t, srv))
	if _, _, err := repo.ListTemplates(t.Context(), definition.FormatADL14, definition.WithTemplateID("vital*")); err != nil {
		t.Fatal(err)
	}
	if got := captured.URL.Query().Get("template_id"); got != "vital*" {
		t.Errorf("template_id = %q, want %q", got, "vital*")
	}
}

// TestListTemplatesZoneLessTimestamp pins the zone-less arm of the
// accepted layout set: a `created_timestamp` carrying no zone indicator
// decodes as UTC — never as the client host's local zone, which would read
// one response as different instants on different machines — and the
// fractional second is absorbed by the seconds-bearing layout (REQ-144).
func TestListTemplatesZoneLessTimestamp(t *testing.T) {
	c := jsonServerClient(t, `[{"template_id":"t.v1","created_timestamp":"2022-03-30T07:18:13.591"}]`)

	list, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
	if err != nil {
		t.Fatalf("ListTemplates = %v, want nil error", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	want := time.Date(2022, 3, 30, 7, 18, 13, 591_000_000, time.UTC)
	if !list[0].CreatedOn.Equal(want) {
		t.Errorf("CreatedOn = %s, want %s", list[0].CreatedOn.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// TestListTemplatesMixedZoneCatalog pins that tolerance is per item: a
// catalog holding one zoned and one zone-less timestamp returns both
// entries, each at its own exact instant (REQ-144).
func TestListTemplatesMixedZoneCatalog(t *testing.T) {
	c := jsonServerClient(t, `[
		{"template_id":"zoned.v1","created_timestamp":"2026-05-17T12:00:00Z"},
		{"template_id":"zoneless.v1","created_timestamp":"2022-03-30T07:18:13.591"}
	]`)

	list, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
	if err != nil {
		t.Fatalf("ListTemplates = %v, want nil error", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	want := []struct {
		id string
		at time.Time
	}{
		{"zoned.v1", time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)},
		{"zoneless.v1", time.Date(2022, 3, 30, 7, 18, 13, 591_000_000, time.UTC)},
	}
	for i, w := range want {
		if list[i].TemplateID != w.id {
			t.Errorf("list[%d].TemplateID = %q, want %q", i, list[i].TemplateID, w.id)
		}
		if !list[i].CreatedOn.Equal(w.at) {
			t.Errorf("list[%d].CreatedOn = %s, want %s", i, list[i].CreatedOn.Format(time.RFC3339Nano), w.at.Format(time.RFC3339Nano))
		}
	}
}

// TestListTemplatesSpaceSeparatedTimestamp pins the deployment-interop
// layout: `2006-01-02 15:04:05` is neither ISO 8601 extended nor a
// pin-example form, and is accepted solely because deployments emit it
// (REQ-144).
func TestListTemplatesSpaceSeparatedTimestamp(t *testing.T) {
	c := jsonServerClient(t, `[{"template_id":"t.v1","created_timestamp":"2026-06-22 14:50:55"}]`)

	list, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
	if err != nil {
		t.Fatalf("ListTemplates = %v, want nil error", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	want := time.Date(2026, 6, 22, 14, 50, 55, 0, time.UTC)
	if !list[0].CreatedOn.Equal(want) {
		t.Errorf("CreatedOn = %s, want %s", list[0].CreatedOn.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// TestListTemplatesMinutePrecisionTimestamp pins the minute-precision
// layout and its boundary: `2006-01-02T15:04` carries no seconds element,
// so a value that does carry seconds is matched by a seconds-bearing
// layout instead — both spellings decode, neither is guessed at (REQ-144).
func TestListTemplatesMinutePrecisionTimestamp(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want time.Time
	}{
		{"minute precision", "2026-06-22T14:50", time.Date(2026, 6, 22, 14, 50, 0, 0, time.UTC)},
		{"seconds match a seconds-bearing layout", "2026-06-22T14:50:55", time.Date(2026, 6, 22, 14, 50, 55, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := jsonServerClient(t, `[{"template_id":"t.v1","created_timestamp":"`+tc.wire+`"}]`)

			list, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
			if err != nil {
				t.Fatalf("ListTemplates(%q) = %v, want nil error", tc.wire, err)
			}
			if len(list) != 1 {
				t.Fatalf("len(list) = %d, want 1", len(list))
			}
			if !list[0].CreatedOn.Equal(tc.want) {
				t.Errorf("CreatedOn = %s, want %s", list[0].CreatedOn.Format(time.RFC3339Nano), tc.want.Format(time.RFC3339Nano))
			}
		})
	}
}

// TestTemplateTimestampAbsentNullEmpty pins the absent/empty arm: an
// absent key, a JSON null, and an empty string each yield the zero
// time.Time with no error — the caller distinguishes the case with
// IsZero — and none of the three leaks the key into Extras (REQ-144).
func TestTemplateTimestampAbsentNullEmpty(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"key absent", `{"template_id":"t.v1"}`},
		{"json null", `{"template_id":"t.v1","created_timestamp":null}`},
		{"empty string", `{"template_id":"t.v1","created_timestamp":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var meta definition.TemplateMetadata
			if err := json.Unmarshal([]byte(tc.body), &meta); err != nil {
				t.Fatalf("Unmarshal(%s) = %v, want nil error", tc.body, err)
			}
			if !meta.CreatedOn.IsZero() {
				t.Errorf("CreatedOn = %s, want the zero time", meta.CreatedOn.Format(time.RFC3339Nano))
			}
			if meta.TemplateID != "t.v1" {
				t.Errorf("TemplateID = %q, want t.v1 (the rest of the item still decodes)", meta.TemplateID)
			}
			if _, leaked := meta.Extras["created_timestamp"]; leaked {
				t.Error("created_timestamp leaked into Extras")
			}
		})
	}
}

// TestListTemplatesUnparseableTimestampFails pins the refusal: a non-empty
// value matching no accepted layout fails the containing item's decode and
// therefore the list call, naming the field. Never a silent zero — that
// would present an instant the server never sent as though it had.
// Removing the failure guard fails this test (REQ-144).
func TestListTemplatesUnparseableTimestampFails(t *testing.T) {
	// Each value is refused for its own reason: obvious garbage, a
	// space-separated value at minute precision (the space layout carries
	// seconds), and a date with no time at all. The near misses are the
	// load-bearing ones — they pin that the layout set is closed, not just
	// that nonsense is rejected.
	cases := []struct {
		name string
		wire string
	}{
		{"obvious garbage", "not-a-time"},
		{"space separator at minute precision", "2026-06-22 14:50"},
		{"date only", "2026-06-22"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := jsonServerClient(t, `[{"template_id":"t.v1","created_timestamp":"`+tc.wire+`"}]`)

			list, _, err := definition.ListTemplates(t.Context(), c, definition.FormatADL14)
			if err == nil {
				t.Fatalf("ListTemplates(%q) = nil error with list %+v, want a decode failure", tc.wire, list)
			}
			if !strings.Contains(err.Error(), "created_timestamp") {
				t.Errorf("err = %q, want it to name created_timestamp", err)
			}
			if list != nil {
				t.Errorf("list = %+v, want nil on decode failure", list)
			}
		})
	}
}

// TestZoneLessTimestampRemarshalsAsUTC pins the stated round-trip
// asymmetry: encode stays single-valued RFC 3339, so a zone-less wire
// value re-marshals with a `Z` the wire never carried. The emitted form is
// a correct rendering of the decoded instant, not a transcription of the
// server's spelling (REQ-144).
func TestZoneLessTimestampRemarshalsAsUTC(t *testing.T) {
	var meta definition.TemplateMetadata
	if err := json.Unmarshal([]byte(`{"template_id":"t.v1","created_timestamp":"2022-03-30T07:18:13.591"}`), &meta); err != nil {
		t.Fatalf("Unmarshal = %v, want nil error", err)
	}
	out, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal = %v, want nil error", err)
	}
	const want = `"created_timestamp":"2022-03-30T07:18:13.591Z"`
	if !strings.Contains(string(out), want) {
		t.Errorf("re-marshalled to %s, want it to carry %s", out, want)
	}
}
