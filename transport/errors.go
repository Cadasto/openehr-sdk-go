package transport

import (
	"cmp"
	"encoding/json"
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
	// ErrVersionConflict maps a wire 409: the request conflicts with the
	// current state of the resource, for example deleting a Composition
	// version that is no longer the latest, or creating a resource that
	// already exists. A stale If-Match on a PUT is a 412
	// ([ErrPreconditionFailed]), not a 409.
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
	// ErrInsecureRedirect indicates the transport refused to follow a
	// redirect from an https URL to a URL that is not https, because the
	// request carried a credential header (Authorization,
	// Proxy-Authorization, Cookie or Cookie2), which net/http forwards to a
	// redirect target on the same host or a subdomain whatever its scheme.
	// The request never reached that URL, and the retry policy does not
	// retry it.
	ErrInsecureRedirect = errors.New("transport: redirect from https to a URL that is not https refused")
	// ErrResponseTooLarge indicates the response body was longer than the
	// [WithMaxResponseBody] ceiling, so the transport stopped reading it. The
	// error that wraps it names the limit. On a 2xx it is the cause inside a
	// [BodyReadError], and the retry policy does not send the request again,
	// since every attempt would stop at the same ceiling; on any other status
	// it is wrapped beside the [WireError], which is retried by its status.
	ErrResponseTooLarge = errors.New("transport: response body too large")
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
	// Code is the openEHR error code (e.g. "VALIDATION_FAILED"). It is
	// a coded terminology identifier, treated as non-PHI and always
	// preserved. A server may send it as a JSON string or as a JSON
	// number (the ITS-REST overview shows 90000); either way it is kept
	// here as text.
	Code string `json:"code"`
	// CodedText enumerates the envelope's terminology-coded error entries.
	// It holds the entries of the envelope's coded_text list first, then
	// those of its errors list, the DV_CODED_TEXT list the ITS-REST overview
	// shows. The codes are always preserved; each item's free-text Value is
	// populated only when the client is constructed with
	// WithRawErrorBodies(true).
	CodedText []CodedTextItem `json:"coded_text,omitempty"`
	// ValidationErrors lists the validation messages that the ITS-REST
	// Error schema of a 400 response carries beside the message. Like
	// Message it is free text that may name patients, so it is populated
	// only when the client is constructed with WithRawErrorBodies(true).
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

// UnmarshalJSON reads the envelope member by member, so one member of the
// wrong JSON type is left empty or skipped instead of failing the whole
// envelope. It fails only when the body is not a JSON object.
//
// Code accepts a string or a number; a code of any other JSON type is left
// empty. Message is read only when it is a string. ValidationErrors keeps the
// string entries of a validationErrors list, in order, and skips any other
// entry.
//
// CodedText collects the coded_text entries first, then the errors
// entries. A coded_text entry gives the item's terminology, code string and
// value directly. Each errors entry is a DV_CODED_TEXT whose defining_code
// gives the item's terminology and code string and whose value gives its
// Value. In either list an entry that is not an object, or whose terminology
// or code string is present but not a string (null included), is skipped, as
// is an errors entry with no defining_code; an absent terminology or code
// string is left empty, and a value that is not a string is left empty and
// the entry's codes are kept. A coded_text or errors member that is not a list is
// ignored.
func (d *OpenEHRErrorDetail) UnmarshalJSON(data []byte) error {
	var raw struct {
		Message          json.RawMessage `json:"message"`
		Code             json.RawMessage `json:"code"`
		CodedText        json.RawMessage `json:"coded_text"`
		Errors           json.RawMessage `json:"errors"`
		ValidationErrors json.RawMessage `json:"validationErrors"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	message, _ := jsonString(raw.Message)
	*d = OpenEHRErrorDetail{
		Message:          message,
		Code:             envelopeCode(raw.Code),
		CodedText:        append(envelopeCodedText(raw.CodedText), envelopeErrors(raw.Errors)...),
		ValidationErrors: envelopeStrings(raw.ValidationErrors),
	}
	return nil
}

// codePhrase is the terminology and code string of a coded entry: the
// members of a coded_text entry, or the defining_code of an errors entry.
// Both are kept raw, so that a null can be told apart from an absent member.
type codePhrase struct {
	TerminologyID json.RawMessage `json:"terminology_id"`
	CodeString    json.RawMessage `json:"code_string"`
}

// codedItem returns the item for p and an entry's raw value, and false when
// the terminology_id is present but not an object, or the terminology or code
// string is present but not a JSON string, null included. An absent one is
// left empty. A value that is not a JSON string is left empty, so the codes
// survive it.
func (p codePhrase) codedItem(value json.RawMessage) (CodedTextItem, bool) {
	var terminology struct {
		Value json.RawMessage `json:"value"`
	}
	if len(p.TerminologyID) > 0 && (!isJSONObject(p.TerminologyID) || json.Unmarshal(p.TerminologyID, &terminology) != nil) {
		return CodedTextItem{}, false
	}
	var item CodedTextItem
	var termOK, codeOK bool
	item.TerminologyID.Value, termOK = optionalString(terminology.Value)
	item.CodeString, codeOK = optionalString(p.CodeString)
	if !termOK || !codeOK {
		return CodedTextItem{}, false
	}
	item.Value, _ = jsonString(value)
	return item, true
}

// envelopeCodedText returns the coded items of the envelope's coded_text
// list, in order. It skips an entry that is not an object or whose
// terminology or code string is not a string, and returns nil when the list
// is absent, null or not a JSON array.
func envelopeCodedText(raw json.RawMessage) []CodedTextItem {
	var items []CodedTextItem
	for _, entry := range jsonList(raw) {
		var e struct {
			codePhrase
			Value json.RawMessage `json:"value"`
		}
		if !isJSONObject(entry) || json.Unmarshal(entry, &e) != nil {
			continue
		}
		if item, ok := e.codedItem(e.Value); ok {
			items = append(items, item)
		}
	}
	return items
}

// envelopeErrors returns the coded items of the envelope's errors list, in
// order. It skips an entry that is not an object, has no defining_code, or
// whose terminology or code string is not a string, and returns nil when the
// list is absent, null or not a JSON array. A value that is not a string is
// left empty and the entry's codes are kept. An entry that is not an object
// fails the decode, and a null one has no defining_code, so neither needs a
// check of its own.
func envelopeErrors(raw json.RawMessage) []CodedTextItem {
	var items []CodedTextItem
	for _, entry := range jsonList(raw) {
		var dv struct {
			Value        json.RawMessage `json:"value"`
			DefiningCode *codePhrase     `json:"defining_code"`
		}
		if json.Unmarshal(entry, &dv) != nil || dv.DefiningCode == nil {
			continue
		}
		if item, ok := dv.DefiningCode.codedItem(dv.Value); ok {
			items = append(items, item)
		}
	}
	return items
}

// envelopeStrings returns the string entries of a JSON list, in order,
// skipping any other entry, and nil when the list is absent, null or not a
// JSON array.
func envelopeStrings(raw json.RawMessage) []string {
	var out []string
	for _, entry := range jsonList(raw) {
		if s, ok := jsonString(entry); ok {
			out = append(out, s)
		}
	}
	return out
}

// jsonList returns the raw entries of a JSON array, and nil for an absent
// value, null or any other JSON type.
func jsonList(raw json.RawMessage) []json.RawMessage {
	var entries []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	return entries
}

// optionalString returns the text of a JSON string and true, "" and true for
// an absent member, and "" and false for null or any other JSON type.
func optionalString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	return jsonString(raw)
}

// jsonString returns the text of a JSON string and true, or "" and false for
// an absent value, null or any other JSON type.
func jsonString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// isJSONObject reports whether raw is a JSON object. The decoder hands each
// entry over without leading white space, so the first byte decides. A null
// entry decodes into a struct without error, so a coded_text entry needs
// this check to be skipped.
func isJSONObject(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '{'
}

// envelopeCode returns the text of a JSON string or number code, and ""
// for an absent code, null, or any other JSON type.
func envelopeCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return ""
		}
		return s
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return string(raw)
	}
	return ""
}

// CodedTextItem is one coded entry of the openEHR error envelope: an entry
// of its coded_text list, or the defining code and value of a
// DV_CODED_TEXT in its errors list.
type CodedTextItem struct {
	// TerminologyID names the terminology the code comes from, such as
	// "local" or "openehr".
	TerminologyID struct {
		Value string `json:"value"`
	} `json:"terminology_id"`
	// CodeString is the code within that terminology. Like the terminology,
	// it is always preserved.
	CodeString string `json:"code_string"`
	// Value is the entry's free text, the DV_CODED_TEXT value. Like the
	// envelope's Message it may name patients, so it is populated only when
	// the client is constructed with WithRawErrorBodies(true), and
	// [WireError.Error] never includes it.
	Value string `json:"value,omitempty"`
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
	// identifier. URL is the resolved URL with parameters substituted and
	// any userinfo (user name and password) removed; its path and query are
	// kept as sent. [WireError.Error] never includes it.
	Method, URL, Route string
	// OpenEHR is the parsed openEHR error envelope. Nil when
	// the body could not be parsed as such. OpenEHR.Message and the Value
	// of each OpenEHR.CodedText item may contain PHI and are only
	// populated when the client is built with WithRawErrorBodies(true).
	// OpenEHR.Code and the CodedText items' codes are always present.
	OpenEHR *OpenEHRErrorDetail
	// RawBody preserves the raw response bytes for diagnostics.
	// May contain PHI; only populated when the client is built with
	// WithRawErrorBodies(true). Empty by default.
	RawBody []byte
	// Challenge is the Bearer challenge from the response's
	// WWW-Authenticate header, which says why the server refused the
	// token. It is set only on a 401 or 403 whose header carries a Bearer
	// challenge the transport can read, and is nil otherwise; the status
	// maps to ErrUnauthorized or ErrForbidden either way. [WireError.Error]
	// never includes the challenge's values, so read them here after
	// errors.AsType. The challenge's ErrorDescription, free text from the
	// server, is kept only when the client is built with
	// WithRawErrorBodies(true); the other fields are kept regardless of that
	// option.
	Challenge *BearerChallenge
	// Sentinel is the categorical class for errors.Is.
	Sentinel error
}

// Error implements error. The returned string includes the HTTP status,
// the openEHR error code, quoted as a Go string, and the request route,
// all of which are non-PHI fields.
// The server message, the coded items' free text, the raw body and the
// Bearer challenge are deliberately omitted so WireError values are safe to include in logs,
// traces, and observer callbacks.
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
		// REQ-093: the code is the server's text, so it is quoted: a
		// newline or a fragment that reads as another field stays inside
		// the quotes.
		fmt.Fprintf(&b, " code=%q", e.OpenEHR.Code)
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

// BodyReadError reports a 2xx response whose body could not be read: the
// connection dropped mid-body, the body did not match its Content-Encoding,
// or it was longer than the [WithMaxResponseBody] ceiling. The status is then
// the only trustworthy signal of what the server did; on a write it says the
// server committed. It is distinct from a wire failure ([WireError]), which a
// status that is not 2xx gives even when its body cannot be read, and from a
// body that was read but does not decode ([DecodeError]). Extract it with
// errors.AsType[*transport.BodyReadError](err); the operation-name wrap a
// leaf package adds does not hide it.
//
// [Client.Do] returns it beside a non-nil [Response] that carries the
// StatusCode, the Header and the parsed Metadata, so the ETag and Location of
// a committed write survive, and whose Body is empty. The error carries no
// body bytes: a partial body is not the representation. Its text does not
// carry the read failure's own text either, which can quote bytes the server
// sent; reach the read failure through Inner or Unwrap.
type BodyReadError struct {
	// Method is the HTTP method of the request.
	Method string
	// Route is the route template (e.g. "/ehr/{ehr_id}"), or the stable
	// "(unrouted)" placeholder when the request carried no Route. It is
	// never the expanded URL or Path, which may hold a caller-supplied
	// identifier.
	Route string
	// StatusCode is the 2xx status the server sent before the body failed.
	StatusCode int
	// Inner is the read failure, reachable via Unwrap: io.ErrUnexpectedEOF
	// for a connection cut mid-body, the content-coding error (such as
	// gzip.ErrHeader) for a body that does not match its encoding, or an
	// error wrapping [ErrResponseTooLarge] for a body over the ceiling.
	Inner error
}

// Error names the method, the route, the status and the kind of read failure.
// For a body over the [WithMaxResponseBody] ceiling it names the limit, which
// is the caller's own setting; any other failure gets one fixed phrase. It
// never includes a header value, a body byte, or the read failure's own text,
// because that text can quote bytes the server sent: net/http quotes a
// malformed chunked trailer line in it. Callers that need the read failure
// unwrap the error.
//
// Error works on a nil receiver instead of panicking, so the typed nil a
// failed errors.As leaves behind is safe to print.
func (e *BodyReadError) Error() string {
	if e == nil {
		return "transport: read body: response body could not be read"
	}
	// The unroutedRoute fallback matches DecodeError.Error(): doOnce always
	// sets Route, so this is for a BodyReadError a caller constructs directly.
	return fmt.Sprintf("transport: %s %s: status %d: read body: %s", e.Method, cmp.Or(e.Route, unroutedRoute), e.StatusCode, readFailureText(e.Inner))
}

// Unwrap exposes the read failure, so errors.Is reaches io.ErrUnexpectedEOF,
// the content-coding error or [ErrResponseTooLarge]. It returns nil on a nil
// receiver.
func (e *BodyReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// responseTooLargeError is the read failure of a body over the
// WithMaxResponseBody ceiling. Its text names the limit, without the
// "transport:" prefix, because it is always printed inside an error that
// already carries one, and it unwraps to ErrResponseTooLarge.
type responseTooLargeError struct{ limit int64 }

func (e responseTooLargeError) Error() string {
	return fmt.Sprintf("response exceeds limit of %d bytes", e.limit)
}

func (responseTooLargeError) Unwrap() error { return ErrResponseTooLarge }

// readFailureText names the kind of a body read failure without its own text,
// which can quote bytes the server sent. A body over the ceiling is named with
// the limit, the caller's own setting; any other failure, a nil one included,
// gets one fixed phrase.
func readFailureText(cause error) string {
	if tl, ok := errors.AsType[responseTooLargeError](cause); ok {
		return tl.Error()
	}
	return "body could not be read"
}

// bodyReadFailure is the read failure of a response that is not 2xx, as doOnce
// wraps it beside the WireError the status gives. Its text is readFailureText's,
// so the text added to the WireError's own stays value-free, and Unwrap returns
// the read failure itself, so errors.Is reaches the same causes as on the 2xx
// arm.
type bodyReadFailure struct{ cause error }

func (e *bodyReadFailure) Error() string { return "read body: " + readFailureText(e.cause) }

func (e *bodyReadFailure) Unwrap() error { return e.cause }

// requestBuildError marks a request that http.NewRequestWithContext refused.
// The method and the URL are the same on every attempt, so such a request
// fails the same way each time, and shouldRetry does not send it again
// (REQ-091). Its text and its chain are those of the error it wraps, so a
// caller still reaches the *url.Error beneath it.
type requestBuildError struct{ err error }

func (e *requestBuildError) Error() string { return e.err.Error() }

func (e *requestBuildError) Unwrap() error { return e.err }
