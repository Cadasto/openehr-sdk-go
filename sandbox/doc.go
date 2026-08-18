// Package sandbox will provide the in-memory and replayed-recording
// transports that back the Sandbox and Cassette probe modes — the same
// client interfaces as the production REST clients, for fast, hermetic
// tests of applications built on the SDK.
//
// NOT YET IMPLEMENTED. This package is a reserved name carrying no code:
// the probe suite currently stands up a hand-written net/http/httptest
// server per test instead, and no recording format exists. Both modes are
// specified by REQ-082 (docs/specifications/conformance.md) and sequenced
// by docs/plans/2026-08-18-probe-runnability.md. Do not import it yet.
package sandbox
