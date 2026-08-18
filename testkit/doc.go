// Package testkit hosts the conformance probes and the vendored fixtures
// they read, for SDK consumers and for the SDK's own test suite.
//
// Named testkit (not testing) to avoid collision with the
// standard-library testing package.
//
// What is here: the probes themselves, one package per area, under
// testkit/probes/; corpus-scale parity harnesses under
// testkit/conformance/; vendored fixture documents under
// testkit/cassettes/; and fixture-path resolution in testkit/fixtures/.
// This root package exports nothing.
//
// What is NOT here, despite earlier documentation claiming otherwise:
// test doubles, fluent builders, a clock abstraction, a JWKS fixture, a
// token-source double, recorder/replay helpers, and a probe runner. The
// runner and the recorder are specified by REQ-082
// (docs/specifications/conformance.md) and sequenced by
// docs/plans/2026-08-18-probe-runnability.md; the rest are not currently
// planned. Each probe is reached from its own test, and every probe that
// needs a backend stands up a net/http/httptest server of its own.
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
