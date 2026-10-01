---
kind: adr
id: ADR-0020
title: "Cassette recordings are HTTP Archive 1.2"
status: accepted
date: 2026-09-07
---

# ADR 0020 — Cassette recordings are HTTP Archive 1.2

- **Status:** Accepted, 2026-09-07.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** [STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml).
- **Introduces:** —. **Amends:** [REQ-082](../specifications/conformance.md#req-082--runnability) Cassette encoding.
- **Related:** the side-by-side capture in [strand-11-evidence](../plans/strand-11-evidence/).

## Context

REQ-082 requires Cassette mode to replay a checked-in HTTP recording. The requirement already
states what a recording must carry — provenance, capture-time redaction attestation, and
order-preserving normalised matching — but left the on-disk encoding open as STRAND-11:
HTTP Archive (`.har`) versus a purpose-built YAML schema.

The strand asked for one real capture, serialised both ways, reviewed as a diff. That capture
is a live `POST /ehr` against EHRbase 2.35.1 ([ehr-create.har](../plans/strand-11-evidence/ehr-create.har)
and [ehr-create.yaml](../plans/strand-11-evidence/ehr-create.yaml)). The remaining question is
which file a second implementation (or a reviewer) can treat as the recording.

## Decision

**Cassette recordings are HTTP Archive 1.2 documents with a `.har` suffix.**

- One file is the whole artefact. A PHP or other-language implementation of the same probe
  can replay the same recordings with any HAR reader.
- REQ-082's provenance and redaction attestation — which HAR 1.2 does not model — live on
  `log` as a `_req082` object (HAR's documented extension slot). Tools that ignore unknown
  fields still see a valid `log.entries` list.
- The matching, redaction, and fail-closed replay rules stay in REQ-082. This ADR picks the
  encoding, not those rules.

## Consequences

- The recordings, once captured under `testkit/recordings/`, will be `.har`. A recording without
  `_req082.provenance` or without `_req082.redaction.ran: true` is discarded, not replayed.
- Review diffs are larger and noisier than the YAML alternative. That cost is accepted so
  the recordings stay in a published interchange format rather than an SDK-private schema.
- Browser-oriented HAR fields (`timings`, `cache`, `pageref`) are unused. Recorders are free to
  omit them, and replay does not read them: the normalised match key is defined in REQ-082
  (conformance.md § Cassette mode) and these fields are outside it.
- Reversing this later means migrating the recordings: every checked-in recording would have to be
  rewritten. That is the one-way door this ADR exists to walk.

## Alternatives considered

- **Purpose-built YAML.** Shorter review diffs and native provenance keys (the YAML twin
  in the evidence directory shows this clearly). **Rejected:** a second implementation
  would have to learn an SDK-private schema. Portability of the probe catalog was the
  reason Cassette mode exists; a private encoding spends that reason.
- **HAR plus a sidecar for provenance.** Keeps HAR "pure". **Rejected:** two files can
  drift, and REQ-082 discards a recording whose provenance cannot be stated
  ([conformance.md § Cassette mode](../specifications/conformance.md#cassette-mode)).
  One artefact cannot lose its attestation.
