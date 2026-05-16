// Package directory is the openEHR REST 1.1.0-development Directory
// (folder hierarchy) sub-resource client. Read paths land here; the
// Save / Update / Delete versioned-write surface arrives in Phase 4
// of the REST API client plan.
//
// The Directory is a single versioned FOLDER per EHR. The three read
// variants — latest, at-time, and by-version — match the
// EHR_STATUS sub-resource shape so consumers can rely on a consistent
// surface across versioned-but-singleton resources.
package directory
