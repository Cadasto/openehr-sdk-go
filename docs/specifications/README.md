---
kind: guide
---

# Specifications

Normative, addressable specifications for `github.com/cadasto/openehr-sdk-go`. This tree is **the source of truth** for the SDK's contract: requirements, idioms, wire format, auth flow, conformance. It is **self-contained** — implementing or reviewing the SDK does not require access to the Cadasto architecture sources.

`docs/architecture.md` carries the structural / design *narrative* (mermaid diagram, package map, "why it's shaped this way"). This `docs/specifications/` tree carries the *normative* statements every plan, PR, code change, and test is measured against.

## How to read these specs

Each file uses **RFC 2119 keywords** to mark normative statements:

| Keyword | Force |
|---|---|
| `MUST` / `SHALL` / `REQUIRED` | absolute requirement — non-conformant implementations are buggy |
| `MUST NOT` / `SHALL NOT` | absolute prohibition |
| `SHOULD` / `RECOMMENDED` | strong recommendation — exceptions need a documented reason |
| `SHOULD NOT` / `NOT RECOMMENDED` | strong discouragement |
| `MAY` / `OPTIONAL` | truly optional — no conformance impact |

Statements without these keywords are **informative** — context, rationale, examples. Do not implement informative text as a requirement; do not relax normative text as a suggestion.

## Document kinds

The repo uses several document kinds, each with a distinct role and boundary:

| Kind | Answers | Normative? | Where it lives |
|------|---------|------------|----------------|
| **Specification** (this tree) | How must the system behave / be structured? | Yes (`MUST` / `SHALL` / `SHOULD` / `MAY`) | [`docs/specifications/`](.) — topic specs + REQ.md registry + traceability.yaml + conformance.md (PROBE-NNN) + research-strands.md (STRAND-NN) |
| **ADR** | Which irreversible architectural fork did we take? | Decision record | [`docs/adr/`](../adr/) |
| **Plan** | What exact work implements a slice? | No (delivery tasks) | [`docs/plans/`](../plans/) |
| **Guide** | How do I work in this repo safely? | No | [`docs/architecture.md`](../architecture.md), [`docs/development-process.md`](../development-process.md), [`docs/ai-workflow.md`](../ai-workflow.md), [`docs/ci.md`](../ci.md) |
| **Descriptor** | Machine-readable SDD conventions for tooling | Config (not prose) | [`docs/.sdd.yaml`](../.sdd.yaml) — REQ style, paths, build targets, PROBE/STRAND toggles; read by `sdd-*` skills |
| **Roadmap** | What is still open, and which deployments are targeted? | No (status snapshot) | [`docs/roadmap.md`](../roadmap.md) |

**Boundaries between kinds:**

- **Topic specs** carry RFC 2119 prose only — no checkbox task lists, no implementation file paths, no PR-style summaries (use a plan for those).
- **`REQ.md`** is registry-only — one row per REQ-NNN, generated from `traceability.yaml` by `make spec-generate` — canonical prose lives in the topic spec linked from each row.
- **`traceability.yaml`** is an index only: ids, links and paths, with no notes or comments (`make spec-check` refuses them). Its `tests:` lists are generated from the tests' REQ citations ([development-process.md § The ladder](../development-process.md#the-ladder-full-lane)). A fact worth keeping goes in its canonical spec; history stays in git, plans and ADRs.
- **Plans** are working notes outside the traceability chain: no rule here binds them and no gate reads them ([plans/README.md](../plans/README.md)).
- **ADRs** cover one decision each — long flows or invariants stay in the topic spec; ADRs cite the STRAND-NN they resolve plus any REQ-NNN they amend.
- **Guides** describe how we work — they're informative, not normative; when a guide disagrees with a spec, the spec wins and the guide is updated.

## Source of truth

When code and a spec disagree, the spec wins, unless the maintainer's instruction says otherwise ([AGENTS.md § Source of truth](../../AGENTS.md#source-of-truth)). Which changes owe spec edits, and in what order, is [development-process.md § Two lanes](../development-process.md#two-lanes) and [§ The ladder](../development-process.md#the-ladder-full-lane).

