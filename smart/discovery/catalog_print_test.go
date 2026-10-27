package discovery_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// catalogPass is the password every URL of printedCatalog carries.
const catalogPass = "CATPASS-s3cr3t"

// urlMask is the text url.URL.Redacted, and so internal/urlredact, writes in
// place of a password.
const urlMask = "xxxxx"

// catalogPrintVerbs are the fmt verbs a catalog and its parts are printed
// with: the ones a pointer or a struct suits, the ones it does not, and some
// of each with flags and a width.
var catalogPrintVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%t", "%20v", "%-20s", "%+q", "%#x"}

// passURL returns the https URL of host and path with the user name user and
// the password catalogPass.
func passURL(user, host, path string) *url.URL {
	return &url.URL{Scheme: "https", User: url.UserPassword(user, catalogPass), Host: host, Path: path}
}

// printedEntry returns a service entry with every field set, whose base URL,
// documentation link and OpenAPI link carry catalogPass.
func printedEntry() discovery.ServiceEntry {
	return discovery.ServiceEntry{
		ID:            discovery.ServiceIDOpenEHRRest,
		BaseURL:       passURL("svc", "cdr.example", "/openehr/v1"),
		Version:       "2.36.0",
		SpecVersion:   discovery.SpecVersionPin,
		Description:   "openEHR REST API",
		Documentation: "https://doc:" + catalogPass + "@cdr.example/docs",
		OpenAPI:       "https://api:" + catalogPass + "@cdr.example/openapi.json",
		Capabilities:  []string{"entry-capability"},
	}
}

// printedAuth returns auth endpoints with every field set, each endpoint URL
// carrying catalogPass.
func printedAuth() discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint:                      passURL("authz", "idp.example", "/authorize"),
		TokenEndpoint:                              passURL("token", "idp.example", "/token"),
		JWKSURI:                                    passURL("jwks", "idp.example", "/jwks"),
		RegistrationEndpoint:                       passURL("reg", "idp.example", "/register"),
		IntrospectionEndpoint:                      passURL("intro", "idp.example", "/introspect"),
		RevocationEndpoint:                         passURL("revoke", "idp.example", "/revoke"),
		ManagementEndpoint:                         passURL("mgmt", "idp.example", "/manage"),
		ScopesSupported:                            []string{"openid", "launch"},
		ResponseTypesSupported:                     []string{"code"},
		CodeChallengeMethodsSupported:              []string{"S256"},
		GrantTypesSupported:                        []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported:          []string{"private_key_jwt"},
		TokenEndpointAuthSigningAlgValuesSupported: []string{"RS384"},
		IDTokenSigningAlgValuesSupported:           []string{"RS256"},
		AuthorizationResponseIssParameterSupported: true,
		Capabilities:                               []string{discovery.CapabilityLaunchBase64JSON},
	}
}

// printedCatalog returns a catalog with every exported field set, whose base
// URL, issuer, service and auth endpoint URLs all carry catalogPass.
func printedCatalog() *discovery.ServiceCatalog {
	return &discovery.ServiceCatalog{
		BaseURL:    "https://cdr:" + catalogPass + "@cdr.example/openehr",
		Issuer:     "https://idp:" + catalogPass + "@idp.example",
		Services:   map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: printedEntry()},
		Auth:       printedAuth(),
		ResolvedAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC),
		ETag:       `"catalog-v1"`,
	}
}

// printedStaticConfig returns a static configuration with every exported field
// set, whose base URL, issuer, service and auth endpoint URLs all carry
// catalogPass.
func printedStaticConfig() discovery.StaticConfig {
	return discovery.StaticConfig{
		BaseURL:   "https://cdr:" + catalogPass + "@cdr.example/openehr",
		Issuer:    "https://idp:" + catalogPass + "@idp.example",
		Services:  map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: printedEntry()},
		Auth:      printedAuth(),
		ExpiresAt: time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC),
		ETag:      `"static-v1"`,
	}
}

