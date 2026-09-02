# ehr.Create empty-2xx typing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Date:** 2026-09-01
**Status:** Done (2026-09-02) — Task 1 executed; REQ-094's `ehr.Create` keyed exception closed.
**Owner:** SDK maintainers
**Covers:** [REQ-094](../specifications/transport.md#req-094) (write-result contract / `NoRepresentationError`) — extends it to `ehr.Create`'s empty-body arm. No new id.
**Probes:** none new (unit-covered like the rest of the write-result family).
**Implementation:** landed
**Depends on:** [REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure) (`*transport.DecodeError`, landed — the primitive whose absence deferred this); `NoRepresentationError` ([`openehr/client/ehr/write.go:151`](../../openehr/client/ehr/write.go#L151), landed); `WriteResult[T]` ([`write.go:99`](../../openehr/client/ehr/write.go#L99), landed).
**Defers:** nothing — this closes the one deferred slice recorded in [the archived write-result plan's Defers](archive/2026-08-18-write-result-contract.md).

**Goal:** When `ehr.Create` receives a 2xx with an **empty or JSON-null** body, return a typed `*NoRepresentationError` ("committed, but no usable representation") instead of the bare `transport.ErrInvalidShape` — bringing it in line with every versioned write — **without** changing the shared read contract or the decode-failure arm.

**Architecture:** `ehr.Create` decodes through the shared `transport.Decode[rm.EHR]` (line 141), which the read paths (`ehr.Get`, `ehr.Exists`) also use — so its empty-body error cannot be retyped inside `transport.Decode` without changing reads. The surgical fix retypes **only at the `ehr.Create` layer**: after `transport.Decode`, translate the *empty-body* `ErrInvalidShape` into `*NoRepresentationError`, while leaving the decode-failure arm as the `*transport.DecodeError` REQ-151 already produces. Reads are untouched.

**Tech Stack:** Go error wrapping (`errors.Is` / `errors.AsType`), `openehr/client/ehr`, `transport`.

**Spec:** [transport.md § REQ-094 / REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure) — the keyed exception clause that names this deferral.

## Global Constraints

- **Reads keep the shared contract.** `ehr.Get` / `ehr.Exists` (and every other `transport.Decode` caller) **MUST** be behaviourally unchanged: an empty read body stays bare `transport.ErrInvalidShape`.
- **Decode-failure arm unchanged.** A present-but-undecodable 2xx body stays a `*transport.DecodeError` (REQ-151).
- **Value-free diagnostics** (REQ-093): the error names the classification, never the body.

## Definition of Ready

- Covers REQ-094; its canonical prose (the write-result contract + the keyed exception) exists in transport.md. The primitive dependency (REQ-151) landed. Ready.

## Definition of Done

- Empty/null 2xx body from `ehr.Create` returns `*NoRepresentationError`, recoverable with `errors.AsType[*NoRepresentationError]`.
- `ehr.Get`/`ehr.Exists` behaviour proven unchanged by an existing/added test.
- The transport.md keyed-exception clause is rewritten in the **same PR** — the exception is closed, not merely narrowed; REQ.md/`traceability.yaml` unchanged in status (REQ-094 stays `landed`).
- `make spec-check` and `make ci` pass. Plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec: rewrite the transport.md keyed-exception clause (REQ-094/151) | ✅ closed, not narrowed (`8f2b88e`) |
| Code: retype the empty-body arm in `ehr.Create` | ✅ `8f2b88e` — also classifies the JSON-null body, which `transport.Decode` does not error on (a null literal is a nil-error no-op against a struct target); Create no longer routes through `transport.Decode` (reads untouched) |
| Test: empty-body → `NoRepresentationError`; decode-failure → `DecodeError`; reads unchanged | ✅ `TestCreateEmpty2xxBody` (`ehr_test.go`), `TestGet`/`TestExists*` re-run unchanged |
| `make spec-check` / `make ci` | ✅ `make spec-check` OK at both commits; `make ci` not run — task's verification scope was `go build ./...`, `go test ./openehr/client/... ./transport/... -count=1`, `golangci-lint run ./openehr/client/ehr/...`, `make spec-check`, all green |

## Phases

### Task 1: Retype `ehr.Create`'s empty-body arm

**Files:**
- Modify: `openehr/client/ehr/ehr.go:141` (the `transport.Decode` return in `Create`)
- Modify: `openehr/client/ehr/doc.go:13-17` (drop the "typing is deferred" note)
- Test: `openehr/client/ehr/ehr_test.go` (or the existing create test file)
- Modify: `docs/specifications/transport.md` (keyed-exception clause)

**Interfaces:**
- Produces: `ehr.Create` returns `*NoRepresentationError` (existing type) on empty/null 2xx; `*transport.DecodeError` on undecodable 2xx (unchanged); `*rm.EHR` on success (unchanged).

- [x] **Step 1: Characterisation + target tests.** Stand up an `httptest` server (the pattern the write-result tests already use) returning `201` with (a) an empty body, (b) `null`, (c) a garbage body, (d) a valid EHR. Assert the target contract:

```go
// (a),(b): empty / null -> *NoRepresentationError
_, _, err := ehr.Create(ctx, c)
var nre *ehr.NoRepresentationError
if !errors.As(err, &nre) { t.Fatalf("empty 2xx: got %v, want *NoRepresentationError", err) }

// (c): undecodable -> *transport.DecodeError (REQ-151, unchanged)
var de *transport.DecodeError
if !errors.As(err, &de) { t.Fatalf("garbage 2xx: want *transport.DecodeError") }
```

Run: `go test ./openehr/client/ehr/ -run TestCreateEmpty -v` → cases (a)/(b) FAIL today (bare `ErrInvalidShape`), (c)/(d) already pass. (Executed: (a) failed on the `*NoRepresentationError` assertion as predicted; (b) failed too, but on a different diagnosis than assumed here — the current `transport.Decode` does not error on a `null` body at all (a JSON `null` literal unmarshals into a struct target as a nil-error no-op), so `err` was `nil`, not the bare `ErrInvalidShape` this line predicts. Both are still RED; Step 2 below classifies the raw body directly rather than relying on `transport.Decode`'s error return, so it covers both.)

- [x] **Step 2: Retype only the empty-body arm** in `ehr.Create` (`ehr.go`, replacing the bare `return out, NewVersionMetadata(meta), err`):

```go
out, meta, err := transport.Decode[rm.EHR](ctx, c, req)
if err != nil && out == nil {
	// Decode-failure arm stays a *transport.DecodeError (REQ-151); only the
	// empty/null-body arm — bare ErrInvalidShape — is retyped here (REQ-094).
	var de *transport.DecodeError
	if !errors.As(err, &de) && errors.Is(err, transport.ErrInvalidShape) {
		return nil, NewVersionMetadata(meta), &NoRepresentationError{
			Meta:  NewVersionMetadata(meta),
			Cause: fmt.Errorf("ehr.Create: %w: 201 with no representation body", transport.ErrInvalidShape),
		}
	}
}
return out, NewVersionMetadata(meta), err
```

(Confirm the `NoRepresentationError` field names against `write.go:151` — `Meta`, `Cause` — and adjust the `Meta` type if it is `*VersionMetadata` vs the raw metadata.)

**As landed, this snippet's shape changed:** the `err != nil && out == nil` guard above only catches the empty-body case — a `null` body decodes through `transport.Decode` with `err == nil` (see Step 1's note), so it never reaches this branch. The landed `Create` therefore stops calling `transport.Decode` and instead calls `c.Do` directly, classifying the raw response body itself (empty, whitespace-only, or literal `null` → `*NoRepresentationError`, mirroring `WriteResult`'s REQ-094 rule) before attempting `canjson.Unmarshal`; a decode failure past that point constructs the same `*transport.DecodeError` `transport.Decode` would have (`Method`/`Route` read off `req`, already set to the values `transport.Decode`'s unexported `effective*()` helpers would have produced). `ehr.Get`/`ehr.Exists`/`ehr.GetBySubject` are untouched — they still call `transport.Decode`.

- [x] **Step 3: Run tests — all four cases PASS.** Also run the existing `ehr.Get`/`ehr.Exists` tests to prove reads are untouched: `go test ./openehr/client/ehr/ -run 'TestGet|TestExists' -v`.

- [x] **Step 4: Spec + doc.** Rewrite the transport.md keyed-exception paragraph: the empty-body arm of `ehr.Create` is now `*NoRepresentationError` (REQ-094), matching the versioned-write family; the decode-failure arm remains `*transport.DecodeError` (REQ-151). Delete the "typing is deferred" sentence in `ehr/doc.go`.

- [x] **Step 5: Commit.**

```bash
git add openehr/client/ehr/ehr.go openehr/client/ehr/doc.go openehr/client/ehr/ehr_test.go docs/specifications/transport.md
git commit -m "fix(client/ehr): type ehr.Create empty-2xx as NoRepresentationError (REQ-094)"
```

Landed as `8f2b88e`; `docs/specifications/traceability.yaml` (REQ-094 `tests:` and `notes:`) was included in the same commit — its `plans:` entry for this plan's archived path follows in the archive commit below, so every commit stays `spec-check`-clean (the archive path cannot exist before the `git mv`).

## Decision to weigh (one)

Two ways to do this, both sound:

- **Surgical (recommended, above; chosen):** retype only the empty-body arm at the `ehr.Create` layer; the decode-failure arm keeps the `*transport.DecodeError` typing REQ-151 gives it. Matches transport.md's stated split exactly; no transport change; reads provably untouched.
- **Full reuse:** route `ehr.Create` through `WriteResult[rm.EHR]` like the versioned writes. Simpler call site, but it also re-wraps the *decode-failure* arm as `*NoRepresentationError`, diverging from REQ-151's `DecodeError` typing for this one call — a spec change beyond the deferred slice. Choose this only if you want `ehr.Create` byte-identical to the versioned-write path and are willing to amend REQ-151's `ehr.Create` sentence too.

## Mapping to specs

- [transport.md § REQ-094](../specifications/transport.md#req-094) — write-result contract / `NoRepresentationError` (the clause this closes).
- [transport.md § REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure) — decode-failure arm (unchanged).
