# ADR 0012 — Retire SDK-GAP as a durable identifier; REQ/PROBE is the feature register

- **Status:** Accepted, 2026-07-02.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — direct process/convention decision (brainstormed 2026-07-02); no prior open research strand.
- **Introduces:** — (no new `REQ-NNN`). **Promotes content into:** REQ-055, REQ-057, REQ-107, REQ-083. **Applies:** the SDD conventions in [`.sdd.yaml`](../.sdd.yaml) and [`development-process.md`](../development-process.md).

## Context

`SDK-GAP-NN` grew as an ad-hoc label for implementation increments — gap-closing work between the normative contract and the shipped code. It is **not** a normative identifier: the repo's product/feature register is `REQ-NNN` (registry in [`REQ.md`](../specifications/REQ.md), canonical RFC-2119 prose in one topic spec) plus `PROBE-NNN` conformance probes ([`conformance.md`](../specifications/conformance.md)); open research questions are `STRAND-NN`.

Over twelve increments the GAP label stopped being a throwaway plan handle and leaked into the **durable, living surface**: prose anchors and plan links in [`traceability.yaml`](../specifications/traceability.yaml); notes in [`roadmap.md`](../roadmap.md) and `conformance.md`; inline prose in `wire.md` / `transport.md` / `clinical-modeling.md`; a `research-strands.md` entry; Go **test filenames** (`gap12_test.go`, `write_gap12_test.go`, …) and in-file identifiers; and ~229 reference lines across 87 files.

This conflates two distinct jobs in one token:

1. **A delivery lens** — "here is the distance between spec and code, and the increment that closed it." This is retrospective; once the gap is closed it describes a state of the world that no longer exists.
2. **A durable traceability anchor** — an identifier baked into tests, the trace map, and spec prose, implying it is part of the permanent contract.

Job 1 is transient by nature; job 2 duplicates what `REQ`/`PROBE` already do. The result is a *growing list* that carries no product meaning after close, a second class of identifier contributors must learn, and traceability threaded through a label that was never meant to be normative. An inventory (2026-07-02) confirmed that **nine of twelve gaps are already fully carried by an existing `REQ` + its canonical spec** — the GAP token there is pure redundant scaffolding — and that only **three** gaps carry a durable fact not yet promoted into the normative layer.

Using **GitHub issues** for the work-tracking half was considered and **rejected**: it splits the traceability chain out of the versioned tree, defeats `make spec-check`, and is unavailable offline / air-gapped — all of which contradict this repo's repo-as-source-of-truth discipline (`.sdd.yaml`, the REQ registry, `make spec-check`). See the brainstorming dialogue of 2026-07-02.

## Decision

**`SDK-GAP-NN` is retired as a durable identifier. `REQ-NNN` / `PROBE-NNN` is the sole feature register.**

1. **Going forward.** A newly discovered gap is worked *under a REQ* — either extending an existing `REQ` or creating a new one via [`sdd-specify`](../development-process.md), with `PROBE` for wire-level conformance. A GAP-style label may appear **only** as an ephemeral in-flight plan filename; it is **never** threaded into `traceability.yaml`, test names/identifiers, `doc.go`, or any normative prose. The going-forward rule is codified in `development-process.md` and `AGENTS.md` (part of PR1).

2. **Full purge of the mutable tree (no half-measures).** GAP tokens are removed from **every mutable document, including `CHANGELOG.md`**, and from all code. In code comments a stripped `(SDK-GAP-NN)` is **replaced by its governing `(REQ-NNN/PROBE-NNN)`** token, never merely deleted — traceability is preserved, not lost. The six `sdk-gap-NN` archived plan files are `git mv`-renamed to their descriptive names and all ~15 inbound links updated; the five gap-tokened test files and their in-file identifiers are renamed. **Git history is never rewritten** — commit messages remain the one place the tokens survive, decoded by the crosswalk below.

3. **Content preservation is the governing constraint — nothing a GAP delivered is lost.** The three content-carrying gaps are **promoted into the normative layer *before* their tokens are stripped**:
   - **GAP-16 → REQ-055 / REQ-057** (`wire.md`): the verb-aware EHR scoping (`POST /query/aql` scopes via the `openehr-ehr-id` **request header**; `GET` uses the `ehr_id` query parameter) and the `PutStoredQuery` `Location`-header `{name, version}` recovery become normative prose + trace notes.
   - **GAP-14 → REQ-107** (`clinical-modeling.md`): the seeded synthetic value-fill seam (`instance.ValueFill` / `RandomFill` / `ValueSource`, surfaced as `composition.WithValueFill` / `WithValueSource`) becomes a normative paragraph.
   - **GAP-07 → REQ-083** (`conformance.md`): the Cadasto health-probe contract (default `/health/live` + `/health/ready` paths, `WithLivePath` / `WithReadyPath` overrides, origin-derived URL with the REST API prefix stripped, the public/no-`Authorization` invariant, and the `401/403/404/5xx` → sentinel mapping) becomes a normative subsection under REQ-083.

4. **Dangling references get an honest home, not a silent drop.**
   - The **deferred probes PROBE-077 / 078 / 079** (cited by GAP-15/16 but not implemented) remain as explicitly `Deferred`/`Planned` entries in the `conformance.md` catalog + `traceability.yaml`. Building them is separate conformance work, out of scope here.
   - The **GAP-14 `medium` / `detail_level`** follow-up (a genuinely *undelivered* third synthesis detail level) is recorded as a `Planned` roadmap row plus a "Deferred" note in the REQ-107 spec section — not a STRAND (it is a concrete feature, not a research question), and not yet a REQ (nothing has landed to make normative).

