package ehr

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// LifecycleState is an openEHR "version lifecycle state" terminology code,
// carried in the `openehr-version` request header to set the committed
// VERSION's lifecycle state. The value set is the openEHR *version
// lifecycle state* group of the openEHR terminology bundled with the SDK;
// the constants name every member.
type LifecycleState string

const (
	// LifecycleStateComplete is code 532 ("complete").
	LifecycleStateComplete LifecycleState = "532"
	// LifecycleStateIncomplete is code 553 ("incomplete").
	LifecycleStateIncomplete LifecycleState = "553"
	// LifecycleStateDeleted is code 523 ("deleted").
	LifecycleStateDeleted LifecycleState = "523"
	// LifecycleStateInactive is code 800 ("inactive").
	LifecycleStateInactive LifecycleState = "800"
	// LifecycleStateAbandoned is code 801 ("abandoned").
	LifecycleStateAbandoned LifecycleState = "801"
)

// IsValid reports whether s is a member of the openEHR *version lifecycle
// state* group in the bundled terminology.
func (s LifecycleState) IsValid() bool {
	return terminology.VersionLifecycleState.Has(string(s))
}

// Rubric returns the English rubric for s from the bundled terminology
// ("complete" for 532), and false when s is not a member of the group.
func (s LifecycleState) Rubric() (string, bool) {
	return terminology.VersionLifecycleState.Rubric(string(s))
}

// FormatLifecycleStateHeader encodes a committed VERSION lifecycle_state
// code into the `openehr-version` request-header value defined by openEHR
// REST 1.1.0-development:
//
//	lifecycle_state.code_string="<code>"
//
// It is the dotted-attribute grammar, not JSON, like
// openehr-audit-details. Returns "" for an empty code; returns an error for
// a code carrying control characters (header-injection guard) or one that
// is not a member of the openEHR *version lifecycle state* group.
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
