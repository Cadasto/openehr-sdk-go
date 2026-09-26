# Plan — &lt;short title&gt;

**Date:** YYYY-MM-DD
**Status:** Draft — &lt;one clause of detail, optional&gt;
**Covers:** REQ-xxx, REQ-yyy (link to the canonical spec sections; no normative prose here)
**Probes:** PROBE-xxx (if applicable)
**Depends on:** &lt;other plans or landed packages&gt;
**Defers:** &lt;out of scope for this plan&gt;

The first word of **Status:** is one of `Draft` (not started), `Active` (under way), `Parked` (on hold, with the reason) or `Done`. The [plan index](README.md) is generated from it and from **Covers:**, so keep both on one line. A plan never moves: when it lands, set **Status:** to `Done` in the implementing PR and run `make spec-gen`.

## Goal

One paragraph: what ships and who consumes it.

## Definition of Ready

Implementation may start when:

- **Covers:** lists every REQ-NNN (and STRAND-NN or ADR, if any) this plan implements.
- Canonical normative text exists for each covered REQ, in its topic spec, with a `traceability.yaml` entry.
- Any irreversible fork has an **Accepted** [ADR](../adr/).
- The inputs and states the change must refuse, and how it fails on them, are cited from the canonical spec.
- Phases list concrete tasks and name the verification command (`make ci`, `make spec-check`, probes).

## Definition of Done

All in the implementing PR:

- Code and tests land; tests cite the `REQ-` / `PROBE-` they pin.
- The canonical spec text is current, and `traceability.yaml` lists the landed packages, tests and probes.
- **Status:** is `Done`, and `make spec-gen` has refreshed the generated indexes.
- `make ci` passes (it includes `make spec-check`).

## Phases

### Phase 1 — &lt;outcome&gt;

**Tasks:** …

**Definition of done:** …
