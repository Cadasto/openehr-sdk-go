# Research strands

**Status:** Draft

Open architectural questions that are **not yet decided** but are scoped, named, and tracked. Each strand resolves into an ADR (under `../docs/adr/`) and amends one or more REQs in [REQ.md](REQ.md).

A strand is not a `Draft` REQ — it is an open question whose answer cannot be predicted by reading the spec. Resolving a strand happens by:

1. Producing evidence (a spike, a benchmark, a fit-gap analysis).
2. Writing an ADR proposing the resolution.
3. Amending the affected REQs in this tree.
4. Closing the strand here with a backlink to the ADR.

Strand IDs (`STRAND-NN`) are stable. Renumbering is prohibited.

---

## STRAND-01 — Extraction from reference CDR

**Status:** Resolved.

**Decision:** The SDK is specified and built as an independent module — its surface is defined by the specs, not derived from or gated on any existing codebase. Where a reference CDR already implements a primitive (an HTTP wrapper, RM mapping, auth scaffolding), reuse is opportunistic and case-by-case: generalisable primitives belong in the SDK; consumer-specific concerns stay in the consumer.

**Codified in:** the building-block and boundary rules (REQ-010..014).

---

## STRAND-02 — Shared contract source-of-truth (PHP ↔ Go)

**Status:** Cancelled.

**Rationale:** Not pursued. The SDK is hand-written and independent; wire-level conformance is guaranteed by the openEHR conformance probe suite (REQ-080), so no shared contract source or code-generation pipeline is needed.

---

## STRAND-03 — Go-idiomatic surface validation

**Status:** Resolved.

**Decision:** Package-level functions are the primary surface; repository structs are offered as a convenience for injection seams. Settled by the landed client surface and codified in REQ-021..024 ([idiom.md](idiom.md)) — the SDK follows idiomatic Go rather than mirroring another SDK's source shape.

---

## STRAND-04 — RM polymorphism and codec performance

**Status:** Partially resolved.

**Question:** The RM modeling rules in [rm-modeling.md](rm-modeling.md) (concrete structs + embedded base + interfaces + central type registry) need validation against the full RM 1.1.0-development surface. And: which JSON codec — `encoding/json`, `sonic`, `easyjson` — is the default?

### Resolved sub-questions

| Sub-question | Resolution | ADR |
|---|---|---|
| Abstract generic `EVENT` polymorphism (`History.events`) | Promote `EVENT` to a Go interface; `POINT_EVENT` / `INTERVAL_EVENT` concrete; whitelist in generator | [ADR 0003](../adr/0003-rm-event-polymorphism.md) |
| `Real` / `Integer` wire tolerance (quoted vs numeric JSON) | Strict encode, permissive decode via `rm.Real` / `rm.Integer` defined types | [ADR 0004](../adr/0004-numeric-wire-tolerance.md) |
| Polymorphic round-trip fidelity (REQ-052/040) | Value-in-interface `_type` on encode via `openehr/internal/jsonpoly`; round-tripped `DV_INTERVAL<T>` validated from its bounds' runtime types; corpus round-trips byte-stable | [archived plan](../plans/archive/2026-06-23-polymorphic-encode-decode.md) |

### Still open

- **Full RM inventory:** decode every BMM type through the registry; identify sites that resist the pattern (e.g. further `VERSION[T]` whitelist decisions beyond `EVENT`).
- **Default codec benchmark:** `encoding/json` (current, via generator-emitted `MarshalJSON`) vs `sonic` vs `easyjson` under seeder/benchmark workloads — a *performance* axis (throughput/allocations), codec swapped behind the same generated methods.
- **`encoding/json/v2` as a *simplification* axis:** distinct from the performance candidates above. The generator emits bespoke `MarshalJSON`/`UnmarshalJSON` (`internal/bmmgen/render_json{mar,unmar}.go`) and `openehr/internal/jsonpoly` largely to obtain what `encoding/json` v1 lacks: deterministic field order, correct zero/omit semantics (`omitzero`), and value-in-interface `_type` handling. Go 1.27 ships `encoding/json/v2` and `encoding/json/jsontext` as ordinary stdlib (no `GOEXPERIMENT=jsonv2`); v1 is implemented on v2 with behaviour preserved. Native v2 options could *retire* a large share of that generated + hand-written marshaling surface rather than merely swap the codec behind it. The API is stable; the remaining gate is the byte-stable canjson fit-gap, not experimental status.
- **Validation independence:** confirm `openehr/validation` can validate without taking on the codec's dependencies (REQ-013).

