package demographic_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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

	vp, _, err := demographic.GetVersionedParty(context.Background(), newClient(t, srv), personVOID)
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

	rh, _, err := demographic.GetRevisionHistory(context.Background(), newClient(t, srv), personVOID)
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

	pv, _, err := demographic.GetVersion(context.Background(), newClient(t, srv), personVOID)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Path != "/openehr/v1/demographic/versioned_party/"+personVOID+"/version" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if pv.UID.Value != personVersion {
		t.Errorf("version uid = %q, want %q", pv.UID.Value, personVersion)
	}
	if pv.LifecycleState.Value != "complete" {
		t.Errorf("lifecycle_state = %q, want complete", pv.LifecycleState.Value)
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

func TestGetVersionAtTime(t *testing.T) {
	var captured *http.Request
	body := cassette(t, "original_version.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	at, _ := time.Parse(time.RFC3339, "2026-06-16T08:00:00Z")
	_, _, err := demographic.GetVersionAtTime(context.Background(), newClient(t, srv), personVOID, at)
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

	_, _, err := demographic.GetVersionByID(context.Background(), newClient(t, srv), personVOID, personVersion)
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
		pv, _, err := demographic.GetVersion(context.Background(), newClient(t, srv), personVOID)
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
		_, _, err := demographic.GetVersion(context.Background(), newClient(t, srv), personVOID)
		if !errors.Is(err, transport.ErrInvalidShape) {
			t.Fatalf("empty 200 err = %v, want ErrInvalidShape", err)
		}
	})
}

func TestVersionedReadGuards(t *testing.T) {
	ctx := context.Background()
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
