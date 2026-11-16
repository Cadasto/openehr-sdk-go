---
kind: guide
---

# Implementation plans

A plan is a working note for delivery work that spans several PRs: the phases, tasks and verification commands for one or more REQs.
It is committed so the work can be picked up again, and deleted once its work is served.
It sits outside the SDD traceability chain: no gate, generator, map or reviewer reads a plan, so nothing fails when one is out of date.
Its frontmatter says `kind: plan`, which is what keeps the drift gate out of it, and a specification or an ADR never links to it.
A plan has no required header, status line or index entry.

Start a new plan from [`_template.md`](_template.md), named `YYYY-MM-DD-<slug>.md`. Cite the canonical spec sections the plan implements and never restate their normative text: that lives in [../specifications/](../specifications/).

Per-requirement status is in the [requirements registry](../specifications/REQ.md), and what is still open is in [`../roadmap.md`](../roadmap.md). `make spec-context REQ=NNN` lists any plan that names the REQ.
