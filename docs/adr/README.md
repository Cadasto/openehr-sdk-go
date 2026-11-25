---
kind: guide
---

# Architecture Decision Records

Closed architecture decisions for `openehr-sdk-go`. Each ADR is a numbered Markdown file with the standard headings (Status, Context, Decision, Consequences). Its frontmatter (`id`, `title`, `status`, `date`) is the source of the index below, which `python3 scripts/sdd-check.py generate` writes. Status reaches **Accepted** before the ADR is considered closed.

Open decisions (those that would be ADRs once resolved) live in this repo as **research strands** — [`docs/specifications/research-strands.md`](../specifications/research-strands.md). When a strand is resolved, an ADR lands here, and the index's *Resolves / amends* column names the strand.

## Status

An ADR's status runs `proposed`, then `accepted`.
An accepted ADR is amended in place: its stale part is rewritten to the current state, and one `**Amended:** <date>: <what changed>` bullet sits beside its **Status:** line, updated by each later amendment.
An ADR is superseded when another ADR or a specification section takes over its whole decision, and withdrawn when the decision goes with nothing in its place.
Either way its file is deleted, and an entry in [superseded.md](superseded.md) says where the decision lives now, or that it was withdrawn.
ADR numbers are never reused: a new ADR takes the next number above the highest in the index or in [superseded.md](superseded.md).
A new ADR records only a decision no ADR or specification section covers; the **Amends:** lines of earlier ADRs that name another ADR, such as [ADR 0034](0034-sdk-authored-root-schemas.md)'s, predate this rule.

## Index

<!-- sdd:generated adr-index -->

| ID | Title | Status | Date | Resolves / amends |
|---|---|---|---|---|
| ADR-0002 | BMM code generator structural decisions | Accepted | 2026-05-16 | — |
| ADR-0007 | AQL parser: ANTLR + SDK grammar profile | Accepted | 2026-06-15 | — |
| ADR-0009 | SMART-on-openEHR auth library scope and dependency model | Accepted | 2026-06-18 | Resolves: STRAND-05; Amends: REQ-061, REQ-062, REQ-063, REQ-064 |
| ADR-0011 | RM behavioural-function surface and fallibility policy | Accepted | 2026-06-19 | — |
| ADR-0013 | Generated LOCATABLE identity surface and reverse type lookup | Accepted | 2026-07-12 | Amends: REQ-040, REQ-041 |
| ADR-0014 | The EHRbase `openEHR_SDK` is the reference for the Web Template and the FLAT and STRUCTURED spellings, with two id departures | Accepted | 2026-07-14 | Amends: REQ-053 |
| ADR-0015 | FLAT composition metadata: `ctx/` for the six respelled scalars, real paths for the composer and the EVENT_CONTEXT optionals | Accepted | 2026-08-03 | Amends: REQ-053, REQ-140, REQ-170 |
| ADR-0017 | AQL semantic layer: derived containment relation, overlays, opt-in enforcement | Accepted | 2026-08-22 | Amends: REQ-109 |
| ADR-0018 | Raw response bytes on the typed 2xx decode error | Accepted | 2026-08-30 | — |
| ADR-0019 | Definition metadata timestamps: a closed tolerant layout set on decode, RFC 3339 on encode | Accepted | 2026-08-30 | Amends: REQ-050 |
| ADR-0020 | Cassette recordings are HTTP Archive 1.2 | Accepted | 2026-09-07 | Resolves: STRAND-11; Amends: REQ-082 |
| ADR-0022 | Canonical JSON: `encoding/json/v2` through generated streaming marshalers; member order not promised; numbers strict on encode, tolerant on decode | Accepted | 2026-09-14 | Resolves: STRAND-04 (the `encoding/json/v2` and default-codec sub-questions); Amends: REQ-052 |
| ADR-0023 | SMART discovery: the services map, a soft version gate, and the base URL and issuer as two values | Accepted | 2026-10-04 | Amends: REQ-070, REQ-071, REQ-072, REQ-061 |
| ADR-0025 | Template-driven validation includes the RM floor in one walk, template admission first | Accepted | 2026-10-11 | Resolves: STRAND-14; Amends: REQ-102, REQ-112 |
| ADR-0028 | Generated attribute access replaces the hand-written RM readers | Accepted | 2026-10-13 | Amends: REQ-101, REQ-102, REQ-112, REQ-013 |
| ADR-0033 | A missing mandatory member is reported by the decoder and read by the validators, not recorded on the value | Accepted | 2026-10-26 | Resolves: STRAND-15 (the presence question); Amends: REQ-112, REQ-171, ADR-0002 |
| ADR-0034 | Each generation target's root is an SDK-authored BMM schema over the pinned openEHR schemas; the ADL 1.4 operational template is modelled from Template.xsd | Accepted | 2026-11-02 | Amends: REQ-041, REQ-045, REQ-056, ADR-0002 |
| ADR-0035 | The compiled template: compiled from the generated operational template model, published from `openehr/templatecompile` by type alias | Accepted | 2026-11-03 | Resolves: STRAND-17; Amends: REQ-176, REQ-111, REQ-103, REQ-107, REQ-108, REQ-169, REQ-170, REQ-013 |

<!-- /sdd:generated -->

The departures from the pinned REST contract that several of these ADRs decide are listed, each with the section that specifies it, in [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).
