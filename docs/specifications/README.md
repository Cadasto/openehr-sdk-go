# Specifications

Normative, addressable specifications for `github.com/cadasto/openehr-sdk-go`. This tree is **the source of truth** for the SDK's contract: requirements, idioms, wire format, auth flow, conformance. It is **self-contained** — implementing or reviewing the SDK does not require access to the Cadasto architecture sources.

`docs/architecture.md` carries the structural / design *narrative* (mermaid diagram, package map, "why it's shaped this way"). This `docs/specifications/` tree carries the *normative* statements every plan, PR, code change, and test is measured against.

## How to read these specs

Each file uses **RFC 2119 keywords** to mark normative statements:

| Keyword | Force |
|---|---|
| **MUST** / **SHALL** / **REQUIRED** | absolute requirement — non-conformant implementations are buggy |
| **MUST NOT** / **SHALL NOT** | absolute prohibition |
| **SHOULD** / **RECOMMENDED** | strong recommendation — exceptions need a documented reason |
| **SHOULD NOT** / **NOT RECOMMENDED** | strong discouragement |
| **MAY** / **OPTIONAL** | truly optional — no conformance impact |

Statements without these keywords are **informative** — context, rationale, examples. Do not implement informative text as a requirement; do not relax normative text as a suggestion.

## Document kinds

The repo uses several document kinds, each with a distinct role and boundary:

| Kind | Answers | Normative? | Where it lives |
|------|---------|------------|----------------|
| **Specification** (this tree) | How must the system behave / be structured? | Yes (MUST / SHALL / SHOULD / MAY) | [`docs/specifications/`](.) — topic specs + REQ.md registry + traceability.yaml + conformance.md (PROBE-NNN) + research-strands.md (STRAND-NN) |
| **ADR** | Which irreversible architectural fork did we take? | Decision record | [`docs/adr/`](../adr/) |
| **Plan** | What exact work implements a slice? | No (delivery tasks) | [`docs/plans/`](../plans/) |
| **Guide** | How do I work in this repo safely? | No | [`docs/architecture.md`](../architecture.md), [`docs/ai-workflow.md`](../ai-workflow.md), [`docs/ci.md`](../ci.md) |
| **Roadmap** | What has landed and what hasn't? | No (status snapshot) | [`docs/roadmap.md`](../roadmap.md) |

**Boundaries between kinds:**

- **Topic specs** carry RFC 2119 prose only — no checkbox task lists, no implementation file paths, no PR-style summaries (use a plan for those).
- **`REQ.md`** is registry-only — one row per REQ-NNN — canonical prose lives in the topic spec linked from each row.
- **Plans** MUST cite the REQ-NNN / STRAND-NN identifiers they implement in the header `**Covers:**` line.
- **ADRs** cover one decision each — long flows or invariants stay in the topic spec; ADRs cite the STRAND-NN they resolve plus any REQ-NNN they amend.
- **Guides** describe how we work — they're informative, not normative; when a guide disagrees with a spec, the spec wins and the guide is updated.

## Source of truth

| Mode | When | Order |
|------|------|-------|
| **Spec-first** | New capability, new wire surface, new identifier | REQ row → canonical topic spec (Draft) → ADR if irreversible fork → Plan → Code → spec status update → REQ `Impl.` column |
| **Implementation-aligned** | Hardening, fix-up, perf, behaviour clarification on shipped code | Code change → update topic spec section + `traceability.yaml` in the same PR |

For implementation-aligned PRs, **code wins until the spec is updated in the same PR**.

## Status header

Every spec file starts with a `Status:` line:

| Status | Meaning |
|---|---|
| **Draft** | actively in motion; can change without notice |
| **Stable** | safe to implement against; changes go through a deprecation cycle |
| **Deprecated** | scheduled for removal; new code MUST NOT depend on it |

At v0 scaffolding stage, all specs are **Draft**.

## Traceability

The chain that drift detection works against:

```
docs/specifications/REQ.md (registry index — one row per REQ-NNN)
    └─→ canonical topic spec (packaging.md, wire.md, transport.md, …)
            └─→ docs/specifications/traceability.yaml (packages, probes, tests, plans)
                    └─→ docs/plans/YYYY-MM-DD-*.md
                            └─→ code (Go package)
                                    └─→ tests (*_test.go)
                                            └─→ docs/specifications/conformance.md (PROBE-NNN)
```