## Status header

Every spec file starts with a `Status:` line:

| Status | Meaning |
|---|---|
| **Draft** | **normative and binding for implementation now**; wording may still change before v1 (no deprecation cycle) |
| **Stable** | frozen contract; changes go through a deprecation cycle |
| **Deprecated** | scheduled for removal; new code does not depend on it |

**`Draft` is binding, not provisional.** Pre-v1.0, every topic spec is `Draft` — that marks the *stability promise* (the text may still change), **not** the authority. A `Draft` REQ is normative **right now**; it is not "planned" or "optional." Whether it is *built* is the separate `Impl.` column in [REQ.md](REQ.md) / `implementation:` in [traceability.yaml](traceability.yaml). Promotion `Draft → Stable` happens at v1.

## Traceability

The chain that drift detection works against:

```
docs/specifications/traceability.yaml (the index: packages, probes, tests, status)
    ├─→ docs/specifications/REQ.md (registry, generated from the map)
    └─→ canonical topic spec (packaging.md, wire.md, transport.md, …)
                    └─→ code (Go package)
                            └─→ tests (*_test.go)
                                    └─→ docs/specifications/conformance.md (PROBE-NNN)
```

**Single canonical home:** normative `MUST`/`SHALL` prose lives in exactly one topic spec per REQ (see [REQ.md](REQ.md) registry `Canonical` column). REQ.md is an index only — do not duplicate requirement bodies there.

Cite identifiers when crossing the chain:

