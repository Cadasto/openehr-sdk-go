# Implementation plans

A plan turns one or more normative REQs into sequenced delivery work. It cites the canonical spec sections it implements and never restates their normative text. Start a new plan from [`_template.md`](_template.md), named `YYYY-MM-DD-<slug>.md`.

**Lifecycle.** A plan stays where it is written. Its **Status:** line says where it stands (`Draft`, `Active`, `Parked` or `Done`), and the implementing PR sets it to `Done`. Nothing is moved and no index is edited by hand: the tables below are generated from each plan's **Status:** and **Covers:** lines by `make spec-gen`, and `make spec-check` fails when they are stale.

Plans that landed before this rule are in [`archive/`](archive/README.md); that folder is frozen. Per-requirement status is in the [requirements registry](../specifications/REQ.md); what is still open is in [`../roadmap.md`](../roadmap.md).

## Index

<!-- BEGIN GENERATED: plans (make spec-gen — edit the plans' Status/Covers lines, not this table) -->

### Active

| Plan | Title | Covers |
|---|---|---|
| [2026-08-18-probe-runnability](2026-08-18-probe-runnability.md) | Probe runnability: the sandbox transport and the three-mode runner | REQ-082, REQ-080 |
| [2026-09-24-rm-floor-archetype-roots](2026-09-24-rm-floor-archetype-roots.md) | RM floor: archetype roots and ARCHETYPED | REQ-112 |

### Draft

| Plan | Title | Covers |
|---|---|---|
| [2026-07-16-flat-author-linter](2026-07-16-flat-author-linter.md) | FLAT author linter (pre-submit path validation) | REQ-115 |
| [2026-07-16-opt-author-validator](2026-07-16-opt-author-validator.md) | OPT author validator + CLI | REQ-114 |

### Parked

| Plan | Title | Covers |
|---|---|---|
| [2026-06-23-simplified-formats](2026-06-23-simplified-formats.md) | Simplified formats (WebTemplate export + FLAT/STRUCTURED) — umbrella | REQ-053, REQ-106 |
| [2026-09-01-rm-function-deferred-stubs](2026-09-01-rm-function-deferred-stubs.md) | RM function deferred stubs (arithmetic + refs/inverse-navigation) | REQ-124, REQ-125 |
<!-- END GENERATED: plans -->
