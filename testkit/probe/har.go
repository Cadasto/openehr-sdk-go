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

// ValidateHAR reads the HAR 1.2 recording at path and refuses one that
// must not be replayed (REQ-082): unreadable or malformed bytes, the
// wrong log version, a missing or incomplete ADR 0020 attestation, an
// empty entries list, or a credential the capture-time redaction left
// behind. Every refusal wraps [ErrUnsatisfiableMode], so a caller can
// discard the recording on the sentinel alone.
//
// A refusal names the offending entry and the header or query key, and
// never the value or any recorded payload text (REQ-093). Removing the
// _req082 check must fail TestHARRejectsMissingAttestation in
// har_test.go; removing the credential scan must fail
// TestHARRejectsUnredactedCapture there.
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
	if err := refuseUnredacted(rec); err != nil {
		return HAR{}, err
	}
	return rec, nil
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
	}
	return nil
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
