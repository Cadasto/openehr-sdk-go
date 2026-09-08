package probe_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// strand11HAR is the live capture that settled ADR 0020 — the same
// bytes the PR reviews. go test runs in the package directory, so a
// plain relative path reaches it.
var strand11HAR = filepath.Join("..", "..", "docs", "plans", "strand-11-evidence", "ehr-create.har")

// credentialValue is the fake secret every unredacted-capture case
// carries. No refusal may echo it: REQ-093 lets a diagnostic name a
// header or query key, never its value.
const credentialValue = "s3cr3t-must-not-appear-in-any-message"

// harFile writes raw to a .har file under the test's own temp
// directory and returns the path. ValidateHAR reads from disk, so a
// case's JSON has to land there first.
func harFile(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording.har")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// harWith wraps an entries array in a log whose version and _req082
// attestation are both valid, so a case built on it can only fail on
// the entries it varies — in particular, redaction.ran is true, which
// is the point: the attestation claiming redaction ran must not excuse
// a credential still sitting in the bytes.
func harWith(entries string) string {
	return `{"log":{"version":"1.2","_req082":{"provenance":{"deployment":"d","base_url":"http://x","captured_at":"t","sdk_commit":"c"},"redaction":{"ran":true}},"entries":` + entries + `}}`
}

// harResponseBody, harRequestURL and harRedirectURL each put one value
// in an otherwise-valid single-entry recording, so a case varies only
// the channel it is about. Each value is quoted rather than
// hand-escaped, which lets a case be written as the JSON or the URL it
// actually is.
func harResponseBody(body string) string {
	return harWith(`[{"request":{"method":"GET","url":"http://x"},` +
		`"response":{"status":200,"content":{"text":` + strconv.Quote(body) + `}}}]`)
}

func harRequestURL(rawURL string) string {
	return harWith(`[{"request":{"method":"GET","url":` + strconv.Quote(rawURL) + `},"response":{"status":200}}]`)
}

func harRedirectURL(rawURL string) string {
	return harWith(`[{"request":{"method":"GET","url":"http://x"},` +
		`"response":{"status":302,"redirectURL":` + strconv.Quote(rawURL) + `}}]`)
}

func TestHARAcceptsStrand11Capture(t *testing.T) {
	t.Parallel()
	rec, err := probe.ValidateHAR(strand11HAR)
	if err != nil {
		t.Fatalf("ValidateHAR(%q) error = %v, want nil", strand11HAR, err)
	}
	if got, want := rec.Log.Entries[0].Response.Status, 201; got != want {
		t.Fatalf("ValidateHAR(%q) entry 0 response status = %d, want %d", strand11HAR, got, want)
	}
}

func TestHARRejectsMissingAttestation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string // a substring only this guard's message carries
	}{
		{
			name: "no _req082",
			raw:  `{"log":{"version":"1.2","entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
			want: "log._req082 is required",
		},
		{
			name: "redaction did not run",
			raw:  `{"log":{"version":"1.2","_req082":{"provenance":{"deployment":"x","captured_at":"t","sdk_commit":"c"},"redaction":{"ran":false}},"entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
			want: "redaction.ran is not true",
		},
		{
			name: "empty provenance",
			raw:  `{"log":{"version":"1.2","_req082":{"provenance":{},"redaction":{"ran":true}},"entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
			want: "provenance is incomplete",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, tc.raw, tc.want)
		})
	}
}

// TestHARRejectsMalformedRecording pins the refusals that have nothing
// to do with the attestation block: bytes that are not a HAR at all, a
// log version replay does not understand, and a recording with nothing
// in it. Each must reach the caller as [probe.ErrUnsatisfiableMode] so
// one sentinel covers "discard this recording" (REQ-082).
func TestHARRejectsMalformedRecording(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "truncated file",
			raw:  `{"log":{"version":"1.2","entries":[`,
			want: "decode HAR",
		},
		{
			name: "wrong type for log.version",
			raw:  `{"log":{"version":12,"entries":[]}}`,
			want: "decode HAR",
		},
		{
			name: "unsupported log version",
			raw:  `{"log":{"version":"1.1","_req082":{"provenance":{"deployment":"d","captured_at":"t","sdk_commit":"c"},"redaction":{"ran":true}},"entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
			want: `log.version = "1.1"`,
		},
		{
			name: "no entries",
			raw:  harWith(`[]`),
			want: "log.entries is empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, tc.raw, tc.want)
		})
	}
}

// TestHARRejectsUnredactedCapture pins the content half of REQ-082's
// redaction rule. Every case here attests redaction.ran = true and
// still carries a credential, so passing them would mean the runner
// trusts the capture tool's claim instead of the recorded bytes — the
// difference between an unredacted capture being detectable and merely
// being unlikely.
func TestHARRejectsUnredactedCapture(t *testing.T) {
	t.Parallel()
	requestWith := func(headers string) string {
		return harWith(`[{"request":{"method":"GET","url":"http://x","headers":[` + headers + `]},"response":{"status":200}}]`)
	}
	responseWith := func(headers string) string {
		return harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{"status":200,"headers":[` + headers + `]}}]`)
	}
	header := func(name string) string {
		return `{"name":"` + name + `","value":"` + credentialValue + `"}`
	}

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "request authorization header",
			raw:  requestWith(header("Authorization")),
			want: `entry 0 request header "authorization"`,
		},
		{
			name: "request cookie header",
			raw:  requestWith(header("COOKIE")),
			want: `entry 0 request header "cookie"`,
		},
		{
			name: "request proxy-authorization header",
			raw:  requestWith(header("Proxy-Authorization")),
			want: `entry 0 request header "proxy-authorization"`,
		},
		{
			name: "response set-cookie header",
			raw:  responseWith(header("Set-Cookie")),
			want: `entry 0 response header "set-cookie"`,
		},
		{
			name: "access_token in the request URL",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://x/ehr?access_token=` + credentialValue + `"},"response":{"status":200}}]`),
			want: `entry 0 request URL query key "access_token"`,
		},
		{
			name: "credential in a later entry is still found",
			raw: harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}},` +
				`{"request":{"method":"GET","url":"http://x","headers":[` + header("authorization") + `]},"response":{"status":200}}]`),
			want: `entry 1 request header "authorization"`,
		},
		{
			name: "userinfo in the request URL",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://operator:` + credentialValue + `@x/ehr"},"response":{"status":200}}]`),
			want: "entry 0 request URL userinfo",
		},
		{
			name: "access_token in the response body",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{"status":200,"content":{"text":"{\"access_token\":\"` + credentialValue + `\"}"}}}]`),
			want: `entry 0 response body carries "access_token"`,
		},
		{
			name: "authorization key in the response body",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{"status":200,"content":{"text":"{\"authorization\":\"` + credentialValue + `\"}"}}}]`),
			want: `entry 0 response body carries "authorization"`,
		},
		{
			name: "bearer token in the request body",
			raw: harWith(`[{"request":{"method":"POST","url":"http://x","postData":{"text":"assertion=Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"}},` +
				`"response":{"status":200}}]`),
			want: `entry 0 request body carries "bearer"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, tc.raw, tc.want)
		})
	}
}

// TestHARRejectsUnreadableRecording pins the read guard: a path the
// validator cannot open is refused on the same sentinel as a bad
// recording, so a caller that discards on [probe.ErrUnsatisfiableMode]
// never has to tell a missing file from a malformed one.
func TestHARRejectsUnreadableRecording(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "absent.har")
	_, err := probe.ValidateHAR(path)
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("ValidateHAR(%q) error = %v, want it to wrap %v", path, err, probe.ErrUnsatisfiableMode)
	}
	if !strings.Contains(err.Error(), "read HAR") {
		t.Fatalf("ValidateHAR(%q) error = %q, want it to say the recording could not be read", path, err)
	}
}

// assertRefused validates raw as a recording and requires the refusal
// to carry both the shared sentinel and want — a distinguishing
// substring, so a failing case says which guard fired rather than only
// that something refused. It also requires the message to be
// value-free (REQ-093).
func assertRefused(t *testing.T, raw, want string) {
	t.Helper()
	path := harFile(t, raw)
	_, err := probe.ValidateHAR(path)
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("ValidateHAR(%q) error = %v, want it to wrap %v", path, err, probe.ErrUnsatisfiableMode)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("ValidateHAR(%q) error = %q, want it to contain %q so the message names the guard that fired", path, err, want)
	}
	if strings.Contains(err.Error(), credentialValue) {
		t.Fatalf("ValidateHAR(%q) error = %q, want it never to echo the credential value (REQ-093)", path, err)
	}
}

// TestHARRejectsUnreplayableEntry pins the per-entry structural check.
// Replay needs a method, a URL it can parse, and a status code; an
// entry missing one is a capture that went wrong, and refusing it here
// — where a recording is being chosen — beats letting it surface later
// as an unmatched request or a zero-status response inside a probe.
func TestHARRejectsUnreplayableEntry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no request method",
			raw:  harWith(`[{"request":{"method":"","url":"http://x"},"response":{"status":200}}]`),
			want: "entry 0 request.method is empty",
		},
		{
			name: "no request URL",
			raw:  harWith(`[{"request":{"method":"GET"},"response":{"status":200}}]`),
			want: "entry 0 request.url is empty",
		},
		{
			name: "unparseable request URL",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://[::1:8080/ehr"},"response":{"status":200}}]`),
			want: "entry 0 request.url does not parse",
		},
		{
			name: "no response status",
			raw:  harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{}}]`),
			want: "entry 0 response.status is not set",
		},
		{
			name: "the second entry is the incomplete one",
			raw: harWith(`[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}},` +
				`{"request":{"method":"GET","url":"http://x"},"response":{"status":0}}]`),
			want: "entry 1 response.status is not set",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, tc.raw, tc.want)
		})
	}
}

// TestHARAcceptsClinicalBodyThatReadsLikeACredential is the other half
// of the body scan: it has to stay narrow enough for clinical content.
// "Basic metabolic panel" is a lab test, "bearer of the card" is a
// sentence, and the words token, secret and password all appear in
// ordinary clinical text — a scan that refused those would reject
// sound recordings far more often than leaked ones, and the corpus
// would learn to work around it.
func TestHARAcceptsClinicalBodyThatReadsLikeACredential(t *testing.T) {
	t.Parallel()
	body := `{\"name\":\"Basic metabolic panel\",\"note\":\"the bearer of the card; token given at reception, password on file\"}`
	raw := harWith(`[{"request":{"method":"POST","url":"http://x/ehr","postData":{"text":"` + body + `"}},` +
		`"response":{"status":201,"content":{"text":"` + body + `"}}}]`)
	path := harFile(t, raw)
	if _, err := probe.ValidateHAR(path); err != nil {
		t.Fatalf("ValidateHAR(clinical body) error = %v, want nil — the body scan must not fire on clinical text", err)
	}
}

