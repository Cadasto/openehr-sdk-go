---
kind: plan
---

# Plan — Spec integrity: probe classes, security rules with force, one CHANGELOG rule, real evidence

**Date:** 2026-09-30
**Status:** Done in PR 192; from the audit of the PR 181–189 review leftovers
**Covers:** REQ-082 ([conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability)), REQ-062 and REQ-064 (auth.md, *ID-token verification algorithm agility*), REQ-024 ([idiom.md § Generics policy](../specifications/idiom.md#generics-policy-req-024)), REQ-043 ([bmm-conformance.md § Mapping rules](../specifications/bmm-conformance.md#mapping-rules)), and the process rules in [development-process.md](../development-process.md)
**Probes:** the in-repo probes of the catalogue (listed in Phase 1)
**Depends on:** nothing; the Phase 3 decision is taken (the release cut)
**Defers:** the open items are Known gaps in [conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability) and auth.md *ID-token verification algorithm agility*; the passing REQ mentions in tests (about 45 to 50) are left for a later sweep

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Five places where the specification says something false, or leaves a rule the code enforces without the force that makes it a rule. Each one misleads the next implementer or reviewer.

## Evidence (main `bfa10c10`)

| Problem | Where |
|---|---|
| About 15 probes that take no client and reach no backend declare `Sandbox` in their Modes line, against § REQ-082's own rule for in-repo probes. The guarded sentence "16 of the 75 catalog entries are in-repo by construction" is therefore false (the real count is about 31). The guard counts declarations, so it cannot see this. The audit found PROBE-020, -022 to -028, -030, -031, -033, -034, -038, -074 and -088. | `conformance.md:46-48` and the catalogue entries; `scripts/spec-check.sh:357-372`; the probe signatures under `testkit/probes/` |
| The ID-token rules the code enforces carry no RFC-2119 force: the `none` algorithm is always rejected, the discovery allowlist can narrow the supported set but never widen it, the signature is verified before any claim is read, and the clock skew is 30 seconds. Under the spec's own convention, text without a keyword is informative. | `auth.md:224-231` |
| REQ-024 (no reflection) has no binding sentence: its only keyword is a label inside a code comment, yet other specs cite REQ-024 as the reason for being reflection-free. | `idiom.md:105-119` |
| Three texts require CHANGELOG entries that AGENTS.md forbids outside a release cut: a status transition needs an `[Unreleased]` entry, probe transitions go in the CHANGELOG, and interface growth is named for the CHANGELOG. Agents meet the contradiction on every spec edit. | `specifications/README.md:128`, `conformance.md:940`, `wire.md:379` against `AGENTS.md:68` |
| REQ-043's whole `tests:` list is two files that say they stay independent of REQ-043; at least 12 test files cite a REQ only to disclaim it, and `make spec-gen` counts them as evidence. | `rminfo/probe_094_test.go:529`, `rminfo/probe_098_test.go:495` (REQ-043, REQ-047), `typereg/nilreceiver_census_test.go` (REQ-024), `version_trim_test.go` (REQ-150) |

## Phases

### Phase 1 — the in-repo probes declare what they are

**Tasks:**
- Confirm each listed probe against its signature (no client parameter, no sandbox or recording use), then change its Modes line to `In-repo` and its Status to the inline form § REQ-082 gives in-repo probes. Update the coverage matrix labels.
- Move the census sentence to the true count; `make spec-check` forces the sentence and the declarations to agree.
- Optional, recommended: make the check see the class, not just the declaration. For example, a test under `testkit/probes` that fails when a probe function without a client parameter is not declared `In-repo`.
- Ride-along: the comment at `testkit/probe/replay.go:88-93` points at "REQ-082 traceability notes", which no longer exist; point it at § REQ-082.

**Definition of done:** every in-repo probe says so; the census is true; `make ci` green.

### Phase 2 — security and reflection rules get their force

**Tasks:**
- Restate the four ID-token rules as binding sentences in the auth.md section, and check each has a test that fails when its guard is removed (the `smart` package tests); add the missing ones.
- Give REQ-024 one binding sentence in its canonical section: no reflection in library code, with any exception the section already describes stated explicitly.

**Definition of done:** the `rfc2119` family stops warning on these sections; each rule has a named can-fail test.

### Phase 3 — one CHANGELOG rule

**Maintainer decision:** follow AGENTS.md, where the CHANGELOG is written at the release cut (recommended, since that is the practice), or the spec texts. Decided: the release cut.

**Tasks:**
- Reword the three texts to match the decision. With the recommended option, state where a status transition is recorded instead (the traceability map and the PR body).

**Definition of done:** one rule, stated once and cited elsewhere.

### Phase 4 — evidence that is evidence

**Tasks:**
- Cite REQ-043 in the `internal/bmmgen` tests that do exercise the mapping rules, so its row lists real evidence.
- Reword the disclaiming comments so they name the rule without the REQ id, then run `make spec-gen`. The audit counted 741 (REQ, file) pairs, about 45–50 passing mentions and at least 12 disclaimers; start with the disclaimers.

**Definition of done:** no `tests:` entry cites a REQ only to disclaim it; REQ-043's list names at least one bmmgen test.
