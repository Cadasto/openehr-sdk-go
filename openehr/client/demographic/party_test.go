package demographic_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	personVOID    = "8849182c-82ad-4088-a07f-48ead4180515"
	personVersion = "8849182c-82ad-4088-a07f-48ead4180515::cdr.example.com::1"
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

func personCassette(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", "demographic", "person.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

// assertPerson asserts the decoded Party is a *rm.Person named "Jane Doe".
func assertPerson(t *testing.T, p rm.Party) {
	t.Helper()
	person, ok := p.(*rm.Person)
	if !ok {
		t.Fatalf("decoded Party is %T, want *rm.Person", p)
	}
	if person.Name == nil || person.Name.GetValue() != "Jane Doe" {
		t.Errorf("person name = %v, want Jane Doe", person.Name)
	}
}

func TestGet(t *testing.T) {
	var captured *http.Request
	body := personCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := demographic.Get(context.Background(), newClient(t, srv),
		demographic.Person, openehrclient.LatestOf(personVOID))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %s", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/demographic/person/"+personVOID {
		t.Errorf("path = %q", captured.URL.Path)
	}
	assertPerson(t, got)
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

func TestCreateRoutesByConcreteType(t *testing.T) {
	var captured *http.Request
	body := personCassette(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	person := &rm.Person{Name: rm.DVText{Value: "Jane Doe"}}
	got, meta, err := demographic.Create(context.Background(), newClient(t, srv), person,
		demographic.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %s", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/demographic/person" {
		t.Errorf("path = %q, want .../demographic/person", captured.URL.Path)
	}
	assertPerson(t, got) // Prefer=representation → body decoded
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

func TestCreateMinimalNoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	got, meta, err := demographic.Create(context.Background(), newClient(t, srv),
		&rm.Organisation{Name: rm.DVText{Value: "Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("minimal Create returned a body: %T", got)
	}
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

func TestCreateUnsupportedType(t *testing.T) {
	// A non-PARTY rm value must be rejected before any request.
	_, _, err := demographic.Create(context.Background(), nil, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("nil Party: err = %v, want ErrInvalidConfig", err)
	}
}

func TestUpdateRoutesAndSendsIfMatch(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, _, err := demographic.Update(context.Background(), newClient(t, srv),
		demographic.Person, personVOID, personVersion, &rm.Person{Name: rm.DVText{Value: "Jane Roe"}})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %s", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/demographic/person/"+personVOID {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.Header.Get("If-Match"); got != `"`+personVersion+`"` {
		t.Errorf("If-Match = %q", got)
	}
}

func TestUpdateRequiresIfMatch(t *testing.T) {
	_, _, err := demographic.Update(context.Background(), nil,
		demographic.Person, personVOID, "", &rm.Person{})
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("empty If-Match: err = %v, want ErrInvalidConfig", err)
	}
}

func TestDeleteRoutesAndSendsIfMatch(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, err := demographic.Delete(context.Background(), newClient(t, srv),
		demographic.Person, personVersion, personVersion)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodDelete {
		t.Errorf("method = %s", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/demographic/person/"+personVersion {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.Header.Get("If-Match"); got != `"`+personVersion+`"` {
		t.Errorf("If-Match = %q", got)
	}
}

func TestGetInvalidType(t *testing.T) {
	_, _, err := demographic.Get(context.Background(), nil,
		demographic.Type("widget"), openehrclient.LatestOf(personVOID))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("invalid type: err = %v, want ErrInvalidConfig", err)
	}
}
