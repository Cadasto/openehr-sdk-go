package definition_test

// path_encoding_test.go — REQ-095. The transport is the single canonical path
// encoder (Request.Path is a decoded path), so a definition client MUST
// interpolate the RAW id and let the transport percent-encode it exactly once.
// Pre-escaping with url.PathEscape double-encodes any id with an encodable
// character (a space → `%20` → `%2520`), which the server unescapes to a literal
// `%20` and 404s. Spaced ids are real OPT id shapes in the openEHR corpus
// (e.g. "Referral Request.v1", "Weird Types 1").

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
)

// spacedTemplateID carries a space — a percent-encodable character that
// exercises the single-encode contract.
const spacedTemplateID = "Referral Request.v1"

const (
	wantSpacedDecoded = "/openehr/v1/definition/template/adl1.4/Referral Request.v1"
	wantSpacedEscaped = "/openehr/v1/definition/template/adl1.4/Referral%20Request.v1"
)

// captureServer records the decoded and wire-encoded request path.
func captureServer(t *testing.T, decoded, escaped *string, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*decoded = r.URL.Path
		*escaped = r.URL.EscapedPath()
		if status != 0 {
			w.WriteHeader(status)
		}
		if len(body) > 0 {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertSingleEncoded(t *testing.T, op, decoded, escaped string) {
	t.Helper()
	if decoded != wantSpacedDecoded {
		t.Errorf("%s: server-decoded path = %q, want %q (a double-encode leaks a literal %%20)", op, decoded, wantSpacedDecoded)
	}
	if escaped != wantSpacedEscaped {
		t.Errorf("%s: wire path = %q, want %q (single-encoded, not %%2520)", op, escaped, wantSpacedEscaped)
	}
}

func TestGetTemplatePathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := captureServer(t, &decoded, &escaped, 0, []byte("<template/>"))
	if _, _, err := definition.GetTemplate(t.Context(), newClient(t, srv), spacedTemplateID, definition.FormatADL14); err != nil {
		t.Fatal(err)
	}
	assertSingleEncoded(t, "GetTemplate", decoded, escaped)
}

func TestDeleteTemplatePathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := captureServer(t, &decoded, &escaped, http.StatusNoContent, nil)
	if _, err := definition.DeleteTemplate(t.Context(), newClient(t, srv), spacedTemplateID, definition.FormatADL14); err != nil {
		t.Fatal(err)
	}
	assertSingleEncoded(t, "DeleteTemplate", decoded, escaped)
}

func TestExampleCompositionPathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	// Return non-2xx: we only assert the request path shape here.
	srv := captureServer(t, &decoded, &escaped, http.StatusNotImplemented, nil)
	_, _, _ = definition.ExampleComposition(t.Context(), newClient(t, srv), spacedTemplateID, definition.FormatADL14)
	const wantDecoded = wantSpacedDecoded + "/example"
	const wantEscaped = wantSpacedEscaped + "/example"
	if decoded != wantDecoded {
		t.Errorf("ExampleComposition: server-decoded path = %q, want %q", decoded, wantDecoded)
	}
	if escaped != wantEscaped {
		t.Errorf("ExampleComposition: wire path = %q, want %q (single-encoded)", escaped, wantEscaped)
	}
}

// TestTemplateSpacedIDRoundTrip is the REQ-095 acceptance case: upload a
// template whose id contains a space, then GetTemplate it by that id and
// receive 200 with the body. A double-encoded GET requests a literal
// "Referral%20Request.v1" id, which the store never held → 404.
func TestTemplateSpacedIDRoundTrip(t *testing.T) {
	opt := []byte("<template><template_id><value>Referral Request.v1</value></template_id></template>")
	// Store keyed by the DECODED path, as a real server resolves an id.
	store := map[string][]byte{}
	uploadedPath := "/openehr/v1/definition/template/adl1.4/" + spacedTemplateID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost: // upload to the collection; assign the spaced id
			store[uploadedPath] = opt
			w.Header().Set("Location", uploadedPath)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"template_id":"Referral Request.v1"}`))
		case http.MethodGet:
			b, ok := store[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(b)
		}
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if _, _, err := definition.UploadTemplate(t.Context(), c, definition.FormatADL14, bytes.NewReader(opt)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	got, _, err := definition.GetTemplate(t.Context(), c, spacedTemplateID, definition.FormatADL14)
	if err != nil {
		t.Fatalf("GetTemplate by spaced id: %v (double-encoding would 404 here)", err)
	}
	if !bytes.Equal(got, opt) {
		t.Errorf("round-trip body mismatch: got %q", got)
	}
}

// TestStoredQueryPathSingleEncoded covers the stored-query helpers: a qualified
// name with an encodable character must reach the wire single-encoded too
// (latent today only because reverse-DNS names carry no encodable char).
func TestStoredQueryPathSingleEncoded(t *testing.T) {
	const name = "org.example.q with space"
	const wantGetEscaped = "/openehr/v1/definition/query/org.example.q%20with%20space/1.0.0"
	const wantGetDecoded = "/openehr/v1/definition/query/org.example.q with space/1.0.0"

	t.Run("GetStoredQuery", func(t *testing.T) {
		var decoded, escaped string
		srv := captureServer(t, &decoded, &escaped, 0, []byte(`{"name":"org.example.q with space","version":"1.0.0","q":"SELECT e FROM EHR e"}`))
		if _, _, err := definition.GetStoredQuery(t.Context(), newClient(t, srv), name, "1.0.0"); err != nil {
			t.Fatal(err)
		}
		if decoded != wantGetDecoded {
			t.Errorf("server-decoded path = %q, want %q", decoded, wantGetDecoded)
		}
		if escaped != wantGetEscaped {
			t.Errorf("wire path = %q, want %q (single-encoded)", escaped, wantGetEscaped)
		}
	})

	t.Run("PutStoredQueryVersion", func(t *testing.T) {
		var decoded, escaped string
		srv := captureServer(t, &decoded, &escaped, http.StatusOK, nil)
		if _, _, err := definition.PutStoredQueryVersion(t.Context(), newClient(t, srv), name, "1.0.0", "SELECT e FROM EHR e"); err != nil {
			t.Fatal(err)
		}
		if escaped != wantGetEscaped {
			t.Errorf("wire path = %q, want %q (single-encoded)", escaped, wantGetEscaped)
		}
	})
}
