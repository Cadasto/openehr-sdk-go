// Package ehrstatus is the openEHR REST 1.1.0-development EHR_STATUS
// sub-resource client. Reads get the latest EHR_STATUS for an EHR, a
// specific version, or the version that was current at a given time;
// [Put] writes a new version under If-Match optimistic concurrency.
//
// EHR_STATUS carries the EHR-wide flags (is_queryable, is_modifiable,
// subject linkage).
package ehrstatus
