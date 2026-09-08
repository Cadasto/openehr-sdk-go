package probe

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// HAR is an HTTP Archive 1.2 recording (ADR 0020). Cassette mode
// reads this shape; a recording that fails [ValidateHAR] MUST be
// discarded rather than replayed (REQ-082).
type HAR struct {
	Log HARLog `json:"log"`
}

// HARLog is the HAR 1.2 log object. Provenance and redaction live on
// [_req082] as a custom field — HAR has no native slot for them
// (ADR 0020).
type HARLog struct {
	Version string      `json:"version"`
	Creator *HARCreator `json:"creator,omitempty"`
	Comment string      `json:"comment,omitempty"`
	Req082  *HARReq082  `json:"_req082"`
	Entries []HAREntry  `json:"entries"`
}

// HARReq082 is the REQ-082 attestation carried on every Cassette
// recording. Removing either required field must fail
// TestHARRejectsMissingAttestation in har_test.go.
type HARReq082 struct {
	Provenance HARProvenance `json:"provenance"`
	Redaction  HARRedaction  `json:"redaction"`
}

// HARProvenance names the deployment the recording came from.
type HARProvenance struct {
	Deployment string `json:"deployment"`
	BaseURL    string `json:"base_url"`
	CapturedAt string `json:"captured_at"`
	SDKCommit  string `json:"sdk_commit"`
}

// HARRedaction records that capture-time redaction ran.
type HARRedaction struct {
	Ran             bool     `json:"ran"`
	HeadersStripped []string `json:"headers_stripped,omitempty"`
}

// HARCreator is the HAR 1.2 creator object.
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// HAREntry is one request/response pair. Fields unused by replay
// (timings, cache) are omitted from the type so they are ignored.
type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime,omitempty"`
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
}

// HARRequest is the HAR 1.2 request object.
type HARRequest struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion,omitempty"`
	Headers     []HARHeader `json:"headers,omitempty"`
	BodySize    int         `json:"bodySize,omitempty"`
	PostData    *HARPost    `json:"postData,omitempty"`
}

// HARResponse is the HAR 1.2 response object.
type HARResponse struct {
	Status      int         `json:"status"`
	StatusText  string      `json:"statusText,omitempty"`
	HTTPVersion string      `json:"httpVersion,omitempty"`
	Headers     []HARHeader `json:"headers,omitempty"`
	Content     HARContent  `json:"content"`
	RedirectURL string      `json:"redirectURL,omitempty"`
	BodySize    int         `json:"bodySize,omitempty"`
}

