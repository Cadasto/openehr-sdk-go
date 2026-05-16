# Specifications

Normative, addressable specifications for `github.com/cadasto/openehr-sdk-go`. This tree is **the source of truth** for the SDK's contract: requirements, idioms, wire format, auth flow, conformance. It is **self-contained** — implementing or reviewing the SDK does not require access to the Cadasto architecture sources.

`docs/architecture.md` carries the structural / design *narrative* (mermaid diagram, package map, "why it's shaped this way"). This `specs/` tree carries the *normative* statements every plan, PR, code change, and test is measured against.

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
specs/REQ.md (REQ-NNN)
    └─→ docs/plans/YYYY-MM-DD-*.md
            └─→ code (Go package)
                    └─→ tests (*_test.go)
                            └─→ specs/conformance.md (PROBE-NNN)
```

Cite identifiers when crossing the chain:

- Every plan in `docs/plans/` MUST list the REQ-IDs it implements.
- Every public package's `doc.go` SHOULD reference the REQ-IDs and/or spec sections it covers.
- Every test that exercises a normative requirement SHOULD cite the REQ-ID and (if applicable) PROBE-ID in a comment.
- Every ADR in `docs/adr/` MUST cite the STRAND-ID it resolves (from `research-strands.md`) and any REQ-IDs it amends.

A requirement with no plan, a plan with no code, code with no test, or a conformance probe with no test — each is a mechanically detectable drift signal.

## Identifier scheme

| Prefix | Meaning | Lives in |
|---|---|---|
| `REQ-NNN` | Enumerated SDK requirement | `REQ.md` |
| `PROBE-NNN` | Conformance probe | `conformance.md` |
| `STRAND-NN` | Open research strand | `research-strands.md` |
| `ADR-NNN` | Resolved architectural decision | `../docs/adr/` |

Identifiers MUST be stable once published — they are referenced from outside the file (commit messages, PR titles, code comments, test names). Renumbering is a major doc-version bump.

## Index

| File | Scope |
|---|---|
| [REQ.md](REQ.md) | Enumerated requirements — the SDK's normative checklist |
| [glossary.md](glossary.md) | openEHR, SMART, Cadasto, and SDK-internal terms |
| [scope.md](scope.md) | What is in and out of v1 scope |
| [module-layout.md](module-layout.md) | Package taxonomy, dependency direction, boundary rules, versioning |
| [idiom.md](idiom.md) | Idiomatic Go surface — `context.Context`, `*http.Client` injection, functional options, errors, generics, concurrency |
| [rm-modeling.md](rm-modeling.md) | openEHR Reference Model rules in Go — structs, embedded base, interfaces, type registry |
| [bmm-conformance.md](bmm-conformance.md) | Pinned BMM sources, generator, P_BMM → Go mapping rules, primitive type mapping |
| [auth.md](auth.md) | `auth.TokenSource` contract and the SMART-on-openEHR provider flow |
| [wire.md](wire.md) | openEHR REST 1.1.0-development pin, AQL wire, canonical JSON / FLAT / STRUCTURED |
| [service-discovery.md](service-discovery.md) | Service catalog resolution and refresh |
| [conformance.md](conformance.md) | Probe catalog and cross-SDK parity contract with the PHP SDK |
| [use-cases.md](use-cases.md) | Primary use cases, building-block use cases, POC extraction scope |
| [research-strands.md](research-strands.md) | Open strands awaiting resolution (each becomes an ADR) |

## Editing rules

- New normative statements get a new `REQ-NNN`/`PROBE-NNN` — do not silently re-letter existing ones.
- A spec file MUST link out to the code package(s) it constrains once they exist.
- Status transitions (`Draft` → `Stable`) MUST be accompanied by a CHANGELOG entry under `## [Unreleased]`.
- Removing a normative statement (deprecation) MUST go through a documented cycle: mark `Status: Deprecated` first, then remove in the next major version.
- The specifications source-of-truth is **this tree**. Cadasto architecture sources may inform it, but a divergence between this tree and any external source is resolved by editing this tree, not by handwaving "see external".
