package transport

// challenge_test.go — REQ-166: the WWW-Authenticate parser behind
// WireError.Challenge. Internal because the parser is unexported; what a
// consumer sees on a real 401 or 403 is pinned in reauth_challenge_test.go.

import (
	"errors"
	"maps"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// bearerChallengeCases is the parser table. The fuzz target below seeds its
// corpus from it, so every case also runs as a fuzz seed in go test.
var bearerChallengeCases = []struct {
	name  string
	lines []string
	want  *BearerChallenge
}{
	{name: "no header", lines: nil, want: nil},
	{name: "empty line", lines: []string{""}, want: nil},
	{name: "whitespace and commas only", lines: []string{" , ,\t"}, want: nil},
	{name: "bare scheme", lines: []string{"Bearer"}, want: &BearerChallenge{}},
	{name: "bare scheme with trailing space", lines: []string{"Bearer  "}, want: &BearerChallenge{}},
	{name: "realm only leaves Params nil", lines: []string{`Bearer realm="example"`}, want: &BearerChallenge{Realm: "example"}},
	{
		name:  "every named field",
		lines: []string{`Bearer realm="example", error="invalid_token", error_description="The access token expired", error_uri="https://as.example/errors/1", scope="patient/composition-*.r openid"`},
		want: &BearerChallenge{
			Realm:            "example",
			Error:            "invalid_token",
			ErrorDescription: "The access token expired",
			ErrorURI:         "https://as.example/errors/1",
			Scope:            "patient/composition-*.r openid",
		},
	},
	{
		name:  "other params go to Params",
		lines: []string{`Bearer realm="x", resource_metadata="https://rs.example/.well-known/oauth-protected-resource", max_age=300`},
		want: &BearerChallenge{Realm: "x", Params: map[string]string{
			"resource_metadata": "https://rs.example/.well-known/oauth-protected-resource",
			"max_age":           "300",
		}},
	},
	{name: "scheme lower case", lines: []string{`bearer error="invalid_token"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{name: "scheme upper case", lines: []string{`BEARER error="invalid_token"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{
		name:  "param names any case",
		lines: []string{`Bearer Error="insufficient_scope", SCOPE="a b", Realm="r", Error_Description="d", ERROR_URI="u", Resource_Metadata="m"`},
		want: &BearerChallenge{
			Realm: "r", Error: "insufficient_scope", ErrorDescription: "d", ErrorURI: "u", Scope: "a b",
			Params: map[string]string{"resource_metadata": "m"},
		},
	},
	{name: "values keep their case", lines: []string{`Bearer error="Invalid_Token"`}, want: &BearerChallenge{Error: "Invalid_Token"}},
	{name: "token values", lines: []string{`Bearer error=invalid_token, realm=x`}, want: &BearerChallenge{Realm: "x", Error: "invalid_token"}},
	{name: "quoted-string unescaped", lines: []string{`Bearer realm="a \"quoted\" \\ realm"`}, want: &BearerChallenge{Realm: `a "quoted" \ realm`}},
	{name: "escaped ordinary character", lines: []string{`Bearer realm="\a\b"`}, want: &BearerChallenge{Realm: "ab"}},
	{name: "comma inside a quoted-string", lines: []string{`Bearer scope="a, b", error="invalid_token"`}, want: &BearerChallenge{Scope: "a, b", Error: "invalid_token"}},
	{name: "obs-text kept byte for byte", lines: []string{"Bearer error_description=\"caf\xc3\xa9 \xff\""}, want: &BearerChallenge{ErrorDescription: "caf\xc3\xa9 \xff"}},
	{name: "empty quoted value", lines: []string{`Bearer realm=""`}, want: &BearerChallenge{}},
	{name: "whitespace around equals", lines: []string{"Bearer realm = \"x\" ,\terror=\tinvalid_token"}, want: &BearerChallenge{Realm: "x", Error: "invalid_token"}},
	{name: "empty list elements", lines: []string{`, Bearer realm="x",, error="invalid_token" ,`}, want: &BearerChallenge{Realm: "x", Error: "invalid_token"}},
	{name: "empty element opens the param list", lines: []string{`Bearer , error="insufficient_scope"`}, want: &BearerChallenge{Error: "insufficient_scope"}},
	{name: "empty elements open the param list", lines: []string{"Bearer ,, ,\t, error=\"insufficient_scope\", scope=\"s\""}, want: &BearerChallenge{Error: "insufficient_scope", Scope: "s"}},
	{name: "empty element straight after the scheme", lines: []string{`Bearer, error="insufficient_scope"`}, want: &BearerChallenge{Error: "insufficient_scope"}},
	{name: "doubled empty element between params", lines: []string{`Bearer error="x",, scope="y"`}, want: &BearerChallenge{Error: "x", Scope: "y"}},
	{name: "spaced empty elements between params", lines: []string{"Bearer error=\"x\" , ,\t, scope=\"y\""}, want: &BearerChallenge{Error: "x", Scope: "y"}},
	{name: "trailing empty elements", lines: []string{`Bearer error="x", scope="y",, ,`}, want: &BearerChallenge{Error: "x", Scope: "y"}},
	{name: "empty elements between challenges", lines: []string{`Basic realm="b",, ,Bearer error="x"`}, want: &BearerChallenge{Error: "x"}},
	{name: "empty elements before the next scheme", lines: []string{`Bearer ,, Basic realm="b"`}, want: &BearerChallenge{}},
	{name: "empty elements after a token68", lines: []string{`Basic abc==,, Bearer error="x"`}, want: &BearerChallenge{Error: "x"}},
	{
		name:  "Bearer after Basic on one line",
		lines: []string{`Basic realm="b", Bearer error="insufficient_scope", scope="s"`},
		want:  &BearerChallenge{Error: "insufficient_scope", Scope: "s"},
	},
	{
		name:  "Bearer before Basic on one line",
		lines: []string{`Bearer error="invalid_token", Basic realm="b"`},
		want:  &BearerChallenge{Error: "invalid_token"},
	},
	{
		name:  "bare Bearer between challenges",
		lines: []string{`Basic realm="b", Bearer, Negotiate`},
		want:  &BearerChallenge{},
	},
	{
		name:  "RFC 9110 example without Bearer",
		lines: []string{`Newauth realm="apps", type=1, title="Login to \"apps\"", Basic realm="simple"`},
		want:  nil,
	},
	{
		name:  "RFC 9110 example with Bearer last",
		lines: []string{`Newauth realm="apps", type=1, title="Login to \"apps\"", Bearer realm="simple"`},
		want:  &BearerChallenge{Realm: "simple"},
	},
	{name: "token68 challenge before Bearer", lines: []string{`Basic dXNlcjpwYXNz==, Bearer error="invalid_token"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{name: "token68 with slash and plus", lines: []string{`Negotiate a/b+c==, Bearer realm="x"`}, want: &BearerChallenge{Realm: "x"}},
	{name: "Bearer on the second header line", lines: []string{`Basic realm="b"`, `Bearer error="invalid_token"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{name: "first Bearer wins", lines: []string{`Bearer error="invalid_token"`, `Bearer error="insufficient_scope"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{name: "first Bearer on one line wins", lines: []string{`Bearer error="invalid_token", Bearer error="insufficient_scope", realm="r"`}, want: &BearerChallenge{Error: "invalid_token"}},
	{name: "DPoP only", lines: []string{`DPoP error="invalid_token", algs="ES256 PS256"`}, want: nil},
	{name: "Basic only", lines: []string{`Basic realm="b"`}, want: nil},
	{name: "scheme that starts with Bearer", lines: []string{`BearerX error="invalid_token"`}, want: nil},
	{name: "Bearer as a param name of another scheme", lines: []string{`Basic Bearer="x"`}, want: nil},

	// Malformed: the whole line is dropped, so a complete-looking Bearer
	// param before the fault does not survive it.
	{name: "unterminated quoted-string", lines: []string{`Bearer error="invalid_token`}, want: nil},
	{name: "fault after a complete param", lines: []string{`Bearer error="invalid_token", realm="x`}, want: nil},
	{name: "fault in a later challenge", lines: []string{`Bearer error="invalid_token", Basic realm="b`}, want: nil},
	{name: "missing comma between params", lines: []string{`Bearer realm="x" error="y"`}, want: nil},
	{name: "missing value", lines: []string{`Bearer realm="x", error=`}, want: nil},
	{name: "control character in quoted-string", lines: []string{"Bearer realm=\"a\x01b\""}, want: nil},
	{name: "escaped control character", lines: []string{"Bearer realm=\"a\\\x00b\""}, want: nil},
	{name: "trailing backslash", lines: []string{`Bearer realm="x\`}, want: nil},
	{name: "no space after scheme", lines: []string{`Bearer"x"`}, want: nil},
	{name: "equals straight after scheme", lines: []string{`Bearer=x`}, want: nil},
	{name: "token68 then auth-param", lines: []string{`Bearer abc=, realm="x"`}, want: nil},
	{name: "token68 then empty element then auth-param", lines: []string{`Bearer abc==,, realm="x"`}, want: nil},
	{name: "non-token scheme", lines: []string{`"Bearer" realm="x"`}, want: nil},

	// Well-formed lines whose Bearer challenge breaks RFC 6750.
	{name: "Bearer with token68", lines: []string{`Bearer abc==`}, want: nil},
	{name: "repeated param", lines: []string{`Bearer error="insufficient_scope", error="invalid_token"`}, want: nil},
	{name: "repeated param in another case", lines: []string{`Bearer error="insufficient_scope", ERROR="invalid_token"`}, want: nil},
	{name: "repeated unknown param", lines: []string{`Bearer foo=1, Foo=2`}, want: nil},
	{
		name:  "malformed Bearer skipped for a later one",
		lines: []string{`Bearer error="a", error="b", Bearer realm="r"`},
		want:  &BearerChallenge{Realm: "r"},
	},
	{
		name:  "malformed line skipped for a later line",
		lines: []string{`Bearer error="insufficient_scope`, `Bearer error="invalid_token"`},
		want:  &BearerChallenge{Error: "invalid_token"},
	},
}

// TestParseBearerChallenge — REQ-166: the Bearer challenge is found on every
// header line and inside a line listing several challenges; scheme and param
// names match in any case, values stay verbatim, and anything the parser
// cannot read leaves the result nil.
func TestParseBearerChallenge(t *testing.T) { // REQ-166
	for _, tc := range bearerChallengeCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBearerChallenge(tc.lines)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseBearerChallenge(%q)\n got  %s\n want %s", tc.lines, describeChallenge(got), describeChallenge(tc.want))
			}
		})
	}
}

// FuzzParseBearerChallenge — REQ-166: hostile header text never panics the
// parser. Each input is split on newlines into header lines. Whatever the
// parser accepts must come from a line that names the Bearer scheme, and
// must read back unchanged when written out as a Bearer challenge again.
func FuzzParseBearerChallenge(f *testing.F) { // REQ-166
	for _, tc := range bearerChallengeCases {
		f.Add(strings.Join(tc.lines, "\n"))
	}
	f.Fuzz(func(t *testing.T, in string) {
		lines := strings.Split(in, "\n")
		got := parseBearerChallenge(lines)
		if got == nil {
			return
		}
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(strings.ToLower(l), "bearer") }) {
			t.Fatalf("parseBearerChallenge(%q) = %s, but no line names the Bearer scheme", lines, describeChallenge(got))
		}
		wire := formatBearerChallenge(got)
		again := parseBearerChallenge([]string{wire})
		if !reflect.DeepEqual(again, got) {
			t.Fatalf("round trip changed the challenge\n in:   %q\n got:  %s\n wire: %q\n back: %s", lines, describeChallenge(got), wire, describeChallenge(again))
		}
	})
}

// TestChallengePermitsReauthToleratesABoxedNilWireError — REQ-166, REQ-025:
// the reauth gate reads a field off the extracted *WireError, so a boxed
// typed nil answers false instead of panicking, while a real 401 still
// reaches the challenge check.
func TestChallengePermitsReauthToleratesABoxedNilWireError(t *testing.T) { // REQ-166
	if got := challengePermitsReauth(boxedNilWireError()); got {
		t.Errorf("challengePermitsReauth(boxed typed-nil WireError) = true, want false")
	}
	if got := challengePermitsReauth(errors.New("not a wire error")); got {
		t.Errorf("challengePermitsReauth(plain error) = true, want false")
	}
	noChallenge := &WireError{StatusCode: 401, Sentinel: ErrUnauthorized}
	if got := challengePermitsReauth(noChallenge); !got {
		t.Errorf("challengePermitsReauth(401 without a challenge) = false, want true")
	}
	scoped := &WireError{StatusCode: 401, Sentinel: ErrUnauthorized, Challenge: &BearerChallenge{Error: "insufficient_scope"}}
	if got := challengePermitsReauth(scoped); got {
		t.Errorf("challengePermitsReauth(401 insufficient_scope) = true, want false")
	}
}

// TestParseBearerChallengeLineLimit — REQ-166: a header line longer than
// 8 KiB is skipped without being parsed, one of exactly 8 KiB still parses,
// and the lines beside a skipped one are still read.
func TestParseBearerChallengeLineLimit(t *testing.T) { // REQ-166
	// challengeOfLen returns a valid Bearer challenge exactly n bytes long.
	challengeOfLen := func(n int) string {
		const prefix = `Bearer error="insufficient_scope", realm="`
		return prefix + strings.Repeat("r", n-len(prefix)-1) + `"`
	}
	atLimit, overLimit := challengeOfLen(8192), challengeOfLen(8193)
	if len(atLimit) != 8192 || len(overLimit) != 8193 {
		t.Fatalf("premise gone: built lines of %d and %d bytes, want 8192 and 8193", len(atLimit), len(overLimit))
	}
	cases := []struct {
		name      string
		lines     []string
		wantError string // "" means a nil challenge
	}{
		{name: "8192-byte line parses", lines: []string{atLimit}, wantError: "insufficient_scope"},
		{name: "8193-byte line is skipped", lines: []string{overLimit}},
		{name: "line after a skipped one is read", lines: []string{overLimit, `Bearer error="invalid_token"`}, wantError: "invalid_token"},
		{name: "line before a skipped one is read", lines: []string{`Basic realm="b"`, overLimit, `Bearer error="invalid_token"`}, wantError: "invalid_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBearerChallenge(tc.lines)
			switch {
			case tc.wantError == "" && got != nil:
				t.Errorf("parseBearerChallenge(lines of %v bytes) = challenge with Error %q, want nil", lineLens(tc.lines), got.Error)
			case tc.wantError != "" && got == nil:
				t.Errorf("parseBearerChallenge(lines of %v bytes) = nil, want Error %q", lineLens(tc.lines), tc.wantError)
			case tc.wantError != "" && got.Error != tc.wantError:
				t.Errorf("parseBearerChallenge(lines of %v bytes) = challenge with Error %q, want Error %q", lineLens(tc.lines), got.Error, tc.wantError)
			}
		})
	}
}

// TestParseBearerChallengeBoundedWork — REQ-166: hostile header lines cost
// a bounded amount of memory. A line over the limit is skipped unread, and
// a line within it is scanned one challenge at a time, building only the
// Bearer challenge it returns, so a line full of other challenges
// allocates nothing.
func TestParseBearerChallengeBoundedWork(t *testing.T) { // REQ-166
	distinctParams := func(n int) string {
		var b strings.Builder
		b.WriteString("Bearer ")
		for i := 0; b.Len() < n; i++ {
			b.WriteString("p" + strconv.Itoa(i) + "=1, ")
		}
		return b.String()
	}
	cases := []struct {
		name string
		line string
	}{
		{name: "1 MiB of bare schemes", line: strings.Repeat("a,", 1<<19)},
		{name: "1 MiB of distinct Bearer params", line: distinctParams(1 << 20)},
		{name: "8 KiB of bare schemes", line: strings.Repeat("a,", 4096)},
		{name: "8 KiB of Basic challenges with quoted params", line: strings.Repeat(`Basic realm="a\"b", `, 8192/20)},
	}
	const maxBytesPerParse = 1 << 10
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{tc.line}
			if got := parseBearerChallenge(lines); got != nil {
				t.Errorf("parseBearerChallenge(%d-byte hostile line) = challenge with Error %q, want nil", len(tc.line), got.Error)
			}
			if got := heapBytesPerRun(20, func() { _ = parseBearerChallenge(lines) }); got > maxBytesPerParse {
				t.Errorf("parseBearerChallenge(%d-byte hostile line) allocates %d bytes per call, want at most %d", len(tc.line), got, maxBytesPerParse)
			}
		})
	}
}

