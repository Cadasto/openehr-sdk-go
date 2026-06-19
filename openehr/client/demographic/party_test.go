package demographic_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// cassette reads a vendored Demographic fixture by file name.
func cassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", "demographic", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

// partyName extracts the runtime name from any concrete PARTY type.
func partyName(p rm.Party) string {
	switch v := p.(type) {
	case *rm.Person:
		return v.Name.GetValue()
	case *rm.Organisation:
		return v.Name.GetValue()
	case *rm.Group:
		return v.Name.GetValue()
	case *rm.Agent:
		return v.Name.GetValue()
	case *rm.Role:
		return v.Name.GetValue()
	default:
		return ""
	}
}

// TestGetDecodesEachPartyType drives a Get for every concrete PARTY type and
// asserts the response body is decoded polymorphically (by _type) into the
// matching concrete Go type — guarding the typereg registration of each
// subtype (REQ-040), not just PERSON.
func TestGetDecodesEachPartyType(t *testing.T) {
	cases := []struct {
		typ          demographic.Type
		cassette     string
		wantConcrete string
		wantName     string
	}{
		{demographic.Person, "person.json", "*rm.Person", "Jane Doe"},
		{demographic.Organisation, "organisation.json", "*rm.Organisation", "Acme Hospital"},
		{demographic.Group, "group.json", "*rm.Group", "Cardiology Team"},
		{demographic.Agent, "agent.json", "*rm.Agent", "Triage Bot"},
		{demographic.Role, "role.json", "*rm.Role", "Attending Physician"},
	}
	for _, tc := range cases {
		t.Run(string(tc.typ), func(t *testing.T) {
			body := cassette(t, tc.cassette)
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				w.Header().Set("ETag", `"`+personVersion+`"`)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			got, _, err := demographic.Get(context.Background(), newClient(t, srv),
				tc.typ, openehrclient.LatestOf(personVOID))
			if err != nil {
				t.Fatal(err)
			}
			if captured.URL.Path != "/openehr/v1/demographic/"+string(tc.typ)+"/"+personVOID {
				t.Errorf("path = %q", captured.URL.Path)
			}
			if gotType := fmt.Sprintf("%T", got); gotType != tc.wantConcrete {
				t.Errorf("decoded %s, want %s", gotType, tc.wantConcrete)
			}
			if name := partyName(got); name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
		})
	}
}

func TestGetSendsVersionMetadata(t *testing.T) {
	body := cassette(t, "person.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, meta, err := demographic.Get(context.Background(), newClient(t, srv),
		demographic.Person, openehrclient.LatestOf(personVOID))
	if err != nil {
		t.Fatal(err)
	}
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

// TestCreateRoutesByConcreteType asserts Create derives the resource path
// segment from the value's concrete type, for every PARTY type (pointer form)
// plus one value form.
func TestCreateRoutesByConcreteType(t *testing.T) {
	cases := []struct {
		name     string
		party    rm.Party
		wantPath string
	}{
		{"person_ptr", &rm.Person{Name: rm.DVText{Value: "p"}}, "/openehr/v1/demographic/person"},
		{"organisation_ptr", &rm.Organisation{Name: rm.DVText{Value: "o"}}, "/openehr/v1/demographic/organisation"},
		{"group_ptr", &rm.Group{Name: rm.DVText{Value: "g"}}, "/openehr/v1/demographic/group"},
		{"agent_ptr", &rm.Agent{Name: rm.DVText{Value: "a"}}, "/openehr/v1/demographic/agent"},
		{"role_ptr", &rm.Role{Name: rm.DVText{Value: "r"}}, "/openehr/v1/demographic/role"},
		{"person_value", rm.Person{Name: rm.DVText{Value: "p"}}, "/openehr/v1/demographic/person"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				w.Header().Set("Location", "/demographic/x/"+personVersion)
				w.WriteHeader(http.StatusCreated)
			}))
			defer srv.Close()

			if _, _, err := demographic.Create(context.Background(), newClient(t, srv), tc.party); err != nil {
				t.Fatal(err)
			}
			if captured.Method != http.MethodPost {
				t.Errorf("method = %s", captured.Method)
			}
			if captured.URL.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", captured.URL.Path, tc.wantPath)
			}
		})
	}
}

