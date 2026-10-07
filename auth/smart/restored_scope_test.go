package smart_test

import (
	"maps"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// heldScope is the scope of the access token a restored session is given.
const heldScope = "patient/*.rs openid"

// refreshStep is one refresh a stub token endpoint answers: extra holds the
// members the response carries beyond the access token and a short lifetime
// (which makes the new token stale, so the next Token call refreshes again),
// and want is the scope every place the source reports it must have after.
type refreshStep struct {
	extra map[string]any
	want  string
}

// restoredSource returns a new source that has had no token response, its
// tokens installed with SetTokens like a session an application restores:
// held is the access token (the zero token for none) and "rt-1" the refresh
// token.
func restoredSource(t *testing.T, as *stubServer, held auth.Token, opts ...smart.Option) *smart.Source {
	t.Helper()
	src := as.source(t, as.endpoints(), opts...)
	src.SetTokens(held, "rt-1")
	return src
}

// refreshOnce answers the next token request with a refresh response that
// carries the access token access, a lifetime inside the refresh window and
// extra, then asks src for its token, which refreshes.
func refreshOnce(t *testing.T, as *stubServer, src *smart.Source, access string, extra map[string]any) auth.Token {
	t.Helper()
	body := map[string]any{"expires_in": 5}
	maps.Copy(body, extra)
	as.answerToken(0, launchBody(t, access, nil, body))
	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	if tok.Value != access {
		t.Fatalf("Token().Value = %q, want the refreshed %q", tok.Value, access)
	}
	return tok
}

// checkScope reports an error unless the token a refresh returned, the token
// the source holds, its last token response and the last token change all
// carry want.
func checkScope(t *testing.T, src *smart.Source, log *changeLog, tok auth.Token, want string) {
	t.Helper()
	if tok.Scope != want {
		t.Errorf("Token().Scope = %q, want %q", tok.Scope, want)
	}
	if held, _ := src.HeldTokens(); held.Scope != want {
		t.Errorf("held access token Scope = %q, want %q", held.Scope, want)
	}
	last := src.LastTokenResponse()
	if last.Scope != want {
		t.Errorf("LastTokenResponse().Scope = %q, want %q", last.Scope, want)
	}
	if got := last.Raw["scope"]; got != want {
		t.Errorf("LastTokenResponse().Raw[scope] = %#v, want %q", got, want)
	}
	changes := log.all()
	if len(changes) == 0 {
		t.Fatal("no token change reported, want one per refresh")
	}
	if got := changes[len(changes)-1].Access.Scope; got != want {
		t.Errorf("last TokenChange.Access.Scope = %q, want %q", got, want)
	}
}

// TestRestoredSessionKeepsItsScopeOnRefresh pins REQ-064 and REQ-063: a
// source whose tokens were installed with SetTokens has received no token
// response, so the scope of the access token it holds stands in for an
// earlier scope. A refresh response that leaves out the scope gives an access
// token with it, and the source keeps it for the refreshes after, but a
// refresh response that carries the scope, even as an empty string, replaces it.
func TestRestoredSessionKeepsItsScopeOnRefresh(t *testing.T) { // REQ-064 REQ-063
	tests := []struct {
		name  string
		steps []refreshStep
	}{
		{
			name:  "scope left out",
			steps: []refreshStep{{want: heldScope}},
		},
		{
			name:  "scope left out twice",
			steps: []refreshStep{{want: heldScope}, {want: heldScope}},
		},
		{
			name: "scope carried replaces, then left out keeps the replacement",
			steps: []refreshStep{
				{extra: map[string]any{"scope": "x"}, want: "x"},
				{want: "x"},
			},
		},
		{
			name: "scope left out, then carried",
			steps: []refreshStep{
				{want: heldScope},
				{extra: map[string]any{"scope": "x"}, want: "x"},
			},
		},
		{
			name: "scope carried as an empty string replaces, and stays empty",
			steps: []refreshStep{
				{extra: map[string]any{"scope": ""}, want: ""},
				{want: ""},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			held := staleAccess("old")
			held.Scope = heldScope
			src := restoredSource(t, as, held, smart.WithTokenChange(log.record))

			for i, step := range tc.steps {
				tok := refreshOnce(t, as, src, "at-"+strconv.Itoa(i+1), step.extra)
				checkScope(t, src, &log, tok, step.want)
			}
		})
	}
}

// TestRestoredSessionScopeCarriedAsNullReplaces pins REQ-064: a refresh
// response that carries the scope as null replaces the held token's scope with
// none, because the member is there.
func TestRestoredSessionScopeCarriedAsNullReplaces(t *testing.T) { // REQ-064
	as := newStubServer(t)
	held := staleAccess("old")
	held.Scope = heldScope
	src := restoredSource(t, as, held)

	tok := refreshOnce(t, as, src, "at-1", map[string]any{"scope": nil})
	if tok.Scope != "" {
		t.Errorf("Token().Scope = %q, want none for a refresh response whose scope is null", tok.Scope)
	}
	if got, ok := src.LastTokenResponse().Raw["scope"]; !ok || got != nil {
		t.Errorf("LastTokenResponse().Raw[scope] = %#v (present %t), want null", got, ok)
	}
}

// TestRestoredSessionWithoutAnAccessTokenKeepsNoScope pins REQ-064: a
// restored session that holds only a refresh token has no scope to stand in
// for an earlier one, so a refresh response that leaves the scope out gives an
// access token without one, and the last token response gets no scope member.
func TestRestoredSessionWithoutAnAccessTokenKeepsNoScope(t *testing.T) { // REQ-064
	as := newStubServer(t)
	src := restoredSource(t, as, auth.Token{})

	tok := refreshOnce(t, as, src, "at-1", nil)
	if tok.Scope != "" {
		t.Errorf("Token().Scope = %q, want none: the session held no access token", tok.Scope)
	}
	last := src.LastTokenResponse()
	if last.Scope != "" {
		t.Errorf("LastTokenResponse().Scope = %q, want none", last.Scope)
	}
	if got, ok := last.Raw["scope"]; ok {
		t.Errorf("LastTokenResponse().Raw[scope] = %#v, want no such member: nothing carried one", got)
	}
}

// TestInstalledTokenScopeStandsInOnlyForAnAbsentEarlierScope pins REQ-064:
// when the source has a last token response, a refresh response that leaves
// the scope out keeps the scope member that response carried, even as an
// empty string, over the scope of an access token SetTokens installed
// afterwards; only a last token response with no scope member lets the
// installed token's scope stand in.
func TestInstalledTokenScopeStandsInOnlyForAnAbsentEarlierScope(t *testing.T) { // REQ-064
	const earlier = "openid launch/patient patient/*.rs"
	tests := []struct {
		name     string
		exchange map[string]any
		want     string
	}{
		{name: "earlier response carried a scope", exchange: map[string]any{"scope": earlier}, want: earlier},
		{name: "earlier response carried an empty scope", exchange: map[string]any{"scope": ""}, want: ""},
		{name: "earlier response carried no scope", want: heldScope},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
			as.answerToken(0, launchBody(t, "at-0", tc.exchange, map[string]any{"refresh_token": "rt-1"}))
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
				t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
			}
			installed := staleAccess("installed")
			installed.Scope = heldScope
			src.SetTokens(installed, "rt-1")

			tok := refreshOnce(t, as, src, "at-1", nil)
			checkScope(t, src, &log, tok, tc.want)
		})
	}
}

