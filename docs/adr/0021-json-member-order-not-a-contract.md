# ADR 0021 — Encoded JSON member order is not part of the canonical JSON contract

- **Status:** Accepted, 2026-09-14 (maintainer sign-off on the plan, PR #171); implementation landed under [2026-09-14-json-v2-migration.md](../plans/archive/2026-09-14-json-v2-migration.md).
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** retires the byte-stability premise in [STRAND-04](../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance)'s remaining evidence item.
- **Introduces:** —. **Amends:** [REQ-052](../specifications/wire.md#req-052) (the field-order profile and the probe obligation it carries).
- **Plan:** [2026-09-14-json-v2-migration.md](../plans/archive/2026-09-14-json-v2-migration.md).
- **Related:** [ADR 0022](0022-canonical-json-encoding-json-v2.md) (the codec decision this one unblocks); [REQ-056](../specifications/wire.md#req-056) (canonical XML, deliberately **not** relaxed: element order is part of an XML document's identity in a way member order is not part of a JSON object's, so PROBE-033 keeps its byte assertion); [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (the validation floor the rewritten PROBE-030 asserts).

## Context

`docs/specifications/wire.md:105` states a deterministic encode profile as a MUST: `_type` first, then BMM property declaration order, `Hash` keys lexicographic, with two successive SDK encodes byte-identical. Line 106 names the reason: "the SDK's own output contract, which a consumer may rely on for byte comparison and hashing". PROBE-030 asserts it (`testkit/probes/serialize/probe_030_canjson_round_trip.go:83`, `bytes.Equal(b1, b2)`), and the probe's own doc comment at line 39 calls it a "guarantee for hashing, signing, and diff tooling".

No openEHR specification asks for it. RFC 8259 § 4 assigns no meaning to JSON object member order, the openEHR specifications prescribe none, and CDR implementations differ in the order they emit. The decoder's obligation to accept any order, already stated at `wire.md:110`, is the real interoperability rule and is unaffected.

The promise has a price. It constrains the codec: any encoder the SDK adopts must reproduce one spelling for one value, which is the gate [STRAND-04](../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance) named at `research-strands.md:68`. It also shapes the conformance suite: a probe that compares two encodes tests the encoder's self-consistency, which passes even when the encoder drops the same field on both passes.

## Decision

**Encoded JSON member order is not part of the SDK's contract.**

- `_type` first becomes a **SHOULD**. It has a reason that survives the withdrawal: a consumer decoding as a stream can select the concrete type before reading the rest of the object.
- Beyond `_type`, member order is **unspecified**, and a consumer **MUST NOT** rely on it.
- The decoder **MUST** continue to accept members in any order, `_type` included.
- Conformance is asserted **semantically**: decoded values are compared, and the recovered value is passed through the reference-model floor ([REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)). Where a check on encoded form is useful it is wire-equivalence, defined once in the conformance catalog's Terms section. No probe asserts byte equality of encoded JSON.
- `Hash` (`map[K]V`) members are emitted in lexicographic key order as a **SHOULD**, re-stated as a determinism property of the encoder rather than an order promise to a consumer. Go randomises map iteration, so without it one unchanged value has no reproducible encoding at all. A unit test pins it; no probe does.

**Encoder determinism is a SHOULD, not a MUST.** The case for MUST was that six live assertions depend on one value encoding identically twice: `openehr/serialize/canjson/field_order_test.go:132`, `openehr/serialize/canjson/marshal_sentinel_test.go:66`, `openehr/client/ehr/contribution/builder_test.go:364` and `:373`, `openehr/instance/valuefill_test.go:70`, and `openehr/serialize/simplified/roundtrip_test.go:39`. That case was considered and set aside: none of the six is an interoperability obligation, they are the SDK's own tooling, and stating an internal implementation property as a normative MUST on the wire format would be the category error this ADR exists to correct. The decision is SHOULD.

## Consequences

- **A consumer that hashed, diffed or signed SDK output loses a guarantee it may have relied on.** Pre-1.0 this is a `### Changed` CHANGELOG entry and a minor bump (`CHANGELOG.md:7`), not a deprecation cycle. A consumer needing a stable digest computes it over a canonicalisation of its own choosing. The SDK does not ship one, because an SDK-blessed canonical form would be the same promise wearing a different name.
- **The probe suite gets stronger, not weaker.** A byte comparison of two encodes passes whenever the encoder is self-consistent, including when it drops a field on both passes. Typed deep comparison plus `ValidateRM` catches exactly that class. This is the substantive argument for the change, not its mitigation.
- **Two documented re-encode collapses become visible.** [REQ-052](../specifications/wire.md#req-052) records, in its `DV_TEXT.mappings` and `DV_MULTIMEDIA.data` re-encode-collapse clauses, that both lose the present-but-empty distinction on re-encode, so the decode of a cassette and the decode of its re-encode can legitimately differ. PROBE-030 therefore compares the values on either side of the *second* encode, where both derive from SDK output. The collapses themselves are unchanged.
- **Canonical XML is untouched.** [REQ-056](../specifications/wire.md#req-056)'s element-order profile and PROBE-033's byte assertion stand. The asymmetry is principled: element order is part of an XML document's identity in a way member order is not part of a JSON object's.
- **The withdrawal is one-way.** Restoring an order promise after consumers have absorbed its removal is not a cheap change, which is why this half of the work is the ADR-worthy fork and the codec swap (reversible, one funnel at `openehr/serialize/canjson/marshal.go:28`) is not.

## Alternatives considered

- **Keep the profile as a MUST and satisfy it under any future codec.** Technically reachable: `json.Deterministic(true)` plus a `_type`-first emission gives it. **Rejected:** it preserves a promise no openEHR specification asks for, constrains every future codec choice, and leaves the probe suite testing encoder spelling instead of round-trip fidelity. `go doc encoding/json/v2.Deterministic` also warns that determinism holds "across different instances of identical binaries, but not across different builds of a program (such as different source or toolchain version, different GOOS/GOARCH, different build flags)", so the restated promise would be weaker than the one being withdrawn while reading identical.
- **Keep the profile and publish a separate canonicalisation helper for consumers who hash.** **Rejected:** it is the withdrawn promise under a new name, and it puts the SDK in the business of defining a canonical form for a format whose specification defines none. RFC 8785 exists for consumers who need one.
- **Downgrade the whole profile to a SHOULD without touching the probes.** **Rejected:** the probes are where the promise is actually enforced. A SHOULD that PROBE-030 still asserts byte-wise is a MUST with softer wording.
