# ADR 0010 — Public compiled-template bridge placement

- **Status:** Accepted, 2026-06-17.
- **Supersedes:** —
- **Superseded by:** —
- **Revises:** [ADR 0005](0005-compiled-template-foundation.md) §C2 (the `template.Compile` / `template.Compiled` re-export proposal).
- **Tracks:** [`docs/plans/2026-06-17-public-compiled-template-bridge.md`](../plans/2026-06-17-public-compiled-template-bridge.md). Implements [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge); unblocks external callers of REQ-101 / REQ-102 / REQ-107 / REQ-110.

> **Numbering note.** ADR numbers 0008 (SMART discovery `services` shape) and 0009 (SMART/auth dependency policy) are reserved for decisions in flight on a separate branch and are not yet on `main`. This ADR takes 0010 to avoid a number collision on merge.

## Context

The compiled-template form (`templatecompile.Compiled`) is the argument every template-driven entry point takes — the composition builder (REQ-101), the RM instance synthesiser (REQ-107), the validator (REQ-102 / REQ-110), and the AQL static lint (REQ-109). [ADR 0005](0005-compiled-template-foundation.md) placed the compile engine in `internal/templatecompile/` and (§C2) deferred a public re-export until the composition builder confirmed the shape, naming the expected re-export `template.Compile` / `template.Compiled` — i.e. **inside `openehr/template`**, next to `ParseFile`.

REQ-101/102/107/110 have since landed and the shape is stable enough to expose. But the §C2 placement turns out to be infeasible:

1. **Import cycle.** `internal/templatecompile` imports `openehr/template` (it consumes a `*template.OperationalTemplate`). A public `Compile` *inside* `openehr/template` would have to import the engine, forming `template → templatecompile → template`.
2. **REQ-100 stdlib-only contract.** REQ-100 mandates `openehr/template` import nothing from `openehr/rm/…`. Compilation needs `openehr/rm/rminfo` to inject the implicit attributes an OPT omits. Hosting `Compile` in `openehr/template` would drag `rminfo` into a package the spec requires stay stdlib-only.

So the public constructor cannot live in `openehr/template`. The candidate mechanisms were: (a) **alias re-export** from a sibling public package (consumers keep accepting the same type, now publicly constructable); (b) **promote** the whole `internal/templatecompile` package to public; (c) an **opaque wrapper** exposing a hand-picked method set.

REQ-102 §C2's own wording — *"re-export"* and *"externally callable without code change"* — describes (a): a transparent re-export, not a stripped wrapper.

## Decision

The public compiled-template bridge **MUST** live in the sibling package `openehr/templatecompile`, **not** in `openehr/template`. It re-exports the engine by **type alias**:

- `type Compiled = <internal>.Compiled` — a public alias, so `Compile` output is accepted as-is by composition / instance / validation / aql/lint with no conversion and no behavioural change.
- `func Compile(opt *template.OperationalTemplate, opts ...Option) (*Compiled, error)` delegating to the engine, with functional `Option`s (`WithRMInfo`, `WithoutImplicitAttributes`) that keep the engine's option struct out of the public surface.
- `ErrInvalidInput` / `ErrPathNotFound` re-exported as the same `error` vars so `errors.Is` works across the boundary.

The **committed public surface is `Compile` + `Compiled` + the introspection tree (`CompiledNode`, `CompiledAttribute`) + the options + the two sentinels**, all type aliases of the engine forms. The node types are exposed (not merely reachable through `Compiled`'s methods) so downstream code can navigate the compiled template and hold the types in its own function signatures and struct fields — the form-generation, path-discovery, and custom mapping/validation use cases. Engine-only helpers (`IsAOMPrimitiveShortName`, the raw slot include/exclude strings) stay internal. The owner accepted the larger semver surface (2026-06-18); the one area flagged as likely to change pre-1.0 is multi-language term resolution (`CompiledNode.Term`'s `lang` parameter, REQ-105).

Consuming packages reference the public `*templatecompile.Compiled` in their **exported** signatures (clean rendered docs); unexported code that needs the node-level types imports the internal engine directly (aliased `tcimpl`). Type-alias identity makes the two interchangeable.

This **revises ADR 0005 §C2**: the re-export is `openehr/templatecompile.Compile` / `.Compiled`, not `openehr/template.Compile` / `.Compiled`.

## Consequences

- **Positive:** External modules drive the full parse → compile → build → validate pipeline through public packages with no `internal/` import — REQ-111 acceptance proof in `openehr/templatecompile/external_test.go` and `cmd/examples/compile-build-validate`.
- **Positive:** `openehr/template` keeps its REQ-100 stdlib-only contract; no import cycle.
- **Positive:** The introspection tree is fully navigable by downstream code (form generation, path discovery, custom mapping), and `Compiled`'s node-returning methods render real public types in the API docs.
- **Negative:** Larger semver commitment — `CompiledNode` (~13 methods) and `CompiledAttribute` (~7) are now public contracts on an engine that is still young (reworked under REQ-104/105). Accepted by the owner; multi-language `Term` resolution is pre-flagged as the likely pre-1.0 change. Adding the types was non-breaking; future changes to them are breaking (pre-1.0 still permits them with a changelog note).
- **Negative:** Two packages share the base name `templatecompile` (public `openehr/templatecompile`, internal `internal/templatecompile`). Files needing the engine-only helpers alias the internal one as `tcimpl`. The risk is cognitive only — type-alias identity means a mix-up cannot produce a type error.
