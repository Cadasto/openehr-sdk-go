package smart_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
	appsmart "github.com/cadasto/openehr-sdk-go/smart"
)

// setBody sets what the endpoint answers from now on.
func (te *tokenEndpoint) setBody(body string) {
	te.mu.Lock()
	te.body = body
	te.mu.Unlock()
}

// launchMember is a token-response member that a refresh response may leave
// out while the session keeps it: a SMART launch-context parameter or the
// granted scope.
type launchMember struct {
	key string
	// typed reads the member's typed TokenResponse field. It is nil for
	// fhirContext, which has none and lives in Raw only.
	typed func(smart.TokenResponse) any
	// earlier and later are the member's JSON values in the code exchange's
	// body and in the refresh's body.
	earlier, later any
	// empty is the member's empty JSON value: "" for a string, null else.
	empty any
}

func bannerValue(tr smart.TokenResponse) any {
	if tr.NeedPatientBanner == nil {
		return nil
	}
	return *tr.NeedPatientBanner
}

var launchMembers = []launchMember{
	{key: "patient", typed: func(tr smart.TokenResponse) any { return tr.Patient }, earlier: "P1", later: "P2", empty: ""},
	{key: "encounter", typed: func(tr smart.TokenResponse) any { return tr.Encounter }, earlier: "E1", later: "E2", empty: ""},
	{key: "ehrId", typed: func(tr smart.TokenResponse) any { return tr.EHRID }, earlier: "ehr-1", later: "ehr-2", empty: ""},
	{key: "episodeId", typed: func(tr smart.TokenResponse) any { return tr.EpisodeID }, earlier: "ep-1", later: "ep-2", empty: ""},
	{
		key:     "fhirContext",
		earlier: []any{map[string]any{"reference": "DiagnosticReport/1"}},
		later:   []any{map[string]any{"reference": "DiagnosticReport/2"}},
		empty:   nil,
	},
	{key: "intent", typed: func(tr smart.TokenResponse) any { return tr.Intent }, earlier: "reconcile-medications", later: "review", empty: ""},
	{key: "need_patient_banner", typed: bannerValue, earlier: false, later: true, empty: nil},
	{
		key:     "smart_style_url",
		typed:   func(tr smart.TokenResponse) any { return tr.SMARTStyleURL },
		earlier: "https://ehr.example/style-1.json",
		later:   "https://ehr.example/style-2.json",
		empty:   "",
	},
	{key: "tenant", typed: func(tr smart.TokenResponse) any { return tr.Tenant }, earlier: "tenant-1", later: "tenant-2", empty: ""},
	{
		key:     "scope",
		typed:   func(tr smart.TokenResponse) any { return tr.Scope },
		earlier: "openid launch/patient patient/*.rs",
		later:   "openid patient/*.rs",
		empty:   "",
	},
}

// launchBody is a token-endpoint success body with access token access and
// the given members, plus extra.
func launchBody(t *testing.T, access string, members, extra map[string]any) string {
	t.Helper()
	m := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 3600}
	maps.Copy(m, members)
	maps.Copy(m, extra)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// memberValues returns each launch member's value as pick gives it, leaving
// out the member named omit.
func memberValues(pick func(launchMember) any, omit string) map[string]any {
	out := map[string]any{}
	for _, m := range launchMembers {
		if m.key != omit {
			out[m.key] = pick(m)
		}
	}
	return out
}

func earlierValue(m launchMember) any { return m.earlier }
func laterValue(m launchMember) any   { return m.later }
func emptyValue(m launchMember) any   { return m.empty }

// exchangeLaunch completes a code exchange on src whose response is body,
// then marks the access token stale so the next Token call refreshes.
func exchangeLaunch(t *testing.T, te *tokenEndpoint, src *smart.Source, body string) smart.TokenResponse {
	t.Helper()
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	te.setBody(body)
	_, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	src.SetTokens(staleAccess("at-1"), "rt-1")
	return tr
}

