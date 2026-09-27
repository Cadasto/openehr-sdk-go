---
kind: guide
---

# Reviewer memory: sdd-traceability-auditor

Findings declined in triage, with the reason. Do not raise one again unless the change under review makes its reason untrue; say which part changed.

| Finding | Claim | Why declined |
|---|---|---|
| PR 182 F29 | `make spec-gen` should fail when a test cites an unregistered REQ. | An unknown cited id is the `tree-to-map` family's job in the vendored `sdd-check`, which `make spec-check` runs. A second check in the generator would give the rule two homes. |
| PR 182 F31, second half | A retired map row needs removal-completion or removal-target metadata. | No rule asks for it. `retired` owes no evidence, and REQ-081 withdrew a goal, so no code is left to remove. |
| PR 182 F33 | Reserving REQ ids in `REQ.md` before acceptance criteria exist breaks lazy allocation. | Reservations are this repository's numbering policy (`REQ.md` § Numbering policy, set in PR 181). Lazy allocation is the plugin default the repository overrides. |
| PR 182 F43 | `testkit/conformance/webtemplate/SKIPPED.md` cites REQ-115, which has no map row. | REQ-115 is a reservation listed in `REQ.md` § Numbering policy. The `tree-to-map` warning clears when the requirement is registered. |
