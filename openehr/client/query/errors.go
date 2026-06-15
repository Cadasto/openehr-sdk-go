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
// path-resolution error it also satisfies errors.Is(err, [aql.ErrPathResolution]).
type AQLError struct {
	Message string
	Code    string
	Inner   error
	// pathResolution marks a backend error classified as an AQL path
	// resolution failure (PROBE-021).
	pathResolution bool
}

// Is reports whether the error matches target. A path-resolution AQLError
// matches [aql.ErrPathResolution] so callers can branch without inspecting
// CDR-specific codes.
func (e *AQLError) Is(target error) bool {
	return e.pathResolution && target == aql.ErrPathResolution
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
	var we *transport.WireError
	if !errors.As(err, &we) {
		return err
	}
	if we.OpenEHR != nil && (we.OpenEHR.Message != "" || we.OpenEHR.Code != "") {
		return &AQLError{
			Message:        we.OpenEHR.Message,
			Code:           we.OpenEHR.Code,
			Inner:          err,
			pathResolution: isPathResolution(we.OpenEHR.Code, we.OpenEHR.Message),
		}
	}
	if we.StatusCode == 400 || we.StatusCode == 408 {
		return &AQLError{Message: we.Error(), Inner: err}
	}
	return err
}

// isPathResolution classifies a backend AQL error envelope as a path
// resolution failure. openEHR does not mandate a single code for this, so the
// match is intentionally conservative and extensible: an error code mentioning
// PATH, or a message naming a path the server could not resolve. Code is the
// PHI-free signal and is preferred; the message is consulted only when the
// client surfaces it (WithRawErrorBodies).
func isPathResolution(code, message string) bool {
	if strings.Contains(strings.ToUpper(code), "PATH") {
		return true
	}
	m := strings.ToLower(message)
	return strings.Contains(m, "resolve path") ||
		strings.Contains(m, "path resolution") ||
		strings.Contains(m, "unknown path") ||
		strings.Contains(m, "could not resolve")
}
