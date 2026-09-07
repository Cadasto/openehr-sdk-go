// Package sandbox is the in-memory openEHR backend that backs
// REQ-082's Sandbox probe mode.
//
// [Backend] implements [http.RoundTripper], so a consumer (or the
// probe runner) injects it as the Transport of an *http.Client and
// hands that client to [transport.New]. There is no network listener
// and no credential check — REQ-082 and REQ-013: this package imports
// neither transport/ nor auth/.
//
// State is per-Backend and isolated: two probes sharing one instance
// see each other's writes; two instances do not.
//
// [Handle] / [HandleFunc] / [Scripted] register planted routes that
// take precedence over the built-in EHR surface, so a probe test can
// serve a hostile or fixture response without httptest.NewServer.
package sandbox
