# Plan — Public compiled-template bridge (external composition build + validation)

- **Date:** 2026-06-17
- **Status:** In progress
- **Relates:** REQ-101 (composition builder), REQ-102 / REQ-110 (validation), REQ-107 (instance), REQ-013/REQ-100 (building-block independence)
- **New identifiers:** REQ-111, ADR 0010

## Problem

The composition builder (`openehr/composition` — `NewBuilder`, `NewSkeleton`,
REQ-101), the RM instance synthesiser (`openehr/instance` — `Generate`,
REQ-107), the validator (`openehr/validation` — `Validate`,
`ValidateComposition`, `ValidateEHRStatus`, `ValidateFolder`,
`ValidateDemographic`, `ValidateAQL`, REQ-102/REQ-110) and the AQL static lint
(`openehr/aql/lint` — `Options.Compiled`, REQ-109) **all** take the compiled
template as `internal/templatecompile.Compiled`.

Per Go's `internal/` visibility rule, a different module cannot construct a
`*internal/templatecompile.Compiled`, so **none of these public entry points is
callable from outside this module.** `openehr/template.ParseOPT` yields a
public `*template.OperationalTemplate`, but there is no public function to turn
that parsed OPT into the compiled form the builder/validator require.

REQ-102's canonical text already anticipated the fix:

> the public re-export (`template.Compile` / `template.Compiled`) lands with
> REQ-101 Phase 1 — after which validators become externally callable without
> code change.

## Spec/code conflict to resolve (ADR 0010)

The promised placement — `template.Compile` / `template.Compiled` inside
`openehr/template` — is **infeasible**:

1. **Import cycle.** `internal/templatecompile` imports `openehr/template`. A
   public `Compile` *inside* `openehr/template` would have to import
   `internal/templatecompile`, creating `template → templatecompile → template`.
2. **REQ-100 stdlib-only contract.** `openehr/template` MUST be importable
   without `openehr/rm/…`. Compilation needs `openehr/rm/rminfo` (implicit
   attribute injection). Hosting `Compile` in `template` would pull `rminfo`
   into a package the spec mandates stay stdlib-only.

**Resolution:** the public bridge lives in a **sibling** package
`openehr/templatecompile`, not in `openehr/template`. ADR 0010 records the
decision and rationale; REQ-102's prose is corrected to point at REQ-111 and
the real package.

The spec word **"re-export"** + **"without code change"** signals the intended
mechanism: a **type-alias re-export**, not a stripped wrapper.

## Design (chosen: alias re-export)

New public package **`openehr/templatecompile`** that re-exports the internal
compile engine:

```go
package templatecompile // PUBLIC — github.com/cadasto/openehr-sdk-go/openehr/templatecompile

// Compiled is the public, externally-constructable compiled template.
// It is a type alias of the internal compiled form, so values produced by
// Compile are accepted as-is by openehr/composition, openehr/instance,
// openehr/validation and openehr/aql/lint.
type Compiled = impl.Compiled

// Compile turns a parsed OPT into the compiled driver used by the builder
// and validator. Functional options keep the (internal) Options struct out
// of the public surface and leave room for forward-compatible knobs.
func Compile(opt *template.OperationalTemplate, opts ...Option) (*Compiled, error)

type Option func(*config)
func WithRMInfo(l rminfo.Lookup) Option       // custom RM-info source
func WithoutImplicitAttributes() Option        // OPT-declared attributes only

var ErrInvalidInput = impl.ErrInvalidInput     // same var → errors.Is works across the boundary
var ErrPathNotFound = impl.ErrPathNotFound
```

- **Committed public surface:** `Compile`, `Compiled`, `Option`, `WithRMInfo`,
  `WithoutImplicitAttributes`, `ErrInvalidInput`, `ErrPathNotFound`. The
  node-level types (`CompiledNode`, `CompiledAttribute`) stay internal-named —
  reachable via `Compiled`'s methods but not nameable by external code, so they
  remain free to evolve.
- **Consumer signatures** reference the public alias so pkg.go.dev renders a
  live link to the public type (clean docs). Because the alias *is* the
  internal type, all existing internal-typed code (unexported helpers, struct
  fields) stays type-compatible — no behavioural change. Files that also use
  the node-level types keep the internal import under a local alias.

### Signatures updated to the public alias

- `composition.NewBuilder`, `composition.NewSkeleton`
- `instance.Generate`
- `validation.Validate`, `ValidateComposition`, `ValidateEHRStatus`,
  `ValidateFolder`, `ValidateDemographic`, `ValidateAQL`
- `aql/lint.Options.Compiled` (field type)

## Build order (TDD)

1. **Facade + tests.** Create `openehr/templatecompile` with `Compile`,
   `Compiled` alias, functional options, error re-exports, `doc.go`. Test:
   `Compile(parsed OPT)` returns a usable `*Compiled` whose identity matches a
   direct internal compile; `errors.Is` against the re-exported sentinels;
   options behave.
2. **Wire consumers.** Switch the exported signatures above to the public
   alias. Build must stay green with no behavioural test changes.
3. **External-callability proof.** A public-only path that imports *no*
   `internal/…` package and exercises:
   - `Compile → composition.NewBuilder → Set → Build`
   - round-trip `builder → canjson.Marshal → canjson.Unmarshal` with
     field-level equality on a reference fixture
   - `validation.ValidateEHRStatus(*rm.EHRStatus, Compile(opt))`
   Delivered as a runnable `cmd/examples/` program (public-only) **plus** a
   focused round-trip test.

## Docs / spec updates

- **ADR 0010** — placement + alias-re-export rationale; supersedes REQ-102's
  `template.Compile`/`template.Compiled` promise.
- **REQ-111** (clinical-modeling.md canonical + REQ.md registry row) — "Public
  compiled-template bridge". Cross-refs REQ-101/102/107/110.
- **REQ-102 / REQ-101 / REQ-110 prose** — correct the now-false "lands inside
  `openehr/template`" statement to point at REQ-111 / `openehr/templatecompile`.
- **traceability.yaml** — REQ-111 block; add `openehr/templatecompile` to the
  package lists of REQ-101/102/107/110.
- **roadmap.md** — mark "Composition builder" / "Validation" external
  callability landed.
- **module-layout.md** — add `openehr/templatecompile/` to the tree.
- **examples** — register the new example in `docs/examples.md`,
  `cmd/examples/doc.go`, and `docs/ai-workflow.md` § Examples (and
  `quick-start.md` only if the onboarding path changes).
- **CHANGELOG.md** — one additive bullet.

## Acceptance criteria

- [ ] Public, externally-constructable compiled type, produced from
      `template.ParseOPT` output, importing no `internal/…` package.
- [ ] `composition.NewBuilder`/`NewSkeleton` and `validation.Validate*`
      callable from outside this module (proven by a public-only example/test).
- [ ] Reference fixture round-trips builder → `canjson.Marshal` →
      `canjson.Unmarshal` with field-level equality.
- [ ] `validation.ValidateEHRStatus` callable on an external `*rm.EHRStatus` +
      externally-compiled OPT.
- [ ] `make ci` green (fmt, vet, lint, test, spec-check).

## Out of scope

- Full OPT parsing (already landed, REQ-100).
- Exposing `CompiledNode`/`CompiledAttribute` as named public types.
- AQL-style path-fluent setters (separate ergonomic concern).
