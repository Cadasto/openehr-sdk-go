package transport

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
)

// Sentinel transport errors. Detect classes with errors.Is.
//
// Wire-status mappings track [docs/specifications/wire.md § Error envelope] REQ-093.
// Additional sentinels for non-wire failures (discovery, configuration)
// live in their owning packages.
var (
	// ErrNotFound maps a wire 404.
	ErrNotFound = errors.New("transport: not found")
	// ErrUnauthorized maps a wire 401.
	ErrUnauthorized = errors.New("transport: unauthorized")
	// ErrForbidden maps a wire 403.
	ErrForbidden = errors.New("transport: forbidden")
	// ErrVersionConflict maps a wire 409 (stale If-Match).
	ErrVersionConflict = errors.New("transport: version conflict")
	// ErrPreconditionFailed maps a wire 412.
	ErrPreconditionFailed = errors.New("transport: precondition failed")
	// ErrUnprocessable maps a wire 422 — a well-formed request that
	// failed semantic / template validation (REQ-093).
	ErrUnprocessable = errors.New("transport: unprocessable entity")
	// ErrPreconditionRequired maps a wire 428. Note: openEHR signals a
	// missing-but-expected If-Match as 400, not 428 — this sentinel is
	// retained only as a defensive mapping for non-conformant servers.
	ErrPreconditionRequired = errors.New("transport: precondition required")
	// ErrServerError maps any 5xx.
	ErrServerError = errors.New("transport: server error")
	// ErrServiceUnavailable indicates the configured ServiceCatalog
	// does not advertise the requested service ID.
	ErrServiceUnavailable = errors.New("transport: service not in catalog")
	// ErrInvalidConfig indicates the Client was constructed with
	// missing or contradictory required inputs.
	ErrInvalidConfig = errors.New("transport: invalid configuration")
	// ErrInvalidShape indicates the response body did not match the
	// expected shape (e.g. Prefer=representation got an empty body).
	ErrInvalidShape = errors.New("transport: invalid response shape")
)

// OpenEHRErrorDetail is the parsed openEHR REST error envelope per
// REQ-093. Nil when the response body did not match the envelope shape.
type OpenEHRErrorDetail struct {
	// Message is the human-readable description from the server.
	// May contain PHI (patient identifiers, composition UUIDs, etc.).
	// Populated only when the client is constructed with WithRawErrorBodies(true);
	// empty by default so error values are safe to log and trace.
	// Extract via errors.As when needed; do not include in log lines.
	Message string `json:"message"`
	// Code is the openEHR error code (e.g. "VALIDATION_FAILED").
	// Coded terminology identifier — treated as non-PHI; always preserved.
	Code string `json:"code"`
	// CodedText optionally enumerates terminology-coded error tags.
	CodedText []CodedTextItem `json:"coded_text,omitempty"`
}

// CodedTextItem mirrors the openEHR error envelope's coded_text entry.
type CodedTextItem struct {
	TerminologyID struct {
		Value string `json:"value"`
	} `json:"terminology_id"`
	CodeString string `json:"code_string"`
}

// WireError is the typed wire-level error returned to consumers. Use
// errors.As(err, &w) to extract; errors.Is(err, transport.ErrXxx) to
// classify.
type WireError struct {
	// StatusCode is the HTTP status code received.
	StatusCode int
	// Method, URL, and Route are the captured request identifiers.
	// Route is the path template (e.g. "/ehr/{ehr_id}") when known;
	// URL is the resolved URL with parameters substituted.
	Method, URL, Route string
	// OpenEHR is the parsed openEHR error envelope (REQ-093). Nil when
	// the body could not be parsed as such. OpenEHR.Message may contain
	// PHI and is only populated when the client is built with
	// WithRawErrorBodies(true). OpenEHR.Code is always present.
	OpenEHR *OpenEHRErrorDetail
	// RawBody preserves the raw response bytes for diagnostics.
	// May contain PHI; only populated when the client is built with
	// WithRawErrorBodies(true). Empty by default.
	RawBody []byte
	// Sentinel is the categorical class for errors.Is.
	Sentinel error
}

// Error implements error. The returned string includes the HTTP status,
// the openEHR error code, and the request route — all non-PHI fields.
// The server message and raw body are deliberately omitted so WireError
// values are safe to include in logs, traces, and observer callbacks.
// Callers that need the message (e.g. for user-facing error reporting in
// a controlled environment) should use errors.As to extract the full
// WireError after opting in via WithRawErrorBodies.
func (e *WireError) Error() string {
	var b strings.Builder
	if e.Sentinel != nil {
		b.WriteString(e.Sentinel.Error())
	} else {
		b.WriteString("transport: wire error")
	}
	if e.Method != "" || e.Route != "" {
		fmt.Fprintf(&b, " (%s %s)", e.Method, cmp.Or(e.Route, e.URL))
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " status=%d", e.StatusCode)
	}
	if e.OpenEHR != nil && e.OpenEHR.Code != "" {
		fmt.Fprintf(&b, " code=%s", e.OpenEHR.Code)
	}
	return b.String()
}

// Unwrap exposes the sentinel for errors.Is.
func (e *WireError) Unwrap() error { return e.Sentinel }
