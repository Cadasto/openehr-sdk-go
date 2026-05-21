// Package itemtags implements openEHR REST 1.1.0 ItemTag operations via
// the openehr-item-tag and openehr-version-item-tag headers (REQ-059).
//
// Tags are associated with versioned EHR resources (Composition,
// EHR_STATUS, Directory). Reads use GET; writes attach encoded tag
// lists to PUT on the same resource paths.
package itemtags
