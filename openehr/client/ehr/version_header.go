package ehr

import "errors"

// FormatLifecycleStateHeader encodes a committed VERSION lifecycle_state
// code into the `openehr-version` request-header value defined by openEHR
// REST 1.1.0-development (REQ-059):
//
//	lifecycle_state.code_string="<code>"
//
// The code is an openEHR "version lifecycle state" terminology value —
// "532" complete, "553" incomplete, "523" deleted. It is the dotted-
// attribute grammar, not JSON — the same family as openehr-audit-details.
// Returns "" for an empty code and rejects control characters (header-
// injection guard).
func FormatLifecycleStateHeader(code string) (string, error) {
	if code == "" {
		return "", nil
	}
	if hasCtrlChars(code) {
		return "", errors.New("ehr: lifecycle_state code contains control characters")
	}
	return `lifecycle_state.code_string="` + escapeItemTagValue(code) + `"`, nil
}
