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
// sandbox, recorded cassette, or a live deployment.
//
// Vendored fixtures live under testkit/cassettes/ (templates/,
// compositions/, rm/, its_rest/). Provenance is indexed by vendor in
// testkit/cassettes/README.md. Resolve paths via testkit/fixtures.
package testkit
