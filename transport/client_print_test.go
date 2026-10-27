package transport_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// clientPrintVerbs are the fmt verbs a client is printed with: the ones a
// pointer suits, the ones it does not, and some of each with flags and a
// width.
var clientPrintVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%t", "%x", "%X", "%d", "%20v", "%-20s", "%+q", "%#x"}

// The credentials the clients of TestREQ060_ClientPrintsNoCredential hold,
// each a value the test looks for in printed text.
const (
	printStaticToken    = "STATIC-TOKEN-s3cr3t"
	printCCSecret       = "CC-SECRET-s3cr3t"
	printCCURLPass      = "CC-URLPASS-s3cr3t"
	printSmartSecret    = "SMART-SECRET-s3cr3t"
	printSmartAccess    = "SMART-ACCESS-s3cr3t"
	printSmartRefresh   = "SMART-REFRESH-s3cr3t"
	printCatalogURLPass = "CATALOG-URLPASS-s3cr3t"
)

// TestREQ060_ClientPrintsNoCredential pins REQ-060 (auth.md § REQ-060): a
// *transport.Client printed with any fmt verb, nested in a struct or a
// slice, or logged through log/slog, shows no credential of the token source
// or the reauther it holds, and masks the password in its catalog's base
// URL. Every verb writes the text of String, which shows the base URL. fmt
// calls no method of a value it reaches through an unexported field, so a
// verb that does not suit a pointer, such as %s, %q or %t, would otherwise
// print each source in full. A nil client prints as <nil>, and a client
// without a catalog prints without failing.
func TestREQ060_ClientPrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		BaseURL: "https://cdr:" + printCatalogURLPass + "@cdr.example/openehr",
		Issuer:  "https://idp.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://cdr.example/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatalf("discovery.NewStaticCatalog() error = %v, want nil", err)
	}
	ccSource, err := clientcreds.New("cc-client", printCCSecret,
		"https://svc:"+printCCURLPass+"@idp.example/token",
		clientcreds.WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatalf("clientcreds.New() error = %v, want nil", err)
	}
	smartSource, err := smart.New("smart-client", discovery.AuthEndpoints{
		AuthorizationEndpoint: &url.URL{Scheme: "https", Host: "idp.example", Path: "/authorize"},
		TokenEndpoint:         &url.URL{Scheme: "https", Host: "idp.example", Path: "/token"},
	},
		smart.WithHTTPClient(&http.Client{}),
		smart.WithClientSecret(printSmartSecret),
		smart.WithAudience("https://cdr.example/openehr"))
	if err != nil {
		t.Fatalf("smart.New() error = %v, want nil", err)
	}
	smartSource.SetTokens(auth.Token{Value: printSmartAccess, Type: auth.TokenTypeBearer}, printSmartRefresh)

	tests := []struct {
		name    string
		opt     transport.Option
		secrets []string
	}{
		{
			name:    "static token source",
			opt:     transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: printStaticToken, Type: auth.TokenTypeBearer})),
			secrets: []string{printStaticToken, printCatalogURLPass},
		},
		{
			name:    "clientcreds source with a client secret",
			opt:     transport.WithTokenSource(ccSource),
			secrets: []string{printCCSecret, printCCURLPass, printCatalogURLPass},
		},
		{
			name:    "smart source as the reauther",
			opt:     transport.WithReauthOn401(smartSource),
			secrets: []string{printSmartSecret, printSmartAccess, printSmartRefresh, printCatalogURLPass},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := transport.New(catalog, transport.WithHTTPClient(&http.Client{}), tc.opt)
			if err != nil {
				t.Fatalf("transport.New() error = %v, want nil", err)
			}
			for _, p := range clientPrintProblems(c, tc.secrets, []string{"transport.Client", "cdr.example/openehr"}) {
				t.Error(p)
			}
		})
	}

	t.Run("nil client", func(t *testing.T) {
		t.Parallel()
		var c *transport.Client
		for _, verb := range clientPrintVerbs {
			if out := fmt.Sprintf(verb, c); out != "<nil>" {
				t.Errorf("fmt.Sprintf(%q) of a nil *transport.Client = %q, want <nil>", verb, out)
			}
		}
	})

	t.Run("client without a catalog", func(t *testing.T) {
		t.Parallel()
		for _, p := range clientPrintProblems(&transport.Client{}, nil, []string{"transport.Client"}) {
			t.Error(p)
		}
	})
}

// clientPrintProblems prints c through String and GoString, with every verb
// of clientPrintVerbs, nested in a struct and a slice, and through the text
// and JSON handlers of log/slog. It returns one line for each output that
// shows one of secrets or a panic, for a String text that lacks one of
// shown, and for a verb whose output differs from the String text.
func clientPrintProblems(c *transport.Client, secrets, shown []string) []string {
	var problems []string
	leaks := func(where, out string) {
		for _, s := range secrets {
			if strings.Contains(out, s) {
				problems = append(problems, fmt.Sprintf("%s shows the credential %q: %s", where, s, out))
			}
		}
		if strings.Contains(out, "PANIC") {
			problems = append(problems, fmt.Sprintf("%s panicked: %s", where, out))
		}
	}
	var want string
	if s, ok := any(c).(fmt.Stringer); !ok {
		problems = append(problems, "*transport.Client does not implement fmt.Stringer")
	} else {
		want = s.String()
		leaks("String()", want)
		for _, s := range shown {
			if !strings.Contains(want, s) {
				problems = append(problems, fmt.Sprintf("String() = %q, want it to show %q", want, s))
			}
		}
	}
	if g, ok := any(c).(fmt.GoStringer); !ok {
		problems = append(problems, "*transport.Client does not implement fmt.GoStringer")
	} else if got := g.GoString(); got != want {
		problems = append(problems, fmt.Sprintf("GoString() = %q, want %q, the text of String()", got, want))
	}
	if _, ok := any(c).(fmt.Formatter); !ok {
		problems = append(problems, "*transport.Client does not implement fmt.Formatter")
	}
	for _, verb := range clientPrintVerbs {
		out := fmt.Sprintf(verb, c)
		leaks(fmt.Sprintf("fmt.Sprintf(%q)", verb), out)
		if want != "" && out != want {
			problems = append(problems, fmt.Sprintf("fmt.Sprintf(%q) = %q, want %q, the text of String()", verb, out, want))
		}
		leaks(fmt.Sprintf("a struct field under %q", verb), fmt.Sprintf(verb, struct{ C any }{c}))
		leaks(fmt.Sprintf("a slice element under %q", verb), fmt.Sprintf(verb, []any{c}))
	}
	for name, newHandler := range map[string]func(*bytes.Buffer) slog.Handler{
		"slog JSONHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		"slog TextHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	} {
		var buf bytes.Buffer
		slog.New(newHandler(&buf)).Info("printed", "client", c)
		leaks(name, buf.String())
	}
	return problems
}