// TestREQ060_CatalogPrintsNoURLPassword pins REQ-060 (auth.md § REQ-060): a
// *discovery.ServiceCatalog, a discovery.ServiceEntry, a
// discovery.AuthEndpoints and a discovery.StaticConfig, printed with any fmt
// verb, nested in a struct or a slice, or logged through log/slog, show the
// password of every URL they hold masked. Every verb writes the text of
// String, which names every exported field, and slog logs every exported
// field. A nil catalog prints and logs as nil, and an empty one prints
// without failing.
func TestREQ060_CatalogPrintsNoURLPassword(t *testing.T) { // REQ-060
	t.Parallel()
	staticShown := []string{
		"discovery.StaticConfig{",
		"https://cdr:" + urlMask + "@cdr.example/openehr",
		"https://idp:" + urlMask + "@idp.example",
		"https://svc:" + urlMask + "@cdr.example/openehr/v1",
		"https://token:" + urlMask + "@idp.example/token",
		"2026-10-27T13:00:00Z",
		"static-v1",
	}
	tests := []struct {
		name  string
		v     any
		shown []string
	}{
		{
			name: "*discovery.ServiceCatalog",
			v:    printedCatalog(),
			shown: []string{
				"discovery.ServiceCatalog{",
				"https://cdr:" + urlMask + "@cdr.example/openehr",
				"https://idp:" + urlMask + "@idp.example",
				"https://svc:" + urlMask + "@cdr.example/openehr/v1",
				"https://token:" + urlMask + "@idp.example/token",
				"2026-10-27T12:00:00Z",
				"2026-10-27T13:00:00Z",
				"catalog-v1",
			},
		},
		{
			name: "discovery.ServiceEntry",
			v:    printedEntry(),
			shown: []string{
				"discovery.ServiceEntry{",
				discovery.ServiceIDOpenEHRRest,
				"https://svc:" + urlMask + "@cdr.example/openehr/v1",
				"2.36.0",
				discovery.SpecVersionPin,
				"openEHR REST API",
				"https://doc:" + urlMask + "@cdr.example/docs",
				"https://api:" + urlMask + "@cdr.example/openapi.json",
				"entry-capability",
			},
		},
		{
			name: "discovery.AuthEndpoints",
			v:    printedAuth(),
			shown: []string{
				"discovery.AuthEndpoints{",
				"https://authz:" + urlMask + "@idp.example/authorize",
				"https://token:" + urlMask + "@idp.example/token",
				"https://jwks:" + urlMask + "@idp.example/jwks",
				"https://reg:" + urlMask + "@idp.example/register",
				"https://intro:" + urlMask + "@idp.example/introspect",
				"https://revoke:" + urlMask + "@idp.example/revoke",
				"https://mgmt:" + urlMask + "@idp.example/manage",
				"S256",
				"private_key_jwt",
				"RS384",
				"RS256",
				"true",
				discovery.CapabilityLaunchBase64JSON,
			},
		},
		{
			name:  "discovery.StaticConfig",
			v:     printedStaticConfig(),
			shown: staticShown,
		},
		{
			name:  "*discovery.StaticConfig",
			v:     new(printedStaticConfig()),
			shown: staticShown,
		},
		{
			name:  "empty *discovery.ServiceCatalog",
			v:     &discovery.ServiceCatalog{},
			shown: []string{"discovery.ServiceCatalog{"},
		},
		{
			name:  "empty discovery.AuthEndpoints",
			v:     discovery.AuthEndpoints{},
			shown: []string{"discovery.AuthEndpoints{"},
		},
		{
			name:  "empty discovery.StaticConfig",
			v:     discovery.StaticConfig{},
			shown: []string{"discovery.StaticConfig{"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, p := range catalogPrintProblems(tc.v, tc.shown) {
				t.Error(p)
			}
		})
	}

	t.Run("nil *discovery.ServiceCatalog", func(t *testing.T) {
		t.Parallel()
		var c *discovery.ServiceCatalog
		for _, verb := range catalogPrintVerbs {
			if out := fmt.Sprintf(verb, c); out != "<nil>" {
				t.Errorf("fmt.Sprintf(%q) of a nil *discovery.ServiceCatalog = %q, want <nil>", verb, out)
			}
		}
		if s, ok := any(c).(fmt.Stringer); !ok {
			t.Error("*discovery.ServiceCatalog does not implement fmt.Stringer")
		} else if got := s.String(); got != "<nil>" {
			t.Errorf("String() of a nil *discovery.ServiceCatalog = %q, want <nil>", got)
		}
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Info("printed", "v", c)
		if !strings.Contains(buf.String(), `"v":null`) {
			t.Errorf("slog JSONHandler wrote %q for a nil *discovery.ServiceCatalog, want \"v\":null", buf.String())
		}
	})
}

