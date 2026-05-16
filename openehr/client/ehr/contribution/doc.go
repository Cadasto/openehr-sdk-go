// Package contribution is the openEHR REST 1.1.0-development
// Contribution sub-resource client — multi-version atomic commits
// against an EHR.
//
// A Contribution carries an [github.com/cadasto/openehr-sdk-go/openehr/rm.AuditDetails]
// envelope and a list of versioned references (per REQ-059). The
// server applies the entire batch atomically: either every version
// commits or none do. Errors map per REQ-093; optimistic-concurrency
// failures within the batch return [transport.ErrVersionConflict].
package contribution
