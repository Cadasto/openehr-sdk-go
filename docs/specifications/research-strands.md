---
kind: specification
---

# Research strands

**Status:** Draft

Open architectural questions that are **not yet decided** but are scoped, named, and tracked. Each strand resolves into an ADR (under `../adr/`) and amends one or more REQs in [REQ.md](REQ.md).

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

**Codified in:** the building-block and boundary rules (REQ-013 and [module-layout.md § Boundary rules](module-layout.md#boundary-rules)).

---

## STRAND-02 — Shared contract source-of-truth (PHP ↔ Go)

**Status:** Cancelled.

**Rationale:** Not pursued. The SDK is hand-written and independent; wire-level conformance is guaranteed by the openEHR conformance probe suite ([conformance.md](conformance.md)), so no shared contract source or code-generation pipeline is needed.

---

## STRAND-03 — Go-idiomatic surface validation

**Status:** Resolved.

**Decision:** Package-level functions are the primary surface; repository structs are offered as a convenience for injection seams. Settled by the landed client surface and codified in REQ-021 and [idiom.md § API conventions](idiom.md#api-conventions) — the SDK follows idiomatic Go rather than mirroring another SDK's source shape.

---

## STRAND-04 — RM polymorphism and codec performance

**Status:** Partially resolved.

**Question:** The RM modeling rules in [reference-model.md](reference-model.md) (concrete structs with flattened ancestors, interfaces, and a central type registry) need validation against the full RM 1.2.0 surface, and the migrated codec path needs to stay within a performance budget. The codec choice itself — once framed as `encoding/json` vs `sonic` vs `easyjson` — is settled: [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md) adopts the standard library's `encoding/json/v2`.

### Resolved sub-questions

| Sub-question | Resolution | ADR |
|---|---|---|
| Abstract generic `EVENT` polymorphism (`History.events`) | Promote `EVENT` to a Go interface; `POINT_EVENT` / `INTERVAL_EVENT` concrete; whitelist in generator | [ADR 0003](../adr/0003-rm-event-polymorphism.md) |
| `Real` / `Integer` wire tolerance (quoted vs numeric JSON) | Strict encode, permissive decode via `rm.Real` / `rm.Integer` / `rm.Integer64` defined types | [ADR 0004](../adr/0004-numeric-wire-tolerance.md) |
| `encoding/json/v2` as the canonical-JSON codec; encoded member order is not a contract | Migrate to standard-library v2; retire the generator's per-type `MarshalJSON` / `UnmarshalJSON` and `openehr/internal/jsonpoly`; `_type` first becomes a SHOULD and round-trip fidelity is asserted semantically rather than byte-wise | [ADR 0021](../adr/0021-json-member-order-not-a-contract.md), [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md) |

### Benchmark evidence

Time/op change against the retired generated marshalers (v1 at `90473f9f`), from `benchstat -count=10` on one machine (13th Gen i7-13700H, WSL2, `go1.27.1`). Each of the first two columns was measured against its own v1 run in the same session. The benchmarks live in `openehr/serialize/canjson/`, `openehr/rm/typereg/` and `openehr/serialize/simplified/`. The allocation column gives v1, then PR 171, then PR 174, and takes its v1 figure from the PR 171 run.

The PR 280 column is measured differently. Its baseline is `main` at `6f75aea3`, which already holds PR 171 and PR 174, not v1, and it comes from `benchstat -count=6` on the same machine with `go1.27.2`. Each cell gives the time/op change, then the allocs/op change in brackets: the FLAT corpus has grown since v1, so its absolute counts no longer compare with the allocation column. A `~` is a time change that did not reach p < 0.05. The FLAT rows ran on a busier host (load about 3) than the others (load about 1), so their time cells are the least reliable; allocation counts do not depend on load.

| Benchmark | v2 migration ([PR 171](https://github.com/Cadasto/openehr-sdk-go/pull/171), `93d06e1f`) | single-pass concrete decode ([PR 174](https://github.com/Cadasto/openehr-sdk-go/pull/174)) | hot-path cuts ([PR 280](https://github.com/Cadasto/openehr-sdk-go/pull/280)), against `main` | allocs/op |
|---|---|---|---|---|
| `EncodeComposition_400` | -47.82% | -47.91% | -24.88% (-65.87%) | 9 281 / 16 444 / 16.44k |
| `DecodeComposition_400` | +8.98% | -39.62% | -44.07% (-42.42%) | 21 665 / 16 860 / 13.25k |
| `DecodeCompositionFixture` | -38.30% | -61.05% | -31.62% (-45.65%) | 6 279 / 5 220 / 4 809 |
| `EncodeDVQuantity` | +10.52% | not measured | -21.45% (-44.44%) | 6 / 9 / not measured |
| `DecodeDVQuantity` | +37.88% | -20.19% | -6.62% (-25.00%) | 3 / 5 / 4 |
| `RegistryDecodeElement` | +13.84% | -30.80% | -27.83% (-33.33%) | 29 / 39 / 15 |
| `RegistryDecodeDVQuantity` | +18.80% | -30.04% | -9.62% (-20.00%) | 9 / 18 / 5 |
| `FlatCorpusRoundTrip` | +1.77% | not measured | -5.59% (-3.31%) | 68 400 / 69 940 / not measured |
| `MarshalFlat` | +4.73% | not measured | ~ (-4.55%) | 1 103 / 1 193 / not measured |
| `UnmarshalFlat` | +2.69% | not measured | ~ (-2.17%) | 2 962 / 2 884 / not measured |

### Still open

- **Full RM inventory:** decode every BMM type through the registry; identify sites that resist the pattern (e.g. further `VERSION[T]` whitelist decisions beyond `EVENT`).
- **Codec performance against the standard-library baseline:** with the codec settled by [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md), the open question is no longer which codec but whether the migrated path is within budget for seeder and benchmark workloads, measured against the retired generated marshalers on `openehr/serialize/canjson/bench_test.go` rather than against `sonic` or `easyjson`. A regression is a plan task, not a codec fork.

**Evidence needed (remaining):**

- Memory residency for the `encoding/json/v2` path against the retired generated marshalers.
- Any further abstract-generic class that needs an ADR whitelist (the generator's policy today: `EVENT` only).

The budget line: a benchmark regresses materially when its time/op is more than 20 percent worse than the retired generated marshalers' at p < 0.05, measured with `benchstat`. The codec benchmarks live beside the canonical-JSON codec, the type registry and the FLAT codec.

**Resolution form (remaining):** the full-inventory and performance sub-questions resolve through benchmarks, not a new ADR; the codec choice is resolved by [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md).

---

## STRAND-05 — SMART-on-openEHR auth library

**Status:** Resolved by [ADR 0009](../adr/0009-smart-auth-library-scope.md).

**Decision:** The SDK carries its own SMART-on-openEHR auth library across `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic` and `smart/discovery`, using `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3` and `github.com/go-jose/go-jose/v4` for the JOSE and OIDC cryptography, confined to `auth/` and `smart/`. Token introspection, a resource-server operation, is not part of it.

**Codified in:** [auth.md](auth.md).

---

## STRAND-06 — Concurrency and transport hygiene

**Status:** Open.

**Question:** The federator constructs multiple SDK clients. Do they share a single `*http.Transport` (for connection-pool efficiency across nodes) or own independent transports (for cleaner failure isolation per node)?

**Why it's open:** REQ-021 says "inject your `*http.Client`" — but it does not prescribe sharing. Both approaches are defensible; the trade-off is connection-pool reuse vs blast-radius isolation.

**Evidence needed:**

- A federator-style spike running 4+ clients with both shared and independent transports.
- Measure: connection-establishment latency (cold), pool-exhaustion behaviour under load, failure isolation when one node is slow / unreachable.

**Resolution form:** ADR-NNN documenting the recommendation and the override path. Amends REQ-021 if the default guidance becomes opinionated.

**Implementation gate:** a federator implementation makes this decision real.

---

## STRAND-07 — Versioning and module path

**Status:** **Resolved.**

**Decision:** Module path is `github.com/cadasto/openehr-sdk-go`; semantic-import versioning for v2+; `internal/` boundary per Go convention.

**Rationale:** Locked early to avoid late-cycle import-path churn.

**Codified in:** [module-layout.md § REQ-001](module-layout.md#req-001--module-path), [§ REQ-004](module-layout.md#req-004--semantic-versioning) and [§ The `internal/` boundary](module-layout.md#the-internal-boundary).

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

- Adoption data once the Cadasto extras ship.
- A concrete consumer asking for an EHRbase-compatible build.

**Resolution form:** ADR-NNN either confirming "keep together" or extracting. If extracting: package moves, module path for the sibling, semver implications for both modules.

**Open until:** v1 is in production for at least one minor release. Premature resolution is worse than no resolution.

**Boundary held in v1** (regardless of resolution): the [module-layout.md § Boundary rules](module-layout.md#boundary-rules) hold the cut line so an extraction would be mechanical, not a rewrite.

---

## STRAND-09 — ITS-REST conformance follow-ups

**Status:** Resolved by the dedicated REST probes (PROBE-062, PROBE-102, PROBE-103 and PROBE-104) and by [ADR 0026](../adr/0026-stored-query-paging-fields-only-when-set.md) for the stored-query paging fields.

**Codified in:** [conformance.md](conformance.md) and [wire.md § REQ-057](wire.md#req-057) *Paging*.

---

## STRAND-10 — rmpath: one sentinel for two not-found conditions

**Status:** Open.

**Question:** should `rmpath.ItemAtPath` distinguish *attribute unknown to the navigator* from *attribute known but unpopulated*, instead of returning `ErrPathNotFound` for both?

**Why it's open:** the FLAT encoder treats resolution failure as an absent optional (`skipNotFound`), which is correct for the unpopulated case and silent data loss for the unknown-attribute case — the exact defect class PROBE-086 surfaced (`EVENT.time`, `INSTRUCTION.narrative`/`expiry_time`, `EVENT_CONTEXT.other_context`, `ACTIVITY.timing`). Because the encoder cannot tell the two conditions apart, the coverage rule in [reference-model.md § REQ-121](reference-model.md#req-121--locatable-path-read-access) is enforced by a hand-kept guard test and exemption list rather than derived from the API.

**Resolution options:** a distinct sentinel (e.g. `ErrUnknownAttribute`) letting `skipNotFound` fail loudly on unmodelled attributes and retiring the exemption list; or keeping the single sentinel and the guard-test enforcement. The first adds an exported error to a landed surface ([idiom.md § API conventions](idiom.md#api-conventions) compatibility question); the second keeps the hazard class alive but contained.

**Evidence needed:** whether the exemption list churns (each churn is an argument for the sentinel); whether any consumer outside the encoder needs the distinction. First datum — **2026-08-03 PROBE-086 coverage ratchet:** ten `unserialisableIC` exemptions deleted (ENTRY `language` / `encoding` across all five subtypes) and `rmpath` extended to resolve them, so the list churned once already.

**Resolution form:** ADR-NNN; the REQ-121 open-question paragraph and the guard-test exemption mechanism are updated together with it.

---

## STRAND-11 — Probe recording format: HAR or a purpose-built YAML

**Status:** Resolved by [ADR 0020](../adr/0020-cassette-recording-har.md).

**Decision:** Cassette recordings are HTTP Archive 1.2, with provenance and redaction attestation on `log` as a `_req082` object, so any HAR reader can replay them.

**Codified in:** [conformance.md § REQ-082](conformance.md#req-082--runnability).

---

## STRAND-12 — BMM interface classes carry no `is_abstract` flag

**Status:** Open.

**Question:** should the RM meta-model introspection surface ([REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface)) report a `P_BMM_INTERFACE` class as abstract even though the pinned BMM sets no `is_abstract` flag on it — and is the missing flag an upstream BMM defect this SDK should raise per [REQ-041](reference-model.md#req-041--pinned-bmm-sources)?

**Why it's open:** six classes in REQ-048's universe carry no `is_abstract` flag: `MEASUREMENT_SERVICE`, `TERMINOLOGY_SERVICE`, `OPENEHR_CODE_SET_IDENTIFIERS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS`, and — the sharp cases — `CODE_SET_ACCESS` and `TERMINOLOGY_ACCESS`, which are `P_BMM_INTERFACE` declarations the generator renders as Go **interfaces**. Nothing can instantiate a Go interface, so the surface reports "not abstract" for a class that is unquestionably not instantiable, and a concrete-descendant expansion hands it back. REQ-041 settles the immediate behaviour — the BMM wins, the SDK reports the flag verbatim, and REQ-048 says so normatively — but it does not settle whether the *question* is the right one.

**The trade-off:**

- **Report the flag only** (today's behaviour). One rule, one source, no local model of instantiability; a caller needing the decodable set uses the [REQ-040 registry](reference-model.md#type-registry-req-040), which is the authority for what Go can construct. Against it: a caller who reads "not abstract" as "instantiable" is misled on six classes, and the surface offers no way to notice.
- **Widen abstractness to cover BMM interfaces** — report `is_abstract || _type == P_BMM_INTERFACE`. Matches what the generator actually emits and what a caller means. Against it: it is the SDK second-guessing the BMM, which REQ-041 forbids as a general practice precisely because local corrections diverge silently from every other openEHR implementation reading the same file.
- **Raise it upstream and keep reporting the flag** until the schema changes. Correct in the long run and costs nothing locally; the timescale is not ours to set.

**Evidence needed:** whether the openEHR BMM treats a missing `is_abstract` on `P_BMM_INTERFACE` as implicit (in which case every consumer must infer it, and inferring is not second-guessing) or as an oversight in these six declarations. That is a question for the openEHR specifications repository, not for this tree.

**Resolution form:** an upstream issue first; then either an ADR widening the abstractness question, or a documented acceptance that the flag is the whole answer. Until then the interim rule is REQ-048's — its § *Abstract is the BMM's flag, not a storability verdict* forbids pre-empting this strand in code.

**Affects:** REQ-048, and REQ-041's divergence-reporting duty.

---

## STRAND-13 — Properties inherited from a primitive-mapped ancestor are dropped

**Status:** Open.

**Question:** when a `class_definitions` class inherits a property from a `primitive_types` ancestor that [§ Primitive type mapping](reference-model.md#primitive-type-mapping) maps to a Go primitive, should the generator fold that property in — into the emitted struct, into the `rminfo` attribute tables, or into neither?

**Why it's open:** the generator plans `primitive_types` entries mapped to a Go primitive as primitives, not as classes, so it never walks their properties. On the pinned schemas exactly one class is affected: `Iso8601_timezone` (a `class_definitions` entry, and so in REQ-048's class universe) inherits `value: String` — **mandatory** — from `Iso8601_type`, which maps to Go `string`. The result is consistent between the two SDK surfaces and inconsistent with the BMM: `rm.ISO8601Timezone` is emitted as an **empty struct**, and `rminfo` reports the class as carrying no attributes. The type cannot hold its own value, and nothing flagged it until PROBE-094 compared the shipped attribute sets against an independent BMM reduction.

This is a pre-existing emission gap, not a REQ-048 regression — REQ-048 only made it visible, by removing the attribute-less-class skip that had kept `Iso8601_timezone` out of the table entirely.

**The trade-off:**

- **Fold it into both surfaces** — `rm.ISO8601Timezone` gains a `Value string` field and `rminfo` reports the attribute. Faithful to the BMM, and the type becomes able to hold the value it is defined by. Against it: it changes a generated public struct, and the same rule would then apply to every primitive-mapped ancestor, whose blast radius across the base schema is unmeasured.
- **Fold it into `rminfo` only** — the tables become BMM-faithful while the struct stays empty. Against it: `rminfo` would then describe an attribute no Go type can hold, and the REQ-112 validation floor would demand a field that does not exist. Strictly worse than either consistent option.
- **Leave both as they are** and treat these classes as deliberately value-less in Go, on the grounds that the four ISO 8601 value types are already mapped to `string` and this one is reached through them. Against it: it leaves a mandatory BMM attribute unrepresentable and undocumented outside a test pin.

**Evidence needed:** the census this strand cannot assume — every `class_definitions` class with a primitive-mapped ancestor that declares properties, across all primary schemas, not just the one the pinned RM reduction surfaces. Whether `Iso8601_timezone` is a one-off or the visible member of a family decides which option is affordable.

**Evidence (census):** a census test in the generator walks every `class_definitions` class in each of the eight schema roots, the six pinned ones (`base 1.3.0`, `rm 1.2.0`, `am 1.4.0`, `am 2.4.0`, `lang 1.1.0`, `term 3.1.0`) and the two the SDK authors, and lists each property reachable only through a primitive-mapped ancestor — a property the class or a non-primitive ancestor declares itself is planned and shipped, so it does not count. Result: **exactly one** — `Iso8601_timezone.value` via `Iso8601_type`, present in the seven roots that include `base` (`term 3.1.0` includes no base schema and has no ISO 8601 classes). The four DV temporal types (`DV_DATE`, `DV_TIME`, `DV_DATE_TIME`, `DV_DURATION`) also descend from `Iso8601_type` but redeclare `value` on the class, which is why PROBE-094's reduction never flagged them and why the generated structs carry it. The set is pinned by the test, so growth is loud, and the evidence is guarded three ways: the root list is checked against the schemas actually vendored under `resources/bmm`, so a newly vendored schema cannot pass unwalked; the census logic runs on hand-built schemas covering transitive, shadowed, secondary and cyclic ancestry, so a traversal or filter regression cannot silently empty or inflate the result; and the four DV temporal types are pinned as excluded *because* they redeclare `value`, not because they are absent. What this settles: `Iso8601_timezone` is a one-off, not the visible member of a family — the fold-into-both option's blast radius is one generated struct and one `rminfo` row. What it does not settle: the fold itself, which remains an ADR amending § Mapping rules plus a regenerated tree; the strand stays open.

**Resolution form:** an ADR amending the § Mapping rules inheritance rule, plus a regenerated tree, if either folding option wins; otherwise a documented mapping exemption. Until then the interim prohibition is [REQ-048 § The attribute tables are complete against the BMM](reference-model.md#the-attribute-tables-are-complete-against-the-bmm)'s: the drop is pinned by name, cannot grow silently, and folding it in ahead of this strand fails [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm)'s completeness arm (`unshippedProperties`).

**Affects:** REQ-041, REQ-048.

---

## STRAND-14 — Should template-driven validation also run the RM-floor invariants?

**Status:** Resolved by [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md).

**Decision:** Template-driven validation includes the RM-floor invariants, each defect reported once, with no template-only option.

**Codified in:** [clinical-modeling.md § REQ-102](clinical-modeling.md#req-102--composition-validation) and [§ REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor).

---

## STRAND-15 — Decode-time presence and unmatched-member capture on the generated types

**Status:** Partly resolved 2026-10-26: the presence question by [ADR 0033](../adr/0033-missing-members-reported-by-decoders.md), which puts the record beside the value, not on it: each codec reports the members a document leaves out ([wire.md § REQ-175](wire.md#req-175--missing-members-on-decode)) and the validators read that report ([clinical-modeling.md § REQ-112 *Value-typed mandatory presence*](clinical-modeling.md#value-typed-mandatory-presence)), so `ValidateRMEHRStatusBytes` is deprecated. The report of the members a canonical-XML document sends without a value ([wire.md § REQ-175 *Sent without a value*](wire.md#req-175--missing-members-on-decode), [issue 353](https://github.com/Cadasto/openehr-sdk-go/issues/353)) is a report beside the value in the same way, not a capture, and leaves the capture question where it is. The capture question, whether unmatched members are kept on the value for a lossless round trip, stays open, with the evidence below still to gather. Opened 2026-10-13, after [PR 256](https://github.com/Cadasto/openehr-sdk-go/pull/256).

**Question:** do the generated RM types record, at decode, which by-value mandatory members the input left out, and the members the decoder matched to nothing? And when they capture unmatched members, are those kept on the value so a read, modify and write round trip is lossless, or discarded once reported?

**Why it is open:** two costs of today's design meet here. [REQ-170](wire.md#req-170--unmatched-content-on-decode)'s report and strict options for canonical JSON read the bytes again after the decode to compare each member name with the class's attribute list, in one pass when every object's `_type` is its first member and otherwise in two, the first to find every `_type`, and that name pass links the RM attribute tables into every program that imports `canjson` (measured at 339 KB in the PR 256 review). And a by-value mandatory attribute, `EHR_STATUS.subject` the case in point, could not be told absent from zero after the decode, which is why `ValidateRMEHRStatusBytes` was added ([REQ-112 § Value-typed mandatory presence](clinical-modeling.md#value-typed-mandatory-presence)) and why no other root had such an entry until ADR 0033. The standard library can do both in one pass: `encoding/json/v2`'s embedded fallback (the `embed` option on a `jsontext.Value` or `map[string]T` field) receives every member the struct does not match, and the generated decoders know, as they read, which members they saw.

**The trade-off:**

- **Capture and discard.** The report option reads the fallback instead of the bytes, the second and third read go, and the value stays as REQ-170 defines it: no unmatched content anywhere in it. Against it: a producer's extension members are still lost on a round trip, which is the interoperability case the capture could serve.
- **Capture and keep.** The value carries the captured members and the encoder writes them back, so a read, modify and write is lossless. Against it: it reverses REQ-170's rule that a decoded value holds no unmatched content, touches [REQ-052](wire.md#req-052)'s expected form, and puts one more field on every generated class that the validators, the builder and the generator then have to leave alone.
- **Presence as a record of absence.** A record set only by a decoder, and only for a by-value mandatory member the input left out, lets the floor report the omission on a decoded value and leaves a value built in Go, which has no record, read as present, as today. A record of presence instead would make every Go-built value report its by-value mandatory members absent. The direction of the record is part of the question.
- **The other codecs.** Canonical XML already streams through the generated decoders and a runtime, so a presence record is cheap there; FLAT and STRUCTURED resolve keys against a Web Template, and a mistyped key there is indistinguishable from an extension and vanishes before any validator sees the value. On a write path the report is therefore needed beside the validation result whatever the answer here.

**Evidence needed:** the size and decode cost of one fallback field per class across the 110 registered classes, on the `canjson` benchmarks; the link-size saving once the name pass goes; whether any consumer needs the lossless round trip; and the shape of the absence record that costs least on a decoded composition.

**Work a resolution sets off:** if the generated types capture and record as the options above describe, the generator emits the fallback field and the record on each generated class. Canonical JSON's report and strict options then read the fallback instead of reading the bytes again, so the type pass and the name pass go, and `internal/rmjsonwalk` with them unless `testkit/expectedform` still needs it. The presence half is done: [ADR 0033](../adr/0033-missing-members-reported-by-decoders.md) has each codec report the members a document leaves out ([wire.md § REQ-175](wire.md#req-175--missing-members-on-decode)), the validators read that report and decode and validate in one call, and `ValidateRMEHRStatusBytes` is deprecated; a capture on the generated types would feed that report in place of the second read. FLAT resolves its keys against a Web Template and stays outside it. The `canjson` decode benchmarks with the fallback field present are held to the [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) budget line.

**A larger option, for later:** generated token-level JSON decoders, like the generated decoders canonical XML already streams through, would validate while reading, with presence and unmatched members known at no extra cost and no reliance on the library's hooks. It is the largest step, and it is weighed only after an analysis note that sizes the generated code, measures a prototype on the `canjson` benchmarks against the fallback-field result, and recommends it or not. No requirement and no code come before that note.

**Resolution form:** the presence rule is settled (ADR 0033, REQ-175, REQ-112). What remains, for the capture question, is an ADR amending REQ-170 and REQ-052 and a new requirement for the capture contract, allocated when that ADR lands.

**Affects:** REQ-040, REQ-052, REQ-112, REQ-170, REQ-175.

---

## STRAND-16 — Validator policy beyond the limits

**Status:** Open.

**Question:** which settings, beyond the depth and visit limits, does the validator of [REQ-171](clinical-modeling.md#req-171--validation-engine) take as options, and which stay outside the SDK as the caller's policy over a result?

Three candidates are named and none is built:

1. **Descent under refused content.** Since REQ-171's *One walk* binds, content the template does not admit is reported once and not entered. An authoring tool that shows a form in progress may want the full detail under a refused node anyway. The default is to stop; whether an option may ask for the descent, and whether its findings carry a mark that tells them from a normal result, is open.
2. **A severity policy per rule.** A server that applies a policy to template findings (warn) and another to RM findings (reject) can do so from the `Layer` each finding carries. Whether that is enough, or whether a policy per rule inside the SDK (this rule warns, that one rejects) is wanted so the result's `OK` already reflects it, is open. RM validation stays on whatever the answer: the policy chooses what to do with a finding, never whether to look.
3. **A partial-instance mode.** A form in progress holds values on some nodes and nothing yet on others. An option that keeps every value check and suppresses the presence and lower-bound count findings of both layers would serve that case without any switch that turns the RM rules off, which [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md) refuses. It is additive; what it means for `Result.OK` and `Coverage` is the open part.

**Why it is open:** each candidate is additive on REQ-171's options and cheap to add, and each changes what a `Result` means to the caller who reads it. None has a consumer asking for it yet except the warn-or-reject policy, which a server that validates on commit needs in some form. Adding an option nobody uses is the second kind of waste ADR 0025 named.

**Evidence needed:** a consumer for each candidate, with the result shape it needs; for the partial mode, whether the generator's `Large` skeleton and the composition builder's intermediate states are the instances it would serve.

**Resolution form:** each candidate resolves on its own, by an amendment to REQ-171's *Surface* and *Coverage* with the option's contract; an ADR only where an option changes what an existing result means.

**Affects:** REQ-102, REQ-171.

---

## STRAND-17 — The OPT parser and the generated operational template model

**Status:** Resolved by [ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md).

**Decision:** The compile step reads the generated operational template model, which is the SDK's one OPT reader, and path lookup runs over the compiled template.

**Codified in:** [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) and [§ REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model).

---

## Index

| Strand | Title | Status | Affects |
|---|---|---|---|
| [STRAND-01](#strand-01--extraction-from-reference-cdr) | Extraction from reference CDR | **Resolved** | REQ-013, [Boundary rules](module-layout.md#boundary-rules) |
| [STRAND-02](#strand-02--shared-contract-source-of-truth-php--go) | Shared contract source-of-truth | **Cancelled** | — |
| [STRAND-03](#strand-03--go-idiomatic-surface-validation) | Go-idiomatic surface | **Resolved** | REQ-021, [API conventions](idiom.md#api-conventions) |
| [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) | RM polymorphism + codec perf | **Partially resolved** | [Generics and reflection](idiom.md#generics-and-reflection), REQ-040, REQ-052..053 |
| [STRAND-05](#strand-05--smart-on-openehr-auth-library) | SMART-on-openEHR auth library | **Resolved** ([ADR 0009](../adr/0009-smart-auth-library-scope.md)) | REQ-061..064 |
| [STRAND-06](#strand-06--concurrency-and-transport-hygiene) | Concurrency / transport hygiene | Open | REQ-021, REQ-026 |
| [STRAND-07](#strand-07--versioning-and-module-path) | Versioning + module path | **Resolved** | REQ-001, REQ-004 |
| [STRAND-08](#strand-08--cadasto-extras-boundary-criteria-conditional-extraction) | Cadasto-extras extraction | Open (long-term) | [Boundary rules](module-layout.md#boundary-rules) |
| [STRAND-09](#strand-09--its-rest-conformance-follow-ups) | ITS-REST conformance follow-ups (REST probes; stored-query paging) | **Resolved** ([ADR 0026](../adr/0026-stored-query-paging-fields-only-when-set.md)) | REQ-057, REQ-059, REQ-050, REQ-099 |
| [STRAND-10](#strand-10--rmpath-one-sentinel-for-two-not-found-conditions) | rmpath not-found sentinel split | Open | REQ-053, REQ-121 |
| [STRAND-11](#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) | Probe recording format (HAR vs YAML) | **Resolved** ([ADR 0020](../adr/0020-cassette-recording-har.md)) | REQ-082 |
| [STRAND-12](#strand-12--bmm-interface-classes-carry-no-is_abstract-flag) | BMM interface classes carry no `is_abstract` | Open | REQ-041, REQ-048 |
| [STRAND-13](#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) | Properties inherited from a primitive-mapped ancestor are dropped | Open | REQ-041, REQ-048 |
| [STRAND-14](#strand-14--should-template-driven-validation-also-run-the-rm-floor-invariants) | Template-driven validation and the RM floor | **Resolved** ([ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md)) | REQ-102, REQ-112 |
| [STRAND-15](#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) | Decode-time presence and unmatched-member capture | Partly resolved (presence: [ADR 0033](../adr/0033-missing-members-reported-by-decoders.md); capture open) | REQ-040, REQ-052, REQ-112, REQ-170, REQ-175 |
| [STRAND-16](#strand-16--validator-policy-beyond-the-limits) | Validator policy beyond the limits | Open | REQ-102, REQ-171 |
| [STRAND-17](#strand-17--the-opt-parser-and-the-generated-operational-template-model) | The OPT parser and the generated OPT model | **Resolved** ([ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md)) | REQ-013, REQ-100, REQ-103, REQ-107, REQ-108, REQ-111, REQ-176 |