// catalogPrintProblems prints v through String and GoString, with every verb
// of catalogPrintVerbs, nested in a struct and a slice, and through the text
// and JSON handlers of log/slog. It returns one line for each output that
// shows catalogPass or a panic, for a String text that lacks one of shown or
// the name of an exported field of v's type, for a verb whose output differs
// from the String text, and for a JSON log line that lacks the mask or a key
// for an exported field.
func catalogPrintProblems(v any, shown []string) []string {
	var problems []string
	leaks := func(where, out string) {
		if strings.Contains(out, catalogPass) {
			problems = append(problems, fmt.Sprintf("%s shows the URL password: %s", where, out))
		}
		if strings.Contains(out, "PANIC") {
			problems = append(problems, fmt.Sprintf("%s panicked: %s", where, out))
		}
	}
	rt := reflect.TypeOf(v)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	var fields []string
	for f := range rt.Fields() {
		if f.IsExported() {
			fields = append(fields, f.Name)
		}
	}

	var want string
	if s, ok := v.(fmt.Stringer); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Stringer", v))
	} else {
		want = s.String()
		leaks("String()", want)
		for _, s := range shown {
			if !strings.Contains(want, s) {
				problems = append(problems, fmt.Sprintf("String() = %q, want it to show %q", want, s))
			}
		}
		for _, name := range fields {
			if !strings.Contains(want, name+": ") {
				problems = append(problems, fmt.Sprintf("String() = %q, want it to name the field %s", want, name))
			}
		}
	}
	if g, ok := v.(fmt.GoStringer); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.GoStringer", v))
	} else if got := g.GoString(); got != want {
		problems = append(problems, fmt.Sprintf("GoString() = %q, want %q, the text of String()", got, want))
	}
	if _, ok := v.(fmt.Formatter); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Formatter", v))
	}
	if _, ok := v.(slog.LogValuer); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement slog.LogValuer", v))
	}
	for _, verb := range catalogPrintVerbs {
		out := fmt.Sprintf(verb, v)
		leaks(fmt.Sprintf("fmt.Sprintf(%q)", verb), out)
		if want != "" && out != want {
			problems = append(problems, fmt.Sprintf("fmt.Sprintf(%q) = %q, want %q, the text of String()", verb, out, want))
		}
		leaks(fmt.Sprintf("a struct field under %q", verb), fmt.Sprintf(verb, struct{ V any }{v}))
		leaks(fmt.Sprintf("a slice element under %q", verb), fmt.Sprintf(verb, []any{v}))
	}

	var text bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("printed", "v", v)
	leaks("slog TextHandler", text.String())

	var js bytes.Buffer
	slog.New(slog.NewJSONHandler(&js, nil)).Info("printed", "v", v)
	leaks("slog JSONHandler", js.String())
	if len(shown) > 1 && !strings.Contains(js.String(), urlMask) {
		problems = append(problems, fmt.Sprintf("slog JSONHandler wrote %q, want it to carry the mask %s", js.String(), urlMask))
	}
	var line struct{ V map[string]any }
	if err := json.Unmarshal(js.Bytes(), &line); err != nil {
		problems = append(problems, fmt.Sprintf("slog JSONHandler wrote %q, which does not decode: %v", js.String(), err))
	} else if len(shown) > 1 {
		for _, name := range fields {
			if _, ok := line.V[name]; !ok {
				problems = append(problems, fmt.Sprintf("slog JSONHandler wrote %q, want a key %s", js.String(), name))
			}
		}
	}
	return problems
}