// TestHeldScopeStandingInLeavesAnEarlierResponseUntouched pins REQ-064: the
// scope the held access token lends a refresh goes on a new last token
// response with its own Raw map, because LastTokenResponse hands callers the
// map the source holds, and one that kept an earlier response would see a
// scope member appear in it after a refresh.
func TestHeldScopeStandingInLeavesAnEarlierResponseUntouched(t *testing.T) { // REQ-064
	as := newStubServer(t)
	src := as.source(t, as.endpoints())
	as.answerToken(0, launchBody(t, "at-0", nil, map[string]any{"refresh_token": "rt-1"}))
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}

	before := src.LastTokenResponse()
	if before.Raw == nil {
		t.Fatal("LastTokenResponse().Raw = nil after a code exchange, want the response members")
	}
	if got, ok := before.Raw["scope"]; ok {
		t.Fatalf("LastTokenResponse().Raw[scope] = %#v after a response without one, want no such member", got)
	}
	snapshot := maps.Clone(before.Raw)

	installed := staleAccess("installed")
	installed.Scope = heldScope
	src.SetTokens(installed, "rt-1")
	refreshOnce(t, as, src, "at-1", nil)

	after := src.LastTokenResponse()
	if after.Scope != heldScope {
		t.Errorf("LastTokenResponse().Scope = %q after the refresh, want the held %q", after.Scope, heldScope)
	}
	if got := after.Raw["scope"]; got != heldScope {
		t.Errorf("LastTokenResponse().Raw[scope] = %#v after the refresh, want the held %q", got, heldScope)
	}
	if !maps.Equal(before.Raw, snapshot) {
		t.Errorf("Raw of the response returned before the refresh = %#v, want it as it was, %#v", before.Raw, snapshot)
	}
}