// HARHeader is one HAR header.
type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARPost is the optional request body.
type HARPost struct {
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// HARContent is the response body.
type HARContent struct {
	Size     int    `json:"size,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// credentialHeaders are the header names capture-time redaction has to
// strip. The _req082 attestation is a claim about the capture tool;
// this set is the check on the recorded bytes, so a tool that reported
// a redaction it never performed is caught rather than trusted
// (REQ-082: an unredacted capture must be detectable, not merely
// unlikely).
var credentialHeaders = map[string]struct{}{
	"authorization":       {},
	"cookie":              {},
	"proxy-authorization": {},
	"set-cookie":          {},
}

// A HAR cookies[] array is deliberately not modelled. The types above
// carry no cookies field, so this validator sees a cookie only when it
// rides in the Cookie or Set-Cookie header, which the set above
// refuses. A cookie carried in the cookies[] array alone is invisible
// here and would be caught only by the recorder redacting it at
// capture time — which is where cookies belong anyway, since REQ-082
// requires redaction at capture time, never at review time.

// credentialQueryKeys are request-URL query keys that carry a
// credential in the URL itself, where stripping headers never reaches.
var credentialQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"authorization": {},
	"client_secret": {},
	"password":      {},
	"token":         {},
}

// bodyCredentialMarkers are substrings whose presence in a recorded
// body means a credential survived capture-time redaction. The set is
// deliberately narrow, because the bodies here are clinical openEHR
// JSON: a generic word like "token", "secret" or "password" turns up
// as legitimate content — an archetype term, a template path, a note —
// so scanning for those would refuse sound recordings far more often
// than leaked ones. Every marker below is an OAuth or OIDC field name
// clinical content has no reason to carry, wherever in the body it
// turns up.
//
// The word authorization is not in this list: it is a real word, and a
// JSON value is quoted just like a key, so even `"authorization"` as a
// substring would refuse {"value":"Authorization"} — an ELEMENT named
// Authorization. It is checked in key position instead, by
// [authorizationKeyPresent].
var bodyCredentialMarkers = []string{
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
}

// bodyAuthorizationKey is the JSON key an Authorization header lands
// under when a capture copies headers into a body. The quotes are part
// of the needle; the name a refusal reports is the word without them.
const bodyAuthorizationKey = `"authorization"`

// authSchemes are the HTTP authentication schemes an Authorization
// header value opens with, looked for separately from
// [bodyCredentialMarkers] because the word on its own proves nothing:
// "Basic metabolic panel" is a lab result. A match counts only when
// the scheme is followed by a credential-shaped run — see
// [credentialRun].
var authSchemes = []string{"bearer ", "basic "}

// credentialRunMin is how many credential-shaped characters must
// follow an auth scheme before the pair reads as a leaked header value
// rather than an English phrase. An encoded credential is far longer
// than this; a clinical phrase breaks at its first space well before
// it.
//
// The threshold is a deliberate trade, and it has a known miss: a
// Basic credential shorter than sixteen base64 characters sitting in
// body text goes uncaught. Lowering it to catch that would start
// refusing clinical phrases — "Basic metabolic panel" is the standing
// example — and a scan that refuses sound recordings is one the corpus
// learns to work around. The defence that catches the short credential
// is capture-time redaction, which sees the Authorization header
// before anything copies it into a body (REQ-082).
const credentialRunMin = 16

// ValidateHAR reads the HAR 1.2 recording at path and refuses one that
// must not be replayed (REQ-082): unreadable or malformed bytes, the
// wrong log version, a missing or incomplete ADR 0020 attestation, an
// empty entries list, an entry replay could not use, or a credential
// the capture-time redaction left behind. Every refusal wraps
// [ErrUnsatisfiableMode], so a caller can discard the recording on the
// sentinel alone.
//
// A refusal names the offending entry and the channel — the header,
// the query key, the URL's userinfo, the body marker — and never the
// value or any recorded payload text (REQ-093). Removing the _req082
// check must fail TestHARRejectsMissingAttestation in har_test.go;
// removing the credential scan must fail
// TestHARRejectsUnredactedCapture there; removing the per-entry
// structural check must fail TestHARRejectsUnreplayableEntry.
func ValidateHAR(path string) (HAR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HAR{}, fmt.Errorf("%w: read HAR: %w", ErrUnsatisfiableMode, err)
	}
	var rec HAR
	if err := json.Unmarshal(data, &rec); err != nil {
		return HAR{}, fmt.Errorf("%w: decode HAR: %w", ErrUnsatisfiableMode, err)
	}
	if rec.Log.Version != "1.2" {
		return HAR{}, fmt.Errorf("%w: HAR log.version = %q, want 1.2", ErrUnsatisfiableMode, rec.Log.Version)
	}
	if rec.Log.Req082 == nil {
		return HAR{}, fmt.Errorf("%w: HAR log._req082 is required", ErrUnsatisfiableMode)
	}
	p := rec.Log.Req082.Provenance
	if p.Deployment == "" || p.CapturedAt == "" || p.SDKCommit == "" {
		return HAR{}, fmt.Errorf("%w: HAR log._req082.provenance is incomplete", ErrUnsatisfiableMode)
	}
	if !rec.Log.Req082.Redaction.Ran {
		return HAR{}, fmt.Errorf("%w: HAR log._req082.redaction.ran is not true", ErrUnsatisfiableMode)
	}
	if len(rec.Log.Entries) == 0 {
		return HAR{}, fmt.Errorf("%w: HAR log.entries is empty", ErrUnsatisfiableMode)
	}
	if err := refuseUnreplayable(rec); err != nil {
		return HAR{}, err
	}
	if err := refuseUnredacted(rec); err != nil {
		return HAR{}, err
	}
	return rec, nil
}

// refuseUnreplayable rejects an entry replay has nothing to work with:
// no request method, no request URL or one that does not parse, or no
// response status. HAR 1.2 requires all three, and a recording missing
// one is a capture that went wrong — better refused here, where the
// caller is choosing a recording, than surfacing later as an unmatched
// request or a zero-status response inside a probe.
func refuseUnreplayable(rec HAR) error {
	for i, e := range rec.Log.Entries {
		if strings.TrimSpace(e.Request.Method) == "" {
			return fmt.Errorf("%w: HAR entry %d request.method is empty", ErrUnsatisfiableMode, i)
		}
		raw := strings.TrimSpace(e.Request.URL)
		if raw == "" {
			return fmt.Errorf("%w: HAR entry %d request.url is empty", ErrUnsatisfiableMode, i)
		}
		// The parse error is dropped rather than wrapped: its message
		// quotes the URL it failed on, and a recorded URL is exactly
		// the payload text a refusal must not echo (REQ-093).
		if _, err := url.Parse(raw); err != nil {
			return fmt.Errorf("%w: HAR entry %d request.url does not parse", ErrUnsatisfiableMode, i)
		}
		if e.Response.Status <= 0 {
			return fmt.Errorf("%w: HAR entry %d response.status is not set", ErrUnsatisfiableMode, i)
		}
	}
	return nil
}

// refuseUnredacted scans every recorded request and response for a
// credential that survived capture-time redaction. It is the content
// half of the attestation check: log._req082.redaction.ran says the
// tool ran, this says it worked.
func refuseUnredacted(rec HAR) error {
	for i, e := range rec.Log.Entries {
		if name, found := credentialHeader(e.Request.Headers); found {
			return fmt.Errorf("%w: HAR entry %d request header %q survived redaction", ErrUnsatisfiableMode, i, name)
		}
		if name, found := credentialHeader(e.Response.Headers); found {
			return fmt.Errorf("%w: HAR entry %d response header %q survived redaction", ErrUnsatisfiableMode, i, name)
		}
		if key, found := credentialQueryKey(e.Request.URL); found {
			return fmt.Errorf("%w: HAR entry %d request URL query key %q survived redaction", ErrUnsatisfiableMode, i, key)
		}
		if urlUserinfo(e.Request.URL) {
			return fmt.Errorf("%w: HAR entry %d request URL userinfo survived redaction", ErrUnsatisfiableMode, i)
		}
		if e.Request.PostData != nil {
			if marker, found := bodyCredential(e.Request.PostData.Text); found {
				return fmt.Errorf("%w: HAR entry %d request body carries %q, which survived redaction", ErrUnsatisfiableMode, i, marker)
			}
		}
		if marker, found := bodyCredential(e.Response.Content.Text); found {
			return fmt.Errorf("%w: HAR entry %d response body carries %q, which survived redaction", ErrUnsatisfiableMode, i, marker)
		}
		// A redirect Location is a URL the recording carries just like
		// the request URL, so it gets the same two URL checks. Most
		// recordings leave the field empty; an empty one has nothing to
		// scan.
		if e.Response.RedirectURL != "" {
			if key, found := credentialQueryKey(e.Response.RedirectURL); found {
				return fmt.Errorf("%w: HAR entry %d response redirectURL query key %q survived redaction", ErrUnsatisfiableMode, i, key)
			}
			if urlUserinfo(e.Response.RedirectURL) {
				return fmt.Errorf("%w: HAR entry %d response redirectURL userinfo survived redaction", ErrUnsatisfiableMode, i)
			}
		}
	}
	return nil
}

// urlUserinfo reports whether rawURL carries a user:password
// component. That is the credential channel neither a header strip nor
// a query-key scan reaches — the credential rides in the URL's
// authority instead. Only its presence is reported, never the userinfo
// itself, so a refusal cannot echo it (REQ-093).
func urlUserinfo(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		// A request URL that does not parse is refused by
		// refuseUnreplayable before this scan runs. A redirectURL is not
		// structurally checked, but one that will not parse has no
		// authority to read a userinfo out of either way.
		return false
	}
	return u.User != nil
}

// bodyCredential reports the first credential marker in body, matched
// case-insensitively, and returns the canonical marker from
// [bodyCredentialMarkers], the authorization key, or one of
// [authSchemes] rather than any recorded text — a refusal names what
// was found, never the credential itself (REQ-093).
//
// The body is scanned as opaque text: a base64-encoded or otherwise
// packed body is not decoded first. Decoding arbitrary bodies to hunt
// for credentials inside them is a heuristic with no clean stopping
// point — each layer suggests another — and the same argument as at
// [credentialRunMin] applies: capture-time redaction is what keeps a
// credential out of the body in the first place (REQ-082).
func bodyCredential(body string) (string, bool) {
	if body == "" {
		return "", false
	}
	lower := strings.ToLower(body)
	for _, marker := range bodyCredentialMarkers {
		if strings.Contains(lower, marker) {
			return marker, true
		}
	}
	if authorizationKeyPresent(lower) {
		return strings.Trim(bodyAuthorizationKey, `"`), true
	}
	for _, scheme := range authSchemes {
		if schemeCarriesCredential(lower, scheme) {
			return strings.TrimSpace(scheme), true
		}
	}
	return "", false
}