5. **This ADR carries the permanent crosswalk** (below) — the decoder for any `SDK-GAP-NN` encountered in git history or an old external reference. This satisfies the constraint that everything ever tracked as a GAP now lives in `adr`/`req`/`specs`.

6. **Sequencing — two PRs, both gated on `make spec-check` + green tests.**
   - **PR1** = this ADR + the three normative promotions (decision 3) + the process-doc updates (decision 1). Small, judgment-heavy, reviewable; on merge this ADR flips to **Accepted**.
   - **PR2** = the mechanical purge and all renames (decisions 2, 4) + the anchor/link fixes. Large, low-risk, greppable — verifiable by `! grep -rn 'SDK-GAP\|sdk-gap\|gap[0-9]' <mutable tree>` returning empty.

## Crosswalk — `SDK-GAP-NN` → `REQ` / `PROBE` (permanent decoder)

| GAP | Capability delivered | → REQ | → PROBE | Disposition |
|---|---|---|---|---|
| **07** | Cadasto `admin` Live/Ready health probes | REQ-083 | — | **Promote** into REQ-083, then strip |
| **09** | `Prefer: return=representation` decodes the bare resource | REQ-094 (+052) | 061, 071 | Strip (already normative) |
| **10** | `contribution.Commit` takes `Contribution_create`, not the persisted shape | REQ-050, 095 | 072 | Strip (already normative) |
| **11** | `<Parent>Like` interfaces + generic-bound polymorphic decode | REQ-052, 040 | 038 | Strip (already normative) |
| **12** | Real-world OPT synthesis/validation (name-fill, generic `DV_INTERVAL<T>`, node_id dedup) | REQ-102, 107, 110 | 027 | Strip + rename tests |
| **13** | `_type` emission on value-in-interface slots; interval re-validation from bounds | REQ-052, 040, 102, 107 | 038 | Strip + rename test |
| **14** | Seeded synthetic value fill (`ValueFill`/`RandomFill`/`ValueSource`) | REQ-107 | — | **Promote** seam into REQ-107; `medium`/`detail_level` → Planned |
| **15** | `ValidateRM` + typed sugars walk any RM root via `rminfo` | REQ-112 | 077 *(deferred)* | Strip (already normative) |
| **16** | Verb-aware `openehr-ehr-id` POST scoping; `PutStoredQuery` `Location` recovery | REQ-055, 057 | 078/079 *(deferred)* | **Promote** wire facts into wire.md, then strip |
| **17** | Tier-2 `parse.Query` AST + `Emit` round-trip; `ErrIncompleteAST` | REQ-113 | 080 | Strip (already normative) |
| **18** | `ValidateRMEHRStatusBytes` — value-typed mandatory `subject` via JSON-key presence | REQ-112 | 081 | Strip (already normative) |
| **19** | `ClassExpr.PredicateComparison` + `Comparison.ParsedPath`; `IdentifiedPath` relocation | REQ-113 | 082 | Strip + rename const |

`SDK-GAP-08` was never assigned in this repository. GAP numbering does not resume; new work takes REQ identifiers.

## Consequences

- The living, normative, and CHANGELOG surface becomes GAP-free; a single grep proves it. Contributors and agents learn one identifier scheme (`REQ`/`PROBE`/`STRAND`), and `make spec-context REQ=NNN` remains the one-shot context tool.
- Git commit history retains the tokens; this ADR's crosswalk is the durable decoder, so old references stay resolvable without keeping the identifier alive.
- No content is lost: the three durable facts move up into `REQ-055/057/107/083`; the undelivered `medium` level and the deferred probes are tracked honestly rather than dropped.
- The `PROBE` register is slightly aspirational for GAP-15/16 until PROBE-077/078/079 are built; the interim verification (unit cassette matrices) is documented in `conformance.md`.
- Renaming the archived plans edits the doc-form of the delivery record (the twin of git history) — accepted deliberately in exchange for a fully token-free tree; the dated prefixes and descriptive tails are preserved, and inbound links are updated in the same PR so nothing breaks.
- **Anchor churn is a hard edge:** three `conformance.md` PROBE headings embed a GAP token, and their generated anchors are linked from `wire.md`. The heading rename and the inbound-link fix land in the same edit (PR2) so no cross-reference dangles.

## Alternatives considered

- **Move work-tracking to GitHub issues.** Best-in-class lifecycle/backlog management, natural for a public SDK. Rejected: issues live outside the versioned tree, are invisible to `make spec-check`, break offline/air-gapped access, and split the traceability chain across two systems — squarely against this repo's repo-as-source-of-truth design.
- **Keep GAP as a living identifier (status quo).** Zero migration cost. Rejected: perpetuates the growing, product-meaningless list and the dual-identifier confusion the inventory exposed.
- **Strip the living surface only, keep archived-plan filenames and released CHANGELOG lines (a "decoder-ring" half-purge).** Lower churn, no link updates. Rejected in favour of a full purge so the token survives *only* in immutable git history, with this ADR as the single decoder — cleaner end state and no lingering token in any mutable file.
- **Build PROBE-077/078/079 as part of this change.** Would make the `PROBE` register non-aspirational immediately. Rejected as scope creep: probe implementation is conformance work independent of the identifier retirement.
