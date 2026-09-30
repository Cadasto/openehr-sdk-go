---
kind: guide
---

# Implementation plans

A plan is a working note for delivery work that spans several PRs: the phases, tasks and verification commands for one or more REQs. Plans are committed so the work can be picked up again, but they sit outside the SDD traceability chain. No generator or map reads this folder, but the drift gate checks a plan like any other document: its frontmatter declares its document kind (`kind: plan` for a plan), its links resolve, and it carries no RFC-2119 keyword and no copy of a specification's normative sentence. A broken link or a copied sentence fails `make spec-check`. A plan has no required header, status line or index entry.

Start a new plan from [`_template.md`](_template.md), named `YYYY-MM-DD-<slug>.md`. Cite the canonical spec sections the plan implements and never restate their normative text: that lives in [../specifications/](../specifications/).

Per-requirement status is in the [requirements registry](../specifications/REQ.md), and what is still open is in [`../roadmap.md`](../roadmap.md). `make spec-context REQ=NNN` lists any plan that names the REQ.
