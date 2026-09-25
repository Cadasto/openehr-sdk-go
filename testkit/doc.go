// Package testkit hosts the conformance probes and the vendored fixtures
// they read, for SDK consumers and for the SDK's own test suite.
//
// It is named testkit (not testing) to avoid a collision with the
// standard-library testing package.
//
// What is here: the probes themselves, one package per area, under
// testkit/probes/; the shared result type and catalog runner under
// testkit/probe/; corpus-scale parity harnesses under
// testkit/conformance/; vendored fixture documents under
// testkit/cassettes/; and fixture-path resolution in testkit/fixtures/.
// This root package exports nothing.
//
// Cassette recordings, when captured, land under testkit/recordings/,
// not under testkit/cassettes/, which holds fixture bodies. Several
// backend-facing probe tests still stand up a net/http/httptest server
// instead of running on sandbox/.
//
// Conformance probes are the openEHR wire-conformance contract: defined
// once, implemented in each language, and exercised against a sandbox,
// a replayed recording, or a live deployment.
//
// Vendored fixture documents live under testkit/cassettes/ (templates/,
// compositions/, rm/, its_rest/). Despite the directory name they are
// request and response bodies, not replayable HTTP exchanges. Provenance
// is indexed by vendor in testkit/cassettes/README.md. Resolve paths via
// testkit/fixtures.
package testkit
