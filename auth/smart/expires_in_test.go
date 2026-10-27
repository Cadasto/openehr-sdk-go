package smart_test

import (
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// TestREQ063_LargeExpiresInGivesAFutureExpiry pins REQ-063 (auth.md § Token
// refresh): an expires_in too large to count in nanoseconds as a
// time.Duration still gives the access token an expiry in the future, so the
// source keeps using the token and does not refresh it on every call.
func TestREQ063_LargeExpiresInGivesAFutureExpiry(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn string
		// atLeast is the shortest lifetime the token may get; exact asks for
		// exactly that lifetime.
		atLeast time.Duration
		exact   bool
	}{
		{name: "an hour", expiresIn: "3600", atLeast: time.Hour, exact: true},
		{name: "just past the time.Duration range", expiresIn: "9223372037", atLeast: 50 * 365 * 24 * time.Hour},
		{name: "ten billion seconds", expiresIn: "10000000000", atLeast: 50 * 365 * 24 * time.Hour},
		{name: "the largest int64", expiresIn: "9223372036854775807", atLeast: 50 * 365 * 24 * time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
					return scriptedAnswer(r, http.StatusOK,
						`{"access_token":"at-new","token_type":"Bearer","expires_in":`+tc.expiresIn+`}`), nil
				}}
				src := scriptedSource(t, srv)
				src.SetTokens(auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}, "rt-0")
				start := time.Now()

				tok, err := src.Token(t.Context())
				if err != nil || tok.Value != "at-new" {
					t.Fatalf("Token() = (%q, %v), want (at-new, nil)", tok.Value, err)
				}
				lifetime := tok.ExpiresAt.Sub(start)
				if lifetime < tc.atLeast || (tc.exact && lifetime != tc.atLeast) {
					t.Errorf("expires_in %s gives the token a lifetime of %v, want %v at least (exactly: %t)", tc.expiresIn, lifetime, tc.atLeast, tc.exact)
				}
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-new" {
					t.Errorf("second Token() = (%q, %v), want (at-new, nil)", tok.Value, err)
				}
				if n, _ := srv.counts(); n != 1 {
					t.Errorf("refresh requests = %d, want 1: the refreshed token is still valid", n)
				}
			})
		})
	}
}

// expiresInForms are expires_in members in each form a JSON number takes,
// with the lifetime each gives the access token. A value of zero or below is
// a lifetime of zero seconds (RFC 6749 §5.1), so the token is due at once. A
// member that is missing or null, or a number a float64 cannot hold, gives
// the token no expiry. A member that is not a number fails the response, as
// it did before.
var expiresInForms = []struct {
	name      string
	expiresIn string // the member's JSON text; empty leaves the member out
	want      time.Duration
	none      bool // the token gets no expiry
	wantErr   bool
}{
	{name: "integer", expiresIn: `3600`, want: time.Hour},
	{name: "zero fraction", expiresIn: `3600.0`, want: time.Hour},
	{name: "exponent", expiresIn: `3.6e3`, want: time.Hour},
	{name: "capital exponent", expiresIn: `36E2`, want: time.Hour},
	{name: "fraction rounded down", expiresIn: `3600.5`, want: time.Hour},
	{name: "integer in a string", expiresIn: `"3600"`, want: time.Hour},
	{name: "exponent in a string", expiresIn: `"3.6e3"`, want: time.Hour},
	{name: "largest int64", expiresIn: `9223372036854775807`, want: hundredYears},
	{name: "integer beyond int64", expiresIn: `9223372036854775808`, want: hundredYears},
	{name: "large exponent", expiresIn: `1e300`, want: hundredYears},
	{name: "zero", expiresIn: `0`},
	{name: "zero with a fraction", expiresIn: `0.0`},
	{name: "negative integer", expiresIn: `-5`},
	{name: "negative fraction", expiresIn: `-0.5`},
	{name: "smallest int64", expiresIn: `-9223372036854775808`},
	{name: "missing", expiresIn: ``, none: true},
	{name: "null", expiresIn: `null`, none: true},
	{name: "beyond float64", expiresIn: `1e400`, none: true},
	{name: "not a number", expiresIn: `"soon"`, wantErr: true},
}

// hundredYears is the longest lifetime an expires_in gives a token.
const hundredYears = 100 * 365 * 24 * time.Hour

// TestREQ063_ExpiresInReadsEveryJSONNumberForm pins REQ-063 (auth.md § Token
// refresh): RFC 6749 §5.1 makes expires_in "the lifetime in seconds of the
// access token", a JSON number, so a fraction, an exponent or an integer
// beyond the int64 range gives the token an expiry, rounded down to whole
// seconds and at most 100 years, and the source does not refresh it on
// every call while the token is valid. A value of zero or below is a
// lifetime of zero seconds, so the next call renews the token, and a member
// that is missing or null gives the token no expiry.
func TestREQ063_ExpiresInReadsEveryJSONNumberForm(t *testing.T) {
	for _, tc := range expiresInForms {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
					return scriptedAnswer(r, http.StatusOK,
						expiresInBody("at-new", tc.expiresIn)), nil
				}}
				src := scriptedSource(t, srv)
				src.SetTokens(auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}, "rt-0")
				start := time.Now()

				tok, err := src.Token(t.Context())
				if tc.wantErr {
					if !errors.Is(err, auth.ErrRefreshFailed) || tok.Value != "" {
						t.Errorf("Token() = (%q, %v), want no token and an error matching auth.ErrRefreshFailed", tok.Value, err)
					}
					return
				}
				if err != nil || tok.Value != "at-new" {
					t.Fatalf("Token() = (%q, %v), want (at-new, nil)", tok.Value, err)
				}
				checkLifetime(t, tc.expiresIn, tok, start, tc.want, tc.none)
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-new" {
					t.Errorf("second Token() = (%q, %v), want (at-new, nil)", tok.Value, err)
				}
				if n, _ := srv.counts(); n != renewals(tc.want, tc.none) {
					t.Errorf("refresh requests = %d, want %d: a token with a lifetime of zero is due at once, any other is still valid", n, renewals(tc.want, tc.none))
				}
			})
		})
	}
}

