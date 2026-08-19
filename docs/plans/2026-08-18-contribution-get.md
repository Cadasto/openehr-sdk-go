# Plan — Contribution read (`contribution.Get`)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-142](../specifications/wire.md#req-142--contribution-read)
**Probes:** [PROBE-092](../specifications/conformance.md#probe-092--contribution-read-matches-contribution_get) (Draft)
**Implementation:** planned
**Depends on:** landed `ehrstatus.Get` pattern; `transport.Decode`; vendored ITS-REST pin (`ehr-validation.openapi.yaml`, `contribution_get`)
**Ordering:** [path-segment-validation](2026-08-18-path-segment-validation.md) (REQ-150) SHOULD land first so interpolated ids get transport-side refusal; nothing here compiles against it
**Defers:** Simplified-format `Accept` on contribution read; other missing read leaves

> **Execution:** work the steps sequentially. Run each step's verification command before moving on; a failing step blocks the next. Commit exactly where a step says commit.

**Goal:** Callers read a persisted contribution by uid through a typed leaf, without dropping to raw `transport.Do`. This is the missing half of the contribution round-trip: `Commit` landed (PROBE-072) but its effect is not verifiable through the same client — landing `Get` unblocks commit → read-back integration and conformance coverage, which is why this leaf is sequenced before the [template-list-filters plan](2026-08-18-template-list-filters.md).

**Architecture:** `contribution.Get` mirrors `ehrstatus.Get` (same option-free read shape, same `transport.Decode` leaf idiom).

**Tech Stack:** Go 1.26; `transport.Decode` / `canjson`; existing `rm.Contribution`.

## Global Constraints

- REQ-023 package-level functions; REQ-025 wrapped sentinel errors.
- REQ-150 owns path-segment refusal — this leaf validates only *emptiness* of its ids.
- Ground truth: `resources/its-rest/ehr-validation.openapi.yaml` (`contribution_get`). The pin defines only `Content-Type` on `200_CONTRIBUTION` — do not assert `ETag` / `Location` on reads.
- Do not mention any external consumer or review artefact in commits or prose.

## Definition of Ready

- REQ-142 canonical prose + registry row exist (landed with the spec PR).
- No ADR.
- Verification: `go test ./openehr/client/ehr/contribution/ -count=1`, `make spec-check`, `make ci`.
- Negative space: empty `ehrID` / empty uid → `ErrInvalidConfig`, no request; 404 → `ErrNotFound`.

## Definition of Done

- `Get` landed with `// REQ-142` / `// PROBE-092` citations; `Repository` grew the same read.
- CHANGELOG `### Added` names the read AND the `Repository` interface growth (a compile-time break for out-of-tree implementers — the disclosure REQ-142 mandates, mirroring REQ-143).
- `traceability.yaml` + REQ.md Impl. `planned → landed` for REQ-142.
- PROBE-092 Draft → Implemented (Sandbox).
- `make spec-check` and `make ci` pass.
- Plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry (REQ-142, PROBE-092 Draft) | done in the spec PR |
| `contribution.Get` + `Repository` | |
| Tests + probe with citations | |
| `make spec-check` / `make ci` | |

## Steps

**Files:**

- Modify: `openehr/client/ehr/contribution/contribution.go`, `doc.go`
- Test: `openehr/client/ehr/contribution/contribution_test.go`
- Probe: `testkit/probes/versioned/probe_092_contribution_get.go`

**Interfaces:**

- Produces: `func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, contributionUID string) (*rm.Contribution, *openehrclient.VersionMetadata, error)`
- `Repository` grows `Get` with the same signature (minus `c`); extend the unexported adapter.

- [ ] **Step 1: Failing tests** — empty `ehrID` / empty uid → `ErrInvalidConfig`, zero requests; 200 canonical JSON decodes to `*rm.Contribution`; 404 → `ErrNotFound`; captured method/path is `GET /ehr/{id}/contribution/{uid}`. Do **not** assert populated version metadata — the pin licenses none on a 200 read.

```
go test ./openehr/client/ehr/contribution/ -run TestGet -count=1
```

Expected: FAIL — `Get` undefined.

- [ ] **Step 2: Implement** (mirror `ehrstatus.Get`):

```go
const routeGet = "/ehr/{ehr_id}/contribution/{contribution_uid}"

// Get reads a persisted contribution (REQ-142).
// Wire: GET /ehr/{ehr_id}/contribution/{contribution_uid} (contribution_get).
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

The returned `*VersionMetadata` exists for shape consistency with the other EHR leaves; it carries whatever headers the server happened to send. Add `Get` to `Repository` and the unexported adapter. v1 requests canonical JSON only (no simplified-format `Accept`).

```
go test ./openehr/client/ehr/contribution/ -count=1
```

Expected: PASS.

- [ ] **Step 3: PROBE-092** (Sandbox) — request shape + 200 decode + 404 → non-nil error; no version-metadata assertion; per REQ-080 the probe never asserts sentinel identity — `ErrNotFound` / `ErrInvalidConfig` are pinned by the Step 1 unit tests only. Flip PROBE-092 Draft → Implemented (Sandbox) in `conformance.md`.

- [ ] **Step 4: Close out** — flip REQ-142 to `landed` in `REQ.md` + `traceability.yaml`; update the roadmap Contribution row; archive this plan.

- [ ] **Step 5: Verify and commit**

```
go test ./openehr/client/ehr/contribution/ ./testkit/probes/versioned/ -count=1
make spec-check
make ci
```

```
git commit -m "$(cat <<'EOF'
feat(client/ehr/contribution): add Get

REQ-142 / PROBE-092: GET /ehr/{ehr_id}/contribution/{contribution_uid}
returns the persisted contribution.
EOF
)"
```

## Mapping to specs

- [wire.md § REQ-142](../specifications/wire.md#req-142--contribution-read)
- ITS-REST pin: `resources/its-rest/ehr-validation.openapi.yaml` (`contribution_get`, `200_CONTRIBUTION`)