func TestCreatePreferRepresentation(t *testing.T) {
	body := cassette(t, "person.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, meta, err := demographic.Create(context.Background(), newClient(t, srv),
		&rm.Person{Name: rm.DVText{Value: "Jane Doe"}},
		demographic.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*rm.Person); !ok {
		t.Fatalf("Prefer=representation: got %T, want *rm.Person", got)
	}
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID = %q", meta.VersionUID)
	}
}

func TestCreatePreferMinimalNoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.Header().Set("Location", "/demographic/organisation/"+personVersion)
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

// TestCreatePreferIdentifier exercises the ITS-REST Identifier body
// {"uid": ...} → VersionMetadata.VersionUID resolution (REQ-094).
func TestCreatePreferIdentifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uid":"` + personVersion + `"}`))
	}))
	defer srv.Close()

	got, meta, err := demographic.Create(context.Background(), newClient(t, srv),
		&rm.Person{Name: rm.DVText{Value: "Jane Doe"}},
		demographic.WithPrefer(transport.PreferIdentifier))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("identifier Create returned a body: %T", got)
	}
	if meta.VersionUID != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID (from Identifier body) = %q, want %q", meta.VersionUID, personVersion)
	}
}

func TestCreateNilParty(t *testing.T) {
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

// TestUpdatePreconditionFailed covers the error branch: a 412 maps to
// ErrPreconditionFailed and the version metadata (ETag) is still returned.
func TestUpdatePreconditionFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	_, meta, err := demographic.Update(context.Background(), newClient(t, srv),
		demographic.Person, personVOID, "stale::v::1", &rm.Person{Name: rm.DVText{Value: "x"}})
	if !errors.Is(err, transport.ErrPreconditionFailed) {
		t.Fatalf("err = %v, want ErrPreconditionFailed", err)
	}
	if meta == nil {
		t.Error("expected version metadata alongside the 412 error")
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

// TestDeleteVersionConflict covers the error branch: a 409 (referential
// conflict) maps to ErrVersionConflict.
func TestDeleteVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	_, err := demographic.Delete(context.Background(), newClient(t, srv),
		demographic.Person, personVersion, personVersion)
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Fatalf("err = %v, want ErrVersionConflict", err)
	}
}

func TestGetInvalidType(t *testing.T) {
	_, _, err := demographic.Get(context.Background(), nil,
		demographic.Type("widget"), openehrclient.LatestOf(personVOID))
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("invalid type: err = %v, want ErrInvalidConfig", err)
	}
}

func TestGetRejectsNilRef(t *testing.T) {
	_, _, err := demographic.Get(context.Background(), nil, demographic.Person, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("nil Ref: err = %v, want ErrInvalidConfig", err)
	}
}

func TestUpdateRejectsNilParty(t *testing.T) {
	_, _, err := demographic.Update(context.Background(), nil,
		demographic.Person, personVOID, personVersion, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("nil Party: err = %v, want ErrInvalidConfig", err)
	}
}

// TestGetAtTimeNoVersion pins the documented "204 → nil Party, nil error,
// metadata still returned" contract for a version_at_time with no matching
// version, and asserts the version_at_time query param is actually emitted
// (the LatestAtTime Ref variant — otherwise an as-of-time read would silently
// degrade to a latest read).
func TestGetAtTimeNoVersion(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	at := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	got, meta, err := demographic.Get(context.Background(), newClient(t, srv),
		demographic.Person, openehrclient.LatestAtTime(personVOID, at))
	if err != nil {
		t.Fatalf("204 read: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("204 read: got %T, want nil Party", got)
	}
	if meta == nil {
		t.Error("204 read: expected non-nil metadata")
	}
	if q := captured.URL.Query().Get("version_at_time"); q != at.Format(time.RFC3339) {
		t.Errorf("version_at_time = %q, want %q", q, at.Format(time.RFC3339))
	}
}

// TestGetEmptyBodyAnomaly guards the silent-failure fix: an empty body on a
// non-204 2xx is a wire anomaly and must surface as ErrInvalidShape rather
// than masquerading as "no version found".
func TestGetEmptyBodyAnomaly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 200 with no body
	}))
	defer srv.Close()

	_, _, err := demographic.Get(context.Background(), newClient(t, srv),
		demographic.Person, openehrclient.LatestOf(personVOID))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("empty 200 read: err = %v, want ErrInvalidShape", err)
	}
}

