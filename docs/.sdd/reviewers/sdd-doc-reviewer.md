---
kind: guide
---

# Reviewer memory: sdd-doc-reviewer

## Out of focus

Datamap V2 (REQ-058, `cadasto/datamap`, and the experiments on the `feat/datamap` branch) is postponed by the maintainer. Development has not started, and that branch does not count as an implementation, partial or otherwise. REQ-058 and its section stay as written. Do not raise findings on them, and do not carry datamap items in a ledger or its `Deferred` table, until the maintainer brings datamap back into focus.

## Declined findings

Findings declined in triage, with the reason. Do not raise one again unless the change under review makes its reason untrue; say which part changed.

| Finding | Claim | Why declined |
|---|---|---|
| PR 182 F28, plan half | A full-lane change needs a plan. | Plans are optional working notes that no gate, generator or map reads (`development-process.md`; maintainer ruling on PR 182). |
| PR 182 F36 | The validation-independence item in STRAND-04 should close, because REQ-013 now enforces the imports. | REQ-013 bans direct imports only. The strand asks about the codec's dependencies, and `openehr/validation` still reaches `openehr/serialize/canxml` and `encoding/json/v2` through `openehr/rm`. |
| PR 182 F38 | ADR 0002 records eight decisions under one identifier. | The bundling predates the ADR's frontmatter: the index on `main` already titled it D1 to D8, and the tree cites the decisions as D1 to D8. Splitting an accepted record would break those citations for no change in behaviour. |
| PR 182 F44, package-specific part | The import rules for `openehr/template` and for `openehr/aql/parse` and `openehr/aql/lint` in `clinical-modeling.md` belong in REQ-013. | REQ-013 holds the rule every building block shares. A stricter rule for one package lives in that package's own section. Only REQ-113's restatement of the REQ-109 rule was a defect, and it was fixed. |
