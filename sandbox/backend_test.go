package sandbox_test

import (
	"net/http"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func sandboxClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://sandbox.local",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(b.HTTPClient()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreateAndGetEHR(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	c := sandboxClient(t, b)

	created, _, err := openehrclient.Create(t.Context(), c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created == nil || created.EHRID.Value == "" {
		t.Fatal("Create returned no ehr_id")
	}

	got, _, err := openehrclient.Get(t.Context(), c, openehrclient.EHRID(created.EHRID.Value))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.EHRID.Value != created.EHRID.Value {
		t.Fatalf("Get ehr_id = %q, want %q", got.EHRID.Value, created.EHRID.Value)
	}

	ok, err := openehrclient.Exists(t.Context(), c, openehrclient.EHRID(created.EHRID.Value))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Exists = false after Create")
	}
}

func TestIsolation(t *testing.T) {
	t.Parallel()
	a := sandbox.New()
	b := sandbox.New()
	ca := sandboxClient(t, a)
	cb := sandboxClient(t, b)

	created, _, err := openehrclient.Create(t.Context(), ca)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := openehrclient.Exists(t.Context(), cb, openehrclient.EHRID(created.EHRID.Value))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("second sandbox observed the first sandbox's EHR")
	}
}

func TestNoListener(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	// A RoundTripper-backed client must not bind a port: the Transport
	// is the Backend itself, not something wrapping a real listener.
	if b.HTTPClient().Transport != b {
		t.Fatal("HTTPClient.Transport is not the Backend itself")
	}
}

func TestScriptedRouteOverridesBuiltin(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	b.HandleFunc(http.MethodGet, "/ehr/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[1, 2, 3]`))
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://sandbox.local/openehr/v1/ehr/missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 from the scripted route (not the builtin 404)", resp.StatusCode)
	}
}

func TestScriptedCatchAll(t *testing.T) {
	t.Parallel()
	hits := 0
	b := sandbox.Scripted(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, "https://sandbox.local/openehr/v1/ehr/x/composition/y", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}
