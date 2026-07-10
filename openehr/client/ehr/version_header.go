package ehr

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// LifecycleState is an openEHR "version lifecycle state" terminology code,
// carried in the `openehr-version` request header to set the committed
// VERSION's lifecycle state (REQ-059). The closed value set mirrors the
// openEHR terminology group.
type LifecycleState string

const (
	// LifecycleStateComplete is code 532 ("complete").
	LifecycleStateComplete LifecycleState = "532"
	// LifecycleStateIncomplete is code 553 ("incomplete").
	LifecycleStateIncomplete LifecycleState = "553"
	// LifecycleStateDeleted is code 523 ("deleted").
	LifecycleStateDeleted LifecycleState = "523"
)

// IsValid reports whether s is one of the openEHR version-lifecycle-state
// codes.
func (s LifecycleState) IsValid() bool {
	switch s {
	case LifecycleStateComplete, LifecycleStateIncomplete, LifecycleStateDeleted:
		return true
	default:
		return false
	}
}

// FormatLifecycleStateHeader encodes a committed VERSION lifecycle_state
// code into the `openehr-version` request-header value defined by openEHR
// REST 1.1.0-development (REQ-059):
//
//	lifecycle_state.code_string="<code>"
//
// It is the dotted-attribute grammar, not JSON — the same family as
// openehr-audit-details. Returns "" for an empty code; returns an error for
// a code carrying control characters (header-injection guard) or one that
// is not a recognised version-lifecycle-state value.
func FormatLifecycleStateHeader(s LifecycleState) (string, error) {
	if s == "" {
		return "", nil
	}
	if hasCtrlChars(string(s)) {
		return "", fmt.Errorf("%w: lifecycle_state code contains control characters", transport.ErrInvalidConfig)
	}
	if !s.IsValid() {
		return "", fmt.Errorf("%w: unsupported lifecycle_state code %q", transport.ErrInvalidConfig, string(s))
	}
	return `lifecycle_state.code_string="` + escapeItemTagValue(string(s)) + `"`, nil
}
