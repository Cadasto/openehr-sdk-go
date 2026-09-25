# Plan: canonical JSON on `encoding/json/v2`

**Date:** 2026-09-14
**Status:** landed (2026-09-14, archived in the implementing PR). ADR 0021 and ADR 0022 were accepted 2026-09-14 (maintainer sign-off, ruling R3), and Phases 0 through 4 all landed on this branch across ten tasks; the open-question, results and rulings tables below are filled in from those tasks' reports.
**Owner:** SDK maintainers
**Covers:** [REQ-052](../../specifications/wire.md#req-052) (Canonical JSON, Impl. `landed`, amended in place), [REQ-040](../../specifications/rm-modeling.md#type-registry-req-040) (Type registry, Impl. `landed`, dispatch mechanism changes, contract does not), [REQ-053](../../specifications/wire.md#req-053) (FLAT and STRUCTURED, Impl. `landed`, one test rewrite only). [REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure) is deliberately **not** listed: `transport.Decode` already routes every 2xx body through `canjson.Unmarshal` (`transport/client.go:654`) and keeps doing so, so no package, probe or test row moves in `traceability.yaml`; only the identity of the wrapped cause changes, and Phase 3 re-pins it.
**Probes:** [PROBE-030](../../specifications/conformance.md#probe-030--canonical-json-round-trip) and [PROBE-038](../../specifications/conformance.md#probe-038--rm-polymorphic-decode-coverage) are **rewritten, not new**. No new `PROBE-NNN` id is allocated, so the guarded census sentence at `docs/specifications/conformance.md:44` and the all-three-modes tally at `:46` are untouched. PROBE-089's catalog prose gets a one-line wording fix (`conformance.md:569`). PROBE-031 (unknown `_type`) and PROBE-033 (canonical XML) are unchanged.
**Implementation:** landed
**Depends on:** the Go 1.27.0 module floor (`go.mod:3`), which puts `encoding/json/v2` and `encoding/json/jsontext` in the ordinary standard library; the landed `openehr/serialize/canjson`, `openehr/rm/typereg` and generated RM and AOM trees; [ADR 0002](../../adr/0002-bmm-codegen-decisions.md) (codegen policy, amended by this plan), [ADR 0003](../../adr/0003-rm-event-polymorphism.md) and [ADR 0004](../../adr/0004-numeric-wire-tolerance.md) (both unchanged)
**Defers:** `openehr/serialize/simplified` and the `testkit` decoders that share its reliance on `Decoder.UseNumber` (`flat_decode.go:708-709`, `datatypes.go:787-788`, `testkit/conformance/webtemplate/runner.go:438-439`, `testkit/probes/serialize/probe_076_simplified_round_trip.go:184-185`, `probe_038_canjson_rm_polymorphic_decode.go:98-99`). Trigger: a `UseNumber` replacement with its own tests, plus `json.Deterministic(true)` for the `map[string]any` encode half (`flat_encode.go:29`, `structured.go:33,47,64,77`). Also deferred: the `Extras` carriers (`openehr/client/definition`, `openehr/client/system`, `openehr/aql`), `openehr/template/webtemplate`, `openehr/bmm`, the `auth/*` and `smart/*` packages, `openehr/validation` and `cmd/*`, each with its reason and trigger in Phase 3. Reclassifying the rewritten PROBE-030 to `In-repo` is a REQ-082 question and stays a follow-up, because it would move the guarded count at `conformance.md:44` from 16 to 17.

## Goal

Canonical JSON stops being produced by generated per-type marshalers and is produced by `encoding/json/v2` instead. The generator emits the streaming pair `MarshalJSONTo` / `UnmarshalJSONFrom` per concrete class, delegating to a method-free alias type whose first field is `_type`, plus one `json.UnmarshalFromFunc` per polymorphic interface; `openehr/internal/jsonpoly` is deleted because v2 calls a pointer-receiver marshaler on a non-addressable value. `openehr/serialize/canjson` keeps its public API and its three sentinels, now backed by v2 options. REQ-052's encode profile stops being a MUST: `_type` first becomes a SHOULD, general member order is withdrawn, and PROBE-030 asserts round-trip fidelity semantically (decoded values equal, the recovered value passes the reference-model floor) rather than by comparing bytes. The consumers are SDK users who decode and encode RM values (any caller that hashes or diffs SDK output loses the byte-stability guarantee; a consumer needing a stable digest computes it over a canonicalisation of its own choosing) and the probe suite, whose round-trip oracle gets stronger.

## Why this is the next step

The generated JSON codec is the largest single block of generated code in the tree, and most of it exists to work around defects `encoding/json/v2` does not have.

| Surface | Count | Lines | Where |
|---|---|---|---|
| `openehr/rm` generated `MarshalJSON` | 110 methods, 29 files | 4 717 | `openehr/rm/*_jsonmar_gen.go` |
| `openehr/rm` generated `UnmarshalJSON` | 110 methods, 29 files | 7 424 | `openehr/rm/*_jsonunmar_gen.go` |
| `openehr/aom/aom14` generated `MarshalJSON` | 29 methods | 947 | `openehr/aom/aom14/*_jsonmar_gen.go` |
| `openehr/aom/aom14` generated `UnmarshalJSON` | 29 methods | 1 421 | `openehr/aom/aom14/*_jsonunmar_gen.go` |
| Generator templates | 2 files | 924 | `internal/bmmgen/render_jsonmar.go` (451), `internal/bmmgen/render_jsonunmar.go` (473) |
| Their generator tests | 2 files | 160 | `internal/bmmgen/render_jsonmar_order_test.go`, `internal/bmmgen/render_jsonunmar_polymorphic_test.go` |
| Interface-boxing helper | 1 package, 177 generated call sites | 214 | `openehr/internal/jsonpoly/jsonpoly.go` (80), `jsonpoly_test.go` (134) |
| **Generated JSON codec total** | **278 methods** | **14 509** | 12 141 of the 31 685 generated `openehr/rm` lines, plus 2 368 in `openehr/aom/aom14` |

The recommended design emits roughly 12 lines per class over 139 classes, about 1 700 generated lines against 14 509, so the migration removes about 12 800 generated lines and the whole of `jsonpoly` (Brief A section 2 and 3). Dispatch collapses in the same ratio: 20 named polymorphic interfaces plus the open generic bound serve 181 slot occurrences today through 278 generated methods, and serve them tomorrow through 20 registered hooks (Brief A section 1b).

What blocked this was never a code question. STRAND-04 named one gate at `docs/specifications/research-strands.md:68`: whether `jsontext` plus marshal options can reproduce byte-stable canonical JSON. That gate rests on a promise the SDK made to itself. `docs/specifications/wire.md:105-106` states the deterministic field-order profile as a MUST and calls it "the SDK's own output contract, which a consumer may rely on for byte comparison and hashing". RFC 8259 section 4 assigns no meaning to JSON object member order, the openEHR specifications prescribe none, and CDR implementations differ in what they emit, so nothing outside the SDK asked for it. The maintainer has withdrawn it (shared decisions 1 to 3), which removes the gate rather than satisfying it, and makes the second decision (migrate the canonical-JSON path to `encoding/json/v2`) a mechanical consequence.

## Rulings where the analysis reports differed

Three read-only analyses fed this plan: a code fit-gap (Brief A), specification and probe deltas (Brief B), and blast radius and phase order (Brief C). Where two of them took different positions, the plan follows the ruling below.

| Point | Positions | Ruling and reason |
|---|---|---|
| Design shape | A recommends the `MarshalJSONTo` / `UnmarshalJSONFrom` pair (its shape (b)); C recommends landing the shallow swap first and taking the streaming pair as a separate later plan | Follow A. The maintainer's fourth decision prefers modern code over kept generation, and the shallow swap keeps 278 methods alive for a second migration. C's two objections are real and become Phase 2 tasks, not reasons to split: `CommitVersion` at `openehr/client/ehr/contribution/submission.go:51` constrains on `json.Marshaler`, which a type implementing only `MarshalJSONTo` no longer satisfies, and `openehr/rm/typereg/nilreceiver_census_test.go:71,93` enumerates `json.Unmarshaler` implementations and would silently cover nothing |
| `Hash` key order | A wants `json.Deterministic(true)` so PROBE-030 does not fail intermittently; B wants the lexicographic rule kept as a MUST, re-homed as encoder determinism; C wants it as the one permanent switch | Since PROBE-030 no longer compares bytes, the intermittency argument lapses. The encoder is deterministic as an implementation property at **SHOULD** force, switched on with `json.Deterministic(true)`, and pinned by a unit test rather than a probe. ADR 0021 lists SHOULD versus MUST as an open point for the maintainer, quoting B's count of six live assertions that rely on one value encoding identically twice |
| Meaning of "wire-equivalent" | B defines it as equality after recursive member sort; C offers `jsontext.Value.Canonicalize` as an alternative mechanism | Define it as equality after parsing both encodings into generic JSON values and comparing those trees. That is insensitive to member order and to string escaping at once, which matters because this plan drops HTML escaping. C's `Canonicalize` route is a valid implementation of the same idea and stays available |
| HTML escaping | A and B both note the byte change; B's fork F3 recommends accepting v2's default; C lists `jsontext.EscapeForHTML(true)` as the restoring option | Accept v2's default (no escaping). `docs/specifications/wire.md:113` already puts the obligation on the decoded character rather than the literal bytes, and with byte goldens re-goldened and wire-equivalence parse-based the choice has no test consequence left. It is a visible byte change, so it goes in the CHANGELOG bullet and in ADR 0022's consequences |
| One ADR or two | B drafted one ADR carrying both decisions and recorded the split as fork F1 | Two. The maintainer took two separate decisions, and one decision per ADR is the convention this repository follows |
| Scope of the migration | A leaves the breadth open (its unknown 7); C fixes it at five packages plus one deletion; B's "hybrid" appears as a rejected alternative | Follow C's split, and record it in ADR 0022 as the decision rather than as a rejected alternative. B's objection applies to a *permanent* split of the decode-error taxonomy, which this is not: every package that decodes RM values moves together |
| Cut-over | C argues against a generator flag | Follow C. Rollback is `git revert` plus `make codegen`, proven by `make codegen-verify`, which `make test` runs first (`Makefile:192`) |
| REQ-052 status | B recommends `landed`, with `partial` only if the spec text merges ahead of the code | Stays `landed`. The amendment is implementation-aligned and lands with the code |

## Definition of Ready

Implementation may start when:

- **`Covers:`** lists every REQ this plan implements: REQ-052, REQ-040, REQ-053. No new REQ id is allocated.
- The REQ-052 amendment text exists, as quoted in Phase 0, and replaces the field-order profile at `docs/specifications/wire.md:105-110`.
- **ADR 0021 is Accepted.** Withdrawing the member-order promise is the irreversible half: a consumer who absorbs the withdrawal cannot have it restored cheaply. The draft is quoted in Phase 0.
- **ADR 0022 is Accepted.** It depends on 0021 and records the codec, the scope and the design. The draft is quoted in Phase 0.
- ADR 0002's amendment (a new D8 plus a narrowed consequence bullet) is drafted, as quoted in Phase 0.
- The PROBE-030 and PROBE-038 rewrites are drafted, as quoted in Phase 0, together with the `### Terms` insert that defines "wire-equivalent" once.
- Phases below name concrete tasks and their verification command.

## Definition of Done

- Code and tests land with `// REQ-` and `// PROBE-` citations.
- [`traceability.yaml`](../../specifications/traceability.yaml) and the REQ.md **Impl.** column reflect the implementation. REQ-052 stays `landed` throughout, because the amendment is implementation-aligned and merges with the code it describes; `scripts/spec-check.sh:300-305` enforces agreement between the two, so if the plan ever splits such that the spec text merges first, both move to `partial` in the same commit and back at close-out.
- **The two indexes `make spec-check` cannot see.** The roadmap row for canonical JSON (`docs/roadmap.md:59`) still promises "byte-stable `_type` round-trips" and is rewritten. The REQ.md numbering band is **untouched**: this plan allocates no new REQ id, so no headroom is consumed and no band changes state.
- Canonical spec prose and **Status:** lines updated in the same PR: REQ-052's member-order, escaping, duplicate-name and error-type clauses; PROBE-030, PROBE-038 and PROBE-089's catalog prose; the `### Terms` section.
- STRAND-04's codec sub-question is marked resolved. The strand itself stays **Partially resolved**: the full RM inventory (`research-strands.md:60`) and validation independence (`:63`) are untouched by this work.
- A `### Changed` CHANGELOG bullet is queued for the release that ships this, exactly one sentence, per `AGENTS.md:67`:
  > - **Canonical JSON is encoded by `encoding/json/v2` (REQ-052, ADR 0021/0022).** Encoded member order is no longer a contract, `_type`-first is a recommendation, `<`/`>`/`&` are no longer escaped, and the generated per-type JSON marshalers are retired.
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](archive/) per `sdd-archive`, in the implementing PR.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) | Done |
| Indexes `spec-check` misses (`roadmap.md:59` row; REQ.md numbering band not touched, no new id) | Done |
| Code | Done |
| Tests with `// REQ-` / `// PROBE-` comments | Done |
| `make spec-check` | Done |
| `make ci` | Done |

## Phases

### Phase 0: specify

Documents only. No code, no test, no generated file changes in this phase. Every block below is draft text for the implementer to apply; the plan quotes it, it does not apply it.

#### 0.1 REQ-052: replace the field-order profile

`docs/specifications/wire.md:105-110` today states the deterministic profile as a MUST and calls it "the SDK's own output contract, which a consumer may rely on for byte comparison and hashing". Replace that bullet and its trailing paragraph with the two bullets below. Everything else in the Canonical-JSON properties list is unchanged.

