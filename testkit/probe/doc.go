// Package probe is the single home for the conformance-probe result
// type and the catalog runner (REQ-082).
//
// Every probe family under testkit/probes/ reports through [Result].
// Per-package copies of that type must not exist — a result compared
// across modes has to be the same type, or the runner cannot
// aggregate it.
//
// # The runner picks the backend
//
// A probe receives an already-configured [transport.Client] (or none,
// for an in-repo probe) and must not observe which mode it runs in.
// The mode is a real backend choice, not a label the caller stamps on
// whatever transport it happened to build:
//
//   - In Sandbox mode the runner builds the client itself, from
//     [Config.Sandbox] or a fresh [sandbox.Backend]. A caller-supplied
//     [Config.Client] is refused.
//   - In Cassette mode the caller supplies a client whose transport is
//     a [Replayer] over a HAR that passed [ValidateHAR]. The runner
//     still refuses a sandbox-backed client. Capture a new recording
//     with [Recorder]; the corpus lives under testkit/recordings/.
//   - In Live mode the runner verifies the client is not
//     sandbox-backed. A sandbox transport under either Live or
//     Cassette is a silent fallback to Sandbox, and the summary would
//     claim a deployment or a recording answered when nothing left
//     the process.
//
// # Selection is separate from execution
//
// [Run] executes exactly the entries it is handed, so the whole
// catalog is spelled Run(ctx, cfg, catalog) and a subset is [Select]'s
// output. Neither treats "nothing selected" as "everything": an empty
// list is [ErrEmptySelection].
package probe
