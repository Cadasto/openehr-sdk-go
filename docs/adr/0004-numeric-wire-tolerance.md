---
kind: adr
id: ADR-0004
title: "Strict-encode, permissive-decode for BMM `Real` and `Integer`"
status: accepted
date: 2026-05-16
---

# ADR 0004 — Strict-encode, permissive-decode for BMM `Real` and `Integer`

- **Status:** Accepted, 2026-05-16.
- **Tracks:** REQ-046 (BMM primitive mapping), REQ-052 (canonical-JSON wire profile), STRAND-04 (RM polymorphism + codec performance).
- **Amended 2026-10-15** (maintainer decision): `Integer` decode also accepts a whole number written with a zero fraction or an exponent (`1.0`, `1e0`), bare or quoted, as that integer, and refuses one with a non-zero fraction; the reference's own FLAT writes a DV_PROPORTION `type` as `1.0`. The rule's normative home is [wire.md § Floating-point precision](../specifications/wire.md#floating-point-precision); `Integer64` stays out of scope as below.
- **Amended 2026-10-19** (maintainer decision): `Integer64` takes the `Integer` decode rule above, as the defined type `rm.Integer64` (underlying `int64`), which supersedes its exclusion in the 2026-10-15 amendment. `DV_COUNT.magnitude`, the RM's one `Integer64` attribute, refused `2.0`, `2e0` and `"2"`, which a `DV_QUANTITY.precision` accepts; one RM integer rule now covers both widths. `Double` stays out of scope.

## Context

The BMM-conformance spec ([`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) § Primitive type mapping) pins:

- `Real` → Go `float64`.
- `Integer` → Go `int32`.
- "Mappings are **fixed**; alternative widenings are not permitted."

The canonical-JSON wire profile ([`docs/specifications/wire.md`](../../docs/specifications/wire.md) REQ-052) requires:

- "`DV_QUANTITY` magnitudes are emitted as JSON numbers, not strings, **unless the spec mandates otherwise** (some implementations have used strings to avoid float-precision loss; the SDK takes a position — see § Floating-point precision below)."
- The double-precision rule of [wire.md § Floating-point precision](../specifications/wire.md#floating-point-precision): magnitudes are IEEE 754 double-precision JSON numbers, never coerced through `float32` or a similarly lossy intermediate.

The wire spec foresees that real producers exist that emit `"magnitude": "354"` (string) rather than `"magnitude": 354` (number). The vendored fixtures confirm this — `testkit/corpus/compositions/BMI.json` carries `"magnitude": "354"` etc. The spec ENCODE side is settled (numbers only); the DECODE side has been implicit until now.

A permissive decoder is needed for SDK consumers to round-trip real-world CDR fixtures, but strict downstream consumers (other openEHR clients, third-party producers) need to be able to assume the SDK emits numbers. Asymmetric tolerance is the standard Postel-style answer; it needs to be **explicit** so the SDK's openEHR wire conformance (REQ-080) is unambiguous.

## Decision

The SDK adopts asymmetric numeric tolerance:

- **Encode (`MarshalJSON`):** every `Real`/`Integer` value is emitted as a JSON number per REQ-052. No quoted output, ever. This is the only permitted behaviour.
- **Decode (`UnmarshalJSON`):** every `Real`/`Integer` field accepts EITHER a JSON number OR a quoted decimal string. A quoted form decodes as if it were the corresponding number; a malformed string returns a typed error.

To carry the asymmetric decode tolerance the SDK introduces two **defined types** rather than direct aliases:

```go
package rm

type Real    float64
type Integer int32
```

Both types are emitted by the BMM generator wherever the BMM primitive `Real` / `Integer` appears. The underlying types match REQ-046 (`float64` / `int32`); REQ-046's "fixed mapping" rule is **satisfied** at the structural level. The named types exist solely so the SDK can attach the permissive `UnmarshalJSON`. Encoders rely on the inherited number-emit behaviour via the `MarshalJSON` returning `float64`/`int32` to `encoding/json`.

`primitiveGoType` in [`internal/bmmgen/primitives.go`](../../internal/bmmgen/primitives.go) is the authoritative mapping table; it points `Real` and `Integer` at the alias types `Real` and `Integer`.

### Out of scope for this ADR

- **`Double`**: keeps its direct alias (`float64`) for now. No vendored fixture currently exercises quoted variants for it, and a precedent in PHP / CDR has not yet been observed. Promote it to a defined type if and when a fixture demonstrates the need. (`Integer64` was listed here until the 2026-10-19 amendment.)
- **Other numeric primitives** (`Octet`, `Character`): not affected — they're not floating-point and openEHR producers do not quote them.
- **Validating numeric precision** beyond IEEE 754 (REQ-052): out of scope; an overflow on decode still surfaces via `strconv.ParseFloat` / `strconv.ParseInt` and is wrapped in a typed error.

## Consequences

- Vendored CDR fixtures round-trip cleanly through `canjson` (PROBE-030 across `BMI.json`, `body_weight.json`, `clinical_note.json`, `vital_signs.json`).
- openEHR wire conformance (REQ-080): any conforming client has to handle quoted magnitudes the same way on decode to keep fixture round-trip semantics identical. Without this ADR a strict-number-only decoder would reject the `"magnitude": "354"` form that real CDRs emit. (The earlier wire-level cross-SDK parity requirement, REQ-081, has since been retired.)
- Consumers that need strict-number-only decode can wrap `canjson.Unmarshal` with a pre-pass that rejects quoted numerics, but the SDK itself does not offer a strict-decode mode in v1 — the loss of fixture interoperability outweighs the strictness benefit at this stage.
- The generated `MarshalJSONTo` for every concrete RM type continues to emit numbers (no behaviour change on the encode side).
- Documentation: REQ-046 stays as written — its "fixed mapping" pertains to underlying type only. The Floating-point precision section of REQ-052 references this ADR for decode tolerance. The note that `primitiveGoType` emits the alias type name (not the raw primitive) is captured in [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) § Primitive type mapping.

## References

- [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) — REQ-046 (primitive mapping).
- [`docs/specifications/wire.md`](../../docs/specifications/wire.md) — REQ-052 (canonical-JSON wire profile, Floating-point precision).
- [`openehr/rm/real.go`](../../openehr/rm/real.go), [`openehr/rm/integer.go`](../../openehr/rm/integer.go), [`openehr/rm/integer64.go`](../../openehr/rm/integer64.go) — the defined types implementing this ADR.
- [`internal/bmmgen/primitives.go`](../../internal/bmmgen/primitives.go) — the generator's mapping table.
- [`testkit/corpus/compositions/BMI.json`](../../testkit/corpus/compositions/BMI.json) — concrete fixture with quoted-number magnitudes.
