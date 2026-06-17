package care

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

type fakePartyCodec struct{}

func (fakePartyCodec) ToParty(_ *template.OperationalTemplate, dm map[string]any) (map[string]any, error) {
	return dm, nil
}

func (fakePartyCodec) FromParty(_ *template.OperationalTemplate, party map[string]any) (map[string]any, error) {
	return party, nil
}

func partyTestClient(t *testing.T, srv *httptest.Server) *Client {
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
	rest, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{rest: rest, party: fakePartyCodec{}}
}

func demographicCassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "testkit", "cassettes", "its_rest", "demographic", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestSavePartyRaw(t *testing.T) {
	const version = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example.com::1"
	body := demographicCassette(t, "person.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openehr/v1/demographic/person" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("ETag", `"`+version+`"`)
		w.Header().Set("Location", "/demographic/person/"+version)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	uid, err := c.SavePartyRaw(context.Background(), mustPartyMap(t, body))
	if err != nil {
		t.Fatalf("SavePartyRaw: %v", err)
	}
	if uid != version {
		t.Errorf("version uid = %q, want %q", uid, version)
	}
}

func TestGetPartyRaw(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	body := demographicCassette(t, "person.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/openehr/v1/demographic/person/"+voID {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	got, err := c.GetPartyRaw(context.Background(), demographic.Person, voID)
	if err != nil {
		t.Fatalf("GetPartyRaw: %v", err)
	}
	if got["_type"] != "PERSON" {
		t.Errorf("_type = %v", got["_type"])
	}
}

func TestGetPartyRaw_fullVersionUID(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	const versionUID = voID + "::cdr.example.com::1"
	body := demographicCassette(t, "person.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cadasto-style: full version uid in path (first attempt).
		if r.Method != http.MethodGet || r.URL.Path != "/openehr/v1/demographic/person/"+versionUID {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	got, err := c.GetPartyRaw(context.Background(), demographic.Person, versionUID)
	if err != nil {
		t.Fatalf("GetPartyRaw: %v", err)
	}
	if got["_type"] != "PERSON" {
		t.Errorf("_type = %v", got["_type"])
	}
}

func TestGetPartyRaw_fullVersionUID_fallbackToVOID(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	const versionUID = voID + "::cdr.example.com::1"
	body := demographicCassette(t, "person.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/openehr/v1/demographic/person/"+versionUID {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/openehr/v1/demographic/person/"+voID {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	got, err := c.GetPartyRaw(context.Background(), demographic.Person, versionUID)
	if err != nil {
		t.Fatalf("GetPartyRaw: %v", err)
	}
	if got["_type"] != "PERSON" {
		t.Errorf("_type = %v", got["_type"])
	}
}

func TestGetPartyRaw_continuesAfter400(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	const versionUID = voID + "::cdr.example.com::1"
	body := demographicCassette(t, "person.json")
	envelope := []byte(`{"_type":"ORIGINAL_VERSION","data":` + string(body) + `}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if strings.HasPrefix(r.URL.Path, "/openehr/v1/demographic/person/") {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		specific := "/openehr/v1/demographic/versioned_party/" + voID + "/version/" + versionUID
		latest := "/openehr/v1/demographic/versioned_party/" + voID + "/version"
		if r.URL.Path == specific {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if r.URL.Path != latest {
			t.Errorf("unexpected path %s, want %s", r.URL.Path, latest)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(envelope)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	got, err := c.GetPartyRaw(context.Background(), demographic.Person, versionUID)
	if err != nil {
		t.Fatalf("GetPartyRaw: %v", err)
	}
	if got["_type"] != "PERSON" {
		t.Errorf("_type = %v", got["_type"])
	}
}

func TestGetPartyRaw_versionedPartyFallback(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	const versionUID = voID + "::cdr.example.com::1"
	body := demographicCassette(t, "person.json")
	envelope := []byte(`{"_type":"ORIGINAL_VERSION","data":` + string(body) + `}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if strings.HasPrefix(r.URL.Path, "/openehr/v1/demographic/person/") {
			http.NotFound(w, r)
			return
		}
		specific := "/openehr/v1/demographic/versioned_party/" + voID + "/version/" + versionUID
		latest := "/openehr/v1/demographic/versioned_party/" + voID + "/version"
		if r.URL.Path == specific {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != latest {
			t.Errorf("unexpected path %s, want %s", r.URL.Path, latest)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(envelope)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	got, err := c.GetPartyRaw(context.Background(), demographic.Person, versionUID)
	if err != nil {
		t.Fatalf("GetPartyRaw: %v", err)
	}
	if got["_type"] != "PERSON" {
		t.Errorf("_type = %v", got["_type"])
	}
}

func TestPartyETag(t *testing.T) {
	const voID = "8849182c-82ad-4088-a07f-48ead4180515"
	const version = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example.com::1"
	body := demographicCassette(t, "person.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"`+version+`"`)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := partyTestClient(t, srv)
	etag, err := c.PartyETag(context.Background(), demographic.Person, voID)
	if err != nil {
		t.Fatalf("PartyETag: %v", err)
	}
	if etag != version {
		t.Errorf("etag = %q, want %q", etag, version)
	}
}

func mustPartyMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
