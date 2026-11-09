package definition

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestUploadTemplate_inputTooLarge(t *testing.T) { // REQ-108
	orig := maxUploadBytes
	maxUploadBytes = 16
	t.Cleanup(func() { maxUploadBytes = orig })

	// Feed 100 bytes — over the 16-byte cap.
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 100))
	_, _, err := UploadTemplate(t.Context(), nil, FormatADL14, body)
	if err == nil {
		t.Fatal("expected error when body exceeds maxUploadBytes")
	}
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("error should wrap ErrInvalidConfig, got: %v", err)
	}
	if !errors.Is(err, ErrUploadTooLarge) {
		t.Errorf("error should wrap ErrUploadTooLarge, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}

// TestUploadTemplateSizeLimitSeam pins that the limit tests lower starts at
// the exported one, so the boundary test below speaks for MaxUploadBytes.
func TestUploadTemplateSizeLimitSeam(t *testing.T) { // REQ-108
	if got, want := maxUploadBytes, int64(MaxUploadBytes); got != want {
		t.Errorf("maxUploadBytes = %d, want MaxUploadBytes (%d)", got, want)
	}
}

// TestUploadTemplateSizeLimit pins the boundary of the upload size limit: a
// body exactly at the limit reaches the server, and a body one byte over it is
// refused before any request is sent, with an error that matches both
// ErrUploadTooLarge and transport.ErrInvalidConfig.
func TestUploadTemplateSizeLimit(t *testing.T) { // REQ-108
	const limit = 16
	orig := maxUploadBytes
	maxUploadBytes = limit
	t.Cleanup(func() { maxUploadBytes = orig })

	cases := []struct {
		name    string
		size    int
		refused bool
	}{
		{name: "exactly at the limit", size: limit, refused: false},
		{name: "one byte over the limit", size: limit + 1, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests, received atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				n, _ := io.Copy(io.Discard, r.Body)
				received.Store(n)
				w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/capped.v1")
				w.WriteHeader(http.StatusCreated)
			}))
			t.Cleanup(srv.Close)

			body := bytes.NewReader(bytes.Repeat([]byte("x"), tc.size))
			meta, _, err := UploadTemplate(t.Context(), newCapClient(t, srv), FormatADL14, body)
			if !tc.refused {
				if err != nil {
					t.Fatalf("UploadTemplate(%d-byte body, limit %d) = %v, want nil error", tc.size, limit, err)
				}
				if got := requests.Load(); got != 1 {
					t.Errorf("server saw %d requests, want 1", got)
				}
				if got := received.Load(); got != int64(tc.size) {
					t.Errorf("server received %d body bytes, want %d", got, tc.size)
				}
				if meta == nil || meta.TemplateID != "capped.v1" {
					t.Errorf("UploadTemplate metadata = %+v, want TemplateID capped.v1", meta)
				}
				return
			}
			if !errors.Is(err, ErrUploadTooLarge) {
				t.Errorf("UploadTemplate(%d-byte body, limit %d) = %v, want an error matching ErrUploadTooLarge", tc.size, limit, err)
			}
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("UploadTemplate(%d-byte body, limit %d) = %v, want an error matching transport.ErrInvalidConfig", tc.size, limit, err)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("server saw %d requests, want 0: the body is refused before it is sent", got)
			}
		})
	}
}

// newCapClient returns a transport.Client aimed at srv.
func newCapClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