// TestHARAuthorizationMarkerIsAKey pins where the word authorization
// counts: in JSON key position, not anywhere in the body. A JSON value
// is quoted just like a key, so a scan for the quoted word alone would
// refuse {"value":"Authorization"} — an ELEMENT named Authorization,
// which is clinical content, not a leak. Reverting the check to a
// plain substring match must fail the accepted cases below.
//
// The refused bodies carry "Bearer x", which is far under
// [probe.ValidateHAR]'s credential-run threshold, so the auth-scheme
// rule cannot fire on them: the refusal can only come from the key
// match, which is what isolates it here.
func TestHARAuthorizationMarkerIsAKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string // the refusal substring; empty means the body must be accepted
	}{
		{
			name: "the word as a JSON value is clinical content",
			body: `{"value":"Authorization"}`,
		},
		{
			name: "an element named Authorization is clinical content",
			body: `{"name":"Authorization","value":"x"}`,
		},
		{
			name: "the word as a JSON key is a leak",
			body: `{"authorization":"Bearer x"}`,
			want: `entry 0 response body carries "authorization"`,
		},
		{
			name: "capitalised, with a space before the colon",
			body: `{"Authorization" : "Bearer x"}`,
			want: `entry 0 response body carries "authorization"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := harResponseBody(tc.body)
			if tc.want != "" {
				assertRefused(t, raw, tc.want)
				return
			}
			path := harFile(t, raw)
			if _, err := probe.ValidateHAR(path); err != nil {
				t.Fatalf("ValidateHAR(%s) error = %v, want nil — the word authorization outside key position is clinical content", tc.body, err)
			}
		})
	}
}

// TestHARRejectsCredentialInRedirectURL pins the response redirectURL
// as a scanned channel. A redirect Location is a URL the recording
// carries just like the request URL, and an OAuth flow puts the token
// straight into it — so the query-key and userinfo checks have to
// reach it too. The field was modelled and never read until this test.
func TestHARRejectsCredentialInRedirectURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		redirect string
		want     string
	}{
		{
			name:     "access_token in the redirect query",
			redirect: "http://x/callback?access_token=" + credentialValue,
			want:     `entry 0 response redirectURL query key "access_token"`,
		},
		{
			name:     "userinfo in the redirect authority",
			redirect: "http://user:" + credentialValue + "@x/",
			want:     "entry 0 response redirectURL userinfo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, harRedirectURL(tc.redirect), tc.want)
		})
	}
}

// TestHARRejectsBasicCredentialInBody is the true positive for the
// second auth scheme, which nothing else covered. "Basic metabolic
// panel" has to stay accepted — that is
// [TestHARAcceptsClinicalBodyThatReadsLikeACredential] — but a real
// Basic credential is base64 and runs far past the threshold, so the
// pair reads as a leaked header value rather than an English phrase.
// Deleting "basic " from the scheme list must fail this test.
func TestHARRejectsBasicCredentialInBody(t *testing.T) {
	t.Parallel()
	const encoded = "dXNlcjpzdXBlcnNlY3JldA==" // "user:supersecret"
	path := harFile(t, harResponseBody(`{"note":"Basic `+encoded+`"}`))
	_, err := probe.ValidateHAR(path)
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("ValidateHAR(basic credential) error = %v, want it to wrap %v", err, probe.ErrUnsatisfiableMode)
	}
	if want := `entry 0 response body carries "basic"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("ValidateHAR(basic credential) error = %q, want it to contain %q so the message names the scheme that fired", err, want)
	}
	if strings.Contains(err.Error(), encoded) {
		t.Fatalf("ValidateHAR(basic credential) error = %q, want it never to echo the encoded credential (REQ-093)", err)
	}
}

