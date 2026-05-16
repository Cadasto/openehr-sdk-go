// Package testkit provides test doubles, fluent builders,
// recorder/replay helpers, and conformance probes for SDK consumers
// and for the SDK's own test suite. Includes a clock abstraction, a
// JWKS test fixture, and a token-source double.
//
// Named testkit (not testing) to avoid collision with the
// standard-library testing package.
//
// Conformance probes are the cross-SDK contract with the PHP SDK:
// defined once, implemented in each language, exercised against
// sandbox, recorded cassette, or a live deployment.
package testkit