**Evidence needed (remaining):**

- Benchmark throughput, allocations, and memory residency for codec candidates.
- Document any remaining abstract-generic classes requiring ADR whitelist (generator policy today: `EVENT` only).
- `encoding/json/v2` fit-gap: whether `jsontext` + marshal options reproduce byte-stable canonical JSON (PROBE-030/031/038) and the polymorphic `_type` round-trip (REQ-052/040) without the generator's marshaler emit — and quantify the generated + `jsonpoly` LOC it would remove.
- `encoding/json/v2` stability/timeline: stable stdlib as of Go 1.27; remaining work is the fit-gap (byte-stable `_type`-first canjson, PROBE-030/031/038) before any generator/ADR change.

**Resolution form (remaining):** ADR choosing the default codec (with tuning-knob notes for swapping). Amends REQ-052, REQ-053, possibly REQ-040 if registry shape needs tweaking. A `encoding/json/v2` resolution would additionally touch the codegen policy in [ADR 0002](../adr/0002-bmm-codegen-decisions.md), since it changes what the generator emits.

**Implementation gate:** Phase 1b — affects every read path in `openehr/client/*` and openEHR wire conformance (REQ-080).

---

## STRAND-05 — SMART-on-openEHR auth library

**Status:** Resolved. See [ADR 0009](../adr/0009-smart-auth-library-scope.md).

**Question (resolved):** There is no batteries-included Go substrate for OAuth2 / SMART-on-openEHR; implementing `auth/smart` is first-party work. What's the implementation plan and how is it validated?

**Decision summary:**

- Built the full SMART-on-openEHR auth library across `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic`, `auth/introspect`, and `smart/discovery`. Four flows (PKCE public, confidential symmetric, confidential asymmetric `private_key_jwt`, Backend Services) and three launch modes (standalone, embedded, backend) are covered and exercised by PROBE-001..009.
- Relaxed the OTel-only dependency rule: adopted `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`, and `github.com/go-jose/go-jose/v4` (directly imported for JWS signing; also required by `go-oidc/v3`) for security-sensitive JOSE/OIDC crypto, scoped to `auth/` and `smart/`. Hand-rolling JWS signing and ID-token verification at the RS384/ES384/RS256/ES256 multi-alg level was rejected as a correctness and maintenance risk.
- `auth.FromOAuth2TokenSource` adapter and an issuer-matching multi-EHR helper are recorded as available follow-ups (not built — no current consumer need). See ADR 0009 § (c).

**Codified in:** [ADR 0009](../adr/0009-smart-auth-library-scope.md). Amends REQ-061..064.

---

## STRAND-06 — Concurrency and transport hygiene

**Status:** Open.

**Question:** The federator constructs multiple SDK clients. Do they share a single `*http.Transport` (for connection-pool efficiency across nodes) or own independent transports (for cleaner failure isolation per node)?

**Why it's open:** REQ-021 says "inject your `*http.Client`" — but it does not prescribe sharing. Both approaches are defensible; the trade-off is connection-pool reuse vs blast-radius isolation.

**Evidence needed:**

- Federator-style spike (Phase 3) running 4+ clients with both shared and independent transports.
- Measure: connection-establishment latency (cold), pool-exhaustion behaviour under load, failure isolation when one node is slow / unreachable.