// authorizationKeyPresent reports whether lower (already lower-cased)
// carries [bodyAuthorizationKey] in JSON key position: the quoted name
// followed, after any whitespace, by a colon. That is the one position
// the word cannot be clinical content in — as a value it is an ELEMENT
// name, as prose it is a note. Every occurrence is tried, so the word
// appearing as a value early in a body does not hide a real key later
// in it.
func authorizationKeyPresent(lower string) bool {
	rest := lower
	for {
		i := strings.Index(rest, bodyAuthorizationKey)
		if i < 0 {
			return false
		}
		rest = rest[i+len(bodyAuthorizationKey):]
		if strings.HasPrefix(strings.TrimLeft(rest, " \t\r\n"), ":") {
			return true
		}
	}
}

// schemeCarriesCredential reports whether lower (already lower-cased)
// holds scheme followed by something credential-shaped. Every
// occurrence is tried, so a clinical phrase early in a body does not
// hide a real header value later in it.
func schemeCarriesCredential(lower, scheme string) bool {
	rest := lower
	for {
		i := strings.Index(rest, scheme)
		if i < 0 {
			return false
		}
		rest = rest[i+len(scheme):]
		if credentialRun(rest) >= credentialRunMin {
			return true
		}
	}
}

// credentialRun counts the leading characters of s drawn from the
// base64 and JWT alphabet — the shape an encoded credential has, and
// the shape ordinary prose loses at its first space or punctuation.
// The alphabet carries both letter cases, so the count does not depend
// on the caller having lower-cased s.
func credentialRun(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '+', r == '/', r == '=', r == '-', r == '_', r == '.':
		default:
			return n
		}
		n++
	}
	return n
}