- Every test that exercises a normative requirement cites the REQ-ID and (if applicable) PROBE-ID in a comment; that citation is what lists the test under its REQ in `traceability.yaml`. Maintainer comments may cite them too; godoc on exported API does not (it is written for SDK users; see [AGENTS.md](../../AGENTS.md#spec-driven-workflow-agents)).
- Every ADR in `docs/adr/` cites the STRAND-ID it resolves (from `research-strands.md`) and any REQ-IDs it amends.
- The map lists no ADRs: an ADR names its REQs in its header, and `make spec-context` finds it from there. Plans are not part of the chain.
- When landing code or probes, update the packages and probes in [`traceability.yaml`](traceability.yaml) and run `make spec-generate`, which writes the tests lists.

`make spec-check` catches the drift a machine can see: a cited path, probe or anchor that does not exist, a landed requirement with no packages or tests, and a generated block that no longer matches its source.

## Identifier scheme

| Prefix | Meaning | Lives in |
|---|---|---|
| `REQ-NNN` | Enumerated SDK requirement (index) | `REQ.md` (registry); canonical prose in topic specs |
| `PROBE-NNN` | Conformance probe | `conformance.md` |
| `STRAND-NN` | Open research strand | `research-strands.md` |
| `ADR-NNN` | Resolved architectural decision | `../docs/adr/` |

Identifiers are stable once published — they are referenced from outside the file (commit messages, PR titles, code comments, test names). Renumbering and reuse are prohibited ([REQ.md § Numbering policy](REQ.md#numbering-policy)).

## Index

| File | Scope |
|---|---|
| [REQ.md](REQ.md) | Requirement registry (generated index) — links to canonical topic specs |
| [traceability.yaml](traceability.yaml) | Machine-readable REQ → package / probe / test map |
| [../.sdd.yaml](../.sdd.yaml) | SDD project descriptor — identifier style, document paths, `make` targets (read by `sdd-*` skills) |
| [../development-process.md](../development-process.md) | The two lanes, the delivery ladder, the superpowers boundary, the conformance probe harness ([§ Conformance probes](../development-process.md#conformance-probes)) |
| [packaging.md](packaging.md) | Module path, semantic versioning and the internal boundary (REQ-001, REQ-004, REQ-005) |
| [transport.md](transport.md) | Transport layer — OTel hooks, retry, TLS posture, error-envelope mapping, `Prefer` negotiation, the request observer, path-segment validation, typed 2xx failures, bearer challenges |
| [glossary.md](glossary.md) | openEHR, SMART, Cadasto, and SDK-internal terms |
| [scope.md](scope.md) | What is in and out of v1 scope |
| [module-layout.md](module-layout.md) | Package taxonomy, dependency direction, building-block independence and each block's import rule (REQ-013), boundary rules, versioning |
| [idiom.md](idiom.md) | Idiomatic Go surface — `context.Context`, `*http.Client` injection, errors and no-panic, concurrency (REQ-020, REQ-021, REQ-025, REQ-026), and the informative API conventions (options, surface shape, generics without reflection, naming) |
| [rm-modeling.md](rm-modeling.md) | openEHR Reference Model in Go — the shape of the generated types, the openEHR terminology vocabulary (REQ-034), the type registry and LOCATABLE identity surface (REQ-040) |
| [rm-functions.md](rm-functions.md) | RM behavioural functions — identifier parsing/derivation, locatable path read access, version-derived helpers, temporal DV_* helpers (REQ-120–123) |
| [bmm-conformance.md](bmm-conformance.md) | Pinned BMM sources and generator (REQ-041), BMM loader (REQ-045), primitive type mapping (REQ-046), RM meta-model introspection (REQ-048), P_BMM → Go mapping rules |
| [auth.md](auth.md) | `auth.TokenSource` contract and the SMART-on-openEHR provider flow |
| [wire.md](wire.md) | openEHR REST 1.1.0 pin, AQL wire, canonical JSON / FLAT / STRUCTURED |
| [service-discovery.md](service-discovery.md) | Service catalog resolution and refresh |
| [conformance.md](conformance.md) | openEHR wire-conformance probe catalog |
| [clinical-modeling.md](clinical-modeling.md) | Clinical-modeling artefacts — OPT parse and paths, primitive constraints, validation, the instance generator and composition builder, the Web Template export, AQL parse, lint, containment and builders |
| [use-cases.md](use-cases.md) | Primary use cases, building-block use cases, delivery sequence |
| [research-strands.md](research-strands.md) | Open strands awaiting resolution (each becomes an ADR) |
| [retired-requirements.md](retired-requirements.md) | Lookup of retired `REQ-NNN`: where each rule went (binds nothing, so the gate's keyword rules skip it) |

## Editing rules

- New normative statements get a new `REQ-NNN`/`PROBE-NNN` — take the next free number ([REQ.md § Numbering policy](REQ.md#numbering-policy)); do not silently re-letter existing ones.
- A spec section states the current contract. When and how it got there belongs in git, the plan or an ADR, not in the spec.
- A status transition (`Draft` → `Stable`) is recorded where that status lives: a requirement's in its [`traceability.yaml`](traceability.yaml) `status:`, a spec file's in its [Status header](#status-header). The PR that makes it names it in its body, and the CHANGELOG follows the rule in [AGENTS.md § Code style and conventions](../../AGENTS.md#code-style-and-conventions).
- Removing a normative statement from a `Stable` spec goes through a documented cycle: mark it `Status: Deprecated` first, then remove it in the next major version. A `Draft` spec has no such cycle ([§ Status header](#status-header)): a retired requirement moves its entry to [retired-requirements.md](retired-requirements.md), which points to where its rule went, and its identifier is never reused.
- **Where a keyword goes.** An RFC 2119 keyword binds only behaviour a caller can observe and would break on: the public API, wire and file formats, the error contract, the refusals a caller sees, and security invariants. A default or a recommended path is a `SHOULD`, with its reason in the same sentence. How the code does it — algorithms, visit and precedence orders, internal structure, dependency choices, examples — is prose with no keyword, so the code can change it without an amendment. Specs carry no paths into the code, no test names, no PR numbers and no phase labels. Probe entries and Acceptance lists carry no keywords: they state what a test checks. A lower-case-modal warning from the gate (`make sdd-check` shows it) is fixed by rephrasing ("is", "does", "never", "only"), not by capitalising the modal.
- The specifications source-of-truth is **this tree**. Cadasto architecture sources may inform it, but a divergence between this tree and any external source is resolved by editing this tree, not by handwaving "see external".