// heapBytesPerRun reports the average heap bytes one call of f allocates,
// measured the way testing.AllocsPerRun counts allocations: one warm-up
// call, then runs calls with GOMAXPROCS at 1 so other goroutines add little.
func heapBytesPerRun(runs int, f func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		f()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

func lineLens(lines []string) []int {
	out := make([]int, len(lines))
	for i, l := range lines {
		out[i] = len(l)
	}
	return out
}

// formatBearerChallenge writes c as one Bearer challenge, every value as a
// quoted-string, Params in name order.
func formatBearerChallenge(c *BearerChallenge) string {
	var params []string
	add := func(name, value string) {
		v := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
		params = append(params, name+`="`+v+`"`)
	}
	named := []struct{ name, value string }{
		{"realm", c.Realm},
		{"error", c.Error},
		{"error_description", c.ErrorDescription},
		{"error_uri", c.ErrorURI},
		{"scope", c.Scope},
	}
	for _, p := range named {
		if p.value != "" {
			add(p.name, p.value)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(c.Params)) {
		add(k, c.Params[k])
	}
	if len(params) == 0 {
		return "Bearer"
	}
	return "Bearer " + strings.Join(params, ", ")
}

// describeChallenge prints c with %q values, so a stray byte shows.
func describeChallenge(c *BearerChallenge) string {
	if c == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("{")
	for _, f := range []struct{ name, value string }{
		{"Realm", c.Realm},
		{"Error", c.Error},
		{"ErrorDescription", c.ErrorDescription},
		{"ErrorURI", c.ErrorURI},
		{"Scope", c.Scope},
	} {
		b.WriteString(f.name + ":" + strconv.Quote(f.value) + " ")
	}
	if c.Params == nil {
		b.WriteString("Params:nil}")
		return b.String()
	}
	b.WriteString("Params:{")
	for _, k := range slices.Sorted(maps.Keys(c.Params)) {
		b.WriteString(strconv.Quote(k) + ":" + strconv.Quote(c.Params[k]) + " ")
	}
	b.WriteString("}}")
	return b.String()
}
