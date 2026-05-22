# ADR 0003 — Codec polymorphism for abstract generic RM classes

- **Status:** Accepted, 2026-05-16.
- **Tracks:** STRAND-04 (partial — Event/History cassette decode); [canonical JSON plan](../plans/2026-05-15-canonical-json-serialization.md).

## Context

openEHR RM defines abstract generic classes such as `EVENT[T]` and `VERSION[T]` with concrete descendants (`POINT_EVENT`, `INTERVAL_EVENT`, `ORIGINAL_VERSION`, …). The initial generator policy (ADR 0002 D4) rendered every abstract generic as a Go **struct** because Go cannot attach an `is<X>()` marker to a generic interface.

That policy breaks canonical JSON decode at polymorphic sites. `HISTORY.events` is a list of events whose wire `_type` discriminates `POINT_EVENT` vs `INTERVAL_EVENT`, but the generated field was `[]Event[T]` (struct slice). `encoding/json` cannot select the concrete shape, and generated `UnmarshalJSON` copied the slice without polymorphic dispatch.

## Decision

`bmmgen` maintains an explicit whitelist (`codecPolymorphicAbstractGenericNames`). **EVENT** is whitelisted today; other abstract generics such as **VERSION** remain structs until their call sites are designed.

Consequences for `EVENT`:

- `type Event interface { isEvent() }`
- `PointEvent[T]` / `IntervalEvent[T]` flatten `EVENT` fields and implement `isEvent()`
- `History[T].Events` is `[]Event` (interface slice), not `[]Event[T]`
- Generated `UnmarshalJSON` routes `events` through `typereg.DecodeAs[Event]`

Other abstract generics (including **VERSION**) continue to emit as generic structs until whitelisted.

## Consequences

- Vendored composition cassettes round-trip through `canjson` (PROBE-030 on cassette inputs).
- Callers that referenced `Event[T]` as a struct must use the `Event` interface or concrete `PointEvent` / `IntervalEvent` types.
- ADR 0002 D4 is narrowed: abstract generic → struct only when there are no concrete descendants.

## References

- [`docs/specifications/rm-modeling.md`](../../docs/specifications/rm-modeling.md)
- [`internal/bmmgen/render.go`](../../internal/bmmgen/render.go) — `codecPolymorphicAbstractGeneric`