// renewals is how many refresh requests two Token calls send when the token
// endpoint always answers with a lifetime of want, or none: two when that
// lifetime is zero, so the token is due at once, and one otherwise.
func renewals(want time.Duration, none bool) int {
	if want == 0 && !none {
		return 2
	}
	return 1
}

// expiresInBody is a token response carrying access and, unless expiresIn
// is empty, an expires_in member whose JSON text is expiresIn.
func expiresInBody(access, expiresIn string) string {
	member := ""
	if expiresIn != "" {
		member = `,"expires_in":` + expiresIn
	}
	return `{"access_token":"` + access + `","token_type":"Bearer"` + member + `}`
}

// checkLifetime reports an error unless tok, a token the source got at
// start from a response whose expires_in is expiresIn, lives for want, or
// has no expiry when none is set.
func checkLifetime(t *testing.T, expiresIn string, tok auth.Token, start time.Time, want time.Duration, none bool) {
	t.Helper()
	if none {
		if !tok.ExpiresAt.IsZero() {
			t.Errorf("expires_in %s gives the token an expiry %v after it arrived, want none", expiresIn, tok.ExpiresAt.Sub(start))
		}
		return
	}
	if tok.ExpiresAt.IsZero() {
		t.Errorf("expires_in %s gives the token no expiry, want a lifetime of %v", expiresIn, want)
	} else if got := tok.ExpiresAt.Sub(start); got != want {
		t.Errorf("expires_in %s gives the token a lifetime of %v, want %v", expiresIn, got, want)
	}
}

// TestREQ063_TokenResponseExpiresInConvertsToADuration pins REQ-063 (auth.md
// § Token refresh) at the public field: TokenResponse.ExpiresIn holds every
// form of expires_in rounded down to whole seconds and kept between minus
// and plus 100 years, an integer included, so a caller can turn it into a
// time.Duration without overflow.
func TestREQ063_TokenResponseExpiresInConvertsToADuration(t *testing.T) {
	const maxSeconds = int64(hundredYears / time.Second)
	tests := []struct {
		expiresIn string // the member's JSON text
		want      int64
	}{
		{expiresIn: `3600`, want: 3600},
		{expiresIn: `"3600"`, want: 3600},
		{expiresIn: `3600.5`, want: 3600},
		{expiresIn: `3153600000`, want: maxSeconds},
		{expiresIn: `3153600001`, want: maxSeconds},
		{expiresIn: `9223372036854775807`, want: maxSeconds},
		{expiresIn: `9223372036854775808`, want: maxSeconds},
		{expiresIn: `1e300`, want: maxSeconds},
		{expiresIn: `-5`, want: -5},
		{expiresIn: `-9223372036854775808`, want: -maxSeconds},
		{expiresIn: `-1e300`, want: -maxSeconds},
		{expiresIn: `1e400`, want: 0},
	}
	for _, tc := range tests {
		tr, err := smart.ParseTokenResponse([]byte(`{"access_token":"at","expires_in":` + tc.expiresIn + `}`))
		if err != nil {
			t.Errorf("ParseTokenResponse(expires_in %s) error = %v, want nil", tc.expiresIn, err)
			continue
		}
		if tr.ExpiresIn != tc.want {
			t.Errorf("ParseTokenResponse(expires_in %s).ExpiresIn = %d, want %d", tc.expiresIn, tr.ExpiresIn, tc.want)
		}
	}
}

// TestREQ063_TokenResponseTellsAMissingExpiresInFromZero pins REQ-063
// (auth.md § Token refresh): TokenResponse.HasExpiresIn reports whether the
// response carried expires_in as a number, so an ExpiresIn of 0 from a
// response that says 0 can be told from one that says nothing.
func TestREQ063_TokenResponseTellsAMissingExpiresInFromZero(t *testing.T) {
	tests := []struct {
		expiresIn string // the member's JSON text; empty leaves the member out
		want      int64
		wantHas   bool
	}{
		{expiresIn: `3600`, want: 3600, wantHas: true},
		{expiresIn: `0`, want: 0, wantHas: true},
		{expiresIn: `-5`, want: -5, wantHas: true},
		{expiresIn: ``},
		{expiresIn: `null`},
		{expiresIn: `1e400`},
	}
	for _, tc := range tests {
		tr, err := smart.ParseTokenResponse([]byte(expiresInBody("at", tc.expiresIn)))
		if err != nil {
			t.Errorf("ParseTokenResponse(expires_in %q) error = %v, want nil", tc.expiresIn, err)
			continue
		}
		if tr.ExpiresIn != tc.want || tr.HasExpiresIn != tc.wantHas {
			t.Errorf("ParseTokenResponse(expires_in %q) = (ExpiresIn %d, HasExpiresIn %t), want (%d, %t)", tc.expiresIn, tr.ExpiresIn, tr.HasExpiresIn, tc.want, tc.wantHas)
		}
	}
}