**Single canonical home:** normative MUST/SHALL prose lives in exactly one topic spec per REQ (see [REQ.md](REQ.md) registry `Canonical` column). REQ.md is an index only — do not duplicate requirement bodies there.

Cite identifiers when crossing the chain:

- Every plan in `docs/plans/` MUST list the REQ-IDs it implements.
- Every public package's `doc.go` SHOULD reference the REQ-IDs and/or spec sections it covers.
- Every test that exercises a normative requirement SHOULD cite the REQ-ID and (if applicable) PROBE-ID in a comment.
- Every ADR in `docs/adr/` MUST cite the STRAND-ID it resolves (from `research-strands.md`) and any REQ-IDs it amends.
- When landing code or probes, update [`traceability.yaml`](traceability.yaml) and the registry `Impl.` column in [REQ.md](REQ.md).

A requirement with no plan, a plan with no code, code with no test, or a conformance probe with no test — each is a mechanically detectable drift signal. Run `make spec-check` to catch registry rot.

## Identifier scheme

| Prefix | Meaning | Lives in |
|---|---|---|
| `REQ-NNN` | Enumerated SDK requirement (index) | `REQ.md` (registry); canonical prose in topic specs |
| `PROBE-NNN` | Conformance probe | `conformance.md` |
| `STRAND-NN` | Open research strand | `research-strands.md` |
| `ADR-NNN` | Resolved architectural decision | `../docs/adr/` |

Identifiers MUST be stable once published — they are referenced from outside the file (commit messages, PR titles, code comments, test names). Renumbering is a major doc-version bump.

## Index

| File | Scope |
|---|---|
| [REQ.md](REQ.md) | Requirement registry (index) — links to canonical topic specs |
| [traceability.yaml](traceability.yaml) | Machine-readable REQ → package / probe / test / plan map |
| [packaging.md](packaging.md) | Module identity REQ-001–005 |
| [transport.md](transport.md) | Transport layer REQ-090–094 |
| [glossary.md](glossary.md) | openEHR, SMART, Cadasto, and SDK-internal terms |
| [scope.md](scope.md) | What is in and out of v1 scope |
| [module-layout.md](module-layout.md) | Package taxonomy, dependency direction, boundary rules, versioning |
| [idiom.md](idiom.md) | Idiomatic Go surface — `context.Context`, `*http.Client` injection, functional options, errors, generics, concurrency |
| [rm-modeling.md](rm-modeling.md) | openEHR Reference Model rules in Go — structs, embedded base, interfaces, type registry |
| [bmm-conformance.md](bmm-conformance.md) | Pinned BMM sources, generator, P_BMM → Go mapping rules, primitive type mapping |
| [auth.md](auth.md) | `auth.TokenSource` contract and the SMART-on-openEHR provider flow |
| [wire.md](wire.md) | openEHR REST 1.1.0-development pin, AQL wire, canonical JSON / FLAT / STRUCTURED |
| [service-discovery.md](service-discovery.md) | Service catalog resolution and refresh |
| [conformance.md](conformance.md) | openEHR wire-conformance probe catalog |
| [clinical-modeling.md](clinical-modeling.md) | Clinical-modeling artefacts — OPT parse and paths (REQ-100); composition, validation, AQL paths follow in later REQs |
| [use-cases.md](use-cases.md) | Primary use cases, building-block use cases, delivery sequence |
| [research-strands.md](research-strands.md) | Open strands awaiting resolution (each becomes an ADR) |

## Editing rules

- New normative statements get a new `REQ-NNN`/`PROBE-NNN` — do not silently re-letter existing ones.
- A spec file MUST link out to the code package(s) it constrains once they exist.
- Status transitions (`Draft` → `Stable`) MUST be accompanied by a CHANGELOG entry under `## [Unreleased]`.
- Removing a normative statement (deprecation) MUST go through a documented cycle: mark `Status: Deprecated` first, then remove in the next major version.
- The specifications source-of-truth is **this tree**. Cadasto architecture sources may inform it, but a divergence between this tree and any external source is resolved by editing this tree, not by handwaving "see external".
