# ADR 0011 — RM behavioural-function surface and fallibility policy

- **Status:** Accepted, 2026-06-19.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — direct design decision (brainstormed 2026-06-19); no prior open research strand.
- **Introduces:** REQ-120, REQ-121, REQ-122, REQ-123. **Applies:** REQ-013, REQ-014, REQ-024, REQ-025.

## Context

`bmmgen` emits the openEHR behavioural functions on the identifier (`OBJECT_VERSION_ID`, `ARCHETYPE_ID`, `VERSION_TREE_ID`), `PATHABLE`/`LOCATABLE`, and `change_control` (`VERSION`, `VERSIONED_OBJECT`) classes as **panicking stubs** — e.g. `func (s *Section) ItemAtPath(aPath string) any { panic("not implemented: PATHABLE.item_at_path — implement in a non-generated file") }`. The BMM carries the *signature* but not the *algorithm*, so the generator leaves an explicit hand-written extension point. The functions are currently uncalled, and a public RM method that panics is a latent footgun against the no-panics idiom.

Three forces shape how to realise them:

1. **House idiom.** The SDK favours concrete typed structs, no reflection (REQ-024), package-level functions as the primary surface (REQ-023), wrapped errors over panics (REQ-025), and building-block independence (REQ-013) with a strict dependency direction (REQ-014).
2. **Spec fidelity.** `item_at_path` returns `Any`; preconditions (`path_unique`) are Eiffel-style. A faithful surface is partly at odds with idiomatic Go.
3. **Existing overlap.** `openehr/client/ehr` already splits a version-uid string best-effort; id-parsing logic exists, but in the client layer rather than as a canonical RM capability. And `openehr/validation/rmread` already does reflection-free typed RM navigation, but it sits in the higher validation layer.

## Decision

Realise the stubbed RM behavioural functions with a **hybrid surface**, scoped to the pure/derived subset (REQ-120/121/122):

1. **Concrete-typed derivations as methods.** Identifier component derivation and `is_branch` are implemented as hand-written methods on the RM types, in `*_funcs.go` files beside the generated `*_gen.go` (the generator's documented target). They return the BMM-typed result.
2. **Fallible parses also get an error-returning function.** Because identifier input may be malformed, each parse has a canonical package-level `Parse…` entry point returning `(T, error)`; the BMM-signature method returns a best-effort value with an `ok`/error companion. **No library code panics** on malformed data.
3. **One canonical id parser.** The parser lives in `openehr/rm`; `openehr/client/ehr`'s version-uid helper delegates to it — single canonical home, no duplicate lexical logic.
4. **Path read access as a building block.** `item_at_path`/`items_at_path`/`path_exists`/`path_unique` live in a new `openehr/rm/rmpath` package (sibling of `rminfo`) that carries **its own minimal reflection-free walker** — it does **not** import `openehr/validation/rmread`, preserving the dependency direction (REQ-014) and building-block independence (REQ-013). The generated `LOCATABLE` path methods delegate to it.
5. **Out-of-scope functions stay explicit stubs.** `PATHABLE.parent` / `path_of_item`, every `VERSIONED_OBJECT` container operation, and all `commit_*` mutators are not realised as in-memory RM behaviour (the SDK's versioning is server-mediated). They remain documented stubs that fail loudly rather than return a misleading value.

## Consequences

- The panic stubs are filled incrementally for the in-scope subset; the public RM surface stops panicking for those operations.
- `rmpath` is a new openEHR-core building block with zero third-party and zero `transport`/`auth` dependencies; its own walker means a small overlap with `rmread`'s typed switch, accepted to avoid inverting the layer dependency or churning landed REQ-110 code — convergence to a shared navigator is a later option if it proves worthwhile.
- The client version-uid helper is de-duplicated against the canonical RM parser.
- The one idiom compromise is the spec-mandated `any` return on `item_at_path`/`items_at_path`; consumers needing type safety continue to use the typed `Get*` accessors. This is contained to the path package.
- Container/commit version operations remain server-side; `VERSIONED_OBJECT`/`VERSION` in-memory management is explicitly not offered.
- A future `bmmgen` change could emit non-panicking stubs (error/zero return) for unimplemented functions, removing the footgun for the out-of-scope set; tracked as a generator follow-up, not part of this decision.

## Alternatives considered

- **Package functions only** (leave the methods stubbed, expose `rmid`/`rmpath` free functions). Most idiomatic, but abandons the BMM-generated method surface and leaves panicking stubs on the public types.
- **Spec-faithful methods only** (every function a method, `item_at_path` returns `any`, preconditions panic). Closest to the spec, but spreads `any`-typed returns across the RM and keeps panics — rejected against REQ-024/025.
- **Lift `rmread` to a shared low-level navigator** consumed by both `validation` and `rmpath`. Cleanest single-navigator design, but refactors already-landed, tested REQ-110 code and widens this change's blast radius — deferred.
