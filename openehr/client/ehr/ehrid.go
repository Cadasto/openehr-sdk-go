package ehr

// EHRID identifies an EHR root. Distinct type from the openEHR
// HierObjectID it wraps so call sites cannot accidentally mix it with
// [client.VersionedObjectID] / [client.VersionUID]. The string form is
// the UID value (typically a UUID), not the full HierObjectID JSON
// envelope.
type EHRID string

// String returns the EHRID as a plain string for URL-path composition.
func (e EHRID) String() string { return string(e) }