> - **Member order.** The encoder **SHOULD** emit `_type` as the first member of every encoded concrete RM value, so that a consumer decoding as a stream can select the concrete type before reading the rest of the object. Beyond that, the order in which the encoder writes members is **unspecified**, and a consumer **MUST NOT** rely on it. JSON object member order carries no meaning (RFC 8259 § 4), the openEHR specifications prescribe none, and CDR implementations differ in the order they emit. The decoder **MUST** accept members in any order, `_type` included. The SDK asserts no order for another implementation's output, and it does not defer to a future openEHR member-order rule.
>
>   Two obligations follow, and the conformance probes are bound by them. Round-trip fidelity **MUST** be asserted semantically (the decoded values are equal, and the recovered value passes the reference-model validation floor, [REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)) and **MUST NOT** be asserted by comparing encoded bytes. Where a check on the encoded form is useful, it **MUST** be a *wire-equivalent* comparison as [conformance.md § Terms](conformance.md#terms) defines it, never a byte comparison. The SDK therefore makes **no** byte-level output promise a consumer may hash, diff, or sign against; a consumer needing a stable digest of an RM value **MUST** compute it over a canonicalisation of its own choosing.
>
> - **Map-valued (`Hash`) members.** The encoder **SHOULD** serialize the members of a `Hash` (`map[K]V`) value in lexicographic key order. This is a determinism property of the encoder, not an order promise to a consumer: the rule above still forbids relying on member order. It is stated because Go randomises map iteration, so an encoder that does not sort emits a different spelling on every call for one unchanged value, which would leave the SDK's own equality, caching and diff tooling unable to treat two encodes of one value as the same artefact. The codec obtains it with `json.Deterministic(true)`. A unit test pins it; no conformance probe asserts it.

#### 0.2 REQ-052: five further edits on the same axis

| Edit | Where | Draft text or rule |
|---|---|---|
| Duplicate member names are refused | new sentence in § Decode-side shape sentinel | "A JSON object that carries the same member name twice **MUST** be refused, and the refusal **MUST** wrap `canjson.ErrInvalidShape`. RFC 8259 § 4 states that names SHOULD be unique, and an object carrying duplicates has no single defined value. This is distinct from the malformed-JSON exclusion below: the bytes parse, and it is the object's shape that is rejected." Implementation note for Phase 2: `jsontext` raises this before any SDK code runs, so `canjson` classifies it explicitly rather than inheriting the sentinel |
| Member names match exactly | new sentence beside the member-order bullet | "Canonical-JSON member names are matched exactly. The case-insensitive matching § Unknown response keys requires for preserved `Extras` values (`wire.md:424`) binds the Definition, System and AQL result surfaces, which are not on this codec path." |
| Error types stop being named | `wire.md:117`, `:119`, `:121`, `:128`, `:134` | Today these name `*json.UnsupportedTypeError`, `*json.UnmarshalTypeError`, `*json.SyntaxError`, `io.ErrUnexpectedEOF` and `io.EOF` (the § Floating-point precision arm is at `:134`, not `:130` as one report cited). Rewrite each so it binds the SDK sentinel and the reachability of *some* underlying codec error through unwrapping, and leaves the concrete standard-library type unspecified. The v2 types are `*json.SemanticError` (`go doc encoding/json/v2.SemanticError`) and `*jsontext.SyntacticError` (`go doc encoding/json/jsontext.SyntacticError`), and naming them again would only queue the same errand for the next codec |
| Escaping and UTF-8 | `wire.md:113` | Two sentences change. The parenthetical that `encoding/json` spells `<` and `>` as `\u003c` and `\u003e` becomes an observation that the codec leaves them unescaped and that the obligation stays on the decoded character. The sentence describing silent U+FFFD substitution becomes: the codec refuses invalid UTF-8 and a lone surrogate escape outright, and a genuine U+FFFD still decodes. The XML half of the clause (`&#xD800;` accepted as a genuine U+FFFD) is unchanged |
| The XML asymmetry is stated | one sentence in REQ-052, and ADR 0021's Related line | "[REQ-056](#req-056)'s element-order profile for canonical XML is deliberately **not** relaxed: element order is part of an XML document's identity in a way member order is not part of a JSON object's, so PROBE-033 keeps its byte assertion." Without this, the next axis sweep reads canonical XML as an oversight |

#### 0.3 conformance.md: define "wire-equivalent" once

`docs/specifications/conformance.md` has no terms section. PROBE-038's wire assertion at `conformance.md:601` already uses "wire-equivalent" undefined, and PROBE-089's at `:569` says "byte-for-byte" for a comparison that is order-insensitive in code (`testkit/probes/serialize/probe_089_underscore_round_trip.go:833`). Insert between the catalog field list (ends `conformance.md:165`) and `### Authentication and discovery` (`:167`):

> ### Terms
>
> **Wire-equivalent.** Two JSON documents are *wire-equivalent* when parsing each into a generic JSON value (object as a map, array as a slice, plus string, number, boolean and null) yields equal trees. The comparison is by member name, not by position, so it is insensitive to member order and to the encoder's choice of string escaping. Array element order **is** compared: it is significant in JSON and in the RM. Numbers are compared as their literal text, so an integer past 2^53 compares exactly rather than through `float64`. Wire-equivalence is the **only** admissible check on encoded bytes in this catalog: no probe **MAY** assert byte equality of encoded JSON, for the reason [§ REQ-052](wire.md#req-052) gives.

Adding a `### Terms` heading is safe for the gate: the census guards count `^#### PROBE-` headings and `^- \*\*Modes:\*\*` lines (`scripts/spec-check.sh:352-355`), and a third-level heading carrying neither adds to neither count.

PROBE-089's catalog prose at `conformance.md:569` changes "re-encodes byte-for-byte" to "re-encodes to a [wire-equivalent](#terms) document". One line, no code change.

#### 0.4 PROBE-030, rewritten

Replaces the **Title** and **Wire assertion** lines at `docs/specifications/conformance.md:577-584`. The `#### PROBE-030` heading line, the `- **Modes:**` line and the `- **Status:**` line are reproduced unchanged, so the in-repo census at `conformance.md:44` and the catalog's own punctuation are untouched.

> - **Title:** Decoding a canonical-JSON RM value and re-encoding it, then decoding and encoding that output again, recovers the same value and a document wire-equivalent to the first, and the recovered value satisfies the reference-model floor.
> - **Preconditions:** A reference Composition cassette.
> - **Wire assertion:** Decode, Encode (`b1`), Decode (`A`), Encode (`b2`), Decode (`B`). `A` and `B` **MUST** be equal by typed deep comparison (`reflect.DeepEqual` over the decoded RM values, which compares an interface-typed field by its dynamic type and value and so covers every substitutable slot and every `DV_INTERVAL[T]` bound with no comparison options). `B` **MUST** satisfy `validation.ValidateRM` ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)) with no issues. As a secondary check, `b1` and `b2` **MUST** be [wire-equivalent](#terms). No byte equality is asserted anywhere: not against the input cassette, whose member order is its own (REQ-052), and not between the SDK's own encodes, whose order is not a contract.
> - **Modes:** Sandbox (no network).
> - **Satisfies:** REQ-052, REQ-040, REQ-082

The typed comparison is between `A` and `B`, not between the cassette's decode and `A`, and the reason is a trap worth carrying into the implementation. REQ-052 documents a decode-then-encode collapse at `wire.md:112`: `"mappings": []` decodes to a non-nil empty slice, `omitempty` re-encodes it as an absent key, and the next decode yields nil. `wire.md:113` records the identical collapse for `DV_MULTIMEDIA.data` and `integrity_check`. `reflect.DeepEqual` distinguishes a nil slice from an empty one, and it must, because REQ-112's `Mappings_valid` check depends on that nilness. Comparing on either side of the **second** encode is collapse-immune, because both values derive from SDK output, and it keeps full bite: a discriminator dropped, a field narrowed or a value mangled on the second pass still fails. No cassette carries a present-but-empty list today (Brief B section 3.2), so the trap is latent rather than live.

#### 0.5 PROBE-038, realigned

Replaces only the Wire assertion at `docs/specifications/conformance.md:601`. The implementation is already order-insensitive (`collectDiscriminators`, `testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go:94`, whose comment reads "Robust against ordering changes"), so this is a wording fix that makes the catalog honest.

> - **Wire assertion:** Decode succeeds; the recovered value preserves every original `_type` discriminator, counted as a multiset, so a lost substitution fails while an elided duplicate does not; re-marshalling produces a document [wire-equivalent](#terms) to the same logical content. No member order is asserted.

The retired clause is the parenthetical "(canonical JSON ordering wins ties)", which appeals to the profile being withdrawn.

No advisory probe is added for the `_type`-first SHOULD. The catalog has no advisory level: `testkit/probe/result.go:33-43` defines a closed vocabulary of `StatusPass`, `StatusSkip` and `StatusFail`, and line 31 records that anything else a probe reports is rewritten to `StatusFail`. The SHOULD is witnessed by unit tests instead, at the three positions worth pinning (a root, a nested object, a substitutable slot), listed in Phase 3.

#### 0.6 Two new ADRs

The maintainer took two decisions, and this repository records one decision per ADR. ADR 0021 is the specification relaxation and stands on RFC 8259 alone; ADR 0022 is the codec adoption and cites 0021 as its premise. The next free number is 0021: `docs/adr/` runs 0001 through 0020 and `docs/adr/README.md` lists 0020 as the last row. Both are **Proposed** until accepted. Two rows are added to `docs/adr/README.md`. The drafts below carry no em dashes, to match this plan's house style; the applied files follow the punctuation convention of the existing records (`docs/adr/0018-raw-bytes-on-decode-error.md:1-8`).

##### ADR 0021 draft

> # ADR 0021: encoded JSON member order is not part of the canonical JSON contract
>
> - **Status:** Proposed, 2026-09-14.
> - **Supersedes:** none.
> - **Superseded by:** none.
> - **Strand:** retires the byte-stability premise in [STRAND-04](../../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance)'s remaining evidence item.
> - **Introduces:** none. **Amends:** [REQ-052](../../specifications/wire.md#req-052) (the field-order profile and the probe obligation it carries).
> - **Plan:** [2026-09-14-json-v2-migration.md](2026-09-14-json-v2-migration.md).
> - **Related:** [ADR 0022](0022-canonical-json-encoding-json-v2.md) (the codec decision this one unblocks); [REQ-056](../../specifications/wire.md#req-056) (canonical XML, deliberately not amended); [REQ-112](../../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (the validation floor the rewritten PROBE-030 asserts).
>
> ## Context
>
> `docs/specifications/wire.md:105` states a deterministic encode profile as a MUST: `_type` first, then BMM property declaration order, `Hash` keys lexicographic, with two successive SDK encodes byte-identical. Line 106 names the reason: "the SDK's own output contract, which a consumer may rely on for byte comparison and hashing". PROBE-030 asserts it (`testkit/probes/serialize/probe_030_canjson_round_trip.go:83`, `bytes.Equal(b1, b2)`), and the probe's own doc comment at line 39 calls it a "guarantee for hashing, signing, and diff tooling".
>
> No openEHR specification asks for it. RFC 8259 § 4 assigns no meaning to JSON object member order, the openEHR specifications prescribe none, and CDR implementations differ in the order they emit. The decoder's obligation to accept any order, already stated at `wire.md:110`, is the real interoperability rule and is unaffected.
>
> The promise has a price. It constrains the codec: any encoder the SDK adopts must reproduce one spelling for one value, which is the gate [STRAND-04](../../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance) named at `research-strands.md:68`. It also shapes the conformance suite: a probe that compares two encodes tests the encoder's self-consistency, which passes even when the encoder drops the same field on both passes.
>
> ## Decision
>
> **Encoded JSON member order is not part of the SDK's contract.**
>
> - `_type` first becomes a **SHOULD**. It has a reason that survives the withdrawal: a consumer decoding as a stream can select the concrete type before reading the rest of the object.
> - Beyond `_type`, member order is **unspecified**, and a consumer **MUST NOT** rely on it.
> - The decoder **MUST** continue to accept members in any order, `_type` included.
> - Conformance is asserted **semantically**: decoded values are compared, and the recovered value is passed through the reference-model floor ([REQ-112](../../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)). Where a check on encoded form is useful it is wire-equivalence, defined once in the conformance catalog's Terms section. No probe asserts byte equality of encoded JSON.
> - `Hash` (`map[K]V`) members are emitted in lexicographic key order as a **SHOULD**, re-stated as a determinism property of the encoder rather than an order promise to a consumer. Go randomises map iteration, so without it one unchanged value has no reproducible encoding at all. A unit test pins it; no probe does.
>
> **Open point for the maintainer: SHOULD or MUST for encoder determinism.** The case for MUST is that six live assertions depend on one value encoding identically twice: `openehr/serialize/canjson/field_order_test.go:132`, `openehr/serialize/canjson/marshal_sentinel_test.go:66`, `openehr/client/ehr/contribution/builder_test.go:364` and `:373`, `openehr/instance/valuefill_test.go:70`, and `openehr/serialize/simplified/roundtrip_test.go:39`. The case for SHOULD is that none of them is an interoperability obligation: they are the SDK's own tooling, and stating an internal implementation property as a normative MUST on the wire format is the category error this ADR exists to correct. The draft takes SHOULD.
>
> ## Consequences
>
> - **A consumer that hashed, diffed or signed SDK output loses a guarantee it may have relied on.** Pre-1.0 this is a `### Changed` CHANGELOG entry and a minor bump (`CHANGELOG.md:7`), not a deprecation cycle. A consumer needing a stable digest computes it over a canonicalisation of its own choosing. The SDK does not ship one, because an SDK-blessed canonical form would be the same promise wearing a different name.
> - **The probe suite gets stronger, not weaker.** A byte comparison of two encodes passes whenever the encoder is self-consistent, including when it drops a field on both passes. Typed deep comparison plus `ValidateRM` catches exactly that class. This is the substantive argument for the change, not its mitigation.
> - **Two documented re-encode collapses become visible.** `wire.md:112-113` records that `DV_TEXT.mappings` and `DV_MULTIMEDIA.data` lose the present-but-empty distinction on re-encode, so the decode of a cassette and the decode of its re-encode can legitimately differ. PROBE-030 therefore compares the values on either side of the *second* encode, where both derive from SDK output. The collapses themselves are unchanged.
> - **Canonical XML is untouched.** [REQ-056](../../specifications/wire.md#req-056)'s element-order profile and PROBE-033's byte assertion stand. The asymmetry is principled: element order is part of an XML document's identity in a way member order is not part of a JSON object's.
> - **The withdrawal is one-way.** Restoring an order promise after consumers have absorbed its removal is not a cheap change, which is why this half of the work is the ADR-worthy fork and the codec swap (reversible, one funnel at `openehr/serialize/canjson/marshal.go:28`) is not.
>
> ## Alternatives considered
>
> - **Keep the profile as a MUST and satisfy it under any future codec.** Technically reachable: `json.Deterministic(true)` plus a `_type`-first emission gives it. **Rejected:** it preserves a promise no openEHR specification asks for, constrains every future codec choice, and leaves the probe suite testing encoder spelling instead of round-trip fidelity. `go doc encoding/json/v2.Deterministic` also warns that determinism holds "across different instances of identical binaries, but not across different builds of a program (such as different source or toolchain version, different GOOS/GOARCH, different build flags)", so the restated promise would be weaker than the one being withdrawn while reading identical.
> - **Keep the profile and publish a separate canonicalisation helper for consumers who hash.** **Rejected:** it is the withdrawn promise under a new name, and it puts the SDK in the business of defining a canonical form for a format whose specification defines none. RFC 8785 exists for consumers who need one.
> - **Downgrade the whole profile to a SHOULD without touching the probes.** **Rejected:** the probes are where the promise is actually enforced. A SHOULD that PROBE-030 still asserts byte-wise is a MUST with softer wording.

##### ADR 0022 draft

> # ADR 0022: canonical JSON is encoded by `encoding/json/v2`
>
> - **Status:** Proposed, 2026-09-14.
> - **Supersedes:** none.
> - **Superseded by:** none.
> - **Strand:** resolves the `encoding/json/v2` simplification sub-question and the default-codec sub-question of [STRAND-04](../../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance).
> - **Introduces:** none. **Amends:** [ADR 0002](0002-bmm-codegen-decisions.md) (what the generator emits for JSON: new D8).
> - **Depends on:** [ADR 0021](0021-json-member-order-not-a-contract.md). Without the order withdrawal this decision would be a different and worse one.
> - **Plan:** [2026-09-14-json-v2-migration.md](2026-09-14-json-v2-migration.md).
> - **Related:** [ADR 0003](0003-rm-event-polymorphism.md) (the EVENT interface whitelist, whose codec justification this decision re-homes); [ADR 0004](0004-numeric-wire-tolerance.md) (strict encode, permissive decode, kept hand-written and unchanged); [REQ-040](../../specifications/rm-modeling.md#type-registry-req-040) (the registry the dispatch hooks read).
>
> ## Context
>
> The canonical-JSON codec is generated code. `internal/bmmgen/render_jsonmar.go` (451 lines) and `internal/bmmgen/render_jsonunmar.go` (473 lines) emit one `MarshalJSON` and one `UnmarshalJSON` per generated type; `openehr/rm` carries 110 of each and `openehr/aom/aom14` 29 of each, 14 509 generated lines in total. `openehr/internal/jsonpoly/jsonpoly.go` (80 lines) supplies `_type` for a value held in an interface, through 177 generated call sites. `openehr/serialize/canjson/marshal.go:28` is a thin pass-through over `encoding/json`.
>
> Almost all of it exists to obtain three things `encoding/json` v1 does not give: a discriminator on a value held in an interface, `_type`-driven dispatch on decode, and a per-type funnel for the shape sentinel. Two of the three are v2 defaults. `go doc encoding/json` § Migrating to v2 states: "In v1, MarshalJSON methods declared on a pointer receiver are only called if the Go value is addressable. In contrast, in v2 a MarshalJSON method is always callable regardless of addressability." That single sentence retires `jsonpoly`. Decode dispatch is served by `json.UnmarshalFromFunc` registered through `json.WithUnmarshalers`, one hook per polymorphic interface: 20 hooks against 181 slot occurrences.
>
> Go 1.27 ships `encoding/json/v2` and `encoding/json/jsontext` as ordinary standard library, with v1 reimplemented on v2 and its behaviour preserved. The module floor is already `go 1.27.0` (`go.mod:3`).
>
> ## Decision
>
> **The canonical-JSON path is encoded and decoded by `encoding/json/v2`, and the generator emits the streaming marshaler pair instead of bespoke JSON codec methods.**
>
> - **Design.** Per concrete class the generator emits a method-free alias type and two short methods: `MarshalJSONTo(*jsontext.Encoder) error` (`go doc encoding/json/v2.MarshalerTo`), which calls `json.MarshalEncode` on an anonymous struct whose first field is `_type` and whose second embeds the alias; and `UnmarshalJSONFrom(*jsontext.Decoder) error` (`go doc encoding/json/v2.UnmarshalerFrom`), which calls one shared runtime helper. Substitutable slots are served by one `json.UnmarshalFromFunc` per polymorphic interface, built from `typereg.Default` at init. `openehr/internal/jsonpoly` is deleted.
> - **Scope.** Five packages move: `internal/bmmgen`, `openehr/rm` (generated plus the hand-written primitives), `openehr/aom/aom14`, `openehr/rm/typereg` and `openehr/serialize/canjson`. `openehr/internal/jsonpoly` is deleted. Everything else stays on `encoding/json` v1, which remains supported and is itself implemented on v2. The split is drawn where the canonical-JSON contract ends: every package that decodes an RM value moves together, so there is never more than one decode-error taxonomy behind the sentinels REQ-052 keeps distinct.
> - **Determinism is an option, not an emergent property.** `go doc encoding/json/v2.Marshal` states that a Go map "is traversed in a non-deterministic order", and `go doc encoding/json/v2.MarshalerTo` tells a custom marshaler to "respect the Deterministic option". The generated marshalers set `json.Deterministic(true)`.
> - **Cut-over, not a flag.** The generator emits one shape. Rollback is `git revert` plus `make codegen`, proven by `make codegen-verify`, which `make test` runs first (`Makefile:192`).
>
> ## Consequences
>
> - **About 12 800 generated lines and one internal package go away.** 278 generated methods become roughly 1 700 generated lines plus 20 registered hooks, and `internal/bmmgen`'s two JSON templates shrink from 924 lines while keeping the same shape as each other.
> - **Encoded bytes change, in two ways a consumer can see.** `jsontext.EscapeForHTML` is opt-in in v2 where v1 escaped `<`, `>` and `&` unconditionally, so a `TERM_MAPPING.match` carrying `<` now spells `"<"` on the wire where it spelled `"\u003c"`. REQ-052 already puts that obligation on the decoded character rather than the literal bytes (`wire.md:113`), so no contract breaks. Separately, `omitzero` replaces `omitempty` on pointer fields, so a pointer to an empty string is now omitted where v1 emitted `""`. Container fields keep `omitempty`: under `omitzero` a non-nil empty slice is not zero and would emit `[]`, which `wire.md:112` states outright would be RM-invalid (`Mappings_valid`).
> - **Decode is stricter in three ways.** Invalid UTF-8 and a lone surrogate escape are refused by `jsontext` rather than silently substituted with U+FFFD, which is what `wire.md:113` asks for and moves the refusal out of `rm.Character` into the standard library. An object with duplicate member names is refused. Member names match case-sensitively on this path. A body that used to decode can now fail.
> - **Decode error types change identity.** `*json.SyntaxError` gives way to `*jsontext.SyntacticError`, and `*json.UnmarshalTypeError` and `*json.UnsupportedTypeError` to `*json.SemanticError`. The SDK's own sentinels are unchanged: `canjson.ErrInvalidShape`, `canjson.ErrInvalidValue`, `transport.ErrInvalidShape`, the `typereg.Err*` family and `*transport.DecodeError` keep their identity and their `errors.Is` and `errors.AsType` behaviour. A consumer matching on the SDK sentinels needs no change; one matching on the standard-library types does.
> - **A typed error gains a value-bearing field.** `json.SemanticError` carries `JSONValue jsontext.Value`, the JSON number or string that could not be unmarshaled. That is inside the class REQ-052 already permits for a cause (`wire.md:128`), and the value-free discipline binds only `WireError.Error()` and `DecodeError.Error()`, so no rule breaks. A consumer that logs a wrapped cause wholesale now reaches a payload value it could not reach before.
> - **v2 supplies the diagnostic path the SDK hand-builds.** `json.SemanticError.JSONPointer` is an RFC 6901 pointer computed by the decoder, where `openehr/rm/composition_jsonunmar_gen.go:117` builds `/content/0` by hand. `typereg.DecodeError` stays the public envelope and may read its `Path` from the pointer. One caveat: a nested decode inside a hook restarts the pointer at that slot's own root, so a hook that does not decode in place produces relative paths.
> - **One public constraint is affected.** `openehr/client/ehr/contribution/submission.go:51` declares `CommitVersion interface { json.Marshaler; BMMName() string }`. A type implementing only `MarshalJSONTo` satisfies `json.MarshalerTo`, not `json.Marshaler`, so the constraint moves or the generated types keep a thin `MarshalJSON` beside the streaming one.
> - **`openehr/rm/typereg/nilreceiver_census_test.go` stops finding its subjects.** It enumerates `json.Unmarshaler` implementations at `:71` and `:93`. Under `UnmarshalerFrom` that census silently covers nothing, which is worse than failing. It is made interface-agnostic in the same change.
>
> ## Alternatives considered
>
> - **Keep `encoding/json` v1 and the generated marshalers.** Zero migration risk; every pinned behaviour stays pinned by construction. **Rejected:** it keeps 924 lines of generator template and 278 generated methods whose purpose is to reproduce what v2 gives directly, in the highest-churn part of the tree, since every BMM bump re-emits all of them. With the order promise withdrawn there is nothing left for them to buy.
> - **Swap the codec but keep the v1 method pair (`MarshalJSON` / `UnmarshalJSON`).** Smaller diff, and it keeps `CommitVersion` and the nil-receiver census working unchanged. **Rejected:** it leaves all 278 methods in place and books a second migration to remove them, and it forgoes the allocation win the streaming pair exists for. `go doc encoding/json/v2.MarshalerTo` recommends it over `Marshaler` as "both more performant and more flexible". The two objections become tasks in the implementing plan.
> - **Emit no methods at all and build the codec from registry-driven options.** Fewest generated lines in theory. **Rejected:** the behaviour would then exist only when the caller supplies the options, so `json.Marshal(&rm.DVText{...})` without them emits no `_type`. Today the generated methods make every RM value self-describing under any JSON entry point, and losing that is a public API regression.
> - **`sonic`.** A JIT-assisted encoder with large throughput gains on x86-64. **Rejected:** a third-party dependency in the SDK's hottest correctness path, architecture-sensitive, and it addresses the performance axis while leaving every generated line in place. STRAND-04 keeps the two axes separate on purpose.
> - **`easyjson`.** Generated marshalers, which the SDK already has in its own generator. **Rejected:** it replaces one code-generation surface with another and adds a dependency to do it.
> - **A permanent split: migrate `openehr/rm` and leave `aom14` or the RM primitives on v1.** **Rejected:** `wire.md:122` requires `openehr/aom/aom14` to classify decode failures "on the same terms as the canonical-JSON RM surface", and `transport.Decode` routes every 2xx body through `canjson.Unmarshal` (`transport/client.go:654`). A permanent split leaves two decode-error taxonomies behind one sentinel set. Phasing the work across commits is fine; shipping the split is not.

#### 0.7 ADR 0002 amendment

ADR 0002's Decision section runs D1 through D7 (`docs/adr/0002-bmm-codegen-decisions.md`); D6 and D7 concern BMM functions and are untouched. Add D8 and narrow the Consequences bullet that cites ADR 0003.

> ### D8: the generator emits no bespoke JSON codec methods
>
> Superseding the emission policy `internal/bmmgen/render_jsonmar.go` and `internal/bmmgen/render_jsonunmar.go` implemented, the generator no longer emits a `MarshalJSON` / `UnmarshalJSON` pair per generated type. Canonical JSON is produced and consumed by `encoding/json/v2` ([ADR 0022](0022-canonical-json-encoding-json-v2.md)), and the properties the bespoke methods supplied are obtained as follows.
>
> | Property | Was | Is |
> |---|---|---|
> | `_type` on every concrete RM value | hand-rolled prologue in each generated `MarshalJSON` | a generated `MarshalJSONTo` calling `json.MarshalEncode` on an anonymous struct whose first field is `_type` and whose second embeds a method-free alias of the class |
> | `_type` on a value held in an interface | `openehr/internal/jsonpoly` (80 lines, 177 call sites) | nothing. v2 calls a pointer-receiver marshaler regardless of addressability (`go doc encoding/json` § Migrating to v2) |
> | Polymorphic dispatch at a substitutable slot | `typereg.DecodeAs[T]` called from each generated `UnmarshalJSON` | one `json.UnmarshalFromFunc` per polymorphic interface, built from `typereg.Default` at init and supplied through `json.WithUnmarshalers`. Every hook **MUST** pass `dec.Options()` into any nested decode, or a deeper interface slot silently loses its hook |
> | Member order | struct field order, fixed by emission order | not a contract ([REQ-052](../../specifications/wire.md#req-052)); `_type` first is a SHOULD, and `Hash` sorting is `json.Deterministic(true)` set by the generated marshaler |
> | Zero versus omit | `omitempty` on every generated tag | `omitzero` on pointer fields; `omitempty` retained on container fields. See the warning below |
> | Shape-failure classification (`canjson.ErrInvalidShape`) | `typereg.WrapShapeError` at each generated funnel | one shared runtime helper called by every generated `UnmarshalJSONFrom`, so 110 copies of the classification collapse into one. The sentinel's contract stays [REQ-052](../../specifications/wire.md#req-052) § Decode-side shape sentinel, not this ADR's |
> | Nil-receiver refusal (REQ-025) | first statement of each generated `UnmarshalJSON` (`internal/bmmgen/render_jsonunmar.go:345`) | first statement of each generated `UnmarshalJSONFrom`, unchanged in force |
>
> The invariant D8 asserts: **no per-type JSON codec logic is generated into `openehr/rm/*_gen.go` or `openehr/aom/aom14/*_gen.go` beyond the two-method delegation above.**
>
> **Warning, on the `omitzero` row.** `go doc encoding/json` § Migrating to v2 recommends migrating `omitempty` to `omitzero` for a bool, number, pointer or interface value, and that is right for the 42 `*string`, 16 `*bool`, 23 pointer-to-number, 6 `*map[string]T` and 5 `*any` fields the generator emits. It is wrong for container fields. v2's `omitempty` omits a field that encodes as an empty JSON value, so a nil slice and a non-nil empty slice are both omitted, which is the collapse `wire.md:112` documents and `TestDVTextMappingsDecodePresenceAndEncodeCollapse` pins. Under `omitzero` a non-nil empty slice is not zero and would emit `[]`, which `wire.md:112` states would be RM-invalid (`Mappings_valid`). A blanket sweep would break an RM invariant silently.

Narrowing for ADR 0002's Consequences section (the bullet citing ADR 0003):

> [ADR 0003](0003-rm-event-polymorphism.md)'s whitelist stands, and its justification is re-homed. It was adopted because `encoding/json` could not select a concrete shape at `HISTORY.events` and the generated `UnmarshalJSON` copied the slice without dispatch. With D8 that generated method is gone, so the *codec* argument no longer applies, but the *type-shape* decision does: `History[T].Events` is `[]Event`, an interface slice, and that is the Go API the SDK ships. The whitelist is a public-surface decision now, not a codec workaround, so changing it would be a breaking API change rather than a codec tuning knob.

#### 0.8 STRAND-04 status lines

Three edits to `docs/specifications/research-strands.md`. The strand's **Status** at `:48` stays "Partially resolved" and the index row at `:271` stays "Partially resolved": the full RM inventory (`:60`) and validation independence (`:63`) remain open and are untouched by this work.

Add a row to the resolved sub-questions table (after `research-strands.md:57`):

> | `encoding/json/v2` as the canonical-JSON codec; encoded member order is not a contract | Migrate to standard-library v2; retire the generator's per-type `MarshalJSON` / `UnmarshalJSON` and `openehr/internal/jsonpoly`; `_type` first becomes a SHOULD and round-trip fidelity is asserted semantically rather than byte-wise | [ADR 0021](../../adr/0021-json-member-order-not-a-contract.md), [ADR 0022](../../adr/0022-canonical-json-encoding-json-v2.md) |

Replace the two "Still open" bullets at `:61-62` (default codec benchmark, `encoding/json/v2` as a simplification axis) with one:

> - **Codec performance against the standard-library baseline:** with the codec settled by [ADR 0022](../../adr/0022-canonical-json-encoding-json-v2.md), the open question is no longer which codec but whether the migrated path is within budget for seeder and benchmark workloads, measured against the retired generated marshalers on `openehr/serialize/canjson/bench_test.go` rather than against `sonic` or `easyjson`. A regression is a plan task, not a codec fork.

Replace the two `encoding/json/v2` evidence bullets at `:68-69` with:

> - `encoding/json/v2` migration evidence: `_type` emission (first-member SHOULD) and the polymorphic `_type` round-trip survive the retirement of the generated marshalers, asserted by PROBE-030 and PROBE-038, whose byte assertions are replaced by typed deep comparison plus [wire-equivalence](conformance.md#terms); the generated and `jsonpoly` line count removed is recorded in the plan's close-out.

The "Resolution form (remaining)" paragraph at `:71` already anticipates the ADR 0002 amendment and needs only its tense changed from conditional to past.

#### 0.9 `traceability.yaml` delta sketch

Block shape follows `docs/specifications/traceability.yaml:123-134` (REQ-040) and `:125-175` (REQ-052). Sketch only; the full blocks are not reproduced here.

```yaml
  - id: REQ-052
    packages: [ … ]                                  # DROP openehr/internal/jsonpoly (line 140)
    plans:    [ … , docs/plans/archive/2026-09-14-json-v2-migration.md ]   # ADD
    probes:   [PROBE-030, PROBE-031, PROBE-038]      # unchanged
    adrs:     [ … , docs/adr/0021-….md, docs/adr/0022-….md ]       # ADD both
    tests:
      - openehr/serialize/canjson/field_order_test.go        # RENAME to member_order_test.go (line 135)
      - openehr/internal/jsonpoly/jsonpoly_test.go           # DROP (line 154)
      - openehr/serialize/canjson/wire_equivalence_test.go   # ADD if the helper gets its own test
    notes: >-
      REPLACE the field-order paragraph; ADD the duplicate-name and
      case-sensitivity sentences and the v1-to-v2 error-type restatement
```

- **REQ-040** (`traceability.yaml:123`) lists `packages: [openehr/rm/typereg]` only. Dispatch stays in `typereg` under the chosen design, so the row is unchanged.
- **REQ-053** (`traceability.yaml:696`): no spec text changes. `openehr/serialize/simplified/roundtrip_test.go` is rewritten (census row 6 in Phase 3); check whether it is already in the `tests:` list and add it if not.
- **REQ-151** (`traceability.yaml:2604`): no row changes. The transport decode does not move; only the wrapped cause changes type.
- **PROBE-030 and PROBE-038** have no standalone blocks. They appear as `probes:` members inside REQ entries (REQ-052 at `:131`, REQ-040 at `:129`) and both lists are unchanged; only the catalog prose moves.
- **`REQ.md` Impl. column** (`docs/specifications/REQ.md:51`) stays `landed`. `scripts/spec-check.sh:300-305` enforces agreement between that column and `traceability.yaml`'s `implementation:`, so if the plan ever splits such that the amended spec text merges ahead of the code, both move to `partial` in the same commit.
- **`docs/adr/README.md`** gains two rows, both `Proposed (2026-09-14)` until accepted.

#### 0.10 REQ-052 status: it stays `landed`

REQ-052 went `partial` twice before, both times because a clause was stated in the spec with no implementation behind it: the decode-side shape sentinel with no producer (`docs/plans/archive/README.md:14`) and the floating-point mantissa arm specified and unbuilt (`:13`). Both returned to `landed` on 2026-09-03. The counter-precedent is the 2026-09-04 error-axis plan (`docs/plans/archive/README.md:11`), which made implementation-aligned amendments to five REQs, all of which stayed `landed`. This migration is the second kind: it withdraws one clause and narrows another, and the code lands in the same change, so nothing in the amended text is unbuilt at any point.

**Verification for Phase 0:** `make spec-check` prints `spec-check: OK`. No code changes, so `make ci` is not the gate for this phase. Rewording REQ-052's member-order bullet, PROBE-030's Title and Wire assertion, or PROBE-038's Wire assertion trips no guard: `scripts/spec-check.sh` reads prose in four places only, all of them counts (`conformance.md:44` probe census, `:46` all-three-modes tally, and the runnable-example counts in `docs/examples.md` and `docs/roadmap.md`), plus the structural rule at `scripts/spec-check.sh:384-397` that every `#### PROBE-` heading carries exactly one `- **Modes:**` line. The canonical anchor `docs/specifications/wire.md#req-052` is derived from the `### REQ-052` heading (`scripts/spec-check.sh:69-82`), which does not change.

### Phase 1: fit-gap spike and the nets, time-boxed

One working session. It is not a gate with its own approval: it ends when every question below has a recorded yes or no, and a no has named its fallback. The answers are written back into this plan.

#### 1.1 Already answered, recorded so the spike does not re-run them

Proofs run in a throwaway scratch module against the host toolchain `go1.27.1 linux/amd64`. Nothing in the repository module was touched.

| Question | Answer | Evidence |
|---|---|---|
| Does v2 call a pointer-receiver `MarshalJSON` on a non-addressable value held in an interface? | Yes, with no option and no helper. This is what retires `jsonpoly` | `go doc encoding/json` § Migrating to v2; Brief A Proof 1b: `v1, iface holds T : {"name":{"value":"v"}}` against `v2, iface holds T : {"name":{"_type":"DV_TEXT","value":"v"}}` |
| Do v1 and v2 emit struct members in the same order? | Yes, declaration order in both, embedded fields in place, stable over 1000 encodes. Member order was never a v1 gap | Brief A Proof 1c |
| Does one `json.UnmarshalFromFunc` per interface serve a single slot, a slice of interface, and the narrow-interface fallback when `_type` is absent? | Yes for all three, including through `json.UnmarshalDecode` for streaming | Brief A Proof 2, cases 1 to 3 and case 8; `go doc encoding/json/v2.UnmarshalFromFunc` |
| Does a permuted input with `_type` last at every level still decode? | Yes. That input is the one at `openehr/serialize/canjson/field_order_test.go:20`, so `TestDecodeAcceptsAnyMemberOrder` and `TestDecodePolymorphicSlotWithTypeLast` (`field_order_test.go:18`, `:51`) hold under the new design | Brief A Proof 2b, second input |
| Do the typereg sentinels survive v2's error wrapping? | Yes. `errors.Is` reaches `ErrUnknownType` and `ErrMissingType` through the wrap, and `errors.AsType[*typereg.DecodeError]` still recovers the envelope with its `Path` | Brief A Proof 2b cases 4 and 5, Proof 3b |
| Does v2 sort map keys by default? | No. `json.Deterministic(true)` is required | `go doc encoding/json/v2.Marshal` ("The Go map is traversed in a non-deterministic order"), `go doc encoding/json/v2.Deterministic`; Brief A Proof 1 case G |
| Does v2 escape `<`, `>`, `&`? | No, not by default. `jsontext.EscapeForHTML(true)` restores v1 spelling | Brief A Proof 1 case F |
| Does `omitempty` on a pointer to an empty string behave as it did? | No. v1 emitted `""`, v2 omits it. `omitzero` restores the v1 intent | Brief A Proof 1b, `omitempty` block |
| Is the `DV_TEXT.mappings` nil-versus-empty decode distinction preserved? | Yes: absent and `null` give a nil slice, `[]` gives a non-nil empty slice, exactly as `wire.md:114` requires and `openehr/serialize/canjson/mappings_presence_test.go:27` asserts | Brief A Proof 3 |
| What concrete error values does v2 return? | Recorded per input in Brief A Proof 3: `*json.SemanticError` for a type mismatch, an out-of-range number and a refused quoted number; `*jsontext.SyntacticError` for truncated input, trailing content, a duplicate name (wrapping `jsontext.ErrDuplicateName`), invalid UTF-8, a lone surrogate, and exceeded depth | Brief A Proof 3 |
| Is `json.RawMessage` a different type under v2? | No. `type RawMessage = jsontext.Value` (`go doc encoding/json`), so the 94 generated raw-message wire fields, the `Extras` maps and `openehr/bmm/type.go` need no rewrite even where they survive | Brief C, stdlib ground truth |
| Does `json.Number` keep its lexical form under v2? | Yes: `{"n":1.50}` decodes to `json.Number("1.50")` and re-encodes as `1.50`. Only the decoder option `UseNumber` is gone, which is why the simplified codecs are deferred | Brief C, stdlib ground truth |
| Are float formatting, control-character escaping and indent output byte-identical between v1 and v2? | Yes, over `1.0`, `0.1`, `1e21`, `1e-7`, `1.23e20`, `"a\tb"`, `"a\x01b"`, `"líne"`, and `jsontext.WithIndent("  ")` against `json.MarshalIndent(v, "", "  ")` | Brief C, stdlib ground truth |
| Does `go vet`'s `stdversion` check accept `encoding/json/v2` at this module floor? | Yes. A `go 1.26.0` module is rejected; the repository's `go 1.27.0` floor satisfies it, and `GOEXPERIMENT` is empty, so no build tag is needed | Brief C, lint and vet |
| Is there pre-existing `modernize` debt in the packages that move? | No. `golangci-lint run --enable-only=modernize ./openehr/serialize/... ./openehr/rm/... ./transport/...` reports 0 issues at `d3e9d998` | Brief C, lint and vet |

One rule falls out of the proofs and binds every hook the generator or the runtime helper writes: **a hook must pass `dec.Options()` into any nested decode**. Brief A's Proof 2 failed its slice case on the first run with "cannot derive concrete type for nil interface with finite type set" because a bare `json.Unmarshal(raw, v)` dropped the option set, so the nested interface slot had no hook. `go doc encoding/json/jsontext.Decoder.Options` documents that the returned options include the semantic options passed to an `UnmarshalDecode` call, and `go doc encoding/json/v2.UnmarshalerFrom` says a composite type should unmarshal contained types through `UnmarshalDecode` so those options apply. Forgetting it fails silently and only at depth.

#### 1.2 Open questions

Answered by Task 2's read-only spike (host toolchain `go1.27.1 linux/amd64`, scratch module, nothing in the repository touched). Every standard-library claim below cites a `go doc` symbol confirmed on the host.

| # | Question | Answer | How it is answered | Fallback if the answer is no |
|---|---|---|---|---|
| Q1 | Does an `UnmarshalFromFunc` keyed on `*DVOrdered` fire for `DVInterval[T].Lower` when `T` is instantiated as the interface, and stay quiet when `T` is concrete? | Yes. The hook fires twice for an interface-bound interval and zero times for a concrete-bound one; the generic case needs no bespoke bound router (`go doc encoding/json/v2.UnmarshalFromFunc`) | Throwaway program mirroring Brief A Proof 2b with a generic `DVInterval[T]`. `go doc encoding/json/v2.UnmarshalFromFunc` requires T to be an unnamed pointer or an interface type, which suggests the hook fires on the instantiated type, but the generic case was not exercised. Six `T` and one `*T` wire fields are affected | Keep a hand-written `UnmarshalJSONFrom` on `DVInterval[T]` that routes its bounds through `typereg.DecodeAs[T]`, as `openehr/rm/data_types_quantity_jsonunmar_gen.go:114-127` does today. This is the riskiest unknown in the plan |
| Q2 | Does v2's composite error message prefix break the "message unchanged by the classification" clause at `docs/specifications/wire.md:119`, or can the SDK wrapper strip it? | No hard break. The decoder wraps the SDK's message inside a `*json.SemanticError`, but `SemanticError.Err` recovers it verbatim and `errors.Is` still reaches the sentinel through the wrap; the plan's reword is adequate, and the boundary can strip the outer prefix if ever needed | Throwaway program plus the existing sentinel tests. Brief A Proof 3b recorded the composite form: `json: cannot unmarshal JSON object into Go main.Slot within "/slot": decode /slot: typereg: _type not in registry` | Reword the clause in Phase 0.2's error-type edit so it binds the SDK's own message text and leaves the decoder's prefix out of scope. No code change either way |
| Q3 | Do the 204 JSON cassettes under `testkit/cassettes/` and the 34-body EHRbase FLAT corpus under `testkit/cassettes/flat-conformance/compositions/` decode under v2 defaults with zero duplicate-name and zero case-mismatch rejections? | Yes. All 204 files (the 34-body FLAT corpus included) decode clean with zero duplicate-name and zero case-mismatch rejections; a synthetic duplicate and a synthetic case-mismatch each proved both detectors fire, so the zero count is real | One throwaway program over both corpora, run before any generator change | Set `jsontext.AllowDuplicateNames(true)` or `json.MatchCaseInsensitiveNames(true)` for the affected inputs and record the exception in REQ-052. A hit here also changes the duplicate-name sentence drafted in Phase 0.2 |
| Q4 | Is `rm.Character`'s substituted-U+FFFD detector still reachable once `jsontext` refuses invalid UTF-8 first, and does the genuine-U+FFFD acceptance still hold? | No, the refusal branch is unreachable: `jsontext` refuses invalid UTF-8 and a lone surrogate escape before `rm.Character` ever sees the value. Genuine U+FFFD still decodes. Route 2 (accept the `jsontext` refusal, re-pin the message, delete the JSON-side detector) was taken in Task 7; the XML value rule is untouched | The existing tests, run under v2: `openehr/rm/character_test.go:173` and `:451` (refusals), `:471` (genuine U+FFFD accepted), `:380-381` (the SDK's exact message strings) | Set `jsontext.AllowInvalidUTF8(true)` and keep `jsonLiteralSpellsReplacement` (`openehr/rm/character.go:72-109`) load-bearing on the JSON path. Otherwise accept the `jsontext` refusal, re-pin the message, and delete the JSON-side detector while leaving the XML value rule alone |
| Q5 | Does `json.RejectUnknownMembers(true)`, if a consumer sets it, trip on the `_type` member reaching the method-free alias? | Yes, the alias declares no `_type` field so the option rejects it. The generated alias carries an ignored `_type` field so the option stays usable by a consumer at zero runtime cost | Throwaway program over the design sketch in Phase 2.1 | Give the alias an ignored `_type` field, or strip the member in the shared decode helper. Neither blocks the migration; the answer decides whether the option is usable by a consumer at all |
| Q6 | Do the 13 mandatory container fields keep their `null` spelling with `json.FormatNilSliceAsNull(true)` and `json.FormatNilMapAsNull(true)`, and do the 53 pointer fields behave as intended under `omitzero` with the `DV_TEXT.mappings` pin unchanged? | Yes on both halves. `FormatNilSliceAsNull(true)`/`FormatNilMapAsNull(true)` restore the `null` spelling for mandatory containers; `omitzero` omits a nil pointer and emits a non-nil one (including a pointer to `""`); the `DV_TEXT.mappings` `omitempty` collapse holds unchanged with `FormatNilSliceAsNull(true)` also set | The 13 fields are listed in Brief A section 1d (`openehr/rm/common_resource_jsonmar_gen.go:17`, `:29`, `:98`, `openehr/rm/resource_jsonmar_gen.go:13`, `openehr/rm/common_generic_jsonmar_gen.go:216`, `openehr/rm/demographic_jsonmar_gen.go:71`, `:203`, `:242`, `:311`, `:489`, `:554`, `openehr/aom/aom14/archetype_ontology_jsonmar_gen.go:13`, `:15`). Exercised by a new table test plus `mappings_presence_test.go:27` | Drop the two `FormatNil*` options and let the mandatory containers spell as `[]` and `{}`, which is arguably the better canonical spelling for a BMM-mandatory collection, and record it in REQ-052. The pointer-field half has no fallback: `omitzero` is the ruling, and container fields keep `omitempty` |

Task 2 also answered a seventh question the controller added mid-plan (R12): on Go 1.27, v1 `encoding/json.Marshal`/`Unmarshal` call `MarshalJSONTo`/`UnmarshalJSONFrom` on a type with only those methods, including a value or a pointer held in an interface field (`go doc encoding/json` § Migrating to v2). Yes, confirmed both directions. This is why the stay-on-v1 packages (Phase 3.4) still serialise the migrated RM types correctly, and why `CommitVersion` (Phase 3.3) could move to a `json.MarshalerTo` constraint instead of keeping a v1 shim (ruling R16).

#### 1.3 The nets, built before anything changes

| Task | Can-fail control |
|---|---|
| Differential decode parity over the corpus: a new test decoding all 204 cassettes under both `encoding/json` and `encoding/json/v2` into the same generated types and comparing with `reflect.DeepEqual` | Seed one cassette copy with a key differing from its tag only by case. The test must report that cassette as divergent. If it reports none, the harness is not looking. This also answers Q3 |
| Strengthen `TestEncodeHashKeysLexicographic` (`openehr/serialize/canjson/field_order_test.go:89`) so it is mutation-detectable under a randomised map order | As written it checks three keys in `author` and two in `other_details`; with v2's unsorted default a single encode lands sorted by luck roughly one time in six and one time in two. Widen to six or more keys, or encode 20 times and require every encode sorted. Then dropping `json.Deterministic(true)` from one generated method turns it red reliably |
| Four new benchmarks so the baseline exists on both sides (listed in Phase 3.4) | A benchmark is not a guard; its control is `b.Fatalf` on a setup error, already the pattern at `openehr/serialize/canjson/bench_test.go:56` |

**Corpus figure, corrected.** The "204 cassettes" above is the scanned count, not the compared count. Task 3's built net reports the corpus precisely: 204 scanned, 150 typed (carry a registered `_type`), 103 compared, 47 refused by both codecs identically (46 `submissions/*` CONTRIBUTION cassettes whose `versions[0]` resolves to `*rm.OriginalVersion[interface{}]`, plus one deliberately invalid fixture), and 54 skipped for having no usable `_type` (the FLAT and ITS-REST corpora, which are not canonical JSON). Zero cassettes diverged between the two packages. A can-fail control later showed this net is an *entry-point* parity net (both `encoding/json` and `encoding/json/v2` call the same generated method), not a codec-semantics differential; it stays meaningful after the migration because Go 1.27's `encoding/json` also honours `UnmarshalerFrom` (Q7).

**Exit criterion:** Q1 to Q6 each carry a recorded yes or no in this plan, each no names its fallback, the differential harness is green over the corpus, and the strengthened `Hash` test fails when `Deterministic` is removed. Rollback: delete the added `_test.go` files. Nothing has shipped.

### Phase 2: generator and core

Five tasks in order. Each ends on a green `make ci`, which is what makes it a rollback point. 2.1 and 2.2 stay separate on purpose: they change different observable properties (2.1 key presence and map order, 2.2 escaping and error types), so a bisect between them is informative rather than ambiguous. Until 2.2 lands, the outer v1 `json.Marshal` re-escapes `<`, `>` and `&` inside the bytes a nested marshaler returned, so 2.1 alone changes no escaping.

#### 2.1 The generator emits the streaming pair

`internal/bmmgen/render_jsonmar.go` and `internal/bmmgen/render_jsonunmar.go` stop emitting the v1 method pair and the `json.RawMessage` polymorphic routing (`render_jsonmar.go:372`, `:392`; `render_jsonunmar.go:367`, `:396`). Per concrete class they emit a method-free alias and two short methods:

```go
type rawDVCodedText DVCodedText // drops the methods, so the delegation does not recurse

func (d *DVCodedText) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, &struct {
		Type string `json:"_type"`
		*rawDVCodedText
	}{"DV_CODED_TEXT", (*rawDVCodedText)(d)}, enc.Options())
}

func (d *DVCodedText) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if d == nil {
		return fmt.Errorf("canjson: DV_CODED_TEXT: %w", typereg.ErrNilReceiver) // REQ-025
	}
	return canjsonrt.DecodeInto(dec, "DV_CODED_TEXT", (*rawDVCodedText)(d))
}
```

**Correction (ruling R19).** The alias-only shape above is not universal. `type rawT T` promotes an embedded ancestor's own `MarshalJSONTo`/`UnmarshalJSONFrom` through Go method promotion, so for a class that embeds a marshaler-bearing concrete ancestor (nine direct embeds plus five more that embed one of those, fourteen classes in total: `AccessGroupRef`, `Attestation`, `DVCodedText`, `DVEHRURI`, `LocatableRef`, `OpenehrDefinitions`, `PartyRef`, `PartyRelated`, `TerminologyService`, and the five `Versioned*[T]` wrappers over `VersionedObject[T]`) the alias would silently emit the ancestor's `_type` and drop the subclass's own fields. Those fourteen classes take a flat wire struct instead, built from the same `effectiveFields` enumeration, with field copies both ways; every other concrete class keeps the zero-copy alias shown above. Both shapes call the same shared decode helper and the same encoder option join. Pinned by a registered-type census (`type_census_test.go`) that marshals every registered type's zero value and asserts the wire discriminator matches the registered name.

The shared runtime helper, one copy for the whole tree, carries what 110 generated bodies carry today: the `_type` mismatch check, the `canjson: <RM_TYPE>:` prefix and the `typereg.WrapShapeError` classification.

```go
func DecodeInto(dec *jsontext.Decoder, rmType string, out any) error {
	raw, err := dec.ReadValue()
	// … peek `_type` from raw …
	if head.Type != "" && head.Type != rmType {
		return fmt.Errorf("canjson: expected %q, got %q: %w", rmType, head.Type, errTypeMismatch)
	}
	return json.Unmarshal(raw, out, dec.Options()) // dec.Options() is mandatory
}
```

The interface hooks are built once from `typereg.Default`, one `json.UnmarshalFromFunc` per polymorphic interface: 17 in `openehr/rm` (`DVTextLike` 46 slots, `UIDBasedID` 33, `ObjectRefLike` 27, `ItemStructure` 25, `PartyProxy` 15, `AuditDetailsLike` 4, the open generic bound `T` 4, `PartyIdentifiedLike` 3, `ObjectID` 3, `Item` 3, `DVURILike` 3, `ContentItem` 2, and one each for `Event`, `DVEncapsulated`, `DataValue`, `AuthoredResource`, `AccessControlSettings`) and 4 in `openehr/aom/aom14` (`ExprItem` 4 slots, `CObject` 2, `CPrimitive` 1, `CAttribute` 1). A narrow slot passes its parent's constructor as the fallback for a missing `_type` (`docs/specifications/wire.md:147`); an abstract slot passes none.

Also in this task: `json.Deterministic(true)` in the generated marshalers, the `omitempty` to `omitzero` sweep on pointer fields only, `make codegen`, and re-copying the two generator goldens (`internal/bmmgen/testdata/*.go.golden`, procedure in the comment at `internal/bmmgen/aom14_test.go:131-133`; there is no `-update` flag).

| Guard | Can-fail control |
|---|---|
| `_type` is the first member | `TestEncodeIsDeterministic` (`openehr/serialize/canjson/field_order_test.go:135`) asserts `bytes.HasPrefix(first, []byte("{\"_type\":\"DV_CODED_TEXT\","))`. Commit a generator-level render test that feeds a deliberately reordered plan and asserts the rendered output does not lead with `_type` |
| `Hash` keys lexicographic | The strengthened `TestEncodeHashKeysLexicographic` from Phase 1.3. Mutation: drop `json.Deterministic(true)` from one generated method |
| Repeat-encode identity | `TestEncodeIsDeterministic` (`field_order_test.go:116`) and `openehr/serialize/simplified/roundtrip_test.go:27-44`, which encodes a decoded composition eight times. The second is the stronger control: any map anywhere in the tree that lost `Deterministic` shows up |
| The `omitzero` ruling | New table test: a `DVText` whose `Formatting` points at `""` and whose `OtherDetails` points at an empty map. Assert the chosen wire shape explicitly. Mutation: flip one tag back to `omitempty`; the case must go red. Without this test the change is invisible, because no existing fixture carries a pointer to an empty string |
| nil versus empty on `DV_TEXT.mappings` | `TestDVTextMappingsDecodePresenceAndEncodeCollapse` (`openehr/serialize/canjson/mappings_presence_test.go:27`) already covers both halves. Mutation: remove `omitempty` from `Mappings`; the encoded form becomes `"mappings":[]` and `wantEncoded` fails |
| No field dropped on round trip | `TestRoundTripStructuralEquivalence` (`openehr/serialize/canjson/roundtrip_test.go:104`). Mutation: delete one field assignment from a generated decode path (for example `c.PreferredTerm` at `openehr/rm/data_types_text_jsonunmar_gen.go:50`). Commit it as a generator test that renders a plan with one property dropped |
| `_type` preserved at a polymorphic slot | `TestDecodePolymorphicSlotWithTypeLast` (`field_order_test.go:51`) and PROBE-038. Mutation: narrow the slot value to the declared interface's zero concrete before re-encode |
| REQ-025 nil receiver still refused | `openehr/rm/typereg/nilreceiver_census_test.go` enumerates `json.Unmarshaler` implementations at `:71` and `:93`. Under `UnmarshalerFrom` that census silently covers nothing. Make it interface-agnostic in this task, and prove it by adding a type that implements neither: the census must report it |

**Rollback:** revert the two generator files and run `make codegen`. The generated tree is reproducible and `make codegen-verify` proves it (`Makefile:192`).

#### 2.2 `canjson` moves its entry points to v2

`openehr/serialize/canjson/marshal.go:29,43` and `decode.go:98,112,132` are the four public entry points, roughly ten lines. `MarshalIndent` becomes `json.Marshal(v, jsontext.WithIndent(indent), jsontext.WithIndentPrefix(prefix))`; the `Decoder` wraps a `jsontext.Decoder`. The public API, the three sentinels and their `errors.Is` behaviour are unchanged. The package doc (`openehr/serialize/canjson/doc.go`) and the error-type prose at `decode.go:25,91,125,129` and `doc.go:67,101` are rewritten to match the amended REQ-052.

**Correction (ruling R23).** The `MarshalIndent` expression above is literal and would panic: `jsontext`'s option validation rejects an indent or prefix containing any byte other than space or tab, where v1 accepted any byte. The shipped code guards the indent and prefix first and returns `canjson.ErrInvalidValue` before building the `jsontext` options, so a caller who previously relied on v1 accepting an arbitrary indent (for example `"\n"`) now gets an error instead of a panic. REQ-025 (no panics in library code) and the unchanged-public-API constraint outrank the plan's literal expression.

Error classification, restated for v2:

| SDK classification | Attached by | v1 cause | v2 cause |
|---|---|---|---|
| `canjson.ErrInvalidValue` (encode only) | `canjson.Marshal` at `marshal.go:28-34` | `*json.UnsupportedTypeError` | `*json.SemanticError` (NaN, a channel) or `*jsontext.SyntacticError` (invalid UTF-8 on output) |
| `canjson.ErrInvalidShape` (decode only) | `typereg.WrapShapeError` (`openehr/rm/typereg/errors.go:151`), now called once from the shared helper | `*json.UnmarshalTypeError` | `*json.SemanticError` |
| Outside both sentinels: malformed JSON | nothing attaches | `*json.SyntaxError` from `Unmarshal`, `io.ErrUnexpectedEOF` or `io.EOF` from `Decoder.Decode` | `*jsontext.SyntacticError` in every case, from both entry points. This is the clause `wire.md:121` states and Phase 0.2 rewrites |
| `typereg.ErrUnknownType`, `typereg.ErrMissingType` | the interface hook | unchanged | unchanged; `errors.Is` reaches them through v2's wrap |
| `typereg.DecodeError` (re-exported as `canjson.DecodeError`) | the interface hook | hand-built `Path` | still SDK-built; `Path` may be read from `json.SemanticError.JSONPointer` instead of built by hand |
| New: duplicate member name | `canjson` classifies it explicitly | not reachable, v1 kept the last | `*jsontext.SyntacticError` wrapping `jsontext.ErrDuplicateName`, raised before any SDK code runs. Mapped to `canjson.ErrInvalidShape` per Phase 0.2 |
| New: invalid UTF-8 or lone surrogate | `jsontext`, before `rm.Character` sees it | refused by `rm.Character` with `ErrInvalidShape` | `*jsontext.SyntacticError`. Whether the SDK re-classifies it or accepts the bare refusal is Q4 |

The regression net is already in the tree: `openehr/serialize/canjson/marshal_sentinel_test.go:21-56` (`assertEncodeRefusal`), `TestDecodeFailureDoesNotCarryErrInvalidValue` (`marshal_sentinel_test.go:85`), `openehr/serialize/canjson/decode_test.go` (49 sentinel references, including the `Unmarshal` versus `Decoder.Decode` divergence at `:482-520` and the malformed-JSON no-sentinel assertions at `:494` and `:518`), and `openehr/serialize/internal/poly/poly_test.go` with `openehr/rm/typereg/registry_test.go` (23 references).

| Guard | Can-fail control |
|---|---|
| Encode failure wraps `ErrInvalidValue` and the cause stays reachable | Replace `fmt.Errorf("%w: %w", ErrInvalidValue, err)` at `marshal.go:31` with `%w: %v`. The `errors.AsType` for the codec cause must go red. `errorlint` flags the same mistake. `marshal_sentinel_test.go:35` asserts `*json.UnsupportedTypeError` concretely and is rewritten to `*json.SemanticError` |
| The encode sentinel never appears on a decode path | Add `ErrInvalidValue` to the decode wrap in `decode.go`; `TestDecodeFailureDoesNotCarryErrInvalidValue` must go red |
| Shape failures wrap `ErrInvalidShape` | Delete the `typereg.WrapShapeError` call from the shared helper; the per-type cases in `decode_test.go` must go red |
| Malformed JSON carries no sentinel | Make the malformed-input path attach `ErrInvalidShape`; both the "no sentinel" assertion and the type assertion at `decode_test.go:494` and `:518` must fail. Those two sites also assert `*json.SyntaxError` and are re-pinned to `*jsontext.SyntacticError` |
| A duplicate member name is refused and carries `ErrInvalidShape` | Set `jsontext.AllowDuplicateNames(true)`; the new refusal test must go red. Assert the operation-specific facet (the duplicate-name cause, `errors.Is` against `jsontext.ErrDuplicateName`), not only the shared sentinel |
| `Unmarshal` and `Decoder.Decode` still diverge on an empty stream | `decode_test.go:482-520`. v2's whole-input error for `""` is `jsontext: unexpected EOF`, not `unexpected end of JSON input`. Re-pin the strings and types, keep the divergence assertion itself |

**Rollback:** revert `marshal.go` and `decode.go` plus the test edits.

#### 2.3 Delete `openehr/internal/jsonpoly`

The package (214 lines with its test) and its 177 generated call sites go. Before deleting, move its assertions into `openehr/serialize/canjson/polymorphic_encode_test.go` as a table with two rows: the slot holding `DVCodedText{...}` by value and `&DVCodedText{...}` by pointer.

| Guard | Can-fail control |
|---|---|
| A concrete value in an interface slot still carries `_type` | Encode the slot through v1 `json.Marshal` instead of the v2 entry. The **value** row must go red while the pointer row stays green. That asymmetry is the operation-specific facet; a test that only checks the pointer row would pass in both worlds |
| A nil interface still yields `null` under a mandatory field and is omitted under `omitempty` | `openehr/internal/jsonpoly/jsonpoly.go:29-33` documents the behaviour today; Brief C confirmed a nil `jsontext.Value` in a mandatory field encodes as `null` under v2. Pin it as a third row in the same table |

`docs/specifications/traceability.yaml:140` (`packages:`) and `:154` (`tests:`) drop their `jsonpoly` entries in this task, or `make spec-check` fails on a missing path.

**Rollback:** `git revert`. The package is self-contained and its call sites are generated.

#### 2.4 `openehr/rm/typereg` moves to v2

`openehr/rm/typereg/registry.go:151` uses `json.NewDecoder` for the concrete-type decode. It becomes a v2 decode with the option set threaded through, because leaving the dispatch hub on v1 would decode every nested value under different escaping and matching rules than the enclosing type. The public surface (`Registry.Decode`, `DecodeAs`, the `Err*` family, `DecodeError`) is unchanged.

| Guard | Can-fail control |
|---|---|
| The `_type` peek refuses a non-object and a missing discriminator | Return `nil` instead of `ErrMissingType` from the empty-`head.Type` arm at `registry.go:143`; the case in `registry_test.go` must go red |
| The depth guard still bites | `maxDecodeDepth = 512` (`registry.go:25`) is stricter than `jsontext`'s own limit, which Brief A measured at 10 000 nesting levels, and is kept deliberately. Raise it; the `ErrMaxDepthExceeded` case must go red |
| Unknown `_type` still reaches `ErrUnknownType` | PROBE-031 (`docs/specifications/conformance.md:586`) and `registry_test.go`. Unchanged by the migration, kept as a witness |

#### 2.5 The hand-written RM primitives

`openehr/rm/character.go:159,180,205`, `integer.go:32,45,54` and `real.go:130,147,159` attach `typereg.ErrInvalidShape` over the codec cause; the cause type flips. Nine call sites. `character.go:152-155`'s comment about silent U+FFFD substitution becomes untrue and is rewritten with the Q4 ruling.

Three behaviours are codec-independent and stay hand-written:

- **Quoted-number tolerance (ADR 0004).** `real.go:128` and `integer.go:30` sniff `b[0] == '"'`. v2's `json.StringifyNumbers` cannot replace them: it is symmetric, and ADR 0004 requires strict encode against permissive decode (`docs/adr/0004-numeric-wire-tolerance.md:27-28`). Can-fail control: delete the quoted arm; `openehr/rm/quoted_literal_test.go` and `real_test.go` must go red with a shape error rather than a value.
- **The 17-significant-digit budget.** `maxSignificantDigits` (`real.go:42`), `significantDigits` (`:51`), `errPrecisionLoss` (`:93`). v2 has no equivalent and honours the custom decoder; Brief C confirmed `1e400` still reports out of range, only the error type differs.
- **The one-rune `Character` rule.** `characterFault` (`character.go:62`), shared by four entry points including the XML path, which has no JSON sentinel.

**Rollback for 2.4 and 2.5:** revert one and three files respectively, plus their tests.

### Phase 3: probes, assertions, goldens and the re-pins

#### 3.1 The two probe implementations

`testkit/probes/serialize` already owns the comparison mechanism, in the same Go package as PROBE-030: `decodeNumberMap` (`testkit/probes/serialize/probe_076_simplified_round_trip.go:183`) decodes to `map[string]any` with `json.Number`, and `flatMapsEqual` (`:171`) compares with `reflect.DeepEqual`. Go compares maps by key, so recursive member sorting is free and no sort pass is needed, and the `json.Number` decision is already made and documented there: comparing through `float64` would round integers above 2^53 on both sides and mask a regression of exactly the precision guarantee the codec documents. Generalise that pair under a name that is not FLAT-specific, `wireEquivalent(a, b []byte) bool`, and point `flatMapsEqual`, `flatDiff` (`probe_089_underscore_round_trip.go:833`) and the new PROBE-030 at it.

PROBE-030's implementation (`testkit/probes/serialize/probe_030_canjson_round_trip.go`) becomes decode, encode, decode, encode, decode, with `reflect.DeepEqual` between the two decoded values that straddle the second encode, `validation.ValidateRM` on the last one, and `wireEquivalent` as the secondary check. Its doc comment at line 39 ("guarantee for hashing, signing, and diff tooling") is rewritten. No RM-aware comparison options are needed: `reflect.DeepEqual` follows pointers and compares an interface by dynamic type plus value, which is what the `<Parent>Like` interfaces (`wire.md:145`), the `EVENT` interface (ADR 0003) and `DVInterval[DVOrdered]` require, and the one behaviour that must not be relaxed, nil-versus-empty slice discrimination, is the behaviour REQ-112 reads. Adding `go-cmp` would put a new module requirement in `go.mod` for no capability the standard library lacks here; neither `go-cmp` nor `testify` is a dependency today.

PROBE-038's implementation is already order-insensitive and needs no code change; only its catalog wording moves (Phase 0.5).

#### 3.2 The byte-equality census

Sixty-six assertion sites were swept (`bytes.Equal`, string comparison against a JSON literal, `strings.Contains` and `HasPrefix` against a member sequence, golden `.json` files, and the verified example transcripts). Thirty-six are already semantic or order-insensitive and stay. The thirty that change are below, one line each. The AQL text goldens are excluded by construction (REQ-055 canonicalisation, not JSON), as are canonical XML (REQ-056 keeps its element-order MUST and PROBE-033 its byte assertion), generated Go source determinism, OPT and template bytes, and the HAR cassettes, which are replayed on a method-and-path key and never body-compared (`testkit/probe/replay.go:94`, `:50-54`).

Class (i-a), byte-identical double-encode of separately decoded trees:

| # | Site | Change |
|---|---|---|
| 1 | `testkit/probes/serialize/probe_030_canjson_round_trip.go:83` | `bytes.Equal(b1, b2)` becomes the semantic oracle of 3.1 |
| 2 | `openehr/serialize/canjson/roundtrip_test.go:93` | `TestRoundTripStableSimpleValues`: typed deep equality plus `wireEquivalent` |
| 3 | `openehr/serialize/canjson/roundtrip_test.go:166` | `TestRoundTripCassettes`, PROBE-030's package-level twin: same rewrite over the whole vendored corpus |
| 4 | `openehr/templatecompile/external_test.go:126` | `bytes.Equal(second, third)`, "canonical form not idempotent": `wireEquivalent` |
| 5 | `openehr/instance/polymorphic_roundtrip_test.go:75` | `bytes.Equal(data, again)`, commented "Byte-stability (sub-gap A)": typed deep equality |
| 6 | `openehr/serialize/simplified/roundtrip_test.go:39` | `TestDecodeIdempotent`, eight runs. Purpose is semantic (JSON **array** element order is meaningful, unlike member order) and survives unchanged: `wireEquivalent` sorts members, never array elements |

Class (i-b), member order, `_type` position or an exact encoded spelling:

| # | Site | Change |
|---|---|---|
| 7 | `openehr/serialize/canjson/field_order_test.go:40` | two encodes of two differently-ordered inputs: typed deep equality plus `wireEquivalent` |
| 8 | `openehr/serialize/canjson/field_order_test.go:43` | the full literal `DV_CODED_TEXT` spelling: withdraw, it is the profile MUST in literal form |
| 9 | `openehr/serialize/canjson/field_order_test.go:78` | keep the `_type`-first half as the SHOULD witness, drop the `bytes.Equal(ea, eb)` half |
| 10 | `openehr/serialize/canjson/field_order_test.go:104` | `author` and `other_details` key order: keep, it witnesses the `Hash` determinism property, and widen per Phase 1.3 |
| 11 | `openehr/serialize/canjson/field_order_test.go:132` | two encodes of one value: keep, this is encoder determinism, not member order |
| 12 | `openehr/serialize/canjson/field_order_test.go:135` | `HasPrefix(first, '{"_type":"DV_CODED_TEXT",')`: keep as the `_type`-first SHOULD witness at a root |
| 13 | `openehr/serialize/canjson/encode_test.go:21` | `HasPrefix(got, '{"_type":"DV_QUANTITY"')`: keep as a SHOULD witness |
| 14 | `openehr/serialize/canjson/encode_test.go:92` | `HasPrefix(s, '{"_type":"COMPOSITION"')`: keep as a SHOULD witness |
| 15 | `openehr/serialize/canjson/encode_test.go:95` | `Contains(s, '{"_type":"OBSERVATION"')`: keep, `_type` first inside a nested object |
| 16 | `openehr/serialize/canjson/encode_test.go:98` | `Contains(s, '{"_type":"PARTY_SELF"')`: keep, same |
| 17 | `openehr/serialize/canjson/polymorphic_encode_test.go:32` | `Contains(js, '"name":{"_type":"DV_CODED_TEXT"')`: keep, `_type` first at a substitutable slot |
| 18 | `openehr/serialize/canjson/polymorphic_encode_test.go:35` | `Contains(js, '"defining_code":{"_type":"CODE_PHRASE"')`: keep, same |
| 19 | `openehr/serialize/canjson/marshal_sentinel_test.go:66` | `string(got) == '{"a":1,"b":2}'` on a `map[string]int`: fails outright under v2 defaults, passes with `json.Deterministic(true)`; keep as a determinism witness |
| 20 | `openehr/serialize/canjson/mappings_presence_test.go:63` | `string(out) == '{"_type":"DV_TEXT","value":"x"}'`: rewrite as key-set equality, the collapse it pins is the point, not the spelling |
| 21 | `openehr/rm/character_test.go:58` | asserts the encoder emits `"match":"\u003c"`: fails under v2 defaults, and is re-goldened to `"match":"<"` per the escaping ruling |
| 22 | `openehr/instance/instance_test.go:460` | `Contains(b, '"uid":{"_type":"HIER_OBJECT_ID"')`: keep, and fix the comment, which claims stability against a field-order convention that no longer exists |
| 23 | `openehr/internal/jsonpoly/jsonpoly_test.go:46` | retires with the package (Phase 2.3); the value-in-interface assertion is re-homed first |
| 24 | `openehr/internal/jsonpoly/jsonpoly_test.go:56` | same |
| 25 | `openehr/internal/jsonpoly/jsonpoly_test.go:77` | same |
| 26 | `openehr/internal/jsonpoly/jsonpoly_test.go:110` | same |
| 27 | `openehr/internal/jsonpoly/jsonpoly_test.go:131` | same, the nil-interface row |

Class (ii), golden files compared byte for byte:

| # | Site | Change |
|---|---|---|
| 28 | `testkit/probes/versioned/probes_test.go:673` | `testkit/probes/versioned/testdata/probe_084_built_body.json` is canjson output: re-golden with `go test ./testkit/probes/versioned/ -run TestProbe084BuiltBodyGolden -update` (`probes_test.go:660`). Besides order it pins which optional keys are emitted at all, so the `omitzero` sweep moves it |
| 29 | `openehr/template/webtemplate/golden_test.go:70` | the six `openehr/template/webtemplate/testdata/webtemplate/*.json`: unchanged, `webtemplate` stays on v1 |
| 30 | `openehr/template/webtemplate/golden_test.go:46` | two `webtemplate.Marshal` calls byte-equal: unchanged for the same reason; if that package ever moves it needs `json.Deterministic(true)` |

Four artefacts are re-goldened in total: `probe_084_built_body.json` (row 28), the two `internal/bmmgen/testdata/*.go.golden` files (Phase 2.1, by `cp`, no `-update` flag), and the byte count in the `compile-build-validate` transcript at `docs/examples.md:374` ("composition: 3162 bytes canonical JSON, round-tripped"), which `cmd/examples/transcripts_test.go` compares verbatim and which `cmd/examples/compile-build-validate/main.go:90` prints. The 204 JSON cassettes and the 3 HAR recordings are kept as they are: cassettes are decode inputs, and PROBE-030 compares two SDK encodes, never the input.

Rename `openehr/serialize/canjson/field_order_test.go` to `member_order_test.go` in the same change, so no file is still named after a withdrawn clause, and update `docs/specifications/traceability.yaml:135` to match.

#### 3.3 `transport` and the client leaves

No import changes here. `transport` keeps its v1 imports, and `transport.Decode` (`transport/client.go:654`) keeps calling `canjson.Unmarshal`. What changes is the identity of the cause travelling inside `*transport.DecodeError` and out through every `openehr/client/*` leaf that decodes through `canjson`.

| Guard | Can-fail control |
|---|---|
| A 2xx body that will not decode is a `*transport.DecodeError` carrying the raw bytes (REQ-151, [ADR 0018](../../adr/0018-raw-bytes-on-decode-error.md)) | `transport/decode_error_test.go`. Return the bare `canjson` error instead of building `&DecodeError{...}` at `transport/client.go:657`; the `Body` assertion must go red. `decode_error_test.go:137` asserts the wrapped type and is re-pinned to `*json.SemanticError` |
| An empty or `null` body is refused before decode | `transport/null_body_test.go` and `IsNoRepresentationBody` (`transport/body.go:18`). This matters more under v2, which always zeroes on a `null` where v1 sometimes no-ops (`json.MergeWithLegacySemantics` restores the old behaviour and is not used). Mutation: replace the call with `len(body) == 0`; the `null`-body case must go red |
| `CommitVersion` still accepts every generated version type | `openehr/client/ehr/contribution/submission.go:51` constrains on `json.Marshaler`. Decide in this phase whether the constraint moves to `json.MarshalerTo` or the generated types keep a thin `MarshalJSON` beside the streaming one, and pin the decision with a compile-time assertion in the contribution package |

#### 3.4 What stays on `encoding/json` v1, and what would move it

`encoding/json` v1 remains supported and is itself implemented on v2 in Go 1.27, so a split is coherent rather than a debt. There are 118 non-generated files importing `encoding/json`; only the five packages of Phase 2 move.

| Package | v1 reliance | Reason to stay | Later trigger |
|---|---|---|---|
| `openehr/serialize/simplified` | `flat_decode.go:708-709` and `datatypes.go:787-788` use `Decoder.UseNumber`; `flat_encode.go:29` and `structured.go:33,47,64,77` marshal a `map[string]any` and rely on sorted keys | v2 has no `UseNumber`, and a replacement is a per-kind unmarshaler with no fall-through, since `go doc -all encoding/json/v2` lists one package variable (`ErrUnknownName`) and no skip sentinel. The RM half already moves, through `canjson` at `flat_decode.go:80` and `datatypes.go:277,783` | a `UseNumber` shim with its own tests. Its ready-made can-fail control exists: `openehr/serialize/simplified/datatypes_test.go:512-531` asserts `9007199254740993` survives decode as a `json.Number`; swap the shim for a plain decode into `any` and it must go red. `json.Deterministic(true)` covers the encode half the same day |
| `openehr/client/definition`, `openehr/client/system` | `wire.md:424` makes v1 case-insensitive matching **normative** for `Extras`, pinned by `openehr/client/definition/stored_query_test.go:668,736`, `template_test.go` and `openehr/client/system/system.go:79,84,115,133` | moving breaks a stated MUST | only alongside an amendment to REQ-050 or REQ-144 § Unknown response keys |
| `openehr/aql` | `result.go:40,46`, the same `Extras` shape with no normative clause | consistency with the other two carriers | with `openehr/client/definition` |
| `openehr/bmm` | `jsonorder.go:25-51` reads wire key order through `Decoder.Token`, `json.Delim` and `Decoder.More`; `internal.go:23`; `type.go:173-303` | `jsontext.Decoder` has a different token API (`ReadToken`, `PeekKind`), and this is a build-time BMM loader with no wire role | none |
| `transport` (its own imports) | `client.go:497` best-effort error envelope; `headers.go:57` marshals a `map[string]string` into an HTTP header value | header bytes must not become order-random | if moved, `headers.go:57` needs `json.Deterministic(true)` and a test that two `HeaderJSON()` calls agree |
| `auth/*`, `smart/*` | `json.Number` for `expires_in`, `exp`, `iat` (`clientcreds.go:261`, `introspect.go:128-132`, `jwtbearer.go:193`, `smart/exchange.go:119`, `smart/idtoken.go:162,299`); `json.RawMessage` JWKS keys (`smart/jwks.go:24`) | identical behaviour under v2, and these interoperate with `go-oidc` and `oauth2`, which return v1 types | none |
| `openehr/template/webtemplate` | `webtemplate.go:105` | six byte-for-byte goldens and no wire role; its determinism contract is REQ-106 and ADR 0014's, not REQ-052's | if Web Template output ever becomes a wire contract |
| `openehr/validation` | `rmfloor_bytes.go:49,60,89` decodes into `map[string]json.RawMessage` to read key presence | identical behaviour, and REQ-013 keeps the package codec-independent | none |
| `testkit/*` | `testkit/conformance/webtemplate/runner.go:438-439`, `testkit/probes/serialize/probe_076_simplified_round_trip.go:184-185` and `probe_038_canjson_rm_polymorphic_decode.go:98-99` need `UseNumber`; `testkit/probe/har.go:200` | the same `UseNumber` blocker | with `openehr/serialize/simplified` |
| `cmd/*` | `probe-record/main.go:206` `MarshalIndent`; `webtemplate-export/main.go:68` and `contribution-build/main.go:201` `json.Indent` | presentation only | none |

#### 3.5 Benchmarks, before and after

The existing benchmarks are already on `b.Loop()` and `b.ReportAllocs()`: `BenchmarkEncodeComposition_400`, `BenchmarkDecodeComposition_400`, `BenchmarkEncodeDVQuantity` and `BenchmarkDecodeDVQuantity` (`openehr/serialize/canjson/bench_test.go:52,68,83,97`). There are none for `openehr/rm/typereg` or `openehr/serialize/simplified`, so four are added in Phase 1.3 and measured on both sides:

| New benchmark | Home | Input |
|---|---|---|
| `BenchmarkDecodeCompositionCassette` | `openehr/serialize/canjson/bench_test.go` | `testkit/cassettes/compositions/Demonstration.v1.json` (97 725 bytes, the largest real cassette), decode and encode, `b.SetBytes` on the input |
| `BenchmarkRegistryDecodeElement`, `BenchmarkRegistryDecodeDVQuantity` | new `openehr/rm/typereg/bench_test.go` | isolates the `_type` peek plus the concrete decode, which is where the option-threading cost lands |
| `BenchmarkFlatCorpusRoundTrip` | new `openehr/serialize/simplified/bench_test.go` | the 34 bodies under `testkit/cassettes/flat-conformance/compositions/`, decode then encode, one `b.Loop()` pass per corpus sweep |
| `BenchmarkMarshalFlat`, `BenchmarkUnmarshalFlat` | same | `ehrbase_conformance_party_related.json` (17 648 bytes, the largest FLAT body) |

Baseline measured at `d3e9d998` with `-benchtime 200x` on a 13th Gen i7-13700H:

```
BenchmarkEncodeComposition_400-12    2920117 ns/op  3250259 B/op   9283 allocs/op
BenchmarkDecodeComposition_400-12    6038604 ns/op  2767771 B/op  21665 allocs/op   35.94 MB/s
BenchmarkEncodeDVQuantity-12            1069 ns/op      357 B/op      6 allocs/op
BenchmarkDecodeDVQuantity-12            1515 ns/op      372 B/op      3 allocs/op   34.99 MB/s
```

Measurement procedure, on one machine with the same power profile and nothing else running. `benchstat` is not on the host today.

```
go install golang.org/x/perf/cmd/benchstat@latest

git switch main
go test -run '^$' -bench . -benchmem -count=10 \
  ./openehr/serialize/canjson/ ./openehr/rm/typereg/ ./openehr/serialize/simplified/ \
  | tee /tmp/bench-v1.txt

git switch <branch>
go test -run '^$' -bench . -benchmem -count=10 \
  ./openehr/serialize/canjson/ ./openehr/rm/typereg/ ./openehr/serialize/simplified/ \
  | tee /tmp/bench-v2.txt

benchstat /tmp/bench-v1.txt /tmp/bench-v2.txt
```

Results, from Task 9's like-for-like `-count=10` run (v1 at `90473f9f`, v2 at `93d06e1f`, both measured back to back on the same 13th Gen i7-13700H, WSL2, `go1.27.1`). Both columns are `-count=10` at the default one-second benchtime, so they are directly comparable with each other; the plan's pre-filled v1 column above came from a separate `-benchtime 200x` run at `d3e9d998` and reads a little differently for that reason (for example `EncodeDVQuantity` 1069 ns/op there against 749.4 ns/op in the like-for-like run), so the table below supersedes it. The Delta column is time/op (`sec/op`), the metric ruling R26 judges.

| Benchmark | v1 ns/op | v2 ns/op | v1 allocs/op | v2 allocs/op | Delta |
|---|---|---|---|---|---|
| `BenchmarkEncodeComposition_400` | 2 936 000 | 1 532 000 | 9 281 | 16 444 | -47.82% |
| `BenchmarkDecodeComposition_400` | 5 998 000 | 6 536 000 | 21 665 | 16 860 | +8.98% |
| `BenchmarkEncodeDVQuantity` | 749.4 | 828.2 | 6 | 9 | +10.52% |
| `BenchmarkDecodeDVQuantity` | 863.4 | 1 190.5 | 3 | 5 | +37.88% |
| `BenchmarkDecodeCompositionCassette` | 5 293 000 | 3 266 000 | 6 279 | 5 220 | -38.30% |
| `BenchmarkRegistryDecodeElement` | 5 520 | 6 284 | 29 | 39 | +13.84% |
| `BenchmarkRegistryDecodeDVQuantity` | 1 652 | 1 962 | 9 | 18 | +18.80% |
| `BenchmarkFlatCorpusRoundTrip` | 12 640 000 | 12 860 000 | 68 400 | 69 940 | +1.77% |
| `BenchmarkMarshalFlat` | 128 200 | 134 300 | 1 103 | 1 193 | +4.73% |
| `BenchmarkUnmarshalFlat` | 650 400 | 667 900 | 2 962 | 2 884 | +2.69% |

**R26 verdict, one line per benchmark** (material regression: v2 time/op worse than v1 by more than 20 percent at p < 0.05; every p-value below is 0.000 to 0.007, all below the 0.05 bar):

- `BenchmarkEncodeComposition_400`: within threshold (-47.82%, an improvement).
- `BenchmarkDecodeComposition_400`: within threshold (+8.98%).
- `BenchmarkEncodeDVQuantity`: within threshold (+10.52%).
- `BenchmarkDecodeDVQuantity`: **material regression** (+37.88%). The smallest decode payload, where the fixed per-call overhead of the `DecodeInto` buffer-peek-unmarshal path (three passes per nesting level) dominates; allocations rise from 3 to 5. Named follow-up, not a codec change: read the discriminator from the declared wire field instead of `peekType`.
- `BenchmarkDecodeCompositionCassette`: within threshold (-38.30%, an improvement).
- `BenchmarkRegistryDecodeElement`: within threshold (+13.84%).
- `BenchmarkRegistryDecodeDVQuantity`: within threshold (+18.80%).
- `BenchmarkFlatCorpusRoundTrip`: within threshold (+1.77%).
- `BenchmarkMarshalFlat`: within threshold (+4.73%).
- `BenchmarkUnmarshalFlat`: within threshold (+2.69%).

The number the plan flagged to watch went the intended direction: `BenchmarkDecodeComposition_400` allocations dropped from 21 665 to 16 860 (-22.18%) and its bytes from 2 703 KiB to 1 016 KiB (-62.40%), the streaming pair removing the intermediate `[]byte` at each marshal and unmarshal boundary as designed. The large real-payload paths improve on every axis at once: `EncodeComposition_400` -47.82% time and -57.12% bytes; `DecodeCompositionCassette` -38.30% time, -88.01% bytes and 62.11% higher throughput. The cost lands entirely on the tiny leaf values, where one benchmark of ten crosses the R26 line: `BenchmarkDecodeDVQuantity`, +37.88 percent, a follow-up rather than a reason to reopen the codec choice.

### Phase 4: close-out

| Task | Detail |
|---|---|
| Roadmap row | `docs/roadmap.md:59` still reads "Deterministic encode profile as the SDK's own output contract, order-agnostic decode, and byte-stable `_type` round-trips". Rewrite it: the codec is `encoding/json/v2`, `_type` first is a recommendation, decode is order-agnostic, and round-trip fidelity is asserted semantically (PROBE-030/031/038). The `Real` 17-significant-digit sentence in the same row is unchanged. This row is one of the two indexes `make spec-check` cannot see |
| REQ.md Impl. column | `docs/specifications/REQ.md:51` stays `landed` (Phase 0.10). REQ-040 and REQ-053 stay `landed`. No numbering band moves: this plan allocates no new REQ id, so the band table in [REQ.md § Numbering policy](../../specifications/REQ.md#numbering-policy) is untouched, which is the second index the gate cannot see |
| STRAND-04 | The codec sub-question is marked resolved by ADR 0021 and ADR 0022 (Phase 0.8). The strand stays **Partially resolved**: the full RM inventory and validation independence are untouched |
| CHANGELOG | Queue the `### Changed` bullet from the Definition of Done for the release that ships this. Following the live precedent at `CHANGELOG.md:11`, which already carries a `### Changed` section under `## [Unreleased]`. `AGENTS.md:67` says pre-1.0 entries are `### Added` only; that wording question is a maintainer item outside this plan, and `AGENTS.md` is not edited here |
| Consumer note | Downstream callers need the four facts the CHANGELOG bullet compresses: encoded bytes differ from v0.27.x and any stored hash or byte-compared snapshot of SDK output must be recomputed; the SDK's own sentinels keep their identity and `errors.Is` behaviour while the standard-library error types beneath them change; decode is stricter on invalid UTF-8, a lone surrogate escape and duplicate member names; and the packages that did not move, listed in Phase 3.4, behave as before with one exception: a v1 caller decoding an RM type (for example `validation.ValidateRMEHRStatusBytes`) now refuses a duplicate member name, because the RM decode methods reject duplicates by default, while keeping the case-insensitive member matching a v1 caller gets (`decodeOptions` joins the caller's own options first) |
| Archive | Move this plan to [`docs/plans/archive/`](archive/) and add its row to `archive/README.md`, per `sdd-archive`, inside the implementing PR. Remove it from the Active plans table in [`README.md`](README.md) |

**Verification:** `make spec-check` prints `spec-check: OK`, and `make ci` passes. `make ci` runs `fmt-check mod-tidy-check vet test lint spec-check flat-conformance-verify terminology-verify build` (`Makefile:428`), and `make test` runs `codegen-verify` first (`Makefile:192`), so any generator change not followed by `make codegen` fails the gate immediately.

### Close-out facts

- **Removed line counts.** Task 4's generator rewrite and regeneration (`90473f9f..cc2ceaf2`) net -7 629 lines: 131 files changed, 6 256 insertions, 13 885 deletions. The `make codegen` regeneration commit alone removed 13 136 lines across 104 files (110 rm and 29 aom `MarshalJSON`/`UnmarshalJSON` method pairs and their generator templates), including the 214-line `openehr/internal/jsonpoly` package (`jsonpoly.go` 80 lines, `jsonpoly_test.go` 134 lines). Task 6 re-homed `jsonpoly`'s test assertions into `openehr/serialize/canjson/polymorphic_encode_test.go` (1 file, +186/-2) once Task 4 had already deleted the package; no further deletion was needed there.
- **Value-receiver fact.** `MarshalJSONTo` is generated with a value receiver (ADR 0002 D8, ADR 0022) because v1 entry points skip a pointer-receiver marshal method on an unaddressable value obtained from an interface (`json.CallMethodsWithLegacySemantics`, the default for `encoding/json.Marshal`); v2 has no such restriction either way. Without the value receiver, a concrete value (not a pointer) held in a `DVTextLike`-typed field would silently drop its `_type` under a v1 caller, reproducing the exact defect `jsonpoly` existed to route around.
- **Work outside the plan's scope, exposed by the probe rewrite.** Task 8's PROBE-030 rewrite (Phase 3.1) surfaced a pre-existing gap in `openehr/validation/rmread`: `readActionSingle` read only the ENTRY-level attributes, so `ValidateRM` reported `ACTION.time` and `ACTION.ism_transition` (both RM-mandatory) as absent on every well-formed ACTION. `openehr/validation` is outside this plan's Phase 2 scope, but the fix is three switch arms in one file (`read.go`) with its own table test, so it landed in Task 8's fix round (ruling R27) rather than being routed around with a per-cassette exclusion list.

### Rulings recorded during implementation (Tasks 4 to 9)

Rendered in this plan's own shape: what, why, cost if wrong. Full detail and evidence sit in each task's own report; this transcribes the substance the maintainer needs on the plan itself.

- **R19, hybrid generator shape:** classes with no marshaler-bearing embedded ancestor get the zero-copy alias; the 14 that embed one (nine direct embeds plus five `Versioned*[T]` wrappers) get a flat wire struct built from `effectiveFields`, because the alias would otherwise promote the ancestor's `_type` and drop the subclass's fields. Pinned by a registered-type census (`canjson/type_census_test.go`) marshalling every registered type's zero value and checking the wire discriminator. Cost if wrong: a second generator template to maintain; reverting to a uniform flat shape is a generator-only change plus `make codegen`.
- **R20, `jsonpoly` deletion timing:** Task 4's generator rewrite deleted `openehr/internal/jsonpoly` as part of the same change (the generator no longer emits its call sites, so it became dead code inside that task), narrowing Task 6 to re-homing the test table rather than performing the deletion itself. Cost if wrong: none; the deletion is what Task 6 would have done anyway.
- **R21/R26, the `DecodeInto` three-pass structure and the benchmark threshold:** `DecodeInto` buffers, peeks and unmarshals each subtree, three passes per nesting level; this was deferred to Task 9's benchmarks rather than fixed inline. A benchmark counts as a material regression when v2 time/op is worse than the v1 baseline by more than 20 percent at p < 0.05; only `BenchmarkDecodeDVQuantity` (+37.88 percent) crosses that line, and it becomes a named follow-up (read the discriminator from the declared wire field instead of `peekType`) rather than a fix in this plan. Cost if wrong: a slower small-value decode ships until the follow-up lands; the threshold itself is a judgment call the maintainer can tighten with the measured numbers in hand.
- **R22, two CHANGELOG bullets:** the Go type-surface break (`MarshalerTo`/`UnmarshalerFrom` replacing the v1 pair, `*JSONMarshaller` wire types removed, `CommitVersion` requiring `json.MarshalerTo`, map-held RM values carrying `_type`) is a separate artefact class from the wire-bytes bullet the Definition of Done already names, so it gets its own one-sentence `### Changed` bullet rather than being folded into the first. Cost if wrong: the maintainer deletes one bullet at release cut.
- **R23, `MarshalIndent` refuses instead of panicking:** the plan's literal Phase 2.2 expression (`json.Marshal(v, jsontext.WithIndent(indent), jsontext.WithIndentPrefix(prefix))`) would panic on a non-space/tab indent or prefix, because `jsontext`'s option validation is stricter than v1's. The shipped code guards first and returns `canjson.ErrInvalidValue`. Cost if wrong: a consumer who relied on v1 accepting an arbitrary indent byte now gets an error instead of a panic, which is the safer failure mode.
- **R24, the wire.md TERM_MAPPING.match sentence:** the tokenizer's refusal of invalid UTF-8 and a lone surrogate escape is malformed input carrying no SDK sentinel, cross-referenced to the malformed-JSON exclusion bullet; refusals `rm.Character` itself raises keep `ErrInvalidShape`. Amended in Task 5's fix round rather than deferred, because the spec must not contradict a green test on the branch for a whole task. Cost if wrong: one sentence re-edited by a later task.
- **R25, controller-applied mechanical fixes:** punctuation-only review findings (stray em dashes in comments, a commit subject misattributing work another task already did) were applied directly by the controller in the worktree and verified deterministically (grep, `gofmt`, a focused test run), instead of a full fix-round dispatch and re-review. Cost if wrong: none material; the final whole-branch review sees the file again.
- **R27, the ACTION reader fix replaces a per-cassette exclusion list:** PROBE-030's floor-gate exclusion list (three cassettes) was replaced by fixing the gap it worked around (`readActionSingle` gains `time`, `ism_transition` and `instruction_details`, with a table test), so the probe gates every ACTION-bearing cassette instead of skipping the ones that would have failed. One genuine finding survives as a floor-leg-only hold-out: `clinical_notes.v0.json` carries an empty `action_archetype_id` in vendored content, not something this plan edits. Cost if wrong: a test pinning a specific finding count on an ACTION-bearing cassette may need re-pinning; the risk was judged small (three switch arms, one file) against the alternative of a probe that gates only the cassettes that already pass.
- **R28, the ACTION reader fix gets its own CHANGELOG bullet:** because it changes `ValidateRM`'s consumer-visible findings (an ACTION no longer reports two mandatory attributes as missing), it is not folded silently into another bullet. Cost if wrong: the maintainer deletes one bullet at release cut.

The final whole-branch review added five rulings, recorded here for the same reason as the ones above.

- **R29, the three CHANGELOG bullets carry the upgrade-facing facts:** the `### Changed` section stays three bullets, each one sentence and at most 35 words after the bold lead, together carrying wire bytes and decode strictness (bullet 1, citing ADR 0022), the Go surface and encode refusals (bullet 2), and the ACTION reader fix (bullet 3); the error-identity, omitzero, map-held `_type`, invalid-UTF-8 and `MarshalIndent` facts were distributed across those three rather than adding a bullet. Cost if wrong: the maintainer edits a bullet at release cut.
- **R30, the duplicate-name sentinel binds the registry route too:** `typereg.Registry.Decode` classifies a duplicate member name as `ErrInvalidShape` at both failure sites through a shared `typereg.ClassifyDuplicate` gate, because `typereg.DecodeAs` is a live consumer route (`openehr/client/demographic`) and REQ-052's MUST binds every canonical-JSON decode route; `canjson.classifyDecode` keeps its own application of the gate, because it decodes non-RM targets that never reach the registry, so the registry gate alone would not cover them. Cost if wrong: a consumer branching on the sentinel over a duplicate-name refusal on the `DecodeAs` route would miss it.
- **DecodeAs decision (part of R30):** `registry_test.go` had pinned the opposite (a registry duplicate carrying `jsontext.ErrDuplicateName` but NO `ErrInvalidShape`) as a deliberate implementation choice, not a ruling; that assertion is flipped and a nested-duplicate assertion through `DecodeAs` is added, so the registry and canjson routes now refuse a duplicate the same way. Cost if wrong: the flipped test would need re-pinning if the decision were reversed.
- **R31, the flat wire structs are unexported:** `flatWireTypeName` returns `jsonWire<GoName>` in place of `<GoName>JSONWire`, so the 14 flat wire structs leave `openehr/rm`'s public surface; the names never shipped, so no CHANGELOG entry is needed. Cost if wrong: a generator-only rename plus `make codegen`.
- **R32, PROBE-038 gains the wire-equivalence leg its catalog states:** after the polymorphic decode and re-marshal, the probe decodes the re-marshalled bytes and encodes once more, asserting `wireequiv.Equivalent`, which the conformance.md Wire assertion already promised; all three polymorphic cassettes stay green, so none is held out. Cost if wrong: a cassette that failed the leg would need a named hold-out, never a weaker leg.
- **R33, the compositionJSONExcluded byte-stability rationale is withdrawn:** `testkit/fixtures/discover.go`'s seven-cassette exclusion cites a byte-stability rationale the semantic probe no longer requires; re-evaluating it is a follow-up recorded below, not part of this fix wave. Cost if wrong: seven cassettes stay excluded from a leg they could now feed.

### Follow-ups

- RESOLVED in the final fix wave. The generated decode helper (`openehr/rm/typereg/streaming.go`) now memoises the caller-hook join on the last (aggregate, caller) pointer pair only, so a repeat of one caller hook set (sibling values at a nesting level, and a later decode reusing the same `WithUnmarshalers` value) reuses that join, while a cold decode still joins once per nesting level, because each level's options carry the previous level's joined pointer as that level's caller. It retains no earlier caller's pointer, and treats `WithUnmarshalers(nil)` as no caller. A white-box counter (`callerJoinCount`) pins that a repeated caller hook set reuses its join, and a single-entry memo test pins that two callers alternating join on every call.
- PROBE-038's new wire-equivalence leg (ruling R32) has no can-fail control yet.
- Pre-existing em dashes remain in `openehr/serialize/canjson/decode_test.go` and `openehr/serialize/canjson/doc.go` passages no task in this plan rewrote.
- The CONTRIBUTION decode gap (ruling R17): `versions[0]` into `*rm.OriginalVersion[interface{}]` fails to resolve a concrete type, so 46 `submissions/*` cassettes are refused by both `encoding/json` and `encoding/json/v2` alike. Pre-existing and outside this plan; both codecs agree, so it is not a parity problem, but nothing else in the suite names it.
- `BenchmarkDecodeDVQuantity` is a material regression under ruling R26 (+37.88 percent time/op): read the discriminator from the declared wire field instead of `peekType`, which would remove the extra buffer-peek-unmarshal pass on the smallest decode payloads.
- `clinical_notes.v0.json` (vendored content) carries an empty `action_archetype_id` and is held out of PROBE-030's floor leg only (the polymorphic round-trip leg still runs it); the RM floor genuinely cannot validate that cassette as it stands.
- The `compositionJSONExcluded` list in `testkit/fixtures/discover.go` (with the "not byte-stable" note at `bench_test.go:128`) holds seven cassettes out of the composition corpus on a byte-stability rationale, that the canonical-JSON round trip must pass byte-for-byte. PROBE-030 and PROBE-038 now assert round-trip fidelity semantically (typed deep comparison plus wire-equivalence, never byte equality), so that byte-stability rationale is withdrawn and the exclusion needs re-evaluating under the semantic probes. Recorded by ruling R33; not part of this fix wave.

## Mapping to specs

- [wire.md § REQ-052](../../specifications/wire.md#req-052): the normative contract for canonical JSON (the member-order clause, the decode-side shape sentinel, the encode-side refusal sentinel, and the floating-point precision section)
- [wire.md § REQ-053](../../specifications/wire.md#req-053): FLAT and STRUCTURED, whose codecs stay on `encoding/json` v1 in this plan
- [rm-modeling.md § Type registry (REQ-040)](../../specifications/rm-modeling.md#type-registry-req-040): the registry the polymorphic dispatch hooks read
- [clinical-modeling.md § REQ-112](../../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor): the reference-model floor the rewritten PROBE-030 asserts
- [transport.md § REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure): the typed 2xx decode failure, re-pinned but not amended
- [conformance.md § PROBE-030](../../specifications/conformance.md#probe-030--canonical-json-round-trip) and [§ PROBE-038](../../specifications/conformance.md#probe-038--rm-polymorphic-decode-coverage): the two probes rewritten
- [research-strands.md § STRAND-04](../../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance): the strand whose codec sub-question this closes
- [ADR 0002](../../adr/0002-bmm-codegen-decisions.md): codegen policy, amended with D8
- [ADR 0003](../../adr/0003-rm-event-polymorphism.md), [ADR 0004](../../adr/0004-numeric-wire-tolerance.md), [ADR 0018](../../adr/0018-raw-bytes-on-decode-error.md): unchanged, and each named where the migration could have disturbed it
- [REQ.md](../../specifications/REQ.md): registry rows for REQ-040, REQ-052 and REQ-053