// TestREQ060_CatalogJSONUnchanged pins REQ-060 (auth.md § REQ-060): the
// methods that mask a URL's password when a catalog is printed or logged
// leave its JSON encoding alone. A *discovery.ServiceCatalog, a
// discovery.ServiceEntry, a discovery.AuthEndpoints and a
// discovery.StaticConfig encode as the same fields without those methods do,
// the base URL keeps its password, and the catalog decodes back to a value
// that encodes the same, so a Cache can store catalogs as JSON.
func TestREQ060_CatalogJSONUnchanged(t *testing.T) { // REQ-060
	t.Parallel()
	type plainCatalog discovery.ServiceCatalog
	type plainEntry discovery.ServiceEntry
	type plainAuth discovery.AuthEndpoints
	type plainStatic discovery.StaticConfig
	cat := printedCatalog()
	entry := printedEntry()
	auth := printedAuth()
	static := printedStaticConfig()
	pairs := []struct {
		name        string
		typed, bare any
	}{
		{name: "*discovery.ServiceCatalog", typed: cat, bare: (*plainCatalog)(cat)},
		{name: "discovery.ServiceEntry", typed: entry, bare: plainEntry(entry)},
		{name: "discovery.AuthEndpoints", typed: auth, bare: plainAuth(auth)},
		{name: "discovery.StaticConfig", typed: static, bare: plainStatic(static)},
	}
	for _, p := range pairs {
		got, err := json.Marshal(p.typed)
		if err != nil {
			t.Fatalf("json.Marshal(%s) error = %v", p.name, err)
		}
		want, err := json.Marshal(p.bare)
		if err != nil {
			t.Fatalf("json.Marshal(%s without methods) error = %v", p.name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("json.Marshal(%s) =\n%s\nwant the encoding of the same fields without methods:\n%s", p.name, got, want)
		}
	}

	encoded, err := json.Marshal(cat)
	if err != nil {
		t.Fatalf("json.Marshal(catalog) error = %v", err)
	}
	if !bytes.Contains(encoded, []byte(cat.BaseURL)) {
		t.Errorf("json.Marshal(catalog) = %s, want it to carry the base URL %q as written", encoded, cat.BaseURL)
	}
	var back discovery.ServiceCatalog
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", encoded, err)
	}
	if back.BaseURL != cat.BaseURL {
		t.Errorf("decoded BaseURL = %q, want %q", back.BaseURL, cat.BaseURL)
	}
	again, err := json.Marshal(&back)
	if err != nil {
		t.Fatalf("json.Marshal(decoded catalog) error = %v", err)
	}
	if !bytes.Equal(again, encoded) {
		t.Errorf("the decoded catalog encodes as\n%s\nwant\n%s", again, encoded)
	}
}

// TestREQ025_FormatWithNilStateDoesNotPanic pins REQ-025 (idiom.md § No
// panics): Format called directly with a nil fmt.State returns without a
// panic, for a ServiceCatalog, a nil one included, a ServiceEntry, an
// AuthEndpoints and a StaticConfig.
func TestREQ025_FormatWithNilStateDoesNotPanic(t *testing.T) { // REQ-025
	t.Parallel()
	tests := []struct {
		name string
		v    any
	}{
		{name: "*discovery.ServiceCatalog", v: &discovery.ServiceCatalog{}},
		{name: "nil *discovery.ServiceCatalog", v: (*discovery.ServiceCatalog)(nil)},
		{name: "discovery.ServiceEntry", v: discovery.ServiceEntry{}},
		{name: "discovery.AuthEndpoints", v: discovery.AuthEndpoints{}},
		{name: "discovery.StaticConfig", v: discovery.StaticConfig{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, ok := tc.v.(fmt.Formatter)
			if !ok {
				t.Fatalf("%s does not implement fmt.Formatter", tc.name)
			}
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Format(nil, 'v') panicked: %v", tc.name, r)
				}
			}()
			f.Format(nil, 'v')
		})
	}
}