// checkMember reports an error unless tr carries member m with value want,
// both in Raw and, when m has one, in its typed field.
func checkMember(t *testing.T, tr smart.TokenResponse, m launchMember, want any) {
	t.Helper()
	got, ok := tr.Raw[m.key]
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("LastTokenResponse().Raw[%q] = %#v (present %t), want %#v", m.key, got, ok, want)
	}
	if m.typed == nil {
		return
	}
	if got := m.typed(tr); !reflect.DeepEqual(got, want) {
		t.Errorf("LastTokenResponse() typed %s = %#v, want %#v", m.key, got, want)
	}
}

// TestRefreshKeepsOmittedLaunchContext pins REQ-064: when a refresh response
// leaves out a launch-context parameter or the scope that the last token
// response carried, the source keeps the earlier value on its last token
// response, in the typed field and in Raw; every member the refresh carries
// replaces the earlier value. Raw's other members are the refresh's own, and
// the code exchange's response is not changed.
func TestRefreshKeepsOmittedLaunchContext(t *testing.T) { // REQ-064
	cases := []string{""}
	for _, m := range launchMembers {
		cases = append(cases, m.key)
	}
	for _, omitted := range cases {
		name := "omits " + omitted
		if omitted == "" {
			name = "omits nothing"
		}
		t.Run(name, func(t *testing.T) {
			te := newTokenEndpoint(t, "")
			src := completeSource(t, te, false)
			exchanged := exchangeLaunch(t, te, src,
				launchBody(t, "at-1", memberValues(earlierValue, ""), map[string]any{"refresh_token": "rt-1", "exchange_only": "x"}))
			before := maps.Clone(exchanged.Raw)

			te.setBody(launchBody(t, "at-2", memberValues(laterValue, omitted), nil))
			if _, err := src.Token(t.Context()); err != nil {
				t.Fatalf("Token() error = %v, want a refreshed token", err)
			}

			last := src.LastTokenResponse()
			for _, m := range launchMembers {
				want := m.later
				if m.key == omitted {
					want = m.earlier
				}
				checkMember(t, last, m, want)
			}
			if got := last.Raw["access_token"]; got != "at-2" {
				t.Errorf("LastTokenResponse().Raw[access_token] = %v, want the refresh's at-2", got)
			}
			if got, ok := last.Raw["exchange_only"]; ok {
				t.Errorf("LastTokenResponse().Raw[exchange_only] = %v, want no such member: only launch context and scope are kept", got)
			}
			if !reflect.DeepEqual(exchanged.Raw, before) {
				t.Errorf("the code exchange's TokenResponse.Raw changed to %v, want it left as %v", exchanged.Raw, before)
			}
		})
	}
}

// TestRefreshEmptyLaunchContextReplaces pins REQ-064: a member the refresh
// response carries replaces the earlier value even when it is an empty
// string or null, because the member is there. Only a member left out keeps
// the earlier value.
func TestRefreshEmptyLaunchContextReplaces(t *testing.T) { // REQ-064
	te := newTokenEndpoint(t, "")
	src := completeSource(t, te, false)
	exchangeLaunch(t, te, src, launchBody(t, "at-1", memberValues(earlierValue, ""), map[string]any{"refresh_token": "rt-1"}))

	te.setBody(launchBody(t, "at-2", memberValues(emptyValue, ""), nil))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	last := src.LastTokenResponse()
	for _, m := range launchMembers {
		checkMember(t, last, m, m.empty)
	}
}

