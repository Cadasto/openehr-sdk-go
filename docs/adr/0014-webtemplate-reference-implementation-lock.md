# ADR 0014 — WebTemplate reference implementation and id-generation lock

- **Status:** Accepted, 2026-07-14 — maintainer sign-off on the REQ-106 specification (the reference + version were chosen during brainstorming, superseding the placeholder plan's shared-model-first sketch); implementation lands under [2026-05-22-webtemplate-export.md](../plans/2026-05-22-webtemplate-export.md).
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Introduces:** REQ-106 (WebTemplate JSON export). **Amends:** —. **Applies:** REQ-013 (building-block independence), REQ-111 (public compiled-template bridge — the export's input), REQ-103 (primitive constraints — the input source for `inputs`).
- **Plan:** [2026-05-22-webtemplate-export.md](../plans/2026-05-22-webtemplate-export.md).
- **Related:** the WebTemplate node tree is the same simplified-template projection the [simplified-formats umbrella](../plans/2026-06-23-simplified-formats.md) will share with REQ-053 (FLAT/STRUCTURED); this ADR governs only the WebTemplate JSON export slice.

## Context

**WebTemplate is a de-facto format, not an openEHR-normative artefact.** There is no published openEHR schema for it — only the standardised downstream FLAT/STRUCTURED *serialization* is normative (ITS-REST Simplified Formats). Two reference implementations exist and **diverge**:

- **Better** `web-template` (Kotlin) — camelCase `id`s; the originating reference, but the repository is frozen/build-rotted.
- **EHRbase** `openEHR_SDK` (Java) — lower-snake `id`s, self-reported `version "2.3"`; actively maintained; ships a `test-data` corpus of matched OPT + WebTemplate JSON + FLAT triples.

The **`id` field is consumer-critical and effectively irreversible once shipped.** FLAT composition path keys are built by concatenating WebTemplate `id`s, so a form or datamap that binds to our output depends on the exact sanitisation and sibling-disambiguation rules we emit. Changing the reference (or the id algorithm) after consumers exist would break every stored path — a breaking change for downstream code, often in other languages. The two references would produce *different* ids for the same node (`blood_pressure` vs `bloodPressure`), so we cannot straddle both. A single reference must be pinned before the id algorithm is written.

Because the format has no schema to appeal to, "correct" is defined operationally: **matching a concrete reference artefact.** That requires a vendored fixture from the chosen implementation to test against.

## Decision

Lock the WebTemplate JSON export to **EHRbase `openEHR_SDK`, WebTemplate `version "2.3"`** as the single reference implementation, specifically:

1. **Shape and version.** Emit the EHRbase v2.3 WebTemplate JSON shape (camelCase JSON fields; root `templateId`/`version`/`defaultLanguage`/`languages`/`tree`; the node and input field set); `version` is the string `"2.3"`.
2. **`id`-generation algorithm.** The `id` is a lower-snake sanitisation of the node's default-language display name with EHRbase's sibling-disambiguation rule. The exact normalisation (case, diacritics, non-alphanumeric collapsing, digit-leading guard, RM-type fallback) and disambiguation are **derived empirically from the vendored EHRbase reference fixture and pinned by table-driven tests** — not invented.
3. **Structural parity, not byte parity.** Conformance (PROBE-075) compares the SDK output to the reference **structurally** — the `id` set, `rmType`, `aqlPath`, `min`/`max`, and per-node input `suffix`/`type` — against a **documented-deviations list**. Field ordering, absent optional fields, localized-string packaging, and known id edge cases are recorded deviations, not failures. Byte-for-byte reproduction of EHRbase output is a non-goal.
4. **Fixture provenance.** The reference `corona_anamnese` OPT + WebTemplate JSON are vendored from a commit-pinned EHRbase `openEHR_SDK` revision (Apache-2.0) under `testkit/cassettes/` with a provenance + license note. If the environment blocks the fetch, PROBE-075 is deferred and the export is anchored to SDK-generated goldens until the fixture is supplied — the reason recorded in the probe catalogue.

The Better camelCase variant and multi-version output are **not** produced (see Alternatives).

## Consequences

- **Positive:** "correct" becomes verifiable against a concrete, maintained artefact rather than an interpretation; the id contract is stable for consumers; the vendored fixture gives PROBE-075 a real oracle.
- **Inherited quirks (accepted):** we adopt EHRbase's id choices verbatim, including any that are not what we would design in isolation — parity is worth more than local elegance, because interop is the whole point of the format.
- **Better-flavoured consumers not served:** a consumer expecting Better camelCase ids gets EHRbase snake ids. Adding the Better variant later would be an additive option behind a flag, not a breaking change to the default.
- **Fixture maintenance:** a vendored third-party Apache-2.0 fixture must carry attribution (`testkit/cassettes/THIRD_PARTY_LICENSES.md`) and is pinned to a commit; a reference bump is a deliberate, reviewed change.
- **Deviation drift:** the documented-deviations list is load-bearing — every accepted structural difference from the reference MUST be listed, so an unlisted difference is a test failure rather than silent divergence.

## Alternatives considered

- **Better `web-template` as the reference:** the originating format, but the repo is frozen/build-rotted — no maintained oracle to test against, and its camelCase ids are less common in current deployments. Rejected in favour of the actively-maintained EHRbase corpus.
- **Support both references / multi-version output:** doubles the consumer-critical id surface and the conformance matrix for no current consumer demand — YAGNI. A second variant can be added additively if a consumer needs it.
- **Invent our own id scheme:** defeats the purpose — FLAT-path interoperability with existing tooling requires mirroring a real implementation's ids; a bespoke scheme would be correct against nothing.
- **Byte-exact parity with EHRbase:** rejected as the conformance bar — field ordering and optional-field presence are implementation incidentals; pinning them would make the SDK brittle to cosmetic reference changes without improving interop.

## Acceptance gate

Per the plan's Definition of Ready, implementation is blocked until this ADR reaches **Accepted**. The decision itself (EHRbase v2.3, structural parity) was made during brainstorming; acceptance is the maintainer's sign-off on the REQ-106 specification. On acceptance, flip Status to `Accepted, <date>`, and the implementation PR carries the REQ-106 spec prose, the traceability wiring, and PROBE-075 (or its recorded deferral).
