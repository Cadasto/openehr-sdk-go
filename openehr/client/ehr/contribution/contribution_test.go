package contribution_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const ehrIDFixture openehrclient.EHRID = "bf0b2ad8-7b0e-4f4d-9d33-6a8de69f0a64"

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

func newAudit() *rm.AuditDetails {
	name := "alice"
	return &rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: rm.PartyIdentified{Name: &name},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "creation"},
			DefiningCode: rm.CodePhrase{CodeString: "249"},
		},
		TimeCommitted: rm.DVDateTime{Value: "2026-05-17T10:00:00Z"},
	}
}

func TestCommitMinimal(t *testing.T) {
	var captured *http.Request
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	batch := &rm.Contribution{
		Audit:    *newAudit(),
		Versions: []rm.ObjectRef{},
	}
	out, meta, err := contribution.Commit(context.Background(), newClient(t, srv), ehrIDFixture, batch)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("expected nil Contribution body on PreferMinimal, got %+v", out)
	}
	if captured.URL.Path != "/openehr/v1/ehr/"+string(ehrIDFixture)+"/contribution" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if got := captured.Header.Get("Prefer"); got != "return=minimal" {
		t.Errorf("Prefer = %q (default)", got)
	}
	if !strings.Contains(string(capturedBody), `"system_id":"cdr.example"`) {
		t.Errorf("audit not in body: %s", string(capturedBody))
	}
	if meta.Location == "" {
		t.Error("Location not captured")
	}
}

func TestCommitRejectsInputs(t *testing.T) {
	_, _, err := contribution.Commit(context.Background(), nil, "", nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("empty EHRID: expected ErrInvalidConfig, got %v", err)
	}
	_, _, err = contribution.Commit(context.Background(), nil, ehrIDFixture, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("nil batch: expected ErrInvalidConfig, got %v", err)
	}
}

// TestCommitSubmissionShape pins SDK-GAP-10 / PROBE-072. Skipped at
// Phase 0 — see docs/plans/2026-05-26-contribution-submission-shape.md —
// because the current `contribution.Commit` accepts `*rm.Contribution`
// whose `versions[]` is `[]ObjectRef`, so the wire body can never carry
// `_type:"ORIGINAL_VERSION"` with inline `data`. Phase 1 lands
// `contribution.Submission` and unskips this test; the assertion below
// is the regression gate the new API must satisfy.
func TestCommitSubmissionShape(t *testing.T) {
	t.Skip("PROBE-072 stub — unskips when contribution.Submission lands (plan Phase 1)")
}

func TestCommitMapsVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"batch conflict","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()
	batch := &rm.Contribution{Audit: *newAudit()}
	_, _, err := contribution.Commit(context.Background(), newClient(t, srv), ehrIDFixture, batch)
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}
