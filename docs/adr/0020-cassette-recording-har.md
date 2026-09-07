# ADR 0020 — Cassette recordings are HTTP Archive 1.2

- **Status:** Accepted, 2026-09-07.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** [STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml).
- **Introduces:** —. **Amends:** [REQ-082](../specifications/conformance.md#req-082--runnability) Cassette encoding.
- **Plan:** [2026-08-18-probe-runnability.md](../plans/2026-08-18-probe-runnability.md).
- **Related:** the side-by-side capture in [strand-11-evidence](../plans/strand-11-evidence/).

## Context

REQ-082 requires Cassette mode to replay a checked-in HTTP recording. The requirement already
states what a recording must carry — provenance, capture-time redaction attestation, and
order-preserving normalised matching — but left the on-disk encoding open as STRAND-11:
HTTP Archive (`.har`) versus a purpose-built YAML schema.

The strand asked for one real capture, serialised both ways, reviewed as a diff. That capture
is a live `POST /ehr` against EHRbase 2.35.1 ([ehr-create.har](../plans/strand-11-evidence/ehr-create.har)
and [ehr-create.yaml](../plans/strand-11-evidence/ehr-create.yaml)). The remaining question is
which file a second implementation (or a reviewer) can treat as the corpus.

## Decision

**Cassette recordings are HTTP Archive 1.2 documents with a `.har` suffix.**

- One file is the whole artefact. A PHP or other-language implementation of the same probe
  can replay the same corpus with any HAR reader.
- REQ-082's provenance and redaction attestation — which HAR 1.2 does not model — live on
  `log` as a `_req082` object (HAR's documented extension slot). Tools that ignore unknown
  fields still see a valid `log.entries` list.
- The matching, redaction, and fail-closed replay rules stay in REQ-082. This ADR picks the
  encoding, not those rules.

## Consequences

- The corpus under `testkit/recordings/` is `.har`. A recording without `_req082.provenance`
  or without `_req082.redaction.ran: true` is discarded, not replayed.
- Review diffs are larger and noisier than the YAML alternative. That cost is accepted so
  the corpus stays a published interchange format rather than an SDK-private schema.
- Browser-oriented HAR fields (`timings`, `cache`, `pageref`) are unused. Recorders MAY omit
  them; replayers MUST ignore them.
- Reversing this later is a corpus migration: every checked-in recording would have to be
  rewritten. That is the one-way door this ADR exists to walk.

## Alternatives considered

- **Purpose-built YAML.** Shorter review diffs and native provenance keys (the YAML twin
  in the evidence directory shows this clearly). **Rejected:** a second implementation
  would have to learn an SDK-private schema. Portability of the probe catalog was the
  reason Cassette mode exists; a private encoding spends that reason.
- **HAR plus a sidecar for provenance.** Keeps HAR "pure". **Rejected:** two files can
  drift, and a recording whose provenance cannot be stated MUST be discarded (REQ-082).
  One artefact cannot lose its attestation.
