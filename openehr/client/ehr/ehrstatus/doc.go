// Package ehrstatus is the openEHR REST 1.1.0 EHR_STATUS
// sub-resource client. Reads get the latest EHR_STATUS for an EHR, a
// specific version, or the version that was current at a given time;
// [Put] writes a new version under If-Match optimistic concurrency.
//
// The package also reads the EHR_STATUS's version history: its versioned
// object ([GetVersionedObject]), its revision history
// ([GetRevisionHistory]), and its versions, each with the commit audit
// around the EHR_STATUS: the latest ([GetVersion]), the one current at a
// time ([GetVersionAtTime]), or one by its id ([GetVersionByID]).
//
// EHR_STATUS carries the EHR-wide flags (is_queryable, is_modifiable,
// subject linkage).
package ehrstatus
