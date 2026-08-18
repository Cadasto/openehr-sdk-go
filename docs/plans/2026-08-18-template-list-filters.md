# Plan — Template list filters (`ListTemplates` options)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-143](../specifications/wire.md#req-143--template-list-filters)
**Probes:** [PROBE-093](../specifications/conformance.md#probe-093--template-list-filters-reach-the-wire) (Draft)
**Implementation:** planned
**Depends on:** landed `ListTemplates` / `query.ExecuteOption` patterns; vendored ITS-REST pin (`definition-validation.openapi.yaml`)
**Ordering:** after the [contribution-get plan](2026-08-18-contribution-get.md) — that leaf unblocks conformance coverage; this one is additive ergonomics
**Defers:** ADL 2 (`FormatADL2` does not exist; `TemplateFormat` registers only `FormatADL14` — the pin's ADL 2 list operation references the same five parameter components, so the option set carries over unchanged when ADL 2 support lands); other missing Definition list endpoints

> **Execution:** work the steps sequentially. Run each step's verification command before moving on; a failing step blocks the next. Commit exactly where a step says commit.

**Goal:** Callers filter the template catalog through the typed leaf, without dropping to raw `transport.Do`.

**Architecture:** `ListTemplates` gains a trailing variadic `...ListOption` carrying the five OpenAPI query parameters — source-compatible with existing callers.

**Tech Stack:** Go 1.26; existing `TemplateMetadata` decode unchanged.

## Global Constraints

- REQ-022 functional options; REQ-023 package-level functions.
- Ground truth: `resources/its-rest/definition-validation.openapi.yaml` — parameter components `filter_template_id`, `concept`, `filter_version`, `offset`, `fetch`; the wire query **keys** are their `name:` fields: `template_id`, `concept`, `version`, `offset`, `fetch`.
- A probe asserts parameter **emission** only — a server that silently ignores unknown query parameters is indistinguishable from one that filters, so result-set narrowing is never asserted (REQ-080).
- Do not mention any external consumer or review artefact in commits or prose.

## Definition of Ready

- REQ-143 canonical prose + registry row exist (landed with the spec PR).
- No ADR.
- Verification: `go test ./openehr/client/definition/ -count=1`, `make spec-check`, `make ci`.
- Negative space: negative `offset` / `fetch` → `ErrInvalidConfig`, no request; unset options omit their query keys.

## Definition of Done

- Filtered `ListTemplates` landed with `// REQ-143` / `// PROBE-093` citations.
- `traceability.yaml` + REQ.md Impl. `planned → landed` for REQ-143.
- PROBE-093 Draft → Implemented (Sandbox).
- `make spec-check` and `make ci` pass.
- Plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry (REQ-143, PROBE-093 Draft) | done in the spec PR |
| `ListTemplates` options | |
| Tests + probe with citations | |
| `make spec-check` / `make ci` | |

## Steps

**Files:**

- Modify: `openehr/client/definition/template.go`
- Create: `openehr/client/definition/list_option.go` (or keep options beside `ListTemplates`)
- Test: `openehr/client/definition/template_test.go`
- Probe: `testkit/probes/definition/probe_093_template_list_filters.go`

**Interfaces:**

```go
type ListOption func(*listConfig)

func WithTemplateID(id string) ListOption // wildcard pattern as specified upstream
func WithConcept(concept string) ListOption
func WithVersion(version string) ListOption
func WithOffset(n int) ListOption // explicit 0 is sent
func WithFetch(n int) ListOption  // explicit 0 is sent

func ListTemplates(ctx context.Context, c *transport.Client, format TemplateFormat, opts ...ListOption) ([]TemplateMetadata, *transport.Metadata, error)
```

Use `*Set` boolean flags for offset/fetch exactly as `query.ExecuteOption` does, so an explicit `WithOffset(0)` is distinguishable from unset.

- [ ] **Step 1: Failing tests** — each option appears as its named query key on the captured URL; combined options; `WithOffset(0)` / `WithFetch(0)` present on the wire; no options → empty query; negative offset/fetch → `ErrInvalidConfig`, zero requests. Test against `FormatADL14` only — it is the sole registered `TemplateFormat`.

```
go test ./openehr/client/definition/ -run TestListTemplates -count=1
```

Expected: FAIL — `ListOption` undefined.

- [ ] **Step 2: Implement** — resolve options into `listConfig`, reject negatives before building the request, set `req.Query`, keep the existing `[]TemplateMetadata` decode and the existing `format`-selected path. Signature change is variadic-only: existing callers compile unchanged.

```
go test ./openehr/client/definition/ -count=1
```

Expected: PASS.

- [ ] **Step 3: PROBE-093** (Sandbox) — query-string emission assertion per the Global Constraints (never result narrowing). Flip PROBE-093 Draft → Implemented (Sandbox) in `conformance.md`.

- [ ] **Step 4: Close out** — flip REQ-143 to `landed` in `REQ.md` + `traceability.yaml`; update the roadmap Definition row; archive this plan.

- [ ] **Step 5: Verify and commit**

```
go test ./openehr/client/definition/ ./testkit/probes/definition/ -count=1
make spec-check
make ci
```

```
git commit -m "$(cat <<'EOF'
feat(client/definition): typed list filters on ListTemplates

REQ-143 / PROBE-093: template_id, concept, version, offset, fetch
reach the wire as ITS-REST query parameters.
EOF
)"
```

## Mapping to specs

- [wire.md § REQ-143](../specifications/wire.md#req-143--template-list-filters)
- ITS-REST pin: `resources/its-rest/definition-validation.openapi.yaml` (`definition_template_adl1.4_list`; parameter components `filter_template_id`, `concept`, `filter_version`, `offset`, `fetch`)
