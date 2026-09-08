package probe_test

import (
	"errors"
	"os"
	"path/filepath"
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
