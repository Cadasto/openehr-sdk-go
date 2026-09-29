---
kind: guide
---

# Reviewer memory: go-reviewer

## Out of focus

Datamap V2 (REQ-058, `cadasto/datamap`, and the experiments on the `feat/datamap` branch) is postponed by the maintainer. Development has not started, and that branch does not count as an implementation, partial or otherwise. REQ-058 and its section stay as written. Do not raise findings on them, and do not carry datamap items in a ledger or its `Deferred` table, until the maintainer brings datamap back into focus.

## Declined findings

Findings declined in triage, with the reason. Do not raise one again unless the change under review makes its reason untrue; say which part changed.

| Finding | Claim | Why declined |
|---|---|---|
| PR 187 F42 | With the implicit attributes, an OPT interval node that declares none is no longer a lint leaf, so a path such as `…/value/lower/magnitude` into it warns. | REQ-109 documents this false-positive mode: a path through a non-mandatory RM attribute the OPT did not constrain may still warn, and the check is a Warning for that reason. An attribute-less node of every other class already behaves the same way. Changing the leaf rule is deferred. |
