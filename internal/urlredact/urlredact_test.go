package urlredact_test

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// TestString pins each branch of String: a URL url.Parse accepts is masked
// by url.URL.Redacted only when it has a password, and a string url.Parse
// refuses is masked from the first ":" after "//" to the last "@". The
// strings it cannot find a password in come back unchanged.
func TestString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "parsed, password",
			raw:  "https://svc:s3cret@x.example/token",
			want: "https://svc:xxxxx@x.example/token",
		},
		{
			name: "parsed, relative, password",
			raw:  "//svc:s3cret@x.example/token",
			want: "//svc:xxxxx@x.example/token",
		},
		{
			// url.Parse takes the last "@" in the authority, so the
			// password holds the first one.
			name: "parsed, password with @",
			raw:  "https://svc:s3@cret@x.example/token",
			want: "https://svc:xxxxx@x.example/token",
		},
		{
			name: "parsed, user name only",
			raw:  "https://svc@x.example/token",
			want: "https://svc@x.example/token",
		},
		{
			name: "parsed, no userinfo",
			raw:  "https://x.example/token?a=b#c",
			want: "https://x.example/token?a=b#c",
		},
		{
			name: "parsed, empty",
			raw:  "",
			want: "",
		},
		{
			name: "unparseable, password",
			raw:  "https://svc:s3cret@[::1",
			want: "https://svc:xxxxx@[::1",
		},
		{
			name: "unparseable, bad escape in host",
			raw:  "https://svc:s3cret@x%zz.example/token",
			want: "https://svc:xxxxx@x%zz.example/token",
		},
		{
			// The "/" ends the authority early, so url.Parse reads
			// "s3" as a port and refuses it.
			name: "unparseable, password with /",
			raw:  "https://svc:s3/cret@x.example/token",
			want: "https://svc:xxxxx@x.example/token",
		},
		{
			name: "unparseable, password with ?",
			raw:  "https://svc:s3?cret@x.example/token",
			want: "https://svc:xxxxx@x.example/token",
		},
		{
			name: "unparseable, password with #",
			raw:  "https://svc:s3#cret@x.example/token",
			want: "https://svc:xxxxx@x.example/token",
		},
		{
			name: "unparseable, password with @",
			raw:  "https://svc:s3@cret@[::1",
			want: "https://svc:xxxxx@[::1",
		},
		{
			name: "unparseable, no //",
			raw:  ":svc:s3cret@x.example",
			want: ":svc:s3cret@x.example",
		},
		{
			name: "unparseable, no @",
			raw:  "https://x.example:s3cret[",
			want: "https://x.example:s3cret[",
		},
		{
			name: "unparseable, no : before the @",
			raw:  "https://svc@[::1",
			want: "https://svc@[::1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := urlredact.String(tc.raw); got != tc.want {
				t.Errorf("String(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestStringParsesAsDocumented checks that each "parsed" and "unparseable"
// case of TestString takes the url.Parse branch its name says, so a case
// cannot drift into the other branch unnoticed.
func TestStringParsesAsDocumented(t *testing.T) {
	t.Parallel()
	parsed := []string{
		"https://svc:s3cret@x.example/token",
		"//svc:s3cret@x.example/token",
		"https://svc:s3@cret@x.example/token",
	}
	for _, raw := range parsed {
		if _, err := url.Parse(raw); err != nil {
			t.Errorf("url.Parse(%q) error = %v, want nil", raw, err)
		}
	}
	unparseable := []string{
		"https://svc:s3cret@[::1",
		"https://svc:s3cret@x%zz.example/token",
		"https://svc:s3/cret@x.example/token",
		"https://svc:s3?cret@x.example/token",
		"https://svc:s3#cret@x.example/token",
		"https://svc:s3@cret@[::1",
		":svc:s3cret@x.example",
		"https://x.example:s3cret[",
		"https://svc@[::1",
	}
	for _, raw := range unparseable {
		if _, err := url.Parse(raw); err == nil {
			t.Errorf("url.Parse(%q) error = nil, want an error", raw)
		}
	}
}

// TestError checks that Error masks the URL of the *url.Error in err's
// chain in place, keeps its Op and Err, and returns err itself; that an
// error with no *url.Error comes back as it was; and that a typed-nil
// *url.Error does not panic.
func TestError(t *testing.T) {
	t.Parallel()

	t.Run("url.Parse error", func(t *testing.T) {
		t.Parallel()
		ue := parseError(t, "https://svc:s3cret@[::1")
		err := error(ue)
		op, inner := ue.Op, ue.Err
		if got := urlredact.Error(err); got != err { //nolint:errorlint // identity is the point
			t.Errorf("Error(err) = %v, want err itself", got)
		}
		if want := "https://svc:xxxxx@[::1"; ue.URL != want {
			t.Errorf("url.Error.URL = %q, want %q", ue.URL, want)
		}
		if ue.Op != op || ue.Err != inner { //nolint:errorlint // identity is the point
			t.Errorf("url.Error Op, Err = %q, %v; want %q, %v unchanged", ue.Op, ue.Err, op, inner)
		}
	})

	t.Run("url.Parse error in a chain", func(t *testing.T) {
		t.Parallel()
		// The *url.Error is masked wherever it sits in the chain. A
		// wrapper made by fmt.Errorf has its text already, so callers
		// mask before they wrap; the wrapper here only shows the search.
		perr := parseError(t, "https://svc:s3cret@x%zz.example/token")
		err := fmt.Errorf("issuer: %w", perr)
		if got := urlredact.Error(err); got != err { //nolint:errorlint // identity is the point
			t.Errorf("Error(err) = %v, want err itself", got)
		}
		want := `parse "https://svc:xxxxx@x%zz.example/token": invalid URL escape "%zz"`
		if got := perr.Error(); got != want {
			t.Errorf("wrapped url.Error text = %q, want %q", got, want)
		}
	})

	t.Run("no url.Error", func(t *testing.T) {
		t.Parallel()
		err := errors.New("https://svc:s3cret@x.example")
		if got := urlredact.Error(err); got != err { //nolint:errorlint // identity is the point
			t.Errorf("Error(%v) = %v, want it unchanged", err, got)
		}
		if got := urlredact.Error(nil); got != nil {
			t.Errorf("Error(nil) = %v, want nil", got)
		}
	})

	t.Run("typed-nil url.Error", func(t *testing.T) {
		t.Parallel()
		var ue *url.Error
		err := error(ue)
		if got := urlredact.Error(err); got != err { //nolint:errorlint // identity is the point
			t.Errorf("Error(typed nil) = %v, want it unchanged", got)
		}
	})
}

// parseError returns the *url.Error url.Parse returns for raw, which must be
// a string url.Parse refuses.
func parseError(t *testing.T, raw string) *url.Error {
	t.Helper()
	_, err := url.Parse(raw)
	ue, ok := errors.AsType[*url.Error](err)
	if !ok || ue == nil {
		t.Fatalf("url.Parse(%q) error = %v (%T), want a *url.Error", raw, err, err)
	}
	return ue
}