// credentialHeader reports the first credential-bearing header in
// headers, matched case-insensitively. It returns the canonical
// lower-case name from [credentialHeaders] rather than the recorded
// spelling, so a refusal message can never echo bytes read out of the
// recording (REQ-093).
func credentialHeader(headers []HARHeader) (string, bool) {
	for _, h := range headers {
		name := strings.ToLower(strings.TrimSpace(h.Name))
		if _, bad := credentialHeaders[name]; bad {
			return name, true
		}
	}
	return "", false
}

// credentialQueryKey reports the first credential-bearing query key in
// rawURL, matched case-insensitively, and returns the canonical
// lower-case name for the same reason [credentialHeader] does.
//
// The query is split by hand rather than through [url.ParseQuery]
// because that function drops the pairs it cannot parse: a key whose
// percent-escapes are malformed would then slip past the scan
// unexamined. Here such a key is compared in its raw form instead.
func credentialQueryKey(rawURL string) (string, bool) {
	_, query, ok := strings.Cut(rawURL, "?")
	if !ok {
		return "", false
	}
	query, _, _ = strings.Cut(query, "#")
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		raw, _, _ := strings.Cut(pair, "=")
		key := raw
		if decoded, err := url.QueryUnescape(raw); err == nil {
			key = decoded
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, bad := credentialQueryKeys[key]; bad {
			return key, true
		}
	}
	return "", false
}