// TestLaunchContextAfterRefreshKeepsLaunchContext pins REQ-064: a
// LaunchContext rebuilt from LastTokenResponse after refreshes that leave the
// launch context and the scope out still has them.
func TestLaunchContextAfterRefreshKeepsLaunchContext(t *testing.T) { // REQ-064
	te := newTokenEndpoint(t, "")
	src := completeSource(t, te, false)
	exchangeLaunch(t, te, src, launchBody(t, "at-1", memberValues(earlierValue, ""), map[string]any{"refresh_token": "rt-1"}))

	for i, access := range []string{"at-2", "at-3"} {
		te.setBody(launchBody(t, access, nil, nil))
		if _, err := src.Token(t.Context()); err != nil {
			t.Fatalf("refresh %d: Token() error = %v, want a refreshed token", i+1, err)
		}
		src.SetTokens(staleAccess(access), "rt-1")
	}

	lc, err := appsmart.LaunchContextFromTokenResponse(t.Context(), src.LastTokenResponse())
	if err != nil {
		t.Fatalf("LaunchContextFromTokenResponse(LastTokenResponse()) error = %v", err)
	}
	if lc.Patient != "P1" || lc.Encounter != "E1" || lc.EHRID != "ehr-1" || lc.EpisodeID != "ep-1" ||
		lc.Intent != "reconcile-medications" || lc.SMARTStyleURL != "https://ehr.example/style-1.json" || lc.Tenant != "tenant-1" {
		t.Errorf("LaunchContext after two refreshes = %+v, want the code exchange's launch context", lc)
	}
	if want := strings.Fields("openid launch/patient patient/*.rs"); !slices.Equal(lc.Scopes, want) {
		t.Errorf("LaunchContext.Scopes after two refreshes = %q, want the code exchange's %q", lc.Scopes, want)
	}
	if lc.NeedPatientBanner == nil || *lc.NeedPatientBanner {
		t.Errorf("LaunchContext.NeedPatientBanner after two refreshes = %v, want the code exchange's false", lc.NeedPatientBanner)
	}
	want := []any{map[string]any{"reference": "DiagnosticReport/1"}}
	if got := lc.Raw["fhirContext"]; !reflect.DeepEqual(got, want) {
		t.Errorf("LaunchContext.Raw[fhirContext] after two refreshes = %#v, want the code exchange's %#v", got, want)
	}
}

// TestRefreshKeepsTheScopeOnTheAccessToken pins REQ-064 and REQ-063: the
// access token a refresh installs carries the scope the session keeps, so
// when the refresh response leaves the scope out, Token, the held token and
// the token change all have the earlier grant; a scope the refresh carries
// replaces it.
func TestRefreshKeepsTheScopeOnTheAccessToken(t *testing.T) { // REQ-064 REQ-063
	const earlier = "openid launch/patient patient/*.rs"
	tests := []struct {
		name  string
		extra map[string]any
		want  string
	}{
		{name: "scope left out", want: earlier},
		{name: "scope carried", extra: map[string]any{"scope": "openid patient/*.rs"}, want: "openid patient/*.rs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
			as.answerToken(0, launchBody(t, "at-1", map[string]any{"scope": earlier}, map[string]any{"refresh_token": "rt-1"}))
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
				t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
			}
			src.SetTokens(staleAccess("at-1"), "rt-1")

			as.answerToken(0, launchBody(t, "at-2", nil, tc.extra))
			tok, err := src.Token(t.Context())
			if err != nil {
				t.Fatalf("Token() error = %v, want a refreshed token", err)
			}
			if tok.Scope != tc.want {
				t.Errorf("Token().Scope = %q, want %q", tok.Scope, tc.want)
			}
			if held, _ := src.HeldTokens(); held.Scope != tc.want {
				t.Errorf("held access token Scope = %q, want %q", held.Scope, tc.want)
			}
			if got := src.LastTokenResponse().Scope; got != tc.want {
				t.Errorf("LastTokenResponse().Scope = %q, want %q", got, tc.want)
			}
			if changes := log.all(); len(changes) != 2 || changes[1].Access.Scope != tc.want {
				t.Errorf("refresh TokenChange.Access.Scope = %+v, want %q", changes, tc.want)
			}
		})
	}
}
