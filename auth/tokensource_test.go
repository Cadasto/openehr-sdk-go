package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStaticTokenSource(t *testing.T) {
	tok := Token{Value: "abc", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
	ts := StaticTokenSource(tok)
	got, err := ts.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != tok {
		t.Errorf("Token returned %+v, want %+v", got, tok)
	}
}

func TestStaticTokenSourceHonoursCtx(t *testing.T) {
	ts := StaticTokenSource(Token{Value: "abc"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ts.Token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAnonymousTokenSource(t *testing.T) {
	ts := AnonymousTokenSource()
	got, err := ts.Token(t.Context())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero token, got %+v", got)
	}
}

func TestTokenIsZero(t *testing.T) {
	tests := []struct {
		name string
		tok  Token
		want bool
	}{
		{"zero", Token{}, true},
		{"value only", Token{Value: "x"}, false},
		{"type only", Token{Type: "Bearer"}, false},
		{"populated", Token{Value: "x", Type: "Bearer"}, false},
		{"expires only", Token{ExpiresAt: time.Now()}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tok.IsZero(); got != tc.want {
				t.Errorf("IsZero(%+v) = %v, want %v", tc.tok, got, tc.want)
			}
		})
	}
}

func TestWithTokenSource(t *testing.T) {
	ts := StaticTokenSource(Token{Value: "ctx-bound"})
	ctx := WithTokenSource(t.Context(), ts)
	got, ok := TokenSourceFromContext(ctx)
	if !ok {
		t.Fatal("TokenSourceFromContext returned ok=false")
	}
	if got != ts {
		t.Errorf("got %v, want %v", got, ts)
	}
}

func TestWithTokenSourceNil(t *testing.T) {
	ctx := WithTokenSource(t.Context(), nil)
	if _, ok := TokenSourceFromContext(ctx); ok {
		t.Error("nil TokenSource should not be attached to ctx")
	}
}

func TestTokenSourceFromContextAbsent(t *testing.T) {
	if _, ok := TokenSourceFromContext(t.Context()); ok {
		t.Error("expected no TokenSource on a context that carries none")
	}
}

// TestScopeConstants pins the lexical value of each launch-context and refresh
// scope constant so that accidental renames are caught immediately. [REQ-061]
func TestScopeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ScopeOpenID", ScopeOpenID, "openid"},
		{"ScopeProfile", ScopeProfile, "profile"},
		{"ScopeFHIRUser", ScopeFHIRUser, "fhirUser"},
		{"ScopeLaunch", ScopeLaunch, "launch"},
		{"ScopeLaunchPatient", ScopeLaunchPatient, "launch/patient"},
		{"ScopeLaunchEpisode", ScopeLaunchEpisode, "launch/episode"},
		{"ScopeOfflineAccess", ScopeOfflineAccess, "offline_access"},
		{"ScopeOnlineAccess", ScopeOnlineAccess, "online_access"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestJoinScopesWithConstants verifies that the scope constants round-trip
// correctly through JoinScopes. [REQ-061]
func TestJoinScopesWithConstants(t *testing.T) {
	got := JoinScopes(ScopeOpenID, ScopeLaunchPatient, ScopeOfflineAccess)
	want := "openid launch/patient offline_access"
	if got != want {
		t.Errorf("JoinScopes(ScopeOpenID, ScopeLaunchPatient, ScopeOfflineAccess) = %q, want %q", got, want)
	}
}

func TestBuildScope(t *testing.T) {
	tests := []struct {
		compartment, resource, permission string
		want                              string
	}{
		{"patient", "COMPOSITION", "read", "patient/COMPOSITION.read"},
		{"", "COMPOSITION", "read", "COMPOSITION.read"},
		{"patient", "COMPOSITION", "", "patient/COMPOSITION"},
		{"", "COMPOSITION", "", "COMPOSITION"},
		{" patient ", " * ", " write ", "patient/*.write"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := BuildScope(tc.compartment, tc.resource, tc.permission); got != tc.want {
				t.Errorf("BuildScope(%q, %q, %q) = %q, want %q", tc.compartment, tc.resource, tc.permission, got, tc.want)
			}
		})
	}
}

func TestJoinScopes(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a b"},
		{[]string{"a", "", "b", "   "}, "a b"},
	}
	for _, tc := range tests {
		got := JoinScopes(tc.in...)
		if got != tc.want {
			t.Errorf("JoinScopes(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestReautherFuncSatisfiesInterface verifies that ReautherFunc implements
// the Reauther interface and delegates to its underlying function (REQ-063).
func TestReautherFuncSatisfiesInterface(t *testing.T) {
	called := false
	var r Reauther = ReautherFunc(func(_ context.Context) error {
		called = true
		return nil
	})
	if err := r.Reauth(t.Context()); err != nil {
		t.Fatalf("Reauth returned unexpected error: %v", err)
	}
	if !called {
		t.Error("ReautherFunc.Reauth did not call the underlying function")
	}
}

func TestExchangeError(t *testing.T) {
	inner := errors.New("network unreachable")
	oa := &OAuth2Error{Code: "invalid_grant", Description: "stale code"}
	e := &ExchangeError{Sentinel: ErrTokenExchangeFailed, StatusCode: 400, OAuth2: oa, Inner: inner}

	if !errors.Is(e, ErrTokenExchangeFailed) {
		t.Error("errors.Is(_, ErrTokenExchangeFailed) failed")
	}
	if !errors.Is(e, inner) {
		t.Error("errors.Is(_, inner) failed")
	}
	var got *OAuth2Error
	if !errors.As(e, &got) {
		t.Error("errors.As(_, &OAuth2Error) failed")
	}
	if got != oa {
		t.Errorf("errors.As returned %v, want %v", got, oa)
	}
}
