# Plan — Contribution read and template list filters

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-142](../specifications/wire.md#req-142--contribution-read), [REQ-143](../specifications/wire.md#req-143--template-list-filters)
**Probes:** [PROBE-092](../specifications/conformance.md#probe-092--contribution-read-matches-contribution_get), [PROBE-093](../specifications/conformance.md#probe-093--template-list-filters-reach-the-wire) (Draft)
**Implementation:** planned
**Depends on:** REQ-141 (path segments on `Get`); landed `ehrstatus.Get` / `query.ExecuteOption` patterns; vendored ITS-REST pin
**Defers:** Simplified-format `Accept` on contribution read; other missing Definition list endpoints; AQL diagnostics

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Callers read a contribution by uid and filter the template catalog through typed leaves, without dropping to raw `transport.Do`.

**Architecture:** `contribution.Get` copies `ehrstatus.Get`. `ListTemplates` gains a trailing `...ListOption` with the five OpenAPI query parameters. Two phases so either leaf can review independently.

**Tech Stack:** Go 1.26; `transport.Decode` / `canjson`; existing `TemplateMetadata`.

## Global Constraints

- REQ-022 functional options; REQ-023 package-level functions.
- REQ-141 applies to interpolated path parameters on `Get`.
- Ground truth: `resources/its-rest/ehr-validation.openapi.yaml` (`contribution_get`) and `resources/its-rest/definition-validation.openapi.yaml` (list filters).
- Do not mention any external consumer or review artefact in commits or prose.

## Definition of Ready

- REQ-142 and REQ-143 canonical prose + registry rows exist.
- No ADR.
- Verification: `go test ./openehr/client/ehr/contribution/ ./openehr/client/definition/ -count=1`, `make spec-check`, `make ci`.
- Negative space: empty ids / negative offset|fetch → `ErrInvalidConfig`, no request.

## Definition of Done

- `Get` and filtered `ListTemplates` landed with `// REQ-142` / `// REQ-143` / probe citations.
- `traceability.yaml` + REQ.md Impl. `planned → landed` for both.
- PROBE-092 / PROBE-093 Draft → Implemented (Sandbox).
- `make spec-check` and `make ci` pass.
- Plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry (REQ-142, REQ-143, probes Draft) | done in this change |
| `contribution.Get` | |
| `ListTemplates` options | |
| Tests + probes | |
| `make spec-check` / `make ci` | |

## Phases

### Phase 1 — `contribution.Get` (REQ-142)

**Files:**

- Modify: `openehr/client/ehr/contribution/contribution.go`, `doc.go`
- Test: `openehr/client/ehr/contribution/contribution_test.go`
- Probe: `testkit/probes/versioned/probe_092_contribution_get.go`

**Interfaces:**

- Produces: `func Get(ctx context.Context, c *transport.Client, ehrID EHRID, contributionUID string) (*rm.Contribution, *VersionMetadata, error)`
- Repository grows `Get` with the same signature (minus `c`).

- [ ] **Step 1: Failing tests** — empty `ehrID` / empty uid → `ErrInvalidConfig`, zero requests; 200 canonical JSON decodes; 404 → `ErrNotFound`; captured path is `/ehr/{id}/contribution/{uid}`.

- [ ] **Step 2: Implement** (mirror `ehrstatus.Get`):

```go
const routeGet = "/ehr/{ehr_id}/contribution/{contribution_uid}"

func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, contributionUID string) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("contribution.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if contributionUID == "" {
		return nil, nil, fmt.Errorf("contribution.Get: %w: empty contributionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + string(ehrID) + "/contribution/" + contributionUID,
		Route:  routeGet,
	}
	out, meta, err := transport.Decode[rm.Contribution](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}
```

Add `Get` to `Repository` and the unexported adapter.

- [ ] **Step 3: PROBE-092** Sandbox — request shape + 200 decode + 404 mapping. Flip probe status when the test exists.

- [ ] **Step 4: Commit**

```
git commit -m "$(cat <<'EOF'
feat(client/ehr/contribution): add Get

REQ-142 / PROBE-092: GET /ehr/{ehr_id}/contribution/{contribution_uid}
returns the persisted contribution.
EOF
)"
```

### Phase 2 — `ListTemplates` filters (REQ-143)

**Files:**

- Modify: `openehr/client/definition/template.go`
- Create: `openehr/client/definition/list_option.go` (or keep options beside `ListTemplates`)
- Test: `openehr/client/definition/template_test.go`
- Probe: `testkit/probes/definition/probe_093_template_list_filters.go`

**Interfaces:**

```go
type ListOption func(*listConfig)

func WithTemplateID(id string) ListOption
func WithConcept(concept string) ListOption
func WithVersion(version string) ListOption
func WithOffset(n int) ListOption // explicit 0 is sent
func WithFetch(n int) ListOption  // explicit 0 is sent

func ListTemplates(ctx context.Context, c *transport.Client, format TemplateFormat, opts ...ListOption) ([]TemplateMetadata, *transport.Metadata, error)
```

Query keys (from the vendored OpenAPI): `template_id`, `concept`, `version`, `offset`, `fetch`. Use `*Set` flags for offset/fetch exactly as `query.ExecuteOption` does.

- [ ] **Step 5: Failing tests** — each option on the captured URL; combined options; `WithOffset(0)` present; no options → empty query; negative offset/fetch → `ErrInvalidConfig`, zero requests; ADL 1.4 and ADL 2 paths both accept the options.

- [ ] **Step 6: Implement** — resolve options, reject negatives, set `req.Query`, keep the existing JSON decode of `[]TemplateMetadata`.

- [ ] **Step 7: PROBE-093** Sandbox — query-string assertion. Flip probe status.

- [ ] **Step 8: Flip REQ-142 and REQ-143 to landed**; archive this plan.

- [ ] **Step 9: Verify**

```
go test ./openehr/client/ehr/contribution/ ./openehr/client/definition/ ./testkit/probes/versioned/ ./testkit/probes/definition/ -count=1
make spec-check
make ci
```

## Mapping to specs

- [wire.md § REQ-142](../specifications/wire.md#req-142--contribution-read)
- [wire.md § REQ-143](../specifications/wire.md#req-143--template-list-filters)
- ITS-REST pin: `resources/its-rest/ehr-validation.openapi.yaml` (`contribution_get`); `resources/its-rest/definition-validation.openapi.yaml` (`filter_template_id`, `concept`, `filter_version`, `offset`, `fetch`)
