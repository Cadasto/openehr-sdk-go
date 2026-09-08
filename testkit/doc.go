// Package testkit hosts the conformance probes and the vendored fixtures
// they read, for SDK consumers and for the SDK's own test suite.
//
// Named testkit (not testing) to avoid collision with the
// standard-library testing package.
//
// What is here: the probes themselves, one package per area, under
// testkit/probes/; the shared result type and catalog runner under
// testkit/probe/; corpus-scale parity harnesses under
// testkit/conformance/; vendored fixture documents under
// testkit/cassettes/; and fixture-path resolution in testkit/fixtures/.
// This root package exports nothing.
//
// The shared result type and catalog runner live in testkit/probe/
// (REQ-082). Cassette recordings, when captured, land under
// testkit/recordings/ — not under testkit/cassettes/, which holds
// fixture bodies. Several backend-facing probe tests still stand up a
// net/http/httptest server; migrating those onto sandbox/ is the rest
// of the REQ-082 plan.
//
// Conformance probes are the openEHR wire-conformance contract: defined
// once, implemented in each language, and — once REQ-082's modes are
// built — exercised against a sandbox, a replayed recording, or a live
// deployment.
//
// Vendored fixture documents live under testkit/cassettes/ (templates/,
// compositions/, rm/, its_rest/) — request and response BODIES, not
// replayable HTTP exchanges, despite the directory name. Provenance is
// indexed by vendor in testkit/cassettes/README.md. Resolve paths via
// testkit/fixtures. REQ-082 Cassette-mode recordings are a different
// artefact and land under testkit/recordings/ when that mode is built.
package testkit
