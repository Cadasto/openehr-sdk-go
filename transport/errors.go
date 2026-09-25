package transport

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
)

// Sentinel transport errors. Detect classes with errors.Is.
//
// The wire-status sentinels map HTTP status codes of the openEHR REST
// error envelope. Additional sentinels for non-wire failures (discovery, configuration)
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
	// ErrUnprocessable maps a wire 422: a well-formed request that
	// failed semantic or template validation.
	ErrUnprocessable = errors.New("transport: unprocessable entity")
	// ErrPreconditionRequired maps a wire 428. Note: openEHR signals a
	// missing-but-expected If-Match as 400, not 428; this sentinel exists
	// only as a defensive mapping for non-conformant servers.
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
	// ErrInvalidPathSegment indicates a decoded Request.Path segment is
	// empty, is `.` or `..`, carries `\` or a control character, or (for
	// [ValidatePathSegment]) carries `/`. It also reports a path whose
	// segment count contradicts its Route template, which is how a
	// separator smuggled inside one parameter shows up.
	//
	// Returned errors wrap ErrInvalidConfig as well, so
	// errors.Is(err, ErrInvalidConfig) still means "the request never
	// left the process". The two sentinels are independent values:
	// errors.Is(ErrInvalidPathSegment, ErrInvalidConfig) is false, and
	// only the returned chain carries both.
	ErrInvalidPathSegment = errors.New("transport: invalid path segment")
)

// OpenEHRErrorDetail is the parsed openEHR REST error envelope. It is
// nil when the response body did not match the envelope shape.
type OpenEHRErrorDetail struct {
	// Message is the human-readable description from the server.
	// May contain PHI (patient identifiers, composition UUIDs, etc.).
	// Populated only when the client is constructed with WithRawErrorBodies(true);
	// empty by default so error values are safe to log and trace.
	// Extract via errors.As when needed; do not include in log lines.
	Message string `json:"message"`
	// Code is the openEHR error code (e.g. "VALIDATION_FAILED").
	// It is a coded terminology identifier, treated as non-PHI and always
	// preserved.
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
// errors.AsType[*transport.WireError](err) to extract; errors.Is(err,
// transport.ErrXxx) to classify.
type WireError struct {
	// StatusCode is the HTTP status code received.
	StatusCode int
	// Method, URL, and Route are the captured request identifiers.
	// Route is the path template (e.g. "/ehr/{ehr_id}") when known, or the
	// stable "(unrouted)" placeholder when the request carried no Route.
	// It is never the expanded Path, which may hold a caller-supplied
	// identifier. URL is the resolved URL with parameters substituted.
	Method, URL, Route string
	// OpenEHR is the parsed openEHR error envelope. Nil when
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
// the openEHR error code, and the request route, all of which are
// non-PHI fields.
// The server message and raw body are deliberately omitted so WireError
// values are safe to include in logs, traces, and observer callbacks.
// Callers that need the message (e.g. for user-facing error reporting in
// a controlled environment) should use errors.As to extract the full
// WireError after opting in via WithRawErrorBodies.
//
// A nil receiver returns the zero WireError's Error text instead of
// panicking.
func (e *WireError) Error() string {
	if e == nil {
		return (&WireError{}).Error()
	}
	var b strings.Builder
	if e.Sentinel != nil {
		b.WriteString(e.Sentinel.Error())
	} else {
		b.WriteString("transport: wire error")
	}
	if e.Method != "" || e.Route != "" {
		// REQ-093: never fall back to e.URL here — it is the resolved URL
		// with parameters substituted, so it can carry a caller-supplied
		// identifier. mapWireError always sets Route (to the template, or to
		// the unroutedRoute placeholder when the caller left Route unset),
		// so this branch is defensive for a WireError a caller constructs
		// directly with Route left zero.
		fmt.Fprintf(&b, " (%s %s)", e.Method, cmp.Or(e.Route, unroutedRoute))
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " status=%d", e.StatusCode)
	}
	if e.OpenEHR != nil && e.OpenEHR.Code != "" {
		fmt.Fprintf(&b, " code=%s", e.OpenEHR.Code)
	}
	return b.String()
}

// Unwrap exposes the sentinel for errors.Is. A nil receiver unwraps to
// nil, so the typed nil a failed errors.As or errors.AsType leaves behind
// does not panic.
func (e *WireError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Sentinel
}

// DecodeError reports a 2xx response whose body could not be decoded as the
// requested representation. It is distinct from a wire failure
// ([WireError]) and from an absent body, which fails with
// [ErrInvalidShape]. Extract it with
// errors.AsType[*transport.DecodeError](err); the operation-name wrap a
// leaf package adds does not hide it.
type DecodeError struct {
	// Method is the HTTP method of the request.
	Method string
	// Route is the route template (e.g. "/ehr/{ehr_id}"), or the stable
	// "(unrouted)" placeholder when the request carried no Route. It is
	// never the expanded URL or Path, which may hold a caller-supplied
	// identifier.
	Route string
	// Body is the raw response body as delivered by the injected
	// [http.Client], after any transparent content decoding that client
	// performs, so a gzipped response yields the decompressed bytes
	// rather than the wire form. It is always populated: no option
	// controls it, and WithRawErrorBodies (which governs non-2xx bodies)
	// does not apply. Body is limited only by the client's
	// [WithMaxResponseBody] setting: the 64 MiB default, an explicit
	// positive limit, or no limit when the caller disabled the cap with a
	// negative value.
	//
	// The slice is the response buffer itself, not a copy. A custom
	// UnmarshalJSON that mutates the bytes handed to it violates the
	// encoding/json unmarshaler contract and corrupts these diagnostics;
	// the SDK does not make a defensive copy.
	//
	// May contain PHI: it is the caller's own requested representation,
	// so for a clinical resource it is patient data. Error never
	// includes it, and neither do observers or spans; reading Body is a
	// deliberate act, and an error value retained by the caller retains
	// these bytes with it.
	Body []byte
	// Inner is the decoder's own error, reachable via Unwrap.
	Inner error
}

// Error names the method, the route and the classification only, so the
// message carries no payload values. It never includes Body or the
// wrapped decoder's text, because either may embed the offending value
// (*strconv.NumError always does; the v2 codec's *json.SemanticError does
// when the failing token is a short string or number). Callers that need
// the diagnostics unwrap the error or read Body.
//
// Error works on a nil receiver instead of panicking, so the typed nil a
// failed errors.As leaves behind is safe to print.
func (e *DecodeError) Error() string {
	if e == nil {
		return "transport: decode: response body does not match the requested type"
	}
	// The unroutedRoute fallback matches WireError.Error(): Decode always
	// sets Route (to the template, or to the placeholder when the caller
	// left Request.Route unset), so this is for a DecodeError a caller
	// constructs directly — an empty Route would otherwise render an empty
	// slot in the middle of the sentence.
	return fmt.Sprintf("transport: decode %s %s: response body does not match the requested type", e.Method, cmp.Or(e.Route, unroutedRoute))
}

// Unwrap exposes the decoder's error, so errors.Is and errors.AsType still
// reach the codec's own typed diagnostics (path, type, offset) unchanged. It
// returns nil on a nil receiver.
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}
