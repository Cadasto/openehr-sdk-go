package demographic_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestGetVersionedParty(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "versioned_party.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	vp, _, err := demographic.GetVersionedParty(t.Context(), newClient(t, srv), personVOID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/demographic/versioned_party/"+personVOID {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if vp.UID.Value != personVOID {
		t.Errorf("VERSIONED_PARTY uid = %q, want %q", vp.UID.Value, personVOID)
	}
}

func TestGetRevisionHistory(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "revision_history.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	rh, _, err := demographic.GetRevisionHistory(t.Context(), newClient(t, srv), personVOID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/demographic/versioned_party/"+personVOID+"/revision_history" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if len(rh.Items) != 1 {
		t.Errorf("revision-history items = %d, want 1", len(rh.Items))
	}
}

func TestGetVersionLatest(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "original_version.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	pv, _, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/demographic/versioned_party/"+personVOID+"/version" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if pv.UID.Value != personVersion {
		t.Errorf("version uid = %q, want %q", pv.UID.Value, personVersion)
	}
	if pv.VersionUID() != openehrclient.VersionUID(personVersion) {
		t.Errorf("VersionUID() = %q, want %q", pv.VersionUID(), personVersion)
	}
	if pv.LifecycleState.Value != "complete" {
		t.Errorf("lifecycle_state = %q, want complete", pv.LifecycleState.Value)
	}
	// Envelope fields beyond UID/lifecycle map correctly (guards a field-swap).
	if pv.PrecedingVersionUID == nil || pv.PrecedingVersionUID.Value != personVOID+"::cdr.example.com::0" {
		t.Errorf("preceding_version_uid = %+v", pv.PrecedingVersionUID)
	}
	if pv.CommitAudit == nil {
		t.Error("commit_audit not mapped (nil)")
	}
	if pv.Contribution == nil {
		t.Error("contribution not mapped (nil)")
	}
	// The polymorphic VERSION data decodes to the concrete PARTY type.
	person, ok := pv.Party.(*rm.Person)
	if !ok {
		t.Fatalf("VERSION data = %T, want *rm.Person", pv.Party)
	}
	if person.Name.GetValue() != "Jane Doe" {
		t.Errorf("party name = %q, want Jane Doe", person.Name.GetValue())
	}
}

// originalVersionEnvelope wraps a bare PARTY body in a minimal
// ORIGINAL_VERSION so a single per-type cassette drives a VERSION read.
func originalVersionEnvelope(partyBody []byte) []byte {
	return fmt.Appendf(
		nil,
		`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"%s::cdr::1"},"data":%s}`,
		personVOID, partyBody,
	)
}

// TestGetVersionDecodesEachPartyType drives the ORIGINAL_VERSION<PARTY> decode
// for every concrete PARTY type — guarding typereg registration of each
// subtype through the version envelope, not just PERSON.
func TestGetVersionDecodesEachPartyType(t *testing.T) {
	cases := []struct {
		cassette     string
		wantConcrete string
	}{
		{"person.json", "*rm.Person"},
		{"organisation.json", "*rm.Organisation"},
		{"group.json", "*rm.Group"},
		{"agent.json", "*rm.Agent"},
		{"role.json", "*rm.Role"},
	}
	for _, tc := range cases {
		t.Run(tc.wantConcrete, func(t *testing.T) {
			body := originalVersionEnvelope(cassette(t, tc.cassette))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			pv, _, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%T", pv.Party); got != tc.wantConcrete {
				t.Errorf("VERSION data decoded %s, want %s", got, tc.wantConcrete)
			}
		})
	}
}

// TestGetVersionWireError covers the error branch: a mapped 4xx propagates via
// errors.Is and the version metadata (ETag) is still returned alongside.
func TestGetVersionWireError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+personVersion+`"`)
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	_, meta, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
	if !errors.Is(err, transport.ErrPreconditionFailed) {
		t.Fatalf("err = %v, want ErrPreconditionFailed", err)
	}
	if meta == nil {
		t.Error("expected version metadata alongside the error")
	}
}

