# Plan — SDK-GAP-13: polymorphic `_type` encode/decode symmetry

**Date:** 2026-06-23
**Status:** Proposed (analysis only — fix approach not yet chosen; do not implement without sign-off)
**Owner:** SDK maintainers
**Covers (candidate):** REQ-052 (canonical JSON), REQ-040 (type registry), REQ-102/107 (`openehr/validation`, `openehr/instance` / `composition.NewSkeleton`)
**Relates:** SDK-GAP-11 (decode-side polymorphism — landed; [archive/2026-05-26-rm-polymorphic-decode-coverage.md](archive/2026-05-26-rm-polymorphic-decode-coverage.md)) and the `*Like` ergonomics note ([archive/2026-05-27-rm-like-interface-ergonomics.md](archive/2026-05-27-rm-like-interface-ergonomics.md)); SDK-GAP-12 NewSkeleton ([archive/2026-06-19-sdk-gap-12-newskeleton.md](archive/2026-06-19-sdk-gap-12-newskeleton.md))
**Source (inbound):** a consuming CDR project — write-time template validation + benchmark; observed ~13% of the `NewSkeleton` corpus fails round-trip template validation (`Referral Request.v1`, `Demonstration.v1`), forcing its template validation to permissive (warn) mode instead of strict (reject).
**Reframe vs the inbound draft:** the draft treats this as one "encode-side `_type`" gap. Investigation (below) shows it is **two distinct defects with different fixes** — only the first is an encode bug; the second is decode/validator and the wire is already correct.

## Goal

Make a polymorphic RM value survive a canonical-JSON `Marshal → Unmarshal` round-trip with enough type fidelity that `validation.ValidateComposition` gives the same verdict before and after — so a CDR can persist `NewSkeleton`/`instance.Generate` output and re-decode it without spurious template-validation failures (and run strict / reject template validation).

## The 3-layer model this gap is judged against

openEHR canonical JSON (ITS-JSON) is **self-describing**; the responsibilities are layered, and the fixes must respect the layering:

1. **Producer / encode** (`canjson.Marshal`, fed by `NewSkeleton` / `instance.Generate`) — MUST emit `_type` whenever the runtime type is a *proper subtype* of the statically-declared slot type. Emitting the subtype's discriminator is mandatory, not optional.
2. **Codec / decode** (`canjson.Unmarshal` + `typereg`) — RM-faithful only: resolve the concrete type from `_type`, else fall back to the statically-declared type. It MUST NOT guess a subtype from the presence of properties, and MUST NOT become template-aware.
3. **Template validator** (`validation.ValidateComposition`) — the only template-aware layer; it decides whether the decoded RM value satisfies the OPT's node constraint (e.g. "this `name` must be `DV_CODED_TEXT`", "this interval must be `DV_INTERVAL<DV_QUANTITY>`").

A decoder that yields `DV_TEXT` for a discriminator-less `{"value":…}` is **correct** (layer 2 default). The defects below are at layers 1 and 2/3 respectively — not a "decode silently degrades" bug.

## Investigation findings (reproduced on `main`, SDK v0.10.0)

### Sub-gap A — substituted leaf in a `*Like` slot loses its mandatory `_type` (ENCODE defect)

