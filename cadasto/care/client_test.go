package care

import (
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
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
