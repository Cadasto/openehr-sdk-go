package care

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// fakeCodec satisfies Codec without importing cadasto/datamap (mirrors the
// consumer-wired adapter; identity transforms are enough for the seam test).
type fakeCodec struct{}

func (fakeCodec) ToComposition(_ *template.OperationalTemplate, dm map[string]any) (map[string]any, error) {
	return dm, nil
}

func (fakeCodec) FromComposition(_ *template.OperationalTemplate, comp map[string]any) (map[string]any, error) {
	return comp, nil
}

func validConfig() Config {
	return Config{
		Tenant:       "cataniamc",
		Environment:  "acc",
		BaseURL:      "https://cataniamc.api.acc.cadasto.io/openehr/v1",
		TokenURL:     "https://auth.acc.cadasto.io/oauth2/token",
		ClientID:     "client-x",
		ClientSecret: "secret-x",
		Audience:     "https://cataniamc.api.acc.cadasto.io/openehr/v1",
		HTTPClient:   &http.Client{},
		Codec:        fakeCodec{},
	}
}

func TestNewClientWiring(t *testing.T) {
	c, err := NewClient(validConfig())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c == nil || c.rest == nil {
		t.Fatal("NewClient returned a client without a transport")
	}
	if c.codec == nil {
		t.Error("codec not stored on client")
	}
}

func TestNewClientValidation(t *testing.T) {
	cases := map[string]func(*Config){
		"missing BaseURL":      func(c *Config) { c.BaseURL = "" },
		"missing TokenURL":     func(c *Config) { c.TokenURL = "" },
		"missing ClientID":     func(c *Config) { c.ClientID = "" },
		"missing ClientSecret": func(c *Config) { c.ClientSecret = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			if _, err := NewClient(cfg); err == nil {
				t.Errorf("%s: expected error", name)
			}
		})
	}
}

func TestNewClientDefaultsHTTPClient(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPClient = nil
	if _, err := NewClient(cfg); err != nil {
		t.Fatalf("NewClient with nil HTTPClient should default, got: %v", err)
	}
}

// recordingCodec wraps fakeCodec but remembers the *template.OperationalTemplate
// FromComposition was called with, so the test can assert GetCompositionDatamap
// actually threads a resolved (non-nil, parsed) OPT through to the codec — the
// C1 fix (REQ-0029): a nil-OPT decode falls back to the runtime node name and
// silently produces keys that don't match a template-authored key constant.
type recordingCodec struct {
	fakeCodec
	gotOPT *template.OperationalTemplate
}

func (r *recordingCodec) FromComposition(opt *template.OperationalTemplate, comp map[string]any) (map[string]any, error) {
	r.gotOPT = opt
	return r.fakeCodec.FromComposition(opt, comp)
}

// compositionTestClient builds a *Client wired to an httptest server, mirroring
// partyTestClient's pattern but for the openEHR-REST (not demographic) surface.
func compositionTestClient(t *testing.T, srv *httptest.Server, codec Codec) *Client {
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
	return &Client{rest: rest, codec: codec}
}

// PROBE-0796 proves REQ-0029 — GetCompositionDatamap is the read-path
// symmetric counterpart of SaveData/UpdateData: it resolves the OPT for
// templateID (same resolveOPT path), GETs the canonical composition, and
// decodes it via codec.FromComposition(opt, comp) — passing a REAL resolved
// OPT, not nil. This is exactly the seam C1 found broken: the enrollment
// engine's CarePlanAdapter.GetCarePlan used to call bare GetComposition and
// hand the caller the raw canonical map (never decoded), which made
// decodePathways/mergePathway silently see an empty datamap.
func TestGetCompositionDatamap(t *testing.T) {
	const (
		ehrID      = "8849182c-82ad-4088-a07f-48ead4180515"
		versionUID = "c2a1e6b0-1111-2222-3333-444455556666::cdr.example.com::1"
		templateID = "minimal_action_2"
	)
	optBytes, err := os.ReadFile(filepath.Join("..", "datamap", "testdata", "fixtures", templateID+".opt"))
	if err != nil {
		t.Fatalf("read OPT fixture: %v", err)
	}
	compBytes, err := json.Marshal(canonicalComposition())
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/definition/template/"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(optBytes)
		case strings.Contains(r.URL.Path, "/composition/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(compBytes)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	codec := &recordingCodec{}
	c := compositionTestClient(t, srv, codec)

	dm, err := c.GetCompositionDatamap(context.Background(), ehrID, versionUID, templateID)
	if err != nil {
		t.Fatalf("GetCompositionDatamap: %v", err)
	}
	if dm["_type"] != "COMPOSITION" {
		t.Errorf("decoded datamap _type = %v, want COMPOSITION (identity fakeCodec passthrough)", dm["_type"])
	}
	if codec.gotOPT == nil {
		t.Fatal("FromComposition was called with a nil OPT — the templateID was never resolved")
	}
	if got := codec.gotOPT.TemplateID(); got != templateID {
		t.Errorf("resolved OPT template id = %q, want %q", got, templateID)
	}
}

// TestGetCompositionDatamap_NoCodec mirrors SaveData/UpdateData's guard: a
// Client with no Codec configured must fail loudly rather than panic on a nil
// codec.FromComposition call.
func TestGetCompositionDatamap_NoCodec(t *testing.T) {
	c := &Client{}
	if _, err := c.GetCompositionDatamap(context.Background(), "ehr-1", "vuid-1", "tpl"); err == nil {
		t.Error("expected error for a Client with no Codec configured")
	}
}
