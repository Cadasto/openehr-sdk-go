package transport

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel transport errors. Detect classes with errors.Is.
//
// Wire-status mappings track [specs/wire.md § Error envelope] REQ-093.
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
	// ErrPreconditionRequired maps a wire 428 (PUT without If-Match).
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
	// Message is the human-readable description.
	Message string `json:"message"`
	// Code is the openEHR error code (e.g. "VALIDATION_FAILED").
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
	// the body could not be parsed as such.
	OpenEHR *OpenEHRErrorDetail
	// RawBody preserves the raw response bytes for diagnostics.
	RawBody []byte
	// Sentinel is the categorical class for errors.Is.
	Sentinel error
}

// Error implements error.
func (e *WireError) Error() string {
	var b strings.Builder
	if e.Sentinel != nil {
		b.WriteString(e.Sentinel.Error())
	} else {
		b.WriteString("transport: wire error")
	}
	if e.Method != "" || e.Route != "" {
		fmt.Fprintf(&b, " (%s %s)", e.Method, firstNonEmpty(e.Route, e.URL))
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " status=%d", e.StatusCode)
	}
	if e.OpenEHR != nil {
		if e.OpenEHR.Code != "" {
			fmt.Fprintf(&b, " code=%s", e.OpenEHR.Code)
		}
		if e.OpenEHR.Message != "" {
			fmt.Fprintf(&b, " message=%q", e.OpenEHR.Message)
		}
	}
	return b.String()
}

// Unwrap exposes the sentinel for errors.Is.
func (e *WireError) Unwrap() error { return e.Sentinel }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
