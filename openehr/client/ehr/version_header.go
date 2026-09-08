package ehr

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// LifecycleState is an openEHR "version lifecycle state" terminology code,
// carried in the `openehr-version` request header to set the committed
// VERSION's lifecycle state (REQ-059). The value set is the openEHR *version
// lifecycle state* group of the pinned terminology (REQ-034); the constants
// name every member.
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

// IsValid reports whether s is a member of the pinned openEHR *version
// lifecycle state* group — the membership verdict the group itself gives,
// not a list restated here (REQ-034).
func (s LifecycleState) IsValid() bool {
	return terminology.VersionLifecycleState.Has(string(s))
}

// Rubric returns the pinned English rubric for s — "complete" for 532 — and
// false when s is not a member of the group. Rubrics come from the pin, so
// none is typed beside a code in this package (REQ-034).
func (s LifecycleState) Rubric() (string, bool) {
	return terminology.VersionLifecycleState.Rubric(string(s))
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
