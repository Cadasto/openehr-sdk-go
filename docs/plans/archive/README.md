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
| [2026-05-27-rm-like-interface-ergonomics.md](2026-05-27-rm-like-interface-ergonomics.md) | SDK-GAP-11 follow-up — Get-prefixed accessor methods on all five `<Parent>Like` interfaces (`GetValue`, `GetDefiningCode`, `GetSystemID`, `GetName`, `GetID`, …); interface declarations moved out of the generator into hand-written `openehr/rm/like_interfaces.go` |
| [2026-06-11-security-hardening-and-simplification.md](2026-06-11-security-hardening-and-simplification.md) | Repo-wide security + simplification review remediation (19 tasks: SMART trust/token path, PHI-free errors, bounded + depth-limited untrusted input, dedup/perf, least-privilege CI) — landed via PR #31. No new REQs adopted; REQ-candidates flagged inline |
| [2026-06-11-contribution-update-audit-dv-coded-text.md](2026-06-11-contribution-update-audit-dv-coded-text.md) | Contribution write-audit DTO (SPECITS-95 / ITS-REST PR 131) — `contribution.UpdateAudit` + write-version wrappers drop server-assigned `time_committed`; `change_type` stays `DV_CODED_TEXT`; `_type` defaults to AUDIT_DETAILS with a settable UPDATE_AUDIT fallback. PROBE-072 extended; REQ-050/REQ-095. Landed |
| [2026-05-25-req094-prefer-followups.md](2026-05-25-req094-prefer-followups.md) | REQ-094 write-path `Prefer` follow-ups — `return=identifier` populates the `VersionMetadata` identifier slot (`ehr.ResolveIdentifierBody`); `return=representation` + empty body returns `transport.ErrInvalidShape` (no silent downgrade); applied across composition / directory / ehr_status. PROBE-065 round-trip still deferred. Landed |
| [2026-05-22-template-req100-followups.md](2026-05-22-template-req100-followups.md) | REQ-100 hardening Phases 1–6 + 4-bis — parser hardening, path ergonomics, compiled template foundation ([ADR 0005](../../adr/0005-compiled-template-foundation.md)), walker pattern, REQ-103 primitive constraints. Phases 7–8 deferred to [2026-06-12-template-req104-req105-deferred.md](../2026-06-12-template-req104-req105-deferred.md). Landed |