// TestGetSpecificVersion covers the VersionOf Ref variant: the version UID is
// the path tail and no query param is sent.
func TestGetSpecificVersion(t *testing.T) {
	body := cassette(t, "person.json")
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("ETag", `"`+personVersion+`"`)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, _, err := demographic.Get(context.Background(), newClient(t, srv),
		demographic.Person, openehrclient.VersionOf(personVersion)); err != nil {
		t.Fatal(err)
	}
	if want := "/openehr/v1/demographic/person/" + personVersion; captured.URL.Path != want {
		t.Errorf("path = %q, want %q", captured.URL.Path, want)
	}
	if captured.URL.RawQuery != "" {
		t.Errorf("VersionOf must not send a query, got %q", captured.URL.RawQuery)
	}
}

// TestCreateSendsAuditAndPreferHeaders exercises WithAuditDetails (REQ-059) and
// asserts both the openehr-audit-details header and the default Prefer header
// reach the wire, plus that the request body carries the _type discriminator.
func TestCreateSendsAuditAndPreferHeaders(t *testing.T) {
	var (
		captured     *http.Request
		capturedBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		captured = r.Clone(r.Context())
		w.Header().Set("Location", "/demographic/person/"+personVersion)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	committer := "alice"
	audit := &rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: rm.PartyIdentified{Name: &committer},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	if _, _, err := demographic.Create(context.Background(), newClient(t, srv),
		&rm.Person{Name: rm.DVText{Value: "Jane Doe"}},
		demographic.WithAuditDetails(audit)); err != nil {
		t.Fatal(err)
	}
	if h := captured.Header.Get("openehr-audit-details"); !strings.Contains(h, `system_id="cdr.example"`) {
		t.Errorf("openehr-audit-details header = %q", h)
	}
	if h := captured.Header.Get("Prefer"); h != "return=minimal" {
		t.Errorf("Prefer = %q, want return=minimal", h)
	}
	if !strings.Contains(string(capturedBody), `"_type"`) {
		t.Errorf("Create body missing _type discriminator: %s", capturedBody)
	}
}

// TestCreatePreferRepresentationEmptyBody pins the REQ-094 strict guard: a
// caller that asks for representation but gets no body must see
// ErrInvalidShape, never a silently-nil Party.
func TestCreatePreferRepresentationEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.WriteHeader(http.StatusCreated) // representation requested, but empty body
	}))
	defer srv.Close()

	_, meta, err := demographic.Create(context.Background(), newClient(t, srv),
		&rm.Person{Name: rm.DVText{Value: "Jane Doe"}},
		demographic.WithPrefer(transport.PreferRepresentation))
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("representation empty body: err = %v, want ErrInvalidShape", err)
	}
	if meta == nil {
		t.Error("expected metadata alongside ErrInvalidShape")
	}
}

// TestDeleteSendsAuditDetails covers the WithDeleteAudit option (REQ-059): a
// logical delete is a versioned commit and must be able to carry the audit
// envelope, matching the composition sibling.
func TestDeleteSendsAuditDetails(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	committer := "alice"
	audit := &rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: rm.PartyIdentified{Name: &committer},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "deleted"},
			DefiningCode: rm.CodePhrase{CodeString: "523"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
	if _, err := demographic.Delete(context.Background(), newClient(t, srv),
		demographic.Person, personVersion, personVersion,
		demographic.WithDeleteAudit(audit)); err != nil {
		t.Fatal(err)
	}
	if h := captured.Header.Get("openehr-audit-details"); !strings.Contains(h, `system_id="cdr.example"`) {
		t.Errorf("openehr-audit-details header = %q", h)
	}
}
