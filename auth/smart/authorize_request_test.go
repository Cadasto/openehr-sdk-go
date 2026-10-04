package smart_test

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// launchSource builds a source that needs no live server, with the given
// scopes and issuer.
func launchSource(t *testing.T, issuer string, scopes ...string) *smart.Source {
	t.Helper()
	src, err := newSource("client-id", audienceTestEndpoints(),
		smart.WithHTTPClient(http.DefaultClient),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer(issuer),
		smart.WithScopes(scopes...),
	)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	return src
}

// authorizeQueryFor builds the authorization URL for req and launch and
// returns its query.
func authorizeQueryFor(t *testing.T, src *smart.Source, req smart.AuthorizationRequest, launch string) url.Values {
	t.Helper()
	raw, err := src.AuthorizeURL(req, launch)
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return parsed.Query()
}

// TestBeginAuthorizationRecordsIssuer pins REQ-061: the request records the
// issuer the source is bound to, the catalog's OpenID Connect issuer and
// not its Platform base URL.
func TestBeginAuthorizationRecordsIssuer(t *testing.T) { // REQ-061
	const (
		baseURL = "https://platform.example/openehr"
		issuer  = "https://idp.example/realms/clinic"
	)
	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		BaseURL: baseURL,
		Issuer:  issuer,
		Auth:    audienceTestEndpoints(),
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	src, err := smart.NewFromCatalog(catalog, "client-id", smart.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("NewFromCatalog: %v", err)
	}
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if req.Issuer != issuer {
		t.Errorf("BeginAuthorization().Issuer = %q, want the catalog issuer %q", req.Issuer, issuer)
	}
}

// TestBeginAuthorizationNonceWithOpenID pins REQ-061: with openid among the
// configured scopes the request carries a fresh nonce of at least 32 random
// bytes, base64url-encoded, and the authorization URL sends it once as
// nonce. The scopes may arrive as separate values or as one
// space-separated string.
func TestBeginAuthorizationNonceWithOpenID(t *testing.T) { // REQ-061
	tests := []struct {
		name   string
		scopes []string
	}{
		{name: "openid as its own scope", scopes: []string{"openid", "patient/COMPOSITION.read"}},
		{name: "openid inside a space-separated scope string", scopes: []string{"launch/patient openid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := launchSource(t, "https://idp.example", tc.scopes...)
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(req.Nonce)
			if err != nil {
				t.Fatalf("Nonce %q is not unpadded base64url: %v", req.Nonce, err)
			}
			if len(raw) < 32 {
				t.Errorf("Nonce decodes to %d bytes, want at least 32", len(raw))
			}
			again, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			if again.Nonce == req.Nonce {
				t.Errorf("two launches got the same nonce %q, want a fresh one each", req.Nonce)
			}
			q := authorizeQueryFor(t, src, req, "")
			if got := q["nonce"]; !slices.Equal(got, []string{req.Nonce}) {
				t.Errorf("authorization URL nonce = %q, want exactly [%q]", got, req.Nonce)
			}
		})
	}
}

// TestBeginAuthorizationNoNonceWithoutOpenID pins REQ-061: without openid
// among the configured scopes no nonce is generated or sent.
func TestBeginAuthorizationNoNonceWithoutOpenID(t *testing.T) { // REQ-061
	src := launchSource(t, "https://idp.example", "launch/patient", "patient/COMPOSITION.read")
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if req.Nonce != "" {
		t.Errorf("BeginAuthorization().Nonce = %q, want empty without openid", req.Nonce)
	}
	if q := authorizeQueryFor(t, src, req, ""); q.Has("nonce") {
		t.Errorf("authorization URL carries nonce %q, want none without openid", q["nonce"])
	}
}