**Resolution form:** ADR-NNN documenting the recommendation and the override path. Amends REQ-021 if the default guidance becomes opinionated.

**Implementation gate:** Phase 3 — federator implementation makes this decision real.

---

## STRAND-07 — Versioning and module path

**Status:** **Resolved.**

**Decision:** Module path is `github.com/cadasto/openehr-sdk-go`; semantic-import versioning for v2+; `internal/` boundary per Go convention.

**Rationale:** Locked early to avoid late-cycle import-path churn.

**Codified in:** REQ-001, REQ-004, REQ-005, [module-layout.md § Versioning](module-layout.md#versioning).

---

## STRAND-08 — Cadasto extras: boundary, criteria, conditional extraction

**Status:** Open.

**Question:** Will `cadasto/…` ever be extracted into a sibling Go module? If so, when, and what triggers the decision?

**Why it's open:** the openEHR-core part of this SDK could in principle be vendor-neutral (target EHRbase, other openEHR backends). The Cadasto extras are platform-specific. Whether keeping them in one module or splitting them is right depends on adoption, governance, and cross-backend demand.

**Criteria** (none decisive on its own; jointly assessed):

- **Conceptual.** Is the surface an openEHR concept or a Cadasto-specific concept?
- **Technical.** Does it share types and lifecycle with openEHR core, or is it thinly layered on top?
- **Audience.** Is it needed by every consumer, or by a subset (integration developers using Admin / Datamap)?
- **Governance.** Same release cadence as openEHR core, or faster-churn Cadasto surfaces?
- **Cross-backend demand.** Concrete demand for the openEHR-core to work against EHRbase or another non-Cadasto backend.

**Evidence needed:**

- Adoption data after Phase 4 (Cadasto extras shipped).
- A concrete consumer asking for an EHRbase-compatible build.

**Resolution form:** ADR-NNN either confirming "keep together" or extracting. If extracting: package moves, module path for the sibling, semver implications for both modules.

**Open until:** v1 is in production for at least one minor release. Premature resolution is worse than no resolution.

**Boundary held in v1** (regardless of resolution): REQ-010, REQ-011, REQ-012 enforce the cut line so an extraction would be mechanical, not a rewrite.

---

## STRAND-09 — ITS-REST conformance follow-ups

**Status:** Open (deferred) — opened by the [ITS-REST conformance remediation](../plans/archive/2026-06-19-its-rest-conformance-remediation.md).

**Two follow-ups carried out of that plan:**

1. **Dedicated REST conformance probes (deferred).** The plan called for four `testkit/probes/rest/*` probes (audit-details header grammar, System `OPTIONS`, Admin bulk-delete, Definition `/example`). The corrected wire shapes are currently asserted by package `httptest` tests (request-capture), which provide equivalent coverage for CI but are not part of the ratifiable probe suite (REQ-080/082). **Open until:** the sandbox/cassette probe runner (REQ-082) is the gate for these surfaces; then promote the unit assertions to `rest/*` probes. Affects REQ-059, REQ-095, REQ-099.

2. **Stored-query `fetch` omission vs OpenAPI `required` (decision recorded).** The Definition/Query `Query` schema marks `offset`, `fetch`, and `query_parameters` `required`, but `fetch` has no spec default ("depends on the implementation") and `fetch: 0` means *zero rows*. **Decision:** the SDK always emits `offset` (documented default 0) and `query_parameters`, but omits `fetch` unless the caller sets it (`fetchSet`), so the server applies its own default. This is a deliberate, documented deviation from the literal `required` list (`openehr/client/query/execute.go` `storedBody`). **Revisit if:** a conformant backend rejects a body without `fetch`, in which case escalate to an ADR (emit a sentinel/`-1`, or send the server's advertised default).

---

## STRAND-10 — rmpath: one sentinel for two not-found conditions

**Status:** Open — opened by the PROBE-086 review round ([2026-07-16 plan](../plans/archive/2026-07-16-web-template-tests-conformance.md), follow-ups).

**Question:** should `rmpath.ItemAtPath` distinguish *attribute unknown to the navigator* from *attribute known but unpopulated*, instead of returning `ErrPathNotFound` for both?

**Why it's open:** the FLAT encoder treats resolution failure as an absent optional (`skipNotFound`), which is correct for the unpopulated case and silent data loss for the unknown-attribute case — the exact defect class PROBE-086 surfaced (`EVENT.time`, `INSTRUCTION.narrative`/`expiry_time`, `EVENT_CONTEXT.other_context`, `ACTIVITY.timing`). Because the encoder cannot tell the two conditions apart, the coverage rule in [rm-functions.md § REQ-121](rm-functions.md#req-121--locatable-path-read-access) is enforced by a hand-kept guard test and exemption list rather than derived from the API.

**Resolution options:** a distinct sentinel (e.g. `ErrUnknownAttribute`) letting `skipNotFound` fail loudly on unmodelled attributes and retiring the exemption list; or keeping the single sentinel and the guard-test enforcement. The first adds an exported error to a landed surface (REQ-024 compatibility question); the second keeps the hazard class alive but contained.

**Evidence needed:** whether the exemption list churns (each churn is an argument for the sentinel); whether any consumer outside the encoder needs the distinction. First datum — **2026-08-03 PROBE-086 coverage ratchet:** ten `unserialisableIC` exemptions deleted (ENTRY `language` / `encoding` across all five subtypes) and `rmpath` extended to resolve them, so the list churned once already.

**Resolution form:** ADR-NNN; the REQ-121 open-question paragraph and the guard-test exemption mechanism are updated together with it.

---

## STRAND-11 — Probe recording format: HAR or a purpose-built YAML

**Status:** Open — opened by the [REQ-082 specification pass](../plans/2026-08-18-probe-runnability.md).

**Question:** what serialisation do Cassette-mode recordings use — the HTTP Archive standard (`.har`), or a purpose-built YAML schema of this SDK's own?

**Why it's open:** REQ-082 previously said "`.har` or `.yaml`" and left the choice unmade, which is why no recorder exists. The requirement now states what a recording MUST carry (provenance, capture-time redaction, order-preserving normalised matching) independently of its encoding, so phases 1–2 of the arc are unblocked; the encoding is only decidable with a capture in hand.

**The trade-off:**

- **HAR** is a published standard with an existing tool ecosystem (browser devtools, mitmproxy, Charles all export it), which matters because a probe definition is meant to be implementable in any language — a PHP implementation of the same probe could replay the same corpus. Against it: HAR is verbose, diffs poorly under review, models browser concerns this SDK has no use for (timings, cache, `pageref`), and has no native slot for the provenance and redaction attestation REQ-082 requires — both would become extension fields.
- **Purpose-built YAML** carries provenance and redaction natively, reviews well in a pull request (which is load-bearing: a reviewer must be able to *see* that a recording captured from a real CDR contains no bearer token and no patient data), and matches how this tree already pins vendored corpora. Against it: a bespoke schema, no external tooling, and cross-language portability that rests on the schema being documented well enough for a second implementation.

**Evidence needed:** one real capture against a live CDR, serialised both ways, reviewed as a diff. The auditability question is empirical — whether a reviewer can actually spot an unredacted field — and it cannot be settled by reasoning about the formats.

**Resolution form:** ADR-NNNN, taken when Cassette mode is implemented (phase 3 of the plan). Until then no recorder exists to prejudge it.

**Implementation gate:** blocked on access to a live openEHR deployment to record from — the same dependency that defers phases 3–4.

---

## STRAND-12 — BMM interface classes carry no `is_abstract` flag

**Status:** Open — opened by the [REQ-048 specification pass](../plans/archive/2026-08-18-rminfo-class-hierarchy.md).

**Question:** should the RM meta-model introspection surface ([REQ-048](bmm-conformance.md#req-048--rm-meta-model-introspection-surface)) report a `P_BMM_INTERFACE` class as abstract even though the pinned BMM sets no `is_abstract` flag on it — and is the missing flag an upstream BMM defect this SDK should raise per [REQ-047](bmm-conformance.md#req-047--bmm-spec-divergence-resolution)?

**Why it's open:** six classes in REQ-048's universe carry no `is_abstract` flag: `MEASUREMENT_SERVICE`, `TERMINOLOGY_SERVICE`, `OPENEHR_CODE_SET_IDENTIFIERS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS`, and — the sharp cases — `CODE_SET_ACCESS` and `TERMINOLOGY_ACCESS`, which are `P_BMM_INTERFACE` declarations the generator renders as Go **interfaces**. Nothing can instantiate a Go interface, so the surface reports "not abstract" for a class that is unquestionably not instantiable, and a concrete-descendant expansion hands it back. REQ-047 settles the immediate behaviour — the BMM wins, the SDK reports the flag verbatim, and REQ-048 says so normatively — but it does not settle whether the *question* is the right one.

**The trade-off:**

- **Report the flag only** (today's behaviour). One rule, one source, no local model of instantiability; a caller needing the decodable set uses the [REQ-040 registry](rm-modeling.md#type-registry-req-040), which is the authority for what Go can construct. Against it: a caller who reads "not abstract" as "instantiable" is misled on six classes, and the surface offers no way to notice.
- **Widen abstractness to cover BMM interfaces** — report `is_abstract || _type == P_BMM_INTERFACE`. Matches what the generator actually emits and what a caller means. Against it: it is the SDK second-guessing the BMM, which REQ-047 forbids as a general practice precisely because local corrections diverge silently from every other openEHR implementation reading the same file.
- **Raise it upstream and keep reporting the flag** until the schema changes. Correct in the long run and costs nothing locally; the timescale is not ours to set.

**Evidence needed:** whether the openEHR BMM treats a missing `is_abstract` on `P_BMM_INTERFACE` as implicit (in which case every consumer must infer it, and inferring is not second-guessing) or as an oversight in these six declarations. That is a question for the openEHR specifications repository, not for this tree.

**Resolution form:** an upstream issue first; then either an ADR widening the abstractness question, or a documented acceptance that the flag is the whole answer. Until then the interim rule is REQ-048's — its § *Abstract is the BMM's flag, not a storability verdict* forbids pre-empting this strand in code.

**Affects:** REQ-048, and REQ-047's divergence-reporting duty.

---

## STRAND-13 — Properties inherited from a primitive-mapped ancestor are dropped

**Status:** Open — opened by [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm)'s attribute-set completeness arm ([REQ-048](bmm-conformance.md#req-048--rm-meta-model-introspection-surface) implementation).

**Question:** when a `class_definitions` class inherits a property from a `primitive_types` ancestor that [§ Primitive type mapping](bmm-conformance.md#primitive-type-mapping) maps to a Go primitive, should the generator fold that property in — into the emitted struct, into the `rminfo` attribute tables, or into neither?

**Why it's open:** the generator plans `primitive_types` entries mapped to a Go primitive as primitives, not as classes, so it never walks their properties. On the pinned schemas exactly one class is affected: `Iso8601_timezone` (a `class_definitions` entry, and so in REQ-048's class universe) inherits `value: String` — **mandatory** — from `Iso8601_type`, which maps to Go `string`. The result is consistent between the two SDK surfaces and inconsistent with the BMM: `rm.ISO8601Timezone` is emitted as an **empty struct**, and `rminfo` reports the class as carrying no attributes. The type cannot hold its own value, and nothing flagged it until PROBE-094 compared the shipped attribute sets against an independent BMM reduction.

This is a pre-existing emission gap, not a REQ-048 regression — REQ-048 only made it visible, by removing the attribute-less-class skip that had kept `Iso8601_timezone` out of the table entirely.

**The trade-off:**

- **Fold it into both surfaces** — `rm.ISO8601Timezone` gains a `Value string` field and `rminfo` reports the attribute. Faithful to the BMM, and the type becomes able to hold the value it is defined by. Against it: it changes a generated public struct, and the same rule would then apply to every primitive-mapped ancestor, whose blast radius across the base schema is unmeasured.
- **Fold it into `rminfo` only** — the tables become BMM-faithful while the struct stays empty. Against it: `rminfo` would then describe an attribute no Go type can hold, and the REQ-112 validation floor would demand a field that does not exist. Strictly worse than either consistent option.
- **Leave both as they are** and treat these classes as deliberately value-less in Go, on the grounds that the four ISO 8601 value types are already mapped to `string` and this one is reached through them. Against it: it leaves a mandatory BMM attribute unrepresentable and undocumented outside a test pin.

**Evidence needed:** the census this strand cannot assume — every `class_definitions` class with a primitive-mapped ancestor that declares properties, across all primary schemas, not just the one the pinned RM reduction surfaces. Whether `Iso8601_timezone` is a one-off or the visible member of a family decides which option is affordable.

**Evidence (2026-09-05 census):** `internal/bmmgen/primitive_ancestor_census_test.go` walks every `class_definitions` class in each of the six pinned schema roots (`base 1.3.0`, `rm 1.2.0`, `am 1.4.0`, `am 2.4.0`, `lang 1.1.0`, `term 3.1.0`) and lists each property reachable only through a primitive-mapped ancestor — a property the class or a non-primitive ancestor declares itself is planned and shipped, so it does not count. Result: **exactly one** — `Iso8601_timezone.value` via `Iso8601_type`, present in the five roots that include `base` (`term 3.1.0` includes no base schema and has no ISO 8601 classes). The four DV temporal types (`DV_DATE`, `DV_TIME`, `DV_DATE_TIME`, `DV_DURATION`) also descend from `Iso8601_type` but redeclare `value` on the class, which is why PROBE-094's reduction never flagged them and why the generated structs carry it. The set is pinned by the test, so growth is loud. What this settles: `Iso8601_timezone` is a one-off, not the visible member of a family — the fold-into-both option's blast radius is one generated struct and one `rminfo` row. What it does not settle: the fold itself, which remains an ADR amending § Mapping rules plus a regenerated tree; the strand stays open.

**Resolution form:** an ADR amending the § Mapping rules inheritance rule, plus a regenerated tree, if either folding option wins; otherwise a documented mapping exemption. Until then the interim prohibition is [REQ-048 § The attribute tables are complete against the BMM](bmm-conformance.md#the-attribute-tables-are-complete-against-the-bmm)'s: the drop is pinned by name, cannot grow silently, and folding it in ahead of this strand fails [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm)'s completeness arm (`unshippedProperties`).

**Affects:** REQ-042, REQ-043, REQ-046, REQ-048.

---

## STRAND-14 — Should template-driven validation also run the RM-floor invariants?

**Status:** Open — opened 2026-09-05 from the review round of the [RM canonical-JSON fidelity plan](../plans/archive/2026-09-01-rm-canonical-json-fidelity.md), whose text claimed `ValidateComposition` would report the new `TERM_MAPPING` invariants; it never has, and the claim was corrected there rather than in code.

**Question:** should `ValidateComposition` ([REQ-102](clinical-modeling.md#req-102--composition-validation)) and the [REQ-110](clinical-modeling.md#req-110--template-driven-validation-beyond-composition) entry points run the [REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor) per-type invariant catalogue as part of a template-driven pass, or stay exactly template conformance with the floor a separate call?

**Why it's open:** today the two layers compose but do not chain. Template validity covers RM-mandatory presence, so a template-driven pass already reports the floor's (a) arm; the floor's (b) arm — `DV_INTERVAL` lower > upper, `TERM_MAPPING.match` outside its value set, `Mappings_valid`, `DV_QUANTITY.precision < 0` — fires only through `ValidateRM`. A caller who runs only `ValidateComposition` can therefore commit an RM-invalid composition that is template-valid. Whether that is a gap or a deliberate separation of concerns is the fork.

**The trade-off:**

- **Chain the floor into the template-driven pass.** One call, no way to forget the floor; issue codes stay disjoint (`term_mapping_match` beside the template codes). Against it: `Result` grows for every existing caller, the template walker and the floor walker visit the tree twice (or the floor's checks are re-hosted inside the template walk), and some floor findings duplicate template findings on the same node.
- **Keep them separate and document the composition.** No behaviour change; callers who want both call both, and § REQ-112 says so. Against it: the gap stays reachable by default, and the plan-text defect that opened this strand shows how easily the two are assumed to chain.
- **Opt-in chaining** — an option on the template-driven entry points. Additive and reversible. Against it: two code paths to keep in agreement, and an option nobody sets is the second option under another name.

**Evidence needed:** how often a template-valid, RM-invalid composition reaches a consumer in practice (the consuming CDR project's validation pipeline is the first place to ask); the duplicate-issue rate if the floor is chained over the vendored composition corpus; the cost of a second walk at the benchmark harness's composition sizes.

**Resolution form:** ADR-NNNN; amends REQ-102 / REQ-110 (if chaining) and REQ-112's composition sentence either way. Until then § REQ-112 states the current behaviour and points here, and the answer **MUST NOT** be pre-empted in code.

**Affects:** REQ-102, REQ-110, REQ-112.

---

## Index

| Strand | Title | Status | Affects |
|---|---|---|---|
| [STRAND-01](#strand-01--extraction-from-reference-cdr) | Extraction from reference CDR | **Resolved** | REQ-010..014 |
| [STRAND-02](#strand-02--shared-contract-source-of-truth-php--go) | Shared contract source-of-truth | **Cancelled** | — |
| [STRAND-03](#strand-03--go-idiomatic-surface-validation) | Go-idiomatic surface | **Resolved** | REQ-021..024 |
| [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) | RM polymorphism + codec perf | **Partially resolved** | REQ-024, REQ-040, REQ-052..053 |
| [STRAND-05](#strand-05--smart-on-openehr-auth-library) | SMART-on-openEHR auth library | **Resolved** ([ADR 0009](../adr/0009-smart-auth-library-scope.md)) | REQ-061..064 |
| [STRAND-06](#strand-06--concurrency-and-transport-hygiene) | Concurrency / transport hygiene | Open | REQ-021, REQ-026 |
| [STRAND-07](#strand-07--versioning-and-module-path) | Versioning + module path | **Resolved** | REQ-001, REQ-004, REQ-005 |
| [STRAND-08](#strand-08--cadasto-extras-boundary-criteria-conditional-extraction) | Cadasto-extras extraction | Open (long-term) | REQ-010, REQ-011 |
| [STRAND-09](#strand-09--its-rest-conformance-follow-ups) | ITS-REST conformance follow-ups (REST probes; stored-query `fetch`) | Open (deferred) | REQ-059, REQ-095, REQ-099 |
| [STRAND-10](#strand-10--rmpath-one-sentinel-for-two-not-found-conditions) | rmpath not-found sentinel split | Open | REQ-053, REQ-121 |
| [STRAND-11](#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) | Probe recording format (HAR vs YAML) | Open | REQ-082 |
| [STRAND-12](#strand-12--bmm-interface-classes-carry-no-is_abstract-flag) | BMM interface classes carry no `is_abstract` | Open | REQ-047, REQ-048 |
| [STRAND-13](#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) | Properties inherited from a primitive-mapped ancestor are dropped | Open | REQ-042, REQ-043, REQ-046, REQ-048 |
| [STRAND-14](#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) | Template-driven validation and the RM floor | Open | REQ-102, REQ-110, REQ-112 |
