---
kind: guide
---

# Reviewer memory: sdd-spec-conformance-reviewer

Findings declined in triage, with the reason. Do not raise one again unless the change under review makes its reason untrue; say which part changed.

| Finding | Claim | Why declined |
|---|---|---|
| PR 182 F3 | REQ-058's MUSTs are not implemented: `cadasto/datamap` holds only `doc.go`. | The registry says `planned`. The section is the spec written ahead of the codec, and conformance is checked when the codec lands. |
