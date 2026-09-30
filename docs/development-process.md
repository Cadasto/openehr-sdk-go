---
kind: guide
---

# Development process

How a change moves through this repository. This page owns the two lanes and the ladder. The conventions they use (document kinds, RFC-2119 force, status headers, identifiers, the traceability chain) live once, in [specifications/README.md](specifications/README.md); the agent loop is in [ai-workflow.md § The loop](ai-workflow.md#the-loop).

The machine-readable side (identifier style, paths, build targets, the `PROBE`/`STRAND` toggles, the ground-truth source) is [`.sdd.yaml`](.sdd.yaml). The `sdd-*` skills read it first.

## Two lanes

Ask one question of every change: **does it alter a normative statement?** That covers a requirement's acceptance, a spec section's behaviour, a public API shape and an error contract.

| | Full lane (the answer is yes) | Maintenance lane (the answer is no) |
|---|---|---|
| Typical change | New capability; an API, behaviour or error-contract change; a bug fix that shows the spec was wrong | Refactor, package move, performance, dependency bump, tooling, doc polish; a bug fix that brings code back in line with the existing spec |
| Spec, registry, ADR | Updated in the same PR (spec-first for new capability) | Not touched. Needing to touch one means the change is full lane |
| `traceability.yaml` | Updated to the landed packages and probes; `make spec-gen` writes the tests | Only the package paths `make spec-check` names, when paths moved |
| Gate | `make ci` | `make ci` |
| PR body | The REQ / PROBE ids touched | One line: `Lane: maintenance (no normative change)` |

`make spec-check` runs in both lanes, so the map cannot rot whichever lane a PR claims. A reviewer who finds a normative change in a maintenance-lane PR moves it to the full lane; that is a finding against the PR, not against the process.

## The ladder (full lane)

```
REQ  (capability + acceptance)                  [gate: worth doing]
 └─ SPEC §  (RFC-2119, Status: Draft)            [gate: single canonical home]
     └─ ADR  (only if an irreversible fork)      [gate: Accepted before code]
         └─ CODE + TESTS  (tests cite REQ/PROBE)   [gate: Definition of Ready, then tests green]
             └─ traceability.yaml + make spec-gen  [gate: Definition of Done, same PR]
```

For new capability the spec leads (spec-first). When work on shipped code shows the spec itself was wrong, the code change and the spec correction land in the same PR (implementation-aligned): code wins until the spec is updated, never later than that PR.

- The registry in [REQ.md](specifications/REQ.md) is generated (`make spec-gen`); `make spec-check` fails when it is stale. Edit its source, never the table.
- Each `traceability.yaml` row's `tests:` list is generated too: `make spec-gen` writes the sorted list of test files that cite the row's REQ, and `make spec-check` fails when a list is stale. A test file is a `*_test.go` file or a probe implementation under `testkit/probes/`; it cites `REQ-NNN` when that token appears with no letter, digit or underscore right before or after it, so a hyphen or a dot is a boundary and `pre-REQ-117` counts. So cite the REQ in the test that pins it, and never edit the list by hand.
- A plan in [`plans/`](plans/) is optional, for work that spans several PRs. It is a committed working note outside this ladder: no generator or map reads it, but the drift gate checks it like any other document ([plans/README.md](plans/README.md)).

There is no `SDK-GAP` identifier. `REQ`/`PROBE` is the feature register, and a newly found gap is worked under a REQ with a `PROBE` for wire conformance ([ADR 0012](adr/0012-retire-sdk-gap-identifier.md)).

### Definition of Ready

Implementation may start when:

- The work names every REQ-NNN it implements (and any STRAND-NN or ADR).
- Canonical normative text exists for each of those REQs, in its topic spec, with a `traceability.yaml` entry.
- Any irreversible fork has an **Accepted** [ADR](adr/).
- The inputs and states the change must refuse, and how it fails on them, are cited from the canonical spec.
- The verification command is named (`make ci`, `make spec-check`, probes).

### Definition of Done

All in the implementing PR:

- Code and tests land; tests cite the `REQ-` / `PROBE-` they pin.
- The canonical spec text is current, and `traceability.yaml` lists the landed packages and probes (`make spec-gen` writes its tests lists from the tests' REQ citations).
- `docs/roadmap.md` is updated when the change lands a roadmap row or opens new tracked work ([roadmap.md § Updating this page](roadmap.md#updating-this-page)).
- `make ci` passes (it includes `make spec-check`).

## superpowers + SDD

The superpowers skills own the build, verify and branch loop (brainstorming, planning, TDD, execution, verification, code review, finishing a branch). The `sdd-*` skills own the specification and its traceability. superpowers writes under a `docs/superpowers/` tree by default; that tree must never become a second source of truth:

| superpowers output | Canonical home |
|---|---|
| `brainstorming` design doc | narrative input; its normative statements go into a topic spec in [specifications/](specifications/) via `sdd-specify`, its narrative into [architecture.md](architecture.md) if anywhere |
| `writing-plans` plan | [`docs/plans/YYYY-MM-DD-<slug>.md`](plans/), started from the template |

Never settle an open question silently in a PR: raise a [STRAND](specifications/research-strands.md), land an [ADR](adr/), or ask.
