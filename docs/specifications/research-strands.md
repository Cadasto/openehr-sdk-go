---
kind: specification
---

# Research strands

**Status:** Draft

Open architectural questions that are **not yet decided** but are scoped, named, and tracked.
Each strand resolves into an ADR (under `../adr/`) and amends one or more REQs in [REQ.md](REQ.md).

A strand is not a `Draft` REQ — it is an open question whose answer cannot be predicted by reading the spec.
Resolving a strand happens by:

1. Producing evidence (a spike, a benchmark, a fit-gap analysis).
2. Writing an ADR proposing the resolution.
3. Amending the affected REQs in this tree.
4. Closing the strand here with a backlink to the ADR.

Strand IDs (`STRAND-NN`) are stable.
Renumbering is prohibited.

---

## STRAND-01 — Extraction from reference CDR

Resolved without an ADR: the SDK is an independent module whose surface the specs define; codified in REQ-013 and [module-layout.md § Boundary rules](module-layout.md#boundary-rules).

---

## STRAND-02 — Shared contract source-of-truth (PHP ↔ Go)

Cancelled: the SDK is hand-written and needs no shared contract source, since the probe suite ([conformance.md](conformance.md)) holds its wire conformance.

---

## STRAND-03 — Go-idiomatic surface validation

Resolved without an ADR: package-level functions are the primary surface; codified in REQ-021 and [idiom.md § API conventions](idiom.md#api-conventions).

---

## STRAND-04 — RM polymorphism and codec performance

**Status:** Partially resolved.

[ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md) settles the codec and holds its performance budget, which also decides numeric tolerance and member order, with [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4 for the `EVENT` interface; codified in [reference-model.md § Type registry (REQ-040)](reference-model.md#type-registry-req-040) and [wire.md § REQ-052](wire.md#req-052).

**Still open:**

- **Full RM inventory:** decode every BMM type through the registry, and find the sites that resist the pattern, such as an abstract generic class beyond `EVENT` that needs a whitelist decision (an ADR).
- **Codec performance:** whether the `encoding/json/v2` path stays within ADR 0022's budget for seeder and benchmark workloads; a regression is a plan task, not a codec fork.
- **Memory residency** of the `encoding/json/v2` path against the retired generated marshalers.

**Resolution form:** the inventory and performance items resolve through benchmarks and a registry census, not a new ADR.

---

## STRAND-05 — SMART-on-openEHR auth library

Resolved by [ADR 0009](../adr/0009-smart-auth-library-scope.md); codified in [auth.md](auth.md).

---

## STRAND-06 — Concurrency and transport hygiene

**Question:** The federator constructs multiple SDK clients.
Do they share a single `*http.Transport` (for connection-pool efficiency across nodes) or own independent transports (for cleaner failure isolation per node)?

**Why it's open:** REQ-021 says "inject your `*http.Client`" — but it does not prescribe sharing.
Both approaches are defensible; the trade-off is connection-pool reuse vs blast-radius isolation.

**Evidence needed:**

- A federator-style spike running 4+ clients with both shared and independent transports.
- Measure: connection-establishment latency (cold), pool-exhaustion behaviour under load, failure isolation when one node is slow / unreachable.

**Resolution form:** ADR-NNN documenting the recommendation and the override path.
Amends REQ-021 if the default guidance becomes opinionated.

**Implementation gate:** a federator implementation makes this decision real.

---

## STRAND-07 — Versioning and module path

Resolved without an ADR; codified in [module-layout.md § REQ-001](module-layout.md#req-001--module-path), [§ REQ-004](module-layout.md#req-004--semantic-versioning) and [§ The `internal/` boundary](module-layout.md#the-internal-boundary).

---

## STRAND-08 — Cadasto extras: boundary, criteria, conditional extraction

**Question:** Will `cadasto/…` ever be extracted into a sibling Go module?
If so, when, and what triggers the decision?

**Why it's open:** the openEHR-core part of this SDK could in principle be vendor-neutral (target EHRbase, other openEHR backends).
The Cadasto extras are platform-specific.
Whether keeping them in one module or splitting them is right depends on adoption, governance, and cross-backend demand.

**Criteria** (none decisive on its own; jointly assessed):

- **Conceptual.** Is the surface an openEHR concept or a Cadasto-specific concept?
- **Technical.** Does it share types and lifecycle with openEHR core, or is it thinly layered on top?
- **Audience.** Is it needed by every consumer, or by a subset (integration developers using Admin / Datamap)?
- **Governance.** Same release cadence as openEHR core, or faster-churn Cadasto surfaces?
- **Cross-backend demand.** Concrete demand for the openEHR-core to work against EHRbase or another non-Cadasto backend.

**Evidence needed:**

- Adoption data once the Cadasto extras ship.
- A concrete consumer asking for an EHRbase-compatible build.

**Resolution form:** ADR-NNN either confirming "keep together" or extracting.
If extracting: package moves, module path for the sibling, semver implications for both modules.

**Open until:** v1 is in production for at least one minor release.
Premature resolution is worse than no resolution.

**Boundary held in v1** (regardless of resolution): the [module-layout.md § Boundary rules](module-layout.md#boundary-rules) hold the cut line so an extraction would be mechanical, not a rewrite.

---

## STRAND-09 — ITS-REST conformance follow-ups

Resolved by the REST probes PROBE-062, PROBE-102, PROBE-103 and PROBE-104, and by [wire.md § REQ-057](wire.md#req-057) *Paging* for the stored-query paging fields; codified in [conformance.md](conformance.md).

---

## STRAND-10 — rmpath: one sentinel for two not-found conditions

**Question:** does `rmpath.ItemAtPath` need to tell *attribute unknown to the navigator* from *attribute known but unpopulated*, instead of returning `ErrPathNotFound` for both?

**Why it's open:** the FLAT encoder treats resolution failure as an absent optional (`skipNotFound`), which is correct for the unpopulated case and silent data loss for the unknown-attribute case, the defect class PROBE-086 surfaced.
Because the encoder cannot tell the two conditions apart, the coverage rule in [wire.md § Web Template leaf coverage](wire.md#web-template-leaf-coverage) is enforced by a hand-kept guard test and exemption list rather than derived from the API.

**Resolution options:** a distinct sentinel (e.g. `ErrUnknownAttribute`) letting `skipNotFound` fail loudly on unmodelled attributes and retiring the exemption list; or keeping the single sentinel and the guard-test enforcement.
The first adds an exported error to a landed surface ([idiom.md § API conventions](idiom.md#api-conventions) compatibility question); the second keeps the hazard class alive but contained.

**Evidence needed:** whether the exemption list churns (each churn is an argument for the sentinel, and it has churned once); whether any consumer outside the encoder needs the distinction.

**Resolution form:** ADR-NNN; [wire.md § Web Template leaf coverage](wire.md#web-template-leaf-coverage) and the guard-test exemption mechanism are updated together with it.

---

## STRAND-11 — Probe recording format: HAR or a purpose-built YAML

Resolved by [ADR 0020](../adr/0020-cassette-recording-har.md); codified in [conformance.md § Cassette mode](conformance.md#cassette-mode).

---

## STRAND-12 — BMM interface classes carry no `is_abstract` flag

**Question:** is the RM meta-model introspection surface ([REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface)) right to report a `P_BMM_INTERFACE` class as abstract, even though the pinned BMM sets no `is_abstract` flag on it?
And is the missing flag an upstream BMM defect for this SDK to raise under [REQ-041](reference-model.md#req-041--pinned-bmm-sources)?

**Why it's open:** six classes in REQ-048's universe carry no `is_abstract` flag: `MEASUREMENT_SERVICE`, `TERMINOLOGY_SERVICE`, `OPENEHR_CODE_SET_IDENTIFIERS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS`, and — the sharp cases — `CODE_SET_ACCESS` and `TERMINOLOGY_ACCESS`, which are `P_BMM_INTERFACE` declarations the generator renders as Go **interfaces**.
Nothing can instantiate a Go interface, so the surface reports "not abstract" for a class that is unquestionably not instantiable, and a concrete-descendant expansion hands it back.
REQ-041 settles the immediate behaviour — the BMM wins, the SDK reports the flag verbatim, and REQ-048 says so normatively — but it does not settle whether the *question* is the right one.

**The trade-off:**

- **Report the flag only** (today's behaviour).
  One rule, one source, no local model of instantiability; a caller needing the decodable set uses the [REQ-040 registry](reference-model.md#type-registry-req-040), which is the authority for what Go can construct.
  Against it: a caller who reads "not abstract" as "instantiable" is misled on six classes, and the surface offers no way to notice.
- **Widen abstractness to cover BMM interfaces** — report `is_abstract || _type == P_BMM_INTERFACE`.
  Matches what the generator actually emits and what a caller means.
  Against it: it is the SDK second-guessing the BMM, which REQ-041 forbids as a general practice precisely because local corrections diverge silently from every other openEHR implementation reading the same file.
- **Raise it upstream and keep reporting the flag** until the schema changes.
  Correct in the long run and costs nothing locally; the timescale is not ours to set.

**Evidence needed:** whether the openEHR BMM treats a missing `is_abstract` on `P_BMM_INTERFACE` as implicit (in which case every consumer infers it, and inferring is not second-guessing) or as an oversight in these six declarations.
That is a question for the openEHR specifications repository, not for this tree.

**Resolution form:** an upstream issue first; then either an ADR widening the abstractness question, or a documented acceptance that the flag is the whole answer.
Until then the interim rule is REQ-048's — its § *Abstract is the BMM's flag, not a storability verdict* forbids pre-empting this strand in code.

**Affects:** REQ-048, and REQ-041's divergence-reporting duty.

---

## STRAND-13 — Properties inherited from a primitive-mapped ancestor are dropped

**Question:** when a `class_definitions` class inherits a property from a `primitive_types` ancestor that [§ Primitive type mapping](reference-model.md#primitive-type-mapping) maps to a Go primitive, where does the generator fold that property in: into the emitted struct, into the `rminfo` attribute tables, or into neither?

**Why it's open:** the generator plans `primitive_types` entries mapped to a Go primitive as primitives, not as classes, so it never walks their properties.
On the pinned schemas exactly one class is affected: `Iso8601_timezone` (a `class_definitions` entry, and so in REQ-048's class universe) inherits `value: String` — **mandatory** — from `Iso8601_type`, which maps to Go `string`.
The result is consistent between the two SDK surfaces and inconsistent with the BMM: `rm.ISO8601Timezone` is emitted as an **empty struct**, and `rminfo` reports the class as carrying no attributes.
The type cannot hold its own value.

**The trade-off:**

- **Fold it into both surfaces** — `rm.ISO8601Timezone` gains a `Value string` field and `rminfo` reports the attribute.
  Faithful to the BMM, and the type becomes able to hold the value it is defined by.
  Against it: it changes a generated public struct, and the same rule would then apply to every primitive-mapped ancestor, whose blast radius across the base schema is unmeasured.
- **Fold it into `rminfo` only** — the tables become BMM-faithful while the struct stays empty.
  Against it: `rminfo` would then describe an attribute no Go type can hold, and the REQ-112 validation floor would demand a field that does not exist.
  Strictly worse than either consistent option.
- **Leave both as they are** and treat these classes as deliberately value-less in Go, on the grounds that the four ISO 8601 value types are already mapped to `string` and this one is reached through them.
  Against it: it leaves a mandatory BMM attribute unrepresentable and undocumented outside a test pin.

**Evidence:** a census test in the generator walks every `class_definitions` class of every vendored schema root and finds exactly one property reachable only through a primitive-mapped ancestor, `Iso8601_timezone.value`; the test pins that set, so growth is loud.
The four DV temporal types also descend from `Iso8601_type` but redeclare `value` on the class, so the generated structs carry it.
`Iso8601_timezone` is therefore a one-off, and the fold-into-both option costs one generated struct and one `rminfo` row.
The fold itself stays open.

**Resolution form:** an ADR amending the § Mapping rules inheritance rule, plus a regenerated tree, if either folding option wins; otherwise a documented mapping exemption.
Until then the interim prohibition is [REQ-048 § The attribute tables are complete against the BMM](reference-model.md#the-attribute-tables-are-complete-against-the-bmm)'s: the drop is pinned by name, cannot grow silently, and folding it in ahead of this strand fails [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm)'s completeness arm.

**Affects:** REQ-041, REQ-048.

---

<a id="strand-14"></a>

## STRAND-14 — Should template-driven validation also run the RM-floor invariants?

Resolved by [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md); codified in [clinical-modeling.md § REQ-102](clinical-modeling.md#req-102--composition-validation) and [§ REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor).

---

## STRAND-15 — Decode-time presence and unmatched-member capture on the generated types

The presence half is resolved by [ADR 0033](../adr/0033-missing-members-reported-by-decoders.md): each codec reports the members a document leaves out ([wire.md § REQ-175](wire.md#req-175--missing-members-on-decode)), and the validators read that report ([clinical-modeling.md § REQ-112 *Value-typed mandatory presence*](clinical-modeling.md#value-typed-mandatory-presence)).
The capture question below stays open.

**Question:** do the generated RM types capture, at decode, the members the decoder matched to nothing?
And if they do, are those kept on the value so a read, modify and write round trip is lossless, or discarded once reported?

**Why it's open:** [REQ-170](wire.md#req-170--unmatched-content-on-decode)'s report and strict options for canonical JSON read the bytes again after the decode to compare each member name with the class's attribute list, in one pass when every object's `_type` is its first member and otherwise in two, the first to find every `_type`.
That name pass links the RM attribute tables into every program that imports `canjson`.
The standard library can do it in one pass: `encoding/json/v2`'s embedded fallback (the `embed` option on a `jsontext.Value` or `map[string]T` field) receives every member the struct does not match.

**The trade-off:**

- **Capture and discard.** The report option reads the fallback instead of the bytes, the second and third read go, and the value stays as REQ-170 defines it: no unmatched content anywhere in it.
  Against it: a producer's extension members are still lost on a round trip, which is the interoperability case the capture could serve.
- **Capture and keep.** The value carries the captured members and the encoder writes them back, so a read, modify and write is lossless.
  Against it: it reverses REQ-170's rule that a decoded value holds no unmatched content, touches [REQ-052](wire.md#req-052)'s expected form, and puts one more field on every generated class that the validators, the builder and the generator then have to leave alone.
- **The other codecs.** FLAT and STRUCTURED resolve keys against a Web Template, and a mistyped key there is indistinguishable from an extension and vanishes before any validator sees the value.
  On a write path the report is therefore needed beside the validation result whatever the answer here.

**Evidence needed:** the size and decode cost of one fallback field per registered class, on the `canjson` benchmarks; the link-size saving once the name pass goes; and whether any consumer needs the lossless round trip.

**Work a resolution sets off:** if the generated types capture as the options above describe, the generator emits the fallback field on each generated class.
Canonical JSON's report and strict options then read the fallback instead of reading the bytes again, so the type pass and the name pass go, and `internal/rmjsonwalk` with them unless `testkit/expectedform` still needs it.
A capture on the generated types would feed the missing-member report in place of the second read.
FLAT resolves its keys against a Web Template and stays outside it.
The `canjson` decode benchmarks with the fallback field present are held to the codec performance budget of [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md).

**A larger option, for later:** generated token-level JSON decoders, like the generated decoders canonical XML already streams through, would validate while reading, with presence and unmatched members known at no extra cost and no reliance on the library's hooks.
It is the largest step, and it is weighed only after an analysis note that sizes the generated code, measures a prototype on the `canjson` benchmarks against the fallback-field result, and recommends it or not.
No requirement and no code come before that note.

**Resolution form:** an ADR amending REQ-170 and REQ-052, and a new requirement for the capture contract, allocated when that ADR lands.

**Affects:** REQ-040, REQ-052, REQ-112, REQ-170, REQ-175.

---

## STRAND-16 — Validator policy beyond the limits

**Question:** which settings, beyond the depth and visit limits, does the validator of [REQ-171](clinical-modeling.md#req-171--validation-engine) take as options, and which stay outside the SDK as the caller's policy over a result?

Three candidates are named and none is built:

1. **Descent under refused content.** Since REQ-171's *One walk* binds, content the template does not admit is reported once and not entered.
   An authoring tool that shows a form in progress may want the full detail under a refused node anyway.
   The default is to stop; whether an option may ask for the descent, and whether its findings carry a mark that tells them from a normal result, is open.
2. **A severity policy per rule.** A server that applies a policy to template findings (warn) and another to RM findings (reject) can do so from the `Layer` each finding carries.
   Whether that is enough, or whether a policy per rule inside the SDK (this rule warns, that one rejects) is wanted so the result's `OK` already reflects it, is open.
   RM validation stays on whatever the answer: the policy chooses what to do with a finding, never whether to look.
3. **A partial-instance mode.** A form in progress holds values on some nodes and nothing yet on others.
   An option that keeps every value check and suppresses the presence and lower-bound count findings of both layers would serve that case without any switch that turns the RM rules off, which [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md) refuses.
   It is additive; what it means for `Result.OK` and `Coverage` is the open part.

**Why it's open:** each candidate is additive on REQ-171's options and cheap to add, and each changes what a `Result` means to the caller who reads it.
None has a consumer asking for it yet except the warn-or-reject policy, which a server that validates on commit needs in some form.
Adding an option nobody uses is the second kind of waste ADR 0025 named.

**Evidence needed:** a consumer for each candidate, with the result shape it needs; for the partial mode, whether the generator's `Large` skeleton and the composition builder's intermediate states are the instances it would serve.

**Resolution form:** each candidate resolves on its own, by an amendment to REQ-171's *Surface* and *Coverage* with the option's contract; an ADR only where an option changes what an existing result means.

**Affects:** REQ-102, REQ-171.

---

## STRAND-17 — The OPT parser and the generated operational template model

Resolved by [ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md); codified in [clinical-modeling.md § REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) and [§ REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model).

---

## Index

| Strand | Title | Status | Affects |
|---|---|---|---|
| [STRAND-01](#strand-01--extraction-from-reference-cdr) | Extraction from reference CDR | Resolved | REQ-013, [Boundary rules](module-layout.md#boundary-rules) |
| [STRAND-02](#strand-02--shared-contract-source-of-truth-php--go) | Shared contract source-of-truth | Cancelled | — |
| [STRAND-03](#strand-03--go-idiomatic-surface-validation) | Go-idiomatic surface | Resolved | REQ-021, [API conventions](idiom.md#api-conventions) |
| [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) | RM polymorphism + codec perf | Partially resolved | [Generics and reflection](idiom.md#generics-and-reflection), REQ-040, REQ-052..053 |
| [STRAND-05](#strand-05--smart-on-openehr-auth-library) | SMART-on-openEHR auth library | Resolved | REQ-061..064 |
| [STRAND-06](#strand-06--concurrency-and-transport-hygiene) | Concurrency / transport hygiene | Open | REQ-021, REQ-026 |
| [STRAND-07](#strand-07--versioning-and-module-path) | Versioning + module path | Resolved | REQ-001, REQ-004 |
| [STRAND-08](#strand-08--cadasto-extras-boundary-criteria-conditional-extraction) | Cadasto-extras extraction | Open (long-term) | [Boundary rules](module-layout.md#boundary-rules) |
| [STRAND-09](#strand-09--its-rest-conformance-follow-ups) | ITS-REST conformance follow-ups | Resolved | REQ-057, REQ-059, REQ-050, REQ-099 |
| [STRAND-10](#strand-10--rmpath-one-sentinel-for-two-not-found-conditions) | rmpath not-found sentinel split | Open | REQ-053, REQ-121 |
| [STRAND-11](#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) | Probe recording format (HAR vs YAML) | Resolved | REQ-082 |
| [STRAND-12](#strand-12--bmm-interface-classes-carry-no-is_abstract-flag) | BMM interface classes carry no `is_abstract` | Open | REQ-041, REQ-048 |
| [STRAND-13](#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped) | Properties inherited from a primitive-mapped ancestor are dropped | Open | REQ-041, REQ-048 |
| [STRAND-14](#strand-14) | Template-driven validation and the RM floor | Resolved | REQ-102, REQ-112 |
| [STRAND-15](#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) | Decode-time presence and unmatched-member capture | Partially resolved | REQ-040, REQ-052, REQ-112, REQ-170, REQ-175 |
| [STRAND-16](#strand-16--validator-policy-beyond-the-limits) | Validator policy beyond the limits | Open | REQ-102, REQ-171 |
| [STRAND-17](#strand-17--the-opt-parser-and-the-generated-operational-template-model) | The OPT parser and the generated OPT model | Resolved | REQ-013, REQ-100, REQ-103, REQ-107, REQ-108, REQ-111, REQ-176 |