// TestAuthorizeURLLaunchAddsLaunchScope pins REQ-061: when AuthorizeURL is
// given a launch value the scope it sends contains launch exactly once,
// added when the configured scopes lack it; without a launch value the
// configured scopes go out unchanged.
func TestAuthorizeURLLaunchAddsLaunchScope(t *testing.T) { // REQ-061
	tests := []struct {
		name      string
		scopes    []string
		launch    string
		wantScope []string // the scope parameter, split on spaces; nil means absent
	}{
		{
			name:      "launch added to scopes that lack it",
			scopes:    []string{"openid", "patient/COMPOSITION.read"},
			launch:    "xyz123",
			wantScope: []string{"openid", "patient/COMPOSITION.read", "launch"},
		},
		{
			name:      "launch already configured is not repeated",
			scopes:    []string{"launch", "openid"},
			launch:    "xyz123",
			wantScope: []string{"launch", "openid"},
		},
		{
			name:      "launch inside a space-separated scope string is not repeated",
			scopes:    []string{"openid launch"},
			launch:    "xyz123",
			wantScope: []string{"openid", "launch"},
		},
		{
			name:      "no configured scopes still sends launch",
			launch:    "xyz123",
			wantScope: []string{"launch"},
		},
		{
			name:      "no launch value leaves the scopes alone",
			scopes:    []string{"openid", "patient/COMPOSITION.read"},
			wantScope: []string{"openid", "patient/COMPOSITION.read"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := launchSource(t, "https://idp.example", tc.scopes...)
			req, err := src.BeginAuthorization("state-1")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			q := authorizeQueryFor(t, src, req, tc.launch)
			var got []string
			if q.Has("scope") {
				got = strings.Fields(q.Get("scope"))
			}
			if !slices.Equal(got, tc.wantScope) {
				t.Errorf("AuthorizeURL(launch=%q) scope = %q, want %q", tc.launch, got, tc.wantScope)
			}
			if tc.launch != "" && q.Get("launch") != tc.launch {
				t.Errorf("AuthorizeURL(launch=%q) launch = %q, want it forwarded unchanged", tc.launch, q.Get("launch"))
			}
		})
	}
}

// TestAuthorizeURLLaunchScopeLeavesConfigAlone pins that adding the launch
// scope to one authorization URL writes nothing into the caller's scope
// slice, whose spare capacity an in-place append would fill (and which
// concurrent launches would then race on), and does not change the scopes
// the next URL starts from.
func TestAuthorizeURLLaunchScopeLeavesConfigAlone(t *testing.T) { // REQ-061
	scopes := make([]string, 1, 4)
	scopes[0] = "openid"
	src := launchSource(t, "https://idp.example", scopes...)
	req, err := src.BeginAuthorization("state-1")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	_ = authorizeQueryFor(t, src, req, "xyz123")
	if spare := scopes[1:cap(scopes)]; slices.ContainsFunc(spare, func(s string) bool { return s != "" }) {
		t.Errorf("AuthorizeURL wrote %q into the caller's scope slice", spare)
	}
	q := authorizeQueryFor(t, src, req, "")
	if got := strings.Fields(q.Get("scope")); !slices.Equal(got, []string{"openid"}) {
		t.Errorf("scope after an earlier embedded launch = %q, want [openid]", got)
	}
}

// TestAuthorizeURLFallsBackToRequestLaunch pins REQ-061: with no launch
// argument AuthorizeURL sends the request's Launch, with the launch scope
// it needs; a launch argument wins over it.
func TestAuthorizeURLFallsBackToRequestLaunch(t *testing.T) { // REQ-061
	tests := []struct {
		name       string
		reqLaunch  string
		argLaunch  string
		wantLaunch string
	}{
		{name: "request launch only", reqLaunch: "from-request", wantLaunch: "from-request"},
		{name: "argument wins", reqLaunch: "from-request", argLaunch: "from-argument", wantLaunch: "from-argument"},
		{name: "argument only", argLaunch: "from-argument", wantLaunch: "from-argument"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := launchSource(t, "https://idp.example", "openid")
			req, err := src.BeginAuthorization("state-1")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			req.Launch = tc.reqLaunch
			q := authorizeQueryFor(t, src, req, tc.argLaunch)
			if got := q["launch"]; !slices.Equal(got, []string{tc.wantLaunch}) {
				t.Errorf("AuthorizeURL(req.Launch=%q, launch=%q) launch = %q, want [%q]", tc.reqLaunch, tc.argLaunch, got, tc.wantLaunch)
			}
			if got := strings.Fields(q.Get("scope")); !slices.Contains(got, "launch") {
				t.Errorf("AuthorizeURL(req.Launch=%q, launch=%q) scope = %q, want it to contain launch", tc.reqLaunch, tc.argLaunch, got)
			}
		})
	}
}
