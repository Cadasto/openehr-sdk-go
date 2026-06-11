package query

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// ErrInvalidConfig indicates invalid executor options or query input.
var ErrInvalidConfig = errors.New("query: invalid configuration")

// AQLError is an AQL-level failure distinct from generic transport
// errors (parse error, timeout). Detect with errors.As.
type AQLError struct {
	Message string
	Code    string
	Inner   error
}

func (e *AQLError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("query: %s", e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("query: %s", e.Code)
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
			Message: we.OpenEHR.Message,
			Code:    we.OpenEHR.Code,
			Inner:   err,
		}
	}
	if we.StatusCode == 400 || we.StatusCode == 408 {
		return &AQLError{Message: we.Error(), Inner: err}
	}
	return err
}
