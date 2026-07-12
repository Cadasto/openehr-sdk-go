# ADR 0013 — Generated LOCATABLE identity surface and reverse type lookup

- **Status:** Proposed, 2026-07-12. **An ADR MUST be Accepted before implementation begins** (plan DoR gate).
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** cross-references [STRAND-04](../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance) (RM polymorphism; does **not** resolve it — the codec question stays open).
- **Introduces:** — (no new `REQ`). **Amends:** REQ-031, REQ-040, REQ-043. **Applies:** REQ-013, REQ-014, REQ-024, REQ-042, REQ-044.
- **Plan:** [2026-07-10-modernize-simplify.md § Phase 5](../plans/2026-07-10-modernize-simplify.md).
- **Related:** [ADR 0011](0011-rm-behavioural-functions-surface.md) governs the hand-realised RM behavioural-function surface; the accessors here are SDK-idiom *generated* additions beside it (same `rm` method surface, different emission path) and reuse its sealed-interface and no-panic conventions. [ADR 0002](0002-bmm-codegen-decisions.md) D4/D6/D7 constrain the generator mechanics.

## Context

Because [ADR 0002](0002-bmm-codegen-decisions.md) D4 flattens abstract ancestors, the **31**
LOCATABLE concrete RM types (the `isLocatable()` marker count in `openehr/rm/*_gen.go`)
carry `ArchetypeNodeID string` / `Name DVTextLike` / `UID UIDBasedID` /
`ArchetypeDetails *Archetyped` as own fields, and `rm.Locatable` is a **sealed marker interface** (unexported `isLocatable()`,
value receivers). There is no polymorphic way to read or write a node's identity, so five
files hand-maintain parallel reflection-free `case *rm.X` switches (~341 arms total):
`openehr/validation/rmread/read.go` (116), `openehr/rm/rmpath/walk.go` (72 — `nodeIDOf` and
`nameValueOf` are 36 byte-identical arms *each*), `internal/templateinstance/rmwrite/write.go`
(59), `openehr/validation/composition.go` (58), `openehr/composition/typecheck.go` (36) —
plus `openehr/instance/locatable.go`'s 18 identity arms (`applyLocatableIdentity`).
(Counts as of pre-consolidation `main` @ 15f02f1; plan Phase 4 has since landed via PR #72,
removing 13 *coercion* arms from `write.go` — disjoint from the identity/nil arms here.)
Their own comments demand lock-step maintenance; every BMM bump grows each switch by hand.
`internal/bmmgen` already emits the *forward* registry (`typereg_gen.go`: RM name →
constructor) and per-type attribute metadata (`rminfo/lookup_gen.go`), but neither the
identity accessors nor the *reverse* mapping (Go concrete type → RM class name), which
`validation/composition.go` (`rmTypeInfo`) and `composition/typecheck.go`
(`goConcreteRMType`) each re-implement by hand.

Two Go constraints shape any fix:

1. **Field/method name collision.** A struct cannot have a method named after a field, so
   accessors cannot be called `ArchetypeNodeID()` / `Name()`. The repo already solves this
   exact problem on the `*Like` interfaces (REQ-052/040): `openehr/rm/like_interfaces.go`
   emits `Get<Field>()` accessors (`DVText.GetValue()`, `GetDefiningCode()`) — the
   established convention for method-over-field access on RM types.
2. **Receiver split.** `Locatable`'s marker uses value receivers, so both `T` and `*T`
   satisfy it today; widening it with value-receiver *getters* preserves that. *Setters*
   require pointer receivers — putting them on `Locatable` would silently evict all value
   types from the interface. Getters and setters must live on separate interfaces.

Because the interface is sealed, no external implementer can exist: widening it is **not**
a breaking change for implementers — but the emitted methods and interfaces become permanent
public API on every LOCATABLE type, and the generator change forks `bmmgen`'s emission policy
(AGENTS.md § Do not touch: requires an ADR + an `architecture.md` note). That is the
irreversible fork this ADR records.

## Decision

Extend `internal/bmmgen` to emit, for every LOCATABLE concrete type, a **generated identity
surface** in `*_gen.go` (ADR 0002 D6: the generator never touches non-`_gen.go` files):

1. **Read accessors, widening `rm.Locatable`** (value receivers, uniform `Get<Field>` rule
   per the `*Like`-interface precedent): `GetArchetypeNodeID() string` and
   `GetName() DVTextLike` — name = `Get` + field, **return type = the field's actual
   declared type**, mechanically derivable by the generator. `Name` is `DVTextLike` (not
   `DVText`): returning the interface preserves `DV_CODED_TEXT` node names, and consumers
   read the text via the Like surface's existing `GetValue()`. Accessors return the field
   verbatim (a nil `Name`/`UID` on a partially-built node stays nil — nil handling remains
   with consumers, as today). These are SDK-idiom additions, not BMM functions — they do
   not pass through the D6 panic-stub / D7 skip-set machinery.
