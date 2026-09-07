// Package probe is the single home for the conformance-probe result
// type and the catalog runner (REQ-082).
//
// Every probe family under testkit/probes/ reports through [Result].
// Per-package copies of that type must not exist — a result compared
// across modes has to be the same type, or the runner cannot
// aggregate it.
//
// The runner picks the backend. A probe receives an already-configured
// [transport.Client] (or none, for an in-repo probe) and must not
// observe which mode it runs in.
package probe
