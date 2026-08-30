package query

import (
	"errors"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// ErrInvalidConfig indicates invalid executor options or query input.
var ErrInvalidConfig = errors.New("query: invalid configuration")

// AQLError is an AQL-level failure distinct from generic transport
// errors (parse error, timeout). Detect with errors.AsType. When the failure is
// a path-resolution error it also satisfies errors.Is(err, [aql.ErrPathResolution]);
// when it is a capability gap, errors.Is(err, [aql.ErrEngineCapability]).
//
//	if e, ok := errors.AsType[*query.AQLError](err); ok { … }
//
// A nil *AQLError answers as the zero AQLError on every exported method rather
// than panicking (REQ-025 § No panics). The older errors.As out-parameter form
//
//	var e *query.AQLError
//	if errors.As(err, &e) { … }
//
// leaves that variable nil when the match fails, and passing it onward as an
// error boxes a typed nil in a non-nil interface — so errors.Is would call Is
// on a nil receiver. That is caller-constructible input reachable through the
// documented API, not a programmer error. See orZero.
type AQLError struct {
	Message string
	// Code is the backend's own error code from the openEHR error envelope,
	// carried through verbatim — it is NOT the SDK's classification, and the
	// two can disagree: a 501 whose envelope happens to carry a path-shaped
	// code is still a capability gap, never a path resolution failure. Dispatch
	// with errors.Is against [aql.ErrPathResolution] / [aql.ErrEngineCapability]
	// rather than on this string; the taxonomy is normative in
	// docs/specifications/wire.md § AQL executor.
	Code  string
	Inner error
	// pathResolution marks a backend error classified as an AQL path
	// resolution failure (PROBE-021).
	pathResolution bool
	// capability marks a backend error classified as a capability gap: the
	// query is valid AQL, this deployment does not implement it (REQ-055,
	// PROBE-021). Set from HTTP 501 alone; never from message text.
	capability bool
}

// orZero reads a nil receiver as the zero AQLError (REQ-025 § No panics).
// Every exported method funnels through it, so the nil a failed errors.As
// leaves behind answers instead of dereferencing: the zero value is classified
// as neither error class, so Is reports no match, Error names the generic
// failure, and there is nothing to unwrap. Only that nil path allocates —
// a real AQLError is returned as-is.
func (e *AQLError) orZero() *AQLError {
	if e == nil {
		return &AQLError{}
	}
	return e
}

// Is reports whether the error matches target. A path-resolution AQLError
// matches [aql.ErrPathResolution] and a capability gap matches
// [aql.ErrEngineCapability], so callers can branch without inspecting
// CDR-specific codes. The two classes are disjoint (REQ-055). A nil receiver
// matches nothing.
func (e *AQLError) Is(target error) bool {
	e = e.orZero()
	switch target {
	case aql.ErrPathResolution:
		return e.pathResolution
	case aql.ErrEngineCapability:
		return e.capability
	}
	return false
}

// Error names the failure, preferring the envelope message when the deployment
// surfaces raw error bodies and falling back to the PHI-free code. A nil
// receiver yields the same stable, non-empty generic text as the zero value.
func (e *AQLError) Error() string {
	e = e.orZero()
	if e.Message != "" {
		return "query: " + e.Message
	}
	if e.Code != "" {
		return "query: " + e.Code
	}
	return "query: execution failed"
}

// Unwrap exposes the wrapped wire error so errors.Is/As reach the underlying
// [transport.WireError]. A nil receiver wraps nothing.
func (e *AQLError) Unwrap() error { return e.orZero().Inner }

// mapQueryError wraps transport wire errors that represent AQL failures.
func mapQueryError(err error) error {
	if err == nil {
		return nil
	}
	// A boxed typed-nil *transport.WireError matches with ok=true and a nil
	// pointer, so the nil check is load-bearing (REQ-025 nil-receiver axis):
	// there is no status to map, so the error passes through untouched.
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we == nil {
		return err
	}
	// REQ-055: 501 is a capability gap — the query is valid AQL, this
	// deployment does not implement it. The status alone is the signal (with
	// or without an envelope), and a capability gap is never also a path
	// resolution failure, which is bad AQL and arrives as 400.
	capability := we.StatusCode == 501
	if we.OpenEHR != nil && (we.OpenEHR.Message != "" || we.OpenEHR.Code != "") {
		return &AQLError{
			Message:        we.OpenEHR.Message,
			Code:           we.OpenEHR.Code,
			Inner:          err,
			pathResolution: !capability && isPathResolution(we.OpenEHR.Code, we.OpenEHR.Message),
			capability:     capability,
		}
	}
	if capability || we.StatusCode == 400 || we.StatusCode == 408 {
		return &AQLError{Message: we.Error(), Inner: err, capability: capability}
	}
	return err
}

// isPathResolution classifies a backend AQL error envelope as a path
// resolution failure. openEHR does not mandate a single code for this, so the
// match is a best-effort heuristic pending Live ratification (PROBE-021). It is
// deliberately narrow to avoid false positives: the code must name both a PATH
// and a resolution failure (e.g. AQL_PATH_RESOLUTION) — not a routing code like
// INVALID_PATH_PARAMETER — and message clauses are anchored on "path" so a
// generic "could not resolve <X>" does not match. Code is the PHI-free signal
// and preferred; the message is consulted only when surfaced (WithRawErrorBodies).
func isPathResolution(code, message string) bool {
	c := strings.ToUpper(code)
	if strings.Contains(c, "PATH") && (strings.Contains(c, "RESOL") || strings.Contains(c, "UNKNOWN")) {
		return true
	}
	m := strings.ToLower(message)
	return strings.Contains(m, "resolve path") ||
		strings.Contains(m, "path resolution") ||
		strings.Contains(m, "unknown path")
}
