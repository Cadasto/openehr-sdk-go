package probe

import (
	"encoding/json"
	"fmt"
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
// [TestHARRejectsMissingAttestation].
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

// ValidateHAR decodes a HAR 1.2 recording and refuses one that is
// missing the ADR 0020 attestation. Removing the _req082 check must
// fail [TestHARRejectsMissingAttestation].
func ValidateHAR(data []byte) (HAR, error) {
	var rec HAR
	if err := json.Unmarshal(data, &rec); err != nil {
		return HAR{}, fmt.Errorf("probe: decode HAR: %w", err)
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
	return rec, nil
}
