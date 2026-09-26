# Development process

How a change moves through this repository. This page owns the two lanes and the ladder. The conventions they use (document kinds, RFC-2119 force, status headers, identifiers, the traceability chain) live once, in [specifications/README.md](specifications/README.md); the agent loop is in [ai-workflow.md § The loop](ai-workflow.md#the-loop).

The machine-readable side (identifier style, paths, build targets, the `PROBE`/`STRAND` toggles, the ground-truth source) is [`.sdd.yaml`](.sdd.yaml). The `sdd-*` skills read it first.

## Two lanes

Ask one question of every change: **does it alter a normative statement?** That covers a requirement's acceptance, a spec section's behaviour, a public API shape and an error contract.

| | Full lane (the answer is yes) | Maintenance lane (the answer is no) |
|---|---|---|
| Typical change | New capability; an API, behaviour or error-contract change; a bug fix that shows the spec was wrong | Refactor, package move, performance, dependency bump, tooling, doc polish; a bug fix that brings code back in line with the existing spec |
| Spec, registry, ADR | Updated in the same PR (spec-first for new capability) | Not touched. Needing to touch one means the change is full lane |
| Plan | A plan in [`plans/`](plans/) when the work spans several PRs | None |
| `traceability.yaml` | Updated to the landed packages, tests and probes | Only the rows `make spec-check` names, when paths moved |
| Gate | `make ci` | `make ci` |
| PR body | The REQ / PROBE ids touched | One line: `Lane: maintenance — no normative change` |

`make spec-check` runs in both lanes, so the map cannot rot whichever lane a PR claims. A reviewer who finds a normative change in a maintenance-lane PR moves it to the full lane; that is a finding against the PR, not against the process.

## The ladder (full lane)

```
REQ  (capability + acceptance)                  [gate: worth doing]
 └─ SPEC §  (RFC-2119, Status: Draft)            [gate: single canonical home]
     └─ ADR  (only if an irreversible fork)      [gate: Accepted before code]
         └─ PLAN  (only if several PRs)          [gate: Definition of Ready]
             └─ CODE + TESTS  (tests cite REQ/PROBE)       [gate: tests green]
                 └─ traceability.yaml + make spec-gen      [gate: same PR]
                     └─ plan Status: Done (if a plan exists)   [gate: Definition of Done]
```

For new capability the spec leads (spec-first). When work on shipped code shows the spec itself was wrong, the code change and the spec correction land in the same PR (implementation-aligned): code wins until the spec is updated, never later than that PR.

- The Definition of Ready and Definition of Done, and the plan header, are in [plans/_template.md](plans/_template.md).
- The registry in [REQ.md](specifications/REQ.md) and the [plan index](plans/README.md) are generated (`make spec-gen`); `make spec-check` fails when either is stale. Edit their sources, never the tables.
- A plan never moves when it lands: its **Status:** line changes to `Done`.

There is no `SDK-GAP` identifier. `REQ`/`PROBE` is the feature register, and a newly found gap is worked under a REQ with a `PROBE` for wire conformance ([ADR 0012](adr/0012-retire-sdk-gap-identifier.md)).

## superpowers + SDD

The superpowers skills own the build, verify and branch loop (brainstorming, planning, TDD, execution, verification, code review, finishing a branch). The `sdd-*` skills own the specification and its traceability. superpowers writes under a `docs/superpowers/` tree by default; that tree must never become a second source of truth:

| superpowers output | Canonical home |
|---|---|
| `brainstorming` design doc | narrative input; its normative statements go into a topic spec in [specifications/](specifications/) via `sdd-specify`, its narrative into [architecture.md](architecture.md) if anywhere |
| `writing-plans` plan | [`docs/plans/YYYY-MM-DD-<slug>.md`](plans/), started from the template |

Never settle an open question silently in a PR: raise a [STRAND](specifications/research-strands.md), land an [ADR](adr/), or ask.