2. **Setters on a new sealed interface `rm.MutableLocatable`** (pointer receivers):
   `SetArchetypeNodeID(string)`, `SetName(DVTextLike)`, `SetUID(UIDBasedID)`,
   `SetArchetypeDetails(*Archetyped)` — parameter type = the field's actual declared type
   (`UID` is an interface-valued field, so the setter takes the `UIDBasedID` interface
   directly; a pointer-to-interface would be a Go anti-pattern). Satisfied by `*T` for every
   LOCATABLE `T`, sealed by the same marker so it cannot be implemented outside `rm`.
3. **A generated reverse lookup** `rm.RMTypeName(any) (string, bool)` — the inverse of
   `typereg_gen.go`, one exhaustive generated type-switch with a per-type **typed-nil pointer
   guard** (a typed-nil `*T` reports `("", false)`, never a false positive).

The hand-maintained consumers then shrink to their essential dispatch: `rmpath`'s
`nodeIDOf`/`nameValueOf` collapse to a single `Locatable` assertion (name text read via
`GetName().GetValue()`), `instance`'s `applyLocatableIdentity` uses `MutableLocatable`, and
the typed-nil guards delegate to the generated one. The two duplicated Go→RM-name maps
converge on `RMTypeName` with one nuance: `typecheck`'s `goConcreteRMType` collapses fully,
while `validation`'s `rmTypeInfo(v) (rmType, archetypeNodeID, ok)` does double duty and
therefore **decomposes** into `RMTypeName(v)` plus a `Locatable` assertion for the node id
rather than disappearing. The value-dispatch routers in `rmread`/`rmwrite`/`childrenAt`
**stay** — only identity/nil arms are removed; their lock-step comments are updated to the
reduced set.

## Consequences

- **Positive:** ~200 hand-maintained switch arms disappear; a BMM bump that adds a LOCATABLE
  type extends the identity surface via `make codegen` automatically (REQ-042 drift gate);
  identity access becomes polymorphic without reflection (REQ-024) or new dependencies
  (REQ-013/REQ-014 unchanged).
- **Permanent API cost:** every LOCATABLE type gains 2 methods + 4 pointer-receiver methods,
  plus two sealed interfaces and one lookup function on the public `rm` surface — additive
  (the sealed markers mean no external implementer can be broken), but effectively
  irreversible under the SDK's API-stability policy.
- **Generated-code growth:** 31 LOCATABLE types × 6 one-line methods (~186 generated
  methods) versus the ~200 removed hand-written lines — total LOC roughly nets out;
  *hand-maintained* LOC drops sharply. The trade is deliberate: machine-owned bulk over
  human-owned lock-step. (The `handles_test.go` **54**-type pin is the `rmread` closed
  taxonomy — a superset including non-LOCATABLE types — and stays the regression gate.)
- **Naming:** the `Get<Field>` prefix trades Effective Go's omit-`Get` guideline for
  collision-freedom, mechanical derivability, and consistency with the existing
  `*Like`-interface accessors — a deliberate, house-precedented choice.
- **Spec impact:** `rm-modeling.md` prose (REQ-031 layering, REQ-040 registry — which gains a
  reverse direction, REQ-043 mapping rules) must be updated in the same PR as the generator
  change; `traceability.yaml` gains the ADR references on those REQ entries at implementation.
- **Risk accepted:** if a future BMM class legitimately needs different identity semantics
  (e.g. a name outside the `DV_TEXT` family entirely), the generated surface must grow a per-type override
  mechanism (D7-style skip set) — deferred until a concrete case exists.

## Alternatives considered

- **Hand-written accessors in `*_funcs.go`** (no generator change): keeps `bmmgen` frozen but
  replaces the switch arms with ~186 hand-written methods — *more* human-owned lock-step, the
  opposite of the goal.
- **Reflection-based identity access:** rejected outright (REQ-024).
- **Embedded base struct carrying the fields** (make D4 emit a real `locatable` embed):
  smallest method count, but it changes the wire-visible struct shape assumptions of every
  existing consumer and the JSON marshaling layout — a far larger, riskier fork of ADR 0002
  D4 for the same payoff.
- **Setters on `Locatable` itself** (single interface): silently removes value-type
  satisfaction of `Locatable` — a subtle behavioural break for existing code holding values;
  rejected in favour of the getter/setter split.

## Acceptance gate

Per the plan's Definition of Ready, implementation is **blocked until this ADR's status is
Accepted** (maintainer decision). On acceptance: flip status, add the `architecture.md`
narrative note and REQ/traceability wiring in the implementation PR, and proceed with
Phase 5's generator + consumer-refactor tasks (`make codegen` + `codegen-verify` +
`handles_test.go` 54-type taxonomy pin + full `make test-race` as the verification gate).
