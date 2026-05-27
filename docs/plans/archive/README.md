# Archived implementation plans

Plans moved here when **implementation is landed** or the document is **superseded** by a narrower follow-up plan. Normative contracts remain in [`../../specifications/`](../../specifications/); this folder is historical delivery detail.

**Active plans** live in the parent directory: [`../README.md`](../README.md).

| Archived plan | Reason |
|---|---|
| [2026-05-15-bmm-codegen.md](2026-05-15-bmm-codegen.md) | REQ-041..047 landed |
| [2026-05-15-canonical-json-serialization.md](2026-05-15-canonical-json-serialization.md) | REQ-052 landed |
| [2026-05-15-canonical-xml-serialization.md](2026-05-15-canonical-xml-serialization.md) | REQ-056 landed |
| [2026-05-21-template-parser.md](archive/2026-05-21-template-parser.md) | REQ-100 initial delivery landed |
| [2026-05-21-validation.md](archive/2026-05-21-validation.md) | Umbrella superseded for composition; demographic/AQL scope still **planned** (track in [Phase 2 umbrella](../2026-05-21-phase-2-clinical-building-blocks.md)) |
| [2026-05-24-composition-validation-template-driven.md](2026-05-24-composition-validation-template-driven.md) | REQ-102 `ValidateComposition` landed |
| [2026-05-21-composition-builder.md](2026-05-21-composition-builder.md) | REQ-101 `composition.NewSkeleton` + `Builder.Set/Build` landed (PRs #19 + #20) |
| [2026-05-24-template-instance-example-generator.md](2026-05-24-template-instance-example-generator.md) | REQ-107 `instance.Generate` + `rmwrite` + PROBE-027 landed (PRs #18 + #20) |
| [2026-05-26-c-primitive-object-wire-parser.md](2026-05-26-c-primitive-object-wire-parser.md) | C_PRIMITIVE_OBJECT inner-`<item>` extraction (REQ-100) + `newHierObjectID() *rm.HierObjectID` + `Options.UIDSource` (REQ-107) + PROBE-023 widened to full unmarshal round-trip (REQ-101) — landed via the wire-parser PR |
| [2026-05-25-versioning-strategy.md](2026-05-25-versioning-strategy.md) | Policy docs + `v0.1.0` first tag + tag-driven [`release.yml`](../../../.github/workflows/release.yml) workflow landed (REQ-001, REQ-004). Phase 1 runtime `version` package declined; Phase 3 `v1.0.0` ceremony descoped to a future plan |
| [2026-05-26-contribution-submission-shape.md](2026-05-26-contribution-submission-shape.md) | `contribution.Submission` (ITS-REST `Contribution_create`: inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`) replaces `*rm.Contribution` in `contribution.Commit` — SDK-GAP-10 / PROBE-072 landed |
| [2026-05-26-rm-polymorphic-decode-coverage.md](2026-05-26-rm-polymorphic-decode-coverage.md) | SDK-GAP-11 / PROBE-038 landed — narrow `<Parent>Like` interfaces (`DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`) + generic-over-abstract-bound dispatch (`DVInterval[T]`) close both substitution gaps; lossless decode → re-marshal across `testkit/cassettes/rm/polymorphic/` |
