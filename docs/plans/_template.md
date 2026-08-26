# Plan — &lt;short title&gt;

**Date:** YYYY-MM-DD
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-xxx, REQ-yyy (link to canonical spec sections only — no duplicate normative prose)
**Probes:** PROBE-xxx (if applicable)
**Implementation:** planned | partial | landed
**Depends on:** &lt;other plans or landed packages&gt;
**Defers:** &lt;out of scope for this plan&gt;

## Goal

One paragraph: what ships and who consumes it.

## Definition of Ready

Implementation may start when:

- **`**Covers:**`** lists every REQ-NNN (and STRAND-NN / ADR if applicable) this plan implements.
- Canonical normative prose exists for each covered REQ (topic spec section + registry row in [REQ.md](../specifications/REQ.md)).
- Any irreversible fork has an **Accepted** [ADR](../adr/).
- Phases list concrete tasks and name the verification command (`make ci`, `make spec-check`, probes).

## Definition of Done

The plan is complete when:

- Code and tests land with `// REQ-` / `// PROBE-` citations.
- [`traceability.yaml`](../specifications/traceability.yaml) and the REQ.md **Impl.** column reflect the implementation.
- **The two indexes `make spec-check` cannot see** — a row in [`roadmap.md`](../roadmap.md) for anything that landed, and, when this plan allocated a new REQ id, the band table in [REQ.md § Numbering policy](../specifications/REQ.md#numbering-policy) (headroom consumed, band marked exhausted). Neither is machine-checked, so both are easy to miss.
- Canonical spec prose / **Status:** updated in the same PR when behaviour changed.
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](archive/) (or **Status:** set to complete).

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) | |
| Indexes `spec-check` misses (`roadmap.md` row, REQ.md numbering band) | |
| Code | |
| Tests with `// REQ-` / `// PROBE-` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 1 — &lt;outcome&gt;

**Tasks:** …

**Definition of done:** …

## Mapping to specs

- [docs/specifications/&lt;canonical&gt;.md § REQ-xxx](../../docs/specifications/&lt;file&gt;.md#req-xxx) — normative contract
- [docs/specifications/REQ.md](../../docs/specifications/REQ.md) — registry row
