package demographicprobes_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	demographicprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/demographic"
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

func cassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "cassettes", "its_rest", "demographic", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

// partyEchoServer serves the PARTY body for typed reads/writes and wraps it in
// an ORIGINAL_VERSION envelope on the version sub-path, so a single fixture
// drives the whole create → get → get-version round-trip the probe asserts.
func partyEchoServer(body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+string(demographicprobes073VOID)+`::cdr::1"`)
		w.Header().Set("Location", r.URL.Path)
		if strings.Contains(r.URL.Path, "/version") {
			env := fmt.Sprintf(
				`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"%s::cdr::1"},"data":%s}`,
				demographicprobes073VOID, body,
			)
			_, _ = w.Write([]byte(env))
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write(body)
	}))
}

// demographicprobes073VOID mirrors the probe's fixed versioned-object id for
// the version-envelope uid the fake server emits.
const demographicprobes073VOID = "demographic-probe-vo-1"

func TestProbe073DemographicRoundTrip(t *testing.T) {
	cases := []struct {
		party    rm.Party
		typ      demographic.Type
		cassette string
	}{
		{&rm.Person{Name: rm.DVText{Value: "Jane Doe"}}, demographic.Person, "person.json"},
		{&rm.Organisation{Name: rm.DVText{Value: "Acme Hospital"}}, demographic.Organisation, "organisation.json"},
		{&rm.Group{Name: rm.DVText{Value: "Cardiology Team"}}, demographic.Group, "group.json"},
		{&rm.Agent{Name: rm.DVText{Value: "Triage Bot"}}, demographic.Agent, "agent.json"},
		{&rm.Role{Name: rm.DVText{Value: "Attending Physician"}}, demographic.Role, "role.json"},
	}
	for _, tc := range cases {
		t.Run(string(tc.typ), func(t *testing.T) {
			srv := partyEchoServer(cassette(t, tc.cassette))
			defer srv.Close()

			r, err := demographicprobes.Probe073DemographicRoundTrip(
				context.Background(), newClient(t, srv), tc.party, tc.typ,
			)
			if err != nil {
				t.Fatalf("Probe073: %v", err)
			}
			if r.Status != "pass" {
				t.Fatalf("Probe073 status=%q detail=%q", r.Status, r.Detail)
			}
			if r.Probe != "PROBE-073" {
				t.Errorf("probe id = %q, want PROBE-073", r.Probe)
			}
		})
	}
}

// TestProbe073DetectsTypeDrift confirms the probe fails when the wire body's
// _type does not round-trip to the input party's concrete type.
func TestProbe073DetectsTypeDrift(t *testing.T) {
	// Server returns a PERSON body, but the probe is told to expect an ORGANISATION.
	srv := partyEchoServer(cassette(t, "person.json"))
	defer srv.Close()

	r, err := demographicprobes.Probe073DemographicRoundTrip(
		context.Background(), newClient(t, srv),
		&rm.Organisation{Name: rm.DVText{Value: "x"}}, demographic.Organisation,
	)
	if err != nil {
		t.Fatalf("Probe073: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on type drift, got status=%q", r.Status)
	}
}
