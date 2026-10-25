// Package ehrstatus is the openEHR REST 1.1.0 EHR_STATUS
// sub-resource client. Reads get the latest EHR_STATUS for an EHR, a
// specific version, or the version that was current at a given time;
// [Put] writes a new version under If-Match optimistic concurrency. The
// package also reads the EHR_STATUS's versioned object, its revision
// history and its versions (the latest, the one at a time, or one by id).
//
// EHR_STATUS carries the EHR-wide flags (is_queryable, is_modifiable,
// subject linkage).
package ehrstatus
