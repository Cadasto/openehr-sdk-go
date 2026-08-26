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
// errors (parse error, timeout). Detect with errors.As. When the failure is a
// path-resolution error it also satisfies errors.Is(err, [aql.ErrPathResolution]);
// when it is a capability gap, errors.Is(err, [aql.ErrEngineCapability]).
type AQLError struct {
	Message string
	Code    string
	Inner   error
	// pathResolution marks a backend error classified as an AQL path
	// resolution failure (PROBE-021).
	pathResolution bool
	// capability marks a backend error classified as a capability gap: the
	// query is valid AQL, this deployment does not implement it (REQ-055,
	// PROBE-021). Set from HTTP 501 alone; never from message text.
	capability bool
}

// Is reports whether the error matches target. A path-resolution AQLError
// matches [aql.ErrPathResolution] and a capability gap matches
// [aql.ErrEngineCapability], so callers can branch without inspecting
// CDR-specific codes. The two classes are disjoint (REQ-055).
func (e *AQLError) Is(target error) bool {
	switch target {
	case aql.ErrPathResolution:
		return e.pathResolution
	case aql.ErrEngineCapability:
		return e.capability
	}
	return false
}

func (e *AQLError) Error() string {
	if e.Message != "" {
		return "query: " + e.Message
	}
	if e.Code != "" {
		return "query: " + e.Code
	}
	return "query: execution failed"
}

func (e *AQLError) Unwrap() error { return e.Inner }

// mapQueryError wraps transport wire errors that represent AQL failures.
func mapQueryError(err error) error {
	if err == nil {
		return nil
	}
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok {
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
