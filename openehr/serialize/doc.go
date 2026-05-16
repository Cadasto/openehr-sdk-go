// Package serialize provides codecs for openEHR data: canonical JSON,
// canonical XML, FLAT, and STRUCTURED formats.
//
// Independently usable without the HTTP client or auth — e.g. for
// archival canonicalization, hashing, or diff tooling.
//
// An open research strand evaluates encoding/json vs sonic vs
// easyjson; the default codec choice and tuning knobs are documented
// here once benchmarked.
package serialize
