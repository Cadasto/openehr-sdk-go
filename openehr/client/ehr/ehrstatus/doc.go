// Package ehrstatus is the openEHR REST 1.1.0-development EHR_STATUS
// sub-resource client. Read paths: get the latest EHR_STATUS for an
// EHR, get a specific version, or get the version that was current at
// a given time.
//
// EHR_STATUS carries the EHR-wide flags (is_queryable, is_modifiable,
// subject linkage). Read-only operations land here; writes (Phase 4
// of the REST API client plan) will arrive alongside the optimistic-
// concurrency contract (REQ-054).
package ehrstatus