`MarshalJSON` for `DVText` / `DVCodedText` / `DVInterval[T]` has a **pointer receiver** ([data_types_text_jsonmar_gen.go:65,125](../../openehr/rm/data_types_text_jsonmar_gen.go#L65), [data_types_quantity_jsonmar_gen.go:77](../../openehr/rm/data_types_quantity_jsonmar_gen.go#L77)) and the `_type` is emitted *inside* that method. A concrete **value** placed in a `*Like`/abstract interface field is not in the pointer method set, so `encoding/json` falls back to default struct encoding — omitting `_type`.

Reproduced (`Section.Name DVTextLike`, throwaway test):

| Slot value | Marshalled `name` | Re-decodes as |
|---|---|---|
| `Name: coded` (value `DVCodedText`) | `{"value":"Episode A","defining_code":{…}}` — **no `_type`**, and nested `defining_code` loses `CODE_PHRASE` | `DV_TEXT` (defining_code dropped) |
| `Name: &coded` (pointer) | `{"_type":"DV_CODED_TEXT",…,"defining_code":{"_type":"CODE_PHRASE",…}}` | `DV_CODED_TEXT` ✓ |

`LOCATABLE.name` is statically `DV_TEXT`, so a `DV_CODED_TEXT` there **must** carry `_type` (ITS-JSON). The value form is therefore **non-conformant encoding** — a genuine wire-level data loss. The generator triggers it: [openehr/instance/locatable.go:19](../../openehr/instance/locatable.go#L19) (and :29/:39/:49) assigns `v.Name = rm.DVText{…}` — a value into the `DVTextLike` slot.

**Layer:** 1 (encode). Decode and validator are behaving correctly given the (lossy) bytes.

### Sub-gap B — `DV_INTERVAL<T>` generic parameter collapses on DECODE (wire is correct)

Reproduced (`&DVInterval[DVQuantity]` in `Element.Value DataValue`, throwaway test):

- **Encode is byte-perfect canonical JSON** — no loss:
  ```json
  "value": {"_type":"DV_INTERVAL",
            "lower":{"_type":"DV_QUANTITY","magnitude":30,"units":"cm"},
            "upper":{"_type":"DV_QUANTITY","magnitude":90,"units":"cm"}, …}
  ```
  (Canonically, `_type` is the bare class `DV_INTERVAL`; the `<DV_QUANTITY>` is **never** on the wire — it is carried by the bounds' `_type`.)
- **Decode collapses the Go generic parameter:** the value re-decodes as `*rm.DVInterval[rm.DVOrdered]`, **not** `[DVQuantity]`, because `typereg` has a single registration `"DV_INTERVAL" → DVInterval[DVOrdered]` ([typereg_gen.go:35](../../openehr/rm/typereg_gen.go#L35)). The **bounds survive correctly as `*DVQuantity`** (recovered from their own `_type`).

So nothing is lost on the wire or in the bounds — only the *container's* static `T` collapses `DVQuantity → DVOrdered`. The in-memory `NewSkeleton` instance is `DVInterval[DVQuantity]` (validates OK); the round-tripped value is `DVInterval[DVOrdered]`, and `validation.ValidateComposition` keys off that, reporting `DV_INTERVAL does not satisfy DV_INTERVAL<DV_QUANTITY>`.

**Layer:** 2/3 (decode reconstruction + validator), **not** encode. This is a different defect from sub-gap A; the inbound draft conflates them.

## What is explicitly NOT a defect (do not "fix")

- Decoding a discriminator-less `{"value":…}` as `DV_TEXT` (layer-2 default). Teaching the decoder to infer `DV_CODED_TEXT` from `defining_code` would be wrong and brittle.
- The interval container `_type` being `"DV_INTERVAL"` (not `"DV_INTERVAL<DV_QUANTITY>"`) — that is the correct canonical form.
- Making `canjson` template-aware. Template knowledge stays in layer 3.

## Candidate fixes (decision required before implementing)

### Sub-gap A (encode `_type` for substituted `*Like` values)

- **A1 — codec-level, dynamic-type-driven (recommended).** When a parent's generated `MarshalJSON` emits an interface-typed (`*Like` / abstract) field, inject `_type` resolved from the value's **dynamic** type via `typereg`, independent of pointer-vs-value. Makes wire semantics Go-representation-agnostic and fixes every caller. Largest change (touches the codegen marshal path / a shared helper).
- **A2 — producer-side pointers.** Have `instance`/`NewSkeleton` populate `*Like` slots with pointers (`&rm.DVCodedText{…}`) so the existing pointer-receiver `MarshalJSON` runs. Smaller, but fragile — every present and future caller must remember; the value form still silently mis-encodes.
- **A3 — contract + lint (complementary).** Document the pointer contract in `openehr/rm/doc.go` and add a vet/lint check flagging "concrete RM value assigned into a `*Like` field." Mitigation, not a fix.
- **Lean:** A1 (correct at the codec boundary) + A3 as guardrail. A2 alone is insufficient.

### Sub-gap B (`DV_INTERVAL<T>` round-trip fidelity)

- **B1 — validator inspects the bounds (recommended, most openEHR-faithful).** `validation.ValidateComposition` decides `DV_INTERVAL<T>` conformance from the bounds' runtime types (`DV_QUANTITY`), not the Go generic parameter. The wire is already correct and lossless; conformance is a property of the bounds. Localised to the validator.
- **B2 — decode reconstructs the specific `DVInterval[T]`.** Resolve `T` from the bounds' `_type` (or the statically-declared field/template type) so the Go runtime type matches. Mechanically harder (a parameterised `typereg` factory for generic intervals) and arguably unnecessary if B1 holds.
- **Lean:** B1. Revisit B2 only if a consumer needs the concrete Go generic type post-decode.

## Open decisions (for the maintainer / brainstorm before any code)

1. Sub-gap A: codec-level (A1) vs producer-side (A2) vs both — and whether A1 belongs in the `bmmgen` marshal template or a shared runtime helper.
2. Sub-gap B: validator-side (B1) vs decode-side (B2). Confirm whether any consumer relies on the concrete `DVInterval[T]` Go type after decode (the reporting CDR project does not — it validates).
3. Whether to split this into two separate plans/REQs on landing (the fixes touch different packages and can ship independently).

## Acceptance (from the consuming CDR project)

For every corpus OPT, `canjson.Unmarshal(canjson.Marshal(NewSkeleton(…)))` yields a COMPOSITION that `validation.ValidateComposition` reports `OK` (round-trip-stable). On landing: the consuming project returns its benchmark stack to strict (reject) validation and drops its pointer workarounds.

## Out of scope

- The decode-side polymorphic *coverage* (SDK-GAP-11, landed) and the CDR-side defaults / `/validation` dry-run endpoint (consumer decisions).
- FLAT/STRUCTURED formats.

## Notes

- Reproductions in this dossier were throwaway tests under `openehr/serialize/canjson/` (not committed). They can be promoted to permanent round-trip tests (`canjson` + a `testkit/probes` round-trip-stability probe) as part of whichever fix lands.
- The inbound CDR-project SDK-GAP-13 draft should be updated to reflect the A/B split (its interval section attributes the failure to missing encode-side `_type`, which the repro disproves — the interval wire form is correct).