// TestGetVersionUnknownDataType: an envelope whose data carries an
// unregistered _type surfaces the typereg sentinel rather than a nil Party.
func TestGetVersionUnknownDataType(t *testing.T) {
	body := originalVersionEnvelope([]byte(`{"_type":"NOT_A_PARTY","name":{"_type":"DV_TEXT","value":"x"}}`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Fatalf("err = %v, want typereg.ErrUnknownType", err)
	}
}

// TestGetVersionAtTimeNoMatch: the documented 204-on-no-match contract holds
// through GetVersionAtTime specifically (a clean nil, not an error).
func TestGetVersionAtTimeNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	at, _ := time.Parse(time.RFC3339, "1990-01-01T00:00:00Z")
	pv, _, err := demographic.GetVersionAtTime(t.Context(), newClient(t, srv), personVOID, at)
	if err != nil || pv != nil {
		t.Fatalf("no-match GetVersionAtTime = (%+v, %v), want (nil, nil)", pv, err)
	}
}

// TestVersionedRepositoryWiring confirms the Repository facade delegates each
// versioned read to the matching package function (catches a mis-delegation).
func TestVersionedRepositoryWiring(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/revision_history"):
			_, _ = w.Write(cassette(t, "revision_history.json"))
		case strings.HasSuffix(r.URL.Path, "/version") || strings.Contains(r.URL.Path, "/version/"):
			_, _ = w.Write(cassette(t, "original_version.json"))
		default:
			_, _ = w.Write(cassette(t, "versioned_party.json"))
		}
	}))
	defer srv.Close()

	repo := demographic.NewRepository(newClient(t, srv))
	ctx := t.Context()
	if _, _, err := repo.GetVersionedParty(ctx, personVOID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GetRevisionHistory(ctx, personVOID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GetVersion(ctx, personVOID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GetVersionByID(ctx, personVOID, personVersion); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/openehr/v1/demographic/versioned_party/" + personVOID,
		"/openehr/v1/demographic/versioned_party/" + personVOID + "/revision_history",
		"/openehr/v1/demographic/versioned_party/" + personVOID + "/version",
		"/openehr/v1/demographic/versioned_party/" + personVOID + "/version/" + personVersion,
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Errorf("repository delegated to paths:\n%v\nwant:\n%v", paths, want)
	}
}

func TestGetVersionAtTime(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "original_version.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	at, _ := time.Parse(time.RFC3339, "2026-06-16T08:00:00Z")
	_, _, err := demographic.GetVersionAtTime(t.Context(), newClient(t, srv), personVOID, at)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/demographic/versioned_party/"+personVOID+"/version" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if got := captured.URL.Query().Get("version_at_time"); got != "2026-06-16T08:00:00Z" {
		t.Errorf("version_at_time = %q", got)
	}
}

func TestGetVersionByID(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "original_version.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, err := demographic.GetVersionByID(t.Context(), newClient(t, srv), personVOID, personVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := "/openehr/v1/demographic/versioned_party/" + personVOID + "/version/" + personVersion
	if captured.URL.Path != want {
		t.Errorf("path = %q, want %q", captured.URL.Path, want)
	}
}

// TestGetVersionEmptyBody mirrors getParty's strictness: a 204 yields a nil
// PartyVersion, while any other empty-body 2xx is an ErrInvalidShape anomaly.
func TestGetVersionEmptyBody(t *testing.T) {
	t.Run("204_no_content", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		pv, _, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
		if err != nil {
			t.Fatalf("204 should be a clean nil, got %v", err)
		}
		if pv != nil {
			t.Errorf("204 PartyVersion = %+v, want nil", pv)
		}
	})
	t.Run("empty_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK) // empty body
		}))
		defer srv.Close()
		_, _, err := demographic.GetVersion(t.Context(), newClient(t, srv), personVOID)
		if !errors.Is(err, transport.ErrInvalidShape) {
			t.Fatalf("empty 200 err = %v, want ErrInvalidShape", err)
		}
	})
}

func TestVersionedReadGuards(t *testing.T) {
	ctx := t.Context()
	if _, _, err := demographic.GetVersionedParty(ctx, nil, ""); !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("GetVersionedParty(empty) err = %v", err)
	}
	if _, _, err := demographic.GetRevisionHistory(ctx, nil, ""); !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("GetRevisionHistory(empty) err = %v", err)
	}
	if _, _, err := demographic.GetVersion(ctx, nil, ""); !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("GetVersion(empty) err = %v", err)
	}
	if _, _, err := demographic.GetVersionAtTime(ctx, nil, personVOID, time.Time{}); !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("GetVersionAtTime(zero time) err = %v", err)
	}
	if _, _, err := demographic.GetVersionByID(ctx, nil, personVOID, ""); !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("GetVersionByID(empty version) err = %v", err)
	}
}