// TestHARRefusesEveryCredentialQueryKey walks the query-key denylist
// one entry at a time, the way the header cases already do — a set
// covered by a single example is a set whose other entries can be
// dropped unnoticed. The keys are spelled out here because har_test is
// an external test package and cannot read the unexported set, which
// is the point: removing one from har.go must fail the case named
// after it.
func TestHARRefusesEveryCredentialQueryKey(t *testing.T) {
	t.Parallel()
	keys := []string{
		"access_token",
		"api_key",
		"apikey",
		"authorization",
		"client_secret",
		"password",
		"token",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			raw := harRequestURL("http://x/ehr?" + key + "=" + credentialValue)
			assertRefused(t, raw, `entry 0 request URL query key "`+key+`"`)
		})
	}
}

// TestHARRefusesEveryBodyMarker is the same sweep over the body
// markers: each OAuth or OIDC field name in JSON key position, one
// case per marker, so removing one from har.go fails the case that
// names it. The authorization key has its own test —
// [TestHARAuthorizationMarkerIsAKey] — because it is matched by
// position rather than as a plain substring.
func TestHARRefusesEveryBodyMarker(t *testing.T) {
	t.Parallel()
	markers := []string{
		"access_token",
		"refresh_token",
		"id_token",
		"client_secret",
	}
	for _, marker := range markers {
		t.Run(marker, func(t *testing.T) {
			t.Parallel()
			raw := harResponseBody(`{"` + marker + `":"` + credentialValue + `"}`)
			assertRefused(t, raw, `entry 0 response body carries "`+marker+`"`)
		})
	}
}

// TestHARRefusesMalformedEscapeCredentialKey pins why the query is
// split by hand instead of through [net/url.ParseQuery]: that function
// drops every pair whose percent-escapes it cannot decode, so one
// malformed pair can carry a credential key out of the scan. The three
// cases are the three shapes that matters for — a malformed escape
// beside the credential, an escaped spelling of the key itself, and a
// malformed escape inside the credential's own value, which is the one
// ParseQuery drops even when it returns what it could parse.
func TestHARRefusesMalformedEscapeCredentialKey(t *testing.T) {
	t.Parallel()
	const want = `entry 0 request URL query key "access_token"`
	cases := []struct {
		name  string
		query string
	}{
		{
			name:  "a malformed escape in another pair",
			query: "junk=%ZZ&access_token=" + credentialValue,
		},
		{
			name:  "a percent-encoded credential key",
			query: "%61ccess_token=" + credentialValue,
		},
		{
			name:  "a malformed escape in the credential's own value",
			query: "access_token=%ZZ" + credentialValue,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRefused(t, harRequestURL("http://x/ehr?"+tc.query), want)
		})
	}
}
