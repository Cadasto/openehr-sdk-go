---
kind: guide
---

# Architecture Decision Records

Closed architecture decisions for `openehr-sdk-go`. Each ADR is a numbered Markdown file with the standard headings (Status, Context, Decision, Consequences). Its frontmatter (`id`, `title`, `status`, `date`) is the source of the index below, which `python3 scripts/sdd-check.py generate` writes. Status reaches **Accepted** before the ADR is considered closed.

Open decisions (those that would be ADRs once resolved) live in this repo as **research strands** — [`docs/specifications/research-strands.md`](../specifications/research-strands.md). When a strand is resolved, an ADR lands here.

<!-- sdd:generated adr-index -->

| ID | Title | Status | Date | Resolves / amends |
|---|---|---|---|---|
| ADR-0001 | BMM version-bump runbook | Accepted | 2026-05-16 | — |
| ADR-0002 | BMM code generator structural decisions | Accepted | 2026-05-16 | — |
| ADR-0003 | Codec polymorphism for abstract generic RM classes | Accepted | 2026-05-16 | — |
| ADR-0004 | Strict-encode, permissive-decode for BMM `Real` and `Integer` | Accepted | 2026-05-16 | — |
| ADR-0005 | Compiled OPT foundation (rminfo + internal templatecompile) | Accepted | 2026-05-22 | — |
| ADR-0006 | Composition validation walker package placement | Accepted | 2026-06-11 | — |
| ADR-0007 | AQL parser: ANTLR + SDK grammar profile | Accepted | 2026-06-15 | — |
| ADR-0008 | SMART discovery: canonical `services` map shape | Accepted | 2026-06-17 | — |
| ADR-0009 | SMART-on-openEHR auth library scope and dependency model | Accepted | 2026-06-18 | — |
| ADR-0010 | Public compiled-template bridge placement | Accepted | 2026-06-17 | — |
| ADR-0011 | RM behavioural-function surface and fallibility policy | Accepted | 2026-06-19 | — |
| ADR-0012 | Retire SDK-GAP as a durable identifier; REQ/PROBE is the feature register | Accepted | 2026-07-02 | — |
| ADR-0013 | Generated LOCATABLE identity surface and reverse type lookup | Accepted | 2026-07-12 | — |
| ADR-0014 | WebTemplate reference implementation and id-generation lock | Accepted | 2026-07-14 | — |
| ADR-0015 | Composition-level FLAT metadata: accept both spellings, emit `ctx/` | Accepted | 2026-08-03 | — |
| ADR-0016 | EVENT_CONTEXT optionals ride the underscore grammar, not new `ctx/` short forms | Accepted | 2026-08-05 | — |
| ADR-0017 | AQL semantic layer: derived containment relation, overlays, opt-in enforcement | Accepted | 2026-08-22 | — |
| ADR-0018 | Raw response bytes on the typed 2xx decode error | Accepted | 2026-08-30 | — |
| ADR-0019 | Definition metadata timestamps: a closed tolerant layout set on decode, RFC 3339 on encode | Accepted | 2026-08-30 | — |
| ADR-0020 | Cassette recordings are HTTP Archive 1.2 | Accepted | 2026-09-07 | — |
| ADR-0021 | Encoded JSON member order is not part of the canonical JSON contract | Accepted | 2026-09-14 | — |
| ADR-0022 | Canonical JSON is encoded by `encoding/json/v2` | Accepted | 2026-09-14 | — |
| ADR-0023 | SMART discovery: the Platform base URL and the OIDC issuer are separate values | Proposed | 2026-10-04 | — |

<!-- /sdd:generated -->

See [docs/architecture.md § Open decisions](../architecture.md#open-decisions) for the strand-to-ADR mapping.
