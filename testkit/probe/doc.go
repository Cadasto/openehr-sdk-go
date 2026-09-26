// Package probe is the single home for the conformance-probe result
// type and the catalog runner.
//
// Every probe family under testkit/probes/ reports through [Result].
// There are no per-package copies of that type: a result compared
// across modes has to be the same type, or the runner cannot
// aggregate it.
//
// # The runner picks the backend
//
// A probe receives an already-configured [transport.Client] (or none,
// for an in-repo probe) and cannot observe which mode it runs in.
// The mode is a real backend choice that the runner enforces; the caller
// cannot stamp it as a label on whatever transport it happened to build:
//
//   - In Sandbox mode the runner builds the client itself, from
//     [Config.Sandbox] or a fresh [sandbox.Backend]. A caller-supplied
//     [Config.Client] is refused.
//   - In Cassette mode the runner validates the recording file
//     (<id>.har, via [ValidateHAR]) and refuses a sandbox-backed
//     client, but does not otherwise inspect the transport. Wiring a
//     [Replayer] over the recording as [Config.Client] is the caller's
//     job. Capture a new recording with [Recorder]; the corpus lives
//     under testkit/recordings/.
//   - In Live mode the runner verifies the client is not
//     sandbox-backed. A sandbox transport under either Live or
//     Cassette would be a silent fallback to Sandbox, and the summary would
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
