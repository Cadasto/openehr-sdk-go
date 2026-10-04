package discovery_test

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// fieldRecorder is a slog.Handler that keeps the "field" attribute of every
// record it receives.
type fieldRecorder struct {
	mu     sync.Mutex
	fields []string
}

func (h *fieldRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (h *fieldRecorder) Handle(_ context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "field" {
			h.mu.Lock()
			h.fields = append(h.fields, a.Value.String())
			h.mu.Unlock()
		}
		return true
	})
	return nil
}

func (h *fieldRecorder) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *fieldRecorder) WithGroup(string) slog.Handler      { return h }

// TestResolveWarnsOnEveryPlaintextAuthEndpoint pins REQ-073: under
// WithAllowInsecure a plaintext auth endpoint is accepted with a warning,
// and that holds for every endpoint the catalog carries, the optional
// introspection, revocation and management endpoints included.
func TestResolveWarnsOnEveryPlaintextAuthEndpoint(t *testing.T) { // REQ-073
	endpoints := []string{
		"authorization_endpoint",
		"token_endpoint",
		"jwks_uri",
		"registration_endpoint",
		"introspection_endpoint",
		"revocation_endpoint",
		"management_endpoint",
	}
	members := map[string]string{}
	for _, name := range endpoints {
		members[name] = "http://auth.example.com/" + name
	}
	body := documentWith("https://api.example.com/openehr/v1", members)
	p := startPlatform(t, false, serve(func(string) string { return body }), notFound)
	rec := &fieldRecorder{}
	res := p.resolver(t, discovery.WithAllowInsecure(), discovery.WithLogger(slog.New(rec)))
	if _, err := res.Resolve(t.Context(), p.baseURL()); err != nil {
		t.Fatalf("Resolve(%q) error = %v, want success under WithAllowInsecure", p.baseURL(), err)
	}
	rec.mu.Lock()
	warned := slices.Clone(rec.fields)
	rec.mu.Unlock()
	for _, name := range endpoints {
		if !slices.Contains(warned, name) {
			t.Errorf("no plaintext warning for %s; warned fields %q", name, warned)
		}
	}
}

// TestDiscoveryErrorLabelsBaseURL pins REQ-070: DiscoveryError.Issuer holds
// the Platform base URL, not the OpenID Connect issuer, so Error labels it
// base_url=.
func TestDiscoveryErrorLabelsBaseURL(t *testing.T) { // REQ-070
	err := &discovery.DiscoveryError{Issuer: "https://platform.example/gateway", Reason: discovery.ReasonFetchFailed}
	got := err.Error()
	if !strings.Contains(got, " base_url=https://platform.example/gateway") {
		t.Errorf("Error() = %q, want it to label the base URL base_url=", got)
	}
	if strings.Contains(got, "issuer=") {
		t.Errorf("Error() = %q, want no issuer= label for the base URL", got)
	}
}
