---
kind: guide
---

# Development process

How a change moves through this repository: the two lanes and the ladder, in short for a contributor without the sdd plugin, which defines both, and with this repository's own rules.
The conventions they use (document kinds, RFC-2119 force, status headers, identifiers, the traceability chain) live once, in [specifications/README.md](specifications/README.md).
The agent loop is [ai-workflow.md § The loop](ai-workflow.md#the-loop), the machine-readable side is [`.sdd.yaml`](.sdd.yaml), which the `sdd-*` skills read first, and the conformance probe harness is [probe-harness.md](probe-harness.md).

## Two lanes

Ask one question of every change: **does it alter a normative statement?** That covers a requirement's acceptance, a spec section's behaviour, a public API shape and an error contract.

| | Full lane (the answer is yes) | Maintenance lane (the answer is no) |
|---|---|---|
| Typical change | New capability; an API, behaviour or error-contract change | Refactor, package move, performance, dependency bump, tooling, doc polish; a bug fix that brings code back in line with the spec, or that shows a spec sentence was wrong |
| Spec, registry, ADR | Updated in the same PR (spec-first for new capability) | Only the sentence a bug fix shows was wrong, reworded in the same PR, adding no obligation and widening no contract. Any other spec edit, or an ADR, makes the change full lane |
| `traceability.yaml` | Updated to the landed packages and probes; `make spec-generate` writes the tests | Only the package paths `make spec-check` names, when paths moved |
| Gate | `make ci` | `make ci` |
| PR body | The REQ / PROBE ids touched | One line: `Lane: maintenance (no normative change)` |

A bug fix that shows a spec sentence was wrong carries the corrected sentence in the same PR (the sdd plugin's *implementation-aligned* mode): code wins until the sentence is updated, never later than that PR.

`make spec-check` runs in both lanes, so the map cannot rot whichever lane a PR claims. A reviewer who finds a normative change in a maintenance-lane PR moves it to the full lane; that is a finding against the PR, not against the process.

## The ladder (full lane)

A full-lane change climbs, in one PR: the REQ and its acceptance, its spec section, an ADR only for an irreversible fork, then, once the [Definition of Ready](#definition-of-ready) holds, code and tests, and last the map, which the [Definition of Done](#definition-of-done) closes.
For a new capability the spec leads (spec-first).

- The registry in [REQ.md](specifications/REQ.md) is generated (`make spec-generate`); `make spec-check` fails when it is stale. Edit its source, never the table.
- Each `traceability.yaml` row's `tests:` list is generated too: `make spec-generate` writes the sorted list of test files that cite the row's REQ, and `make spec-check` fails when a list is stale. A test file is a `*_test.go` file or a probe implementation under `testkit/probes/`; it cites `REQ-NNN` when that token appears with no letter, digit or underscore right before or after it, so a hyphen or a dot is a boundary and `pre-REQ-161` counts. So cite the REQ in the test that pins it, and never edit the list by hand.
- A plan in [`plans/`](plans/) is optional, for work that spans several PRs: a working note outside this ladder that no gate or reviewer reads, deleted once its work is served ([plans/README.md](plans/README.md)).

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
- The canonical spec text is current, and `traceability.yaml` lists the landed packages and probes (`make spec-generate` writes its tests lists).
- `docs/roadmap.md` is updated when the change lands a roadmap row or opens new tracked work ([roadmap.md § Updating this page](roadmap.md#updating-this-page)).
- `make ci` passes (it includes `make spec-check`).

## superpowers + SDD

The superpowers skills own the build, verify and branch loop (brainstorming, planning, TDD, execution, verification, code review, finishing a branch). The `sdd-*` skills own the specification and its traceability. superpowers writes under a `docs/superpowers/` tree by default; that tree must never become a second source of truth:

| superpowers output | Canonical home |
|---|---|
| `brainstorming` design doc | narrative input; its normative statements go into a topic spec in [specifications/](specifications/) via `sdd-specify`, its narrative into [architecture.md](architecture.md) if anywhere |
| `writing-plans` plan | [`docs/plans/YYYY-MM-DD-<slug>.md`](plans/), started from the template |

## Conformance probes

How probes are written, run and kept, the CDR cross-check tool and the vendored fixtures are in [probe-harness.md](probe-harness.md).
