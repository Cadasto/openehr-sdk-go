package versionedprobes_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/versioned"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	ehrIDFixture    openehrclient.EHRID             = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"
	compositionVOID openehrclient.VersionedObjectID = "1234abcd-5678-9012-3456-7890abcdef00"
	initialVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::1"
	updatedVUID     openehrclient.VersionUID        = "1234abcd-5678-9012-3456-7890abcdef00::cdr.example::2"
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

func TestProbe010PutWithoutIfMatch(t *testing.T) {
	// PROBE-010 is a compile-time guard exercise — no network needed.
	// Construct a client against any throwaway server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("PROBE-010 must short-circuit before any network call")
	}))
	defer srv.Close()
	r, err := probes.Probe010PutWithoutIfMatch(context.Background(), newClient(t, srv), ehrIDFixture)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-010 status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_412(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"message":"stale","code":"PRECONDITION_FAILED"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (412) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe011PutStaleIfMatch_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"stale","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe011PutStaleIfMatch(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, "stale", &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-011 (409) status = %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe012ETagRoundTrip(t *testing.T) {
	// Phase 1 of the probe: a GET returns the initial VersionUID via
	// Location; phase 2: PUT carries that as If-Match and the server
	// returns a fresh VersionUID. The fake server below alternates
	// between the two phases on a single shared state.
	var phase int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		phase++
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(initialVUID))
			w.Header().Set("ETag", `"`+string(initialVUID)+`"`)
			// Body must be a minimal valid composition for canjson decode.
			_, _ = w.Write([]byte(`{"_type":"COMPOSITION","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"openEHR-EHR-COMPOSITION.x.v1","language":{"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},"territory":{"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}}`))
		case http.MethodPut:
			ifMatch := r.Header.Get("If-Match")
			if ifMatch != `"`+string(initialVUID)+`"` {
				t.Errorf("PUT If-Match = %q, want %q", ifMatch, `"`+string(initialVUID)+`"`)
			}
			w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(updatedVUID))
			w.Header().Set("ETag", `"`+string(updatedVUID)+`"`)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe012ETagRoundTrip(context.Background(), newClient(t, srv), ehrIDFixture, compositionVOID, &rm.Composition{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-012 status = %q (detail: %s)", r.Status, r.Detail)
	}
	if phase < 2 {
		t.Errorf("expected at least 2 server hits (GET + PUT), got %d", phase)
	}
}
