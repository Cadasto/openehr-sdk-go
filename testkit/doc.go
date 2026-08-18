// Package testkit provides test doubles, fluent builders,
// recorder/replay helpers, and conformance probes for SDK consumers
// and for the SDK's own test suite. Includes a clock abstraction, a
// JWKS test fixture, and a token-source double.
//
// Named testkit (not testing) to avoid collision with the
// standard-library testing package.
//
// Conformance probes are the openEHR wire-conformance contract:
// defined once, implemented in each language, exercised against
// sandbox, a replayed recording, or a live deployment (REQ-082).
//
// Vendored fixture documents live under testkit/cassettes/ (templates/,
// compositions/, rm/, its_rest/) — request and response BODIES, not
// replayable HTTP exchanges, despite the directory name. Provenance is
// indexed by vendor in testkit/cassettes/README.md. Resolve paths via
// testkit/fixtures. REQ-082 Cassette-mode recordings are a different
// artefact and land under testkit/recordings/ when that mode is built.
package testkit
