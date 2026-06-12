# Plan — Security hardening & simplification (repo-wide review follow-ups)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-06-11
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** — (remediation of review findings; no existing REQ. Candidate new REQ rows flagged inline as `REQ-candidate`.)
**Probes:** —
**Implementation:** planned
**Depends on:** nothing (all phases independent; tasks within a phase are independent unless noted)
**Defers:** `bmm.Load` context-awareness (API change, revisit pre-1.0 freeze); same-origin JWKS enforcement beyond HTTPS-only (needs product decision); `testkit/fixtures` `runtime.Caller` → `go:embed` migration (test-only)

## Goal

Close the security, robustness, and code-quality findings from the 2026-06-11 repo-wide review (four parallel reviewer passes over `transport`/`auth`/`smart`, `openehr/*`, `internal`/`cmd`/`testkit`/infra, and `openehr/rm|aom|bmm`; every High finding re-verified against source before inclusion). Ships as a series of small PRs grouped by phase: (1) trust & token path, (2) untrusted-input hardening, (3) simplification & perf, (4) tooling & CI. Consumers are SDK users handling PHI against openEHR CDRs — Phase 1 and 2 directly reduce their exposure.

**Architecture:** No new packages. Fixes land in place; shared helpers move to their lowest common package. Generated-code fixes go through `internal/bmmgen` + `make codegen` so `*_gen.go` stays machine-owned.

**Tech Stack:** Go 1.25, stdlib only (plus existing OTel deps). Repo gates: `make ci`, `make spec-check`, gofmt hook.

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`traceability.yaml`, REQ.md row) — only if REQ-candidates are adopted | |
| Code | |
| Tests with `// REQ-` / `// PROBE-` comments where a REQ-candidate is adopted | |
| `make spec-check` | |
| `make ci` | |

## Findings index

| ID | Sev | Where | Summary |
|---|---|---|---|
| S1 | High | `smart/discovery/resolver.go:346` | Discovery document's `issuer` overrides caller-supplied issuer |
| S2 | High | `smart/discovery/resolver.go:376`, `auth/smart/source.go` | `jwks_uri` from document used without HTTPS enforcement |
| S3 | High | `transport/errors.go:96`, `transport/client.go:356` | `WireError` leaks server message + raw body (PHI risk) into logs/traces |
| S4 | High | `transport/client.go:236` | Unbounded `io.ReadAll` of response bodies |
| S5 | Med | `auth/smart/source.go:154` | OAuth `state` accepted with no entropy requirement |
| S6 | Med | `auth/jwtbearer/assertion.go:200` | JTI = timestamp+counter, predictable across restarts |
| S7 | Med | `openehr/client/ehr/itemtag_wire.go:139` | Item-tag header escaping misses CR/LF/control chars |
| S8 | Med | `smart/principal.go:51` | `PrincipalIdentity.Raw` retains full claims map by reference |
| R1 | Med | `openehr/template/parse.go`, `openehr/client/definition/template.go:170`, `openehr/bmm/load.go:20` | Unbounded reads of OPT/BMM input |
| R2 | Med | `openehr/template/path.go:199` + tree build | Unbounded recursion on crafted OPT trees |
| R3 | Med | generator `internal/bmmgen/render_jsonunmar.go` | Recursive polymorphic JSON decode: O(depth×size) re-scan + deep call stacks |
| R4 | Med | `openehr/client/definition/stored_query.go:137` | Missing empty-body guard (siblings have it) |
| R5 | Med | `internal/bmmgen/render_rminfo.go:119` | `effectiveProperties` lacks ancestor-cycle guard |
| R6 | Low | `auth/smart/jwks.go:113` | JWKS body read before status check |
| Q1 | Med | `openehr/template/constraints/string.go:53` | `CString` regex compiled on every `Validate` call |
| Q2 | Med | `openehr/bmm/internal.go:40` | `marshalDiscriminated` marshal→unmarshal→re-marshal round-trip |
| Q3 | Med | `openehr/bmm/function.go:120` | `Function.MarshalJSON` via `map[string]any` → nondeterministic key order |
| Q4 | Low | `auth/clientcreds/clientcreds.go:318`, `auth/jwtbearer/jwtbearer.go:275` | `parseOAuth2Error` duplicates `auth.ParseOAuth2Error` |
| Q5 | Low | `openehr/client/ehr/{composition,directory,ehrstatus}` | `marshalAuditDetails` copy-pasted ×3 |
| Q6 | Low | `openehr/rm/rminfo/lookup.go:123` | `KnownRMTypes` re-sorts on every call |
| Q7 | Low | `internal/bmmgen/render_jsonmar.go:116`, `render_jsonunmar.go:72` | Regex recompiled per file; `strings.Join` for a contains-check |
| Q8 | Low | `transport/audit.go`, `openehr/validation/walk_composition.go:175`, `openehr/client/query/execute.go:45` | Misnamed file; string-concat loop; missing AQL-injection doc warning |
| I1 | Med | `.github/workflows/release.yml:26` | `contents: write` at workflow scope |
| I2 | Low | `scripts/spec-check.sh:46` | Probe IDs interpolated into `grep` BRE pattern |
| I3 | Low | `scripts/ingest-robot-cassettes.sh:82` | `echo "$rel"` expansion; no filename allowlist |
| I4 | Low | `Dockerfile:14` | `GO_VERSION=1.25` floats across patch releases |
| I5 | Low | `internal/bmmgen/generate.go:389` | Generated paths not asserted under `-out` root |

### Verified non-findings (do not re-open)

- **Discovery 304 "body leak"** — false positive: `defer resp.Body.Close()` at `smart/discovery/resolver.go:228` is registered *before* the 304 branch.
- **XXE / billion-laughs in OPT parsing** — Go `encoding/xml` does not parse DTD `<!ENTITY>` declarations and errors on undeclared entities; only input-size limiting is needed (R1).
- **`iss` exact-string comparison** (`smart/idtoken.go:133`) — exact match is what OIDC Core §3.1.3.7 requires; do not add normalization.
- **ReDoS via OPT `C_STRING` patterns** — Go `regexp` is RE2 (linear-time); Q1 is a perf/error-surfacing issue, not ReDoS.
- **Retry of non-idempotent methods on token failure** — `shouldRetry` consults `retriableMethod` before any error-type branch (`transport/retry.go:73`); guard holds.
- **`Token()`/inflight result publication races** — writes to `call.catalog`/`ex.token` happen before `close(done)`; safe per the Go memory model.
- **JSON nesting beyond 10 000 levels** — rejected by `encoding/json`'s scanner depth limit; R3 concerns the sub-10k amplification window only.

---

## Phase 1 — Trust & token path (security)

**Definition of done:** S1–S8 closed; `make ci` green; no public-API breaks except documented option additions.

### Task 1: Reject discovery documents whose `issuer` disagrees with the requested issuer (S1)

**Files:**
- Modify: `smart/discovery/resolver.go:345-354`
- Test: `smart/discovery/resolver_test.go`

Per OIDC Discovery §4.3 the fetched document's `issuer` MUST equal the URL used to fetch it. Today the document value silently wins, letting a hostile/misconfigured server impersonate another issuer downstream (ID-token `iss` checks would then pass for the wrong server).

- [x] **Step 1: Write the failing test** — serve a SMART configuration whose `"issuer"` differs from the resolver's requested issuer; assert `Resolve` returns a `*DiscoveryError` (reason: parse/validation) and a nil catalog. Follow the existing `httptest`-based cases in `resolver_test.go`.
- [x] **Step 2: Run it** — `go test ./smart/discovery/ -run TestResolve -v` → FAIL (catalog currently returned with the document's issuer).
- [x] **Step 3: Implement** — replace the override at `resolver.go:346-348`: *(landed with a dedicated `ReasonIssuerMismatch` instead of `ReasonParseError`; match-success path covered by `TestResolveIssuerMatch`)*

```go
resolvedIssuer := issuer
if wire.Issuer != "" && wire.Issuer != issuer {
	return nil, &DiscoveryError{
		Issuer: issuer,
		Reason: ReasonParseError,
		Inner:  fmt.Errorf("document issuer %q does not match requested issuer %q", wire.Issuer, issuer),
	}
}
```

(Keep `resolvedIssuer` so an absent `wire.Issuer` still resolves to the caller's value; update the now-wrong code comment.)
- [x] **Step 4: Run tests** — `go test ./smart/... -v` → PASS.
- [x] **Step 5: Commit** — `fix(smart/discovery): reject issuer mismatch in discovery document` *(e5030dc + 52b6934)*

### Task 2: Enforce HTTPS on `jwks_uri` (and other endpoint URLs) unless `allowInsecure` (S2)

**Files:**
- Modify: `smart/discovery/resolver.go` (`parse` helper ~line 330, or a check after line 376)
- Test: `smart/discovery/resolver_test.go`

`warnInsecure` (resolver.go:439) only logs. A document pointing `jwks_uri` at `http://…` or an internal host is accepted, and `auth/smart/source.go` will fetch it for signature keys — key-trust over plaintext. Full same-origin enforcement is **deferred** (see header); HTTPS-only is the uncontroversial floor.

- [x] **Step 1: Failing test** — discovery document with `"jwks_uri": "http://attacker.example/keys"`, resolver without `AllowInsecure`; assert resolution fails with a `DiscoveryError`. Add the mirror case: with `AllowInsecure()` it succeeds (warn path).
- [x] **Step 2: Run** — `go test ./smart/discovery/ -v` → FAIL.
- [x] **Step 3: Implement** — in the URL `parse` helper (which already rejects missing scheme/host at lines 334, 364), add: *(landed in `parseAuthEndpoints`; also covers `registration_endpoint`. Residual: `services[].base_url` stays warn-only — candidate follow-up)*

```go
if !r.cfg.allowInsecure && u.Scheme != "https" {
	return nil, fmt.Errorf("%s: scheme %q not allowed (https required)", field, u.Scheme)
}
```

This intentionally covers `authorization_endpoint`/`token_endpoint` too — same trust argument. Keep `warnInsecure` for the `allowInsecure` path.
- [x] **Step 4: Run** — `go test ./smart/... ./auth/... -v` → PASS.
- [x] **Step 5: Commit** — `fix(smart/discovery): require https on catalog endpoints unless AllowInsecure` *(32c464d + efd7679 + 8d648db)*

### Task 3: Stop rendering server error bodies into `WireError.Error()`; gate `RawBody` (S3)

**Files:**
- Modify: `transport/errors.go:93-98`, `transport/client.go:351-363`, `transport/options.go`
- Test: `transport/client_test.go` (existing `WireError` cases)

`WireError.Error()` interpolates `OpenEHR.Message`, and `RawBody` carries the full server response. Both flow into `slog`, OTel span status, and `Observation.Err` — in a healthcare CDR these routinely contain patient identifiers. `REQ-candidate: error values MUST NOT carry server payload content unless explicitly enabled.`

- [x] **Step 1: Failing test** — build a `WireError` with `OpenEHR.Message = "patient 1234 not found"`; assert `err.Error()` does **not** contain `"1234"` but does contain status code + route. Add a test that `WithRawErrorBodies(true)` preserves today's behavior (`RawBody` populated, message rendered).
- [x] **Step 2: Run** — `go test ./transport/ -run TestWireError -v` → FAIL.
- [x] **Step 3: Implement** — *(`Code` + `CodedText` retained by default; only `Message`/`RawBody` gated. Downstream: `openehr/client/query` `AQLError.Error()` now surfaces the PHI-free code when message suppressed — `992d52b`)*
  - `errors.go`: drop the `message=%q` clause from `Error()` (keep `code=` — openEHR error *codes* are not PHI). Document that `OpenEHR.Message`/`RawBody` remain available via `errors.As` for callers who need them.
  - `client.go:356`: populate `RawBody` (and `OpenEHR.Message`) only when `cfg.rawErrorBodies` is set; otherwise truncate `RawBody` to 0 and keep only the parsed error *code*.
  - `options.go`: add

```go
// WithRawErrorBodies opts in to preserving server error payloads on
// WireError. Bodies may contain PHI; leave disabled when error values
// reach logs or traces.
func WithRawErrorBodies(on bool) Option {
	return func(cfg *config) { cfg.rawErrorBodies = on }
}
```

- [x] **Step 4: Run** — `go test ./transport/... -v` → PASS. Check `emitObservation`/OTel paths still compile and span status no longer embeds the message.
- [x] **Step 5: Commit** — `fix(transport): keep server error payloads out of Error() by default (PHI)` *(34127f7 + 992d52b)*

### Task 4: Cap response body reads in the transport (S4)

**Files:**
- Modify: `transport/client.go:236`, `transport/options.go`
- Test: `transport/client_test.go`

- [x] **Step 1: Failing test** — `httptest` server streaming > limit bytes; client with `WithMaxResponseBody(1 << 10)`; assert `Do` returns an error mentioning the limit, not an OOM-sized body.
- [x] **Step 2: Run** — `go test ./transport/ -run TestMaxResponseBody -v` → FAIL.
- [x] **Step 3: Implement** — default 64 MiB, 0 = default, negative = unlimited (documented). *(Landed with exported `DefaultMaxResponseBody` const; error reads `read body: response exceeds limit of N bytes`.)*

```go
limit := c.cfg.maxResponseBody
if limit == 0 {
	limit = 64 << 20
}
var rdr io.Reader = httpResp.Body
if limit > 0 {
	rdr = io.LimitReader(httpResp.Body, limit+1)
}
respBody, err := io.ReadAll(rdr)
if limit > 0 && int64(len(respBody)) > limit {
	return nil, fmt.Errorf("transport: response body exceeds %d bytes", limit)
}
```

Add `WithMaxResponseBody(n int64) Option` following the `WithRetry` pattern at `options.go:69`.
- [x] **Step 4: Run** — `go test ./transport/... -v` → PASS.
- [x] **Step 5: Commit** — `feat(transport): bounded response body reads (default 64 MiB)` *(dfb8c12 + polish)*

### Task 5: Generate or validate OAuth `state`; randomize JTI (S5, S6)

**Files:**
- Modify: `auth/smart/source.go:154-163`, `auth/jwtbearer/assertion.go:200-208`
- Test: `auth/smart/smart_test.go`, `auth/jwtbearer` tests

- [x] **Step 1: Failing tests** — (a) `BeginAuthorization("")` returns a request whose `State` is ≥ 32 base64url chars (exact 43) and unique across two calls; (b) freshly-constructed `ClaimsSigner`s produce distinct JTIs (cross-signer distinctness exercises the rand component).
- [x] **Step 2: Run** — `go test ./auth/... -v` → FAIL.
- [x] **Step 3: Implement** — `source.go` generates when empty (reusing the PKCE `crypto/rand`+base64url idiom, `stateLen=32`); `newJTI` is now 24 bytes = 8 time + 8 counter + 8 `crypto/rand` (kept counter for in-process uniqueness; dropped redundant base64 padding-trim).
- [x] **Step 4: Run** — `go test ./auth/... -v` → PASS.
- [x] **Step 5: Commit** — `fix(auth): generated OAuth state + random JTI entropy` *(01fb841)*

**Deviation (approved):** review surfaced that the project spec ([auth.md:120](../specifications/auth.md) — "the SDK **MUST** verify the `state`", `ErrLaunchInvalidState`) was never implemented, and auto-generating state made the gap acute. Scope was extended (user-approved, breaking change pre-1.0) to **enforce** verification in the SDK: `ExchangeAuthorizationCode` gained a `callbackState` parameter and returns the new `ErrLaunchInvalidState` sentinel on mismatch *before* any token-endpoint call (`a2e7601` + test tightening `83dce0f`). Deferred to Task 16: extract a shared `randBase64URL` helper (dup in `source.go`/`pkce.go`) and migrate `jtiCounter` to `atomic.Uint64`.

### Task 6: Sanitize item-tag header values; copy principal claims (S7, S8)

**Files:**
- Modify: `openehr/client/ehr/itemtag_wire.go:139`, `smart/principal.go:51`
- Test: `openehr/client/ehr` wire tests, `smart` principal tests

- [x] **Step 1: Failing tests** — (a) `FormatItemTagHeader` with `Key: "k\r\nX-Evil: 1"` returns an error (preferred over silent stripping — a tag key with CR/LF is caller error); (b) mutate the map passed into principal construction after the call; assert `PrincipalIdentity.Raw` is unaffected.
- [x] **Step 2: Run** — `go test ./openehr/client/ehr/ ./smart/ -v` → FAIL.
- [x] **Step 3: Implement** — `hasCtrlChars` rejects bytes <0x20 except tab, plus DEL (0x7F), on Key/Value/TargetPath; `principalFromClaims` uses `maps.Clone`. Review follow-up also cloned `LaunchContext.Raw` (same aliasing) — `15641bb`.
- [x] **Step 4: Run** — `go test ./openehr/client/... ./smart/... -v` → PASS.
- [x] **Step 5: Commit** — `fix(ehr,smart): reject control chars in item-tag headers; copy principal claims` *(8b40580 + 15641bb)*

### Task 7: Check JWKS fetch status before reading the body (R6)

**Files:**
- Modify: `auth/smart/jwks.go:113-122`

- [x] **Step 1**: Reorder — status check first, drain ≤ 4 KiB to `io.Discard` on non-2xx, then the existing `io.LimitReader(resp.Body, 1<<20)` read. Behavior-preserving; existing tests must stay green.
- [x] **Step 2: Run** — `go test ./auth/smart/ -v` → PASS.
- [x] **Step 3: Commit** — `chore(auth/smart): check JWKS status before body read` *(a66ac41)*

---

## Phase 2 — Untrusted-input hardening (openehr packages)

**Definition of done:** R1–R4, Q1 closed; crafted-input tests in place; `make ci` green.

### Task 8: Bound OPT / BMM / upload input sizes (R1)

**Files:**
- Modify: `openehr/template/parse.go` (reader entry), `openehr/client/definition/template.go:170`, `openehr/bmm/load.go:20`
- Test: each package's parse/upload tests

- [x] **Step 1: Failing tests** — feed each entry point a reader larger than its cap (use `io.LimitReader` over `rand.Reader` masked to ASCII, or a repeat-reader); assert a size-limit error, not an OOM or parse error.
- [x] **Step 2: Run** — `go test ./openehr/template/ ./openehr/client/definition/ ./openehr/bmm/ -v` → FAIL.
- [x] **Step 3: Implement** — shared idiom, local constants (no new package needed). *(Caps are unexported `var`s for test-overridability; OPT path uses `io.LimitedReader` with an `N==0` check after decode + requireEOF — see boundary test. `bmm.Load` wraps new `ErrInputTooLarge`. Commits a7aab6f + 60d673f.)*

```go
const maxOPTBytes = 32 << 20 // generous: real OPTs are < 5 MiB

lr := io.LimitReader(r, maxOPTBytes+1)
data, err := io.ReadAll(lr)
if err != nil { ... }
if int64(len(data)) > maxOPTBytes {
	return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalidOPT, maxOPTBytes)
}
```

Caps: OPT parse 32 MiB, `UploadTemplate` 32 MiB, `bmm.Load` 32 MiB. For the streaming `xml.Decoder` path in `parse.go`, wrap the reader itself with the limit (decoder errors at the cap; map that error to the message above).
- [x] **Step 4: Run** — `go test ./openehr/... -v` → PASS.
- [x] **Step 5: Commit** — `fix(template,bmm,client): bound untrusted input reads` *(a7aab6f + 60d673f)*

### Task 9: Depth-limit OPT tree build and path walking (R2)

**Files:**
- Modify: `openehr/template/parse.go` (`buildNode`/`buildComplexObject`), `openehr/template/path.go:199` (`walkPath`)
- Test: `openehr/template/parse_test.go` (synthesize a >limit-depth OPT), `path_test.go`

- [x] **Step 1: Failing test** — generate an OPT XML string with deeply nested `children` complex objects; assert `ParseOPT` returns `ErrInvalidOPT` (depth); assert `walkPath` on a hand-built >limit tree returns `ErrPathNotFound`-class error. *(Boundary pinned at exact cap and cap+1.)*
- [x] **Step 2: Run** — `go test ./openehr/template/ -v` → FAIL.
- [x] **Step 3: Implement** — thread `depth int` through the recursive builders and `walkPath`; `var maxOPTDepth = 128` (var for test-overridability; guard fires at `depth > maxOPTDepth`). Error on exceed; no API change.
- [x] **Step 4: Run** — `go test ./openehr/template/ -v` → PASS.
- [x] **Step 5: Commit** — `fix(template): depth limits on OPT tree build and path walk` *(28645b4 + 87c553c)*

### Task 10: Depth-limit generated polymorphic JSON decode (R3 — fix in the generator)

**Files:**
- Modify: `internal/bmmgen/render_jsonunmar.go` (emission templates), `openehr/rm/typereg/` (decode helper), then `make codegen`
- Test: hand-written `openehr/rm/` test with a deeply nested `CLUSTER` document

`encoding/json`'s scanner rejects >10 000 nesting, but below that each generated `UnmarshalJSON` re-enters `typereg.DecodeAs` → fresh `json.Decoder` per `json.RawMessage`, giving O(depth × size) re-validation and deep call stacks. A 5 MB / 9 000-deep document is a CPU sink.

- [x] **Step 1: Failing test** — `TestDecode_maxDepthExceeded` builds ~2 000-deep nested JSON; asserts `errors.Is(ErrMaxDepthExceeded)`. Plus `jsonNestingDepth` unit + boundary tests, and `TestDecode_shallowOK` (no false positive).
- [x] **Step 2: Run** → FAIL first.
- [x] **Step 3: Implement** — **Deviation (simpler, no generator change):** `Registry.Decode` is the single polymorphic-dispatch chokepoint (every child routes through `DecodeAs` → `Decode`). Added `maxDecodeDepth = 512`, a string/escape-aware early-exiting `jsonNestingDepth` scanner, and a pre-dispatch reject wrapping new `ErrMaxDepthExceeded` — all in hand-written `openehr/rm/typereg/registry.go`. **No `internal/bmmgen` change, no regeneration.** Spec review verified the chokepoint has no hole (759 DecodeAs sites; the only non-typereg fallback decodes non-recursive leaf types) and confirmed end-to-end that rm.Cluster decode rejects at `/items/0`.
- [x] **Step 4: Run** — `go test ./... ` green; no codegen needed.
- [x] **Step 5: Commit** — `fix(rm/typereg): bound nesting depth in polymorphic decode (DoS guard)` *(9257d30 + 102e919)*

### Task 11: Empty-body guard in `GetStoredQuery`; cycle guard in `effectiveProperties` (R4, R5)

**Files:**
- Modify: `openehr/client/definition/stored_query.go:137`, `internal/bmmgen/render_rminfo.go:119`
- Test: `stored_query` tests; `internal/bmmgen` unit test with a cyclic two-class schema

- [x] **Step 1: Failing tests** — (a) stub transport returning 200 empty body → assert no decode error; (b) `effectiveProperties` on a cyclic A⇄B ancestor plan returns instead of stack-overflowing.
- [x] **Step 2: Run** — both → FAIL/overflow.
- [x] **Step 3: Implement** — (a) sibling empty-body guard (also trims `version` like `name`); (b) separate `visitedClass` map in the `visit` closure. Diamond-inheritance output identity proven; `make codegen-verify` clean (generated output byte-identical).
- [x] **Step 4: Run** — `go test ./openehr/client/definition/ ./internal/bmmgen/` → PASS.
- [x] **Step 5: Commit** — `fix(definition,bmmgen): empty-body guard; ancestor cycle guard` *(7e18dc6 + 5a878eb)*

### Task 12: Compile `CString` patterns at parse time (Q1)

**Files:**
- Modify: `openehr/template/constraints/string.go`, `openehr/template/parse_primitives.go:178` (`buildString`)
- Test: `openehr/template/parse_primitives_test.go`

Today every `Validate` call recompiles `c.Pattern` (string.go:53), and an unparseable pattern in the OPT only surfaces when a value happens to hit that element.

- [x] **Step 1: Failing test** — in-package tests assert `NewCString` pre-compiles valid patterns (`re != nil`), leaves invalid nil, and that match/mismatch/bad-pattern/zero-value behavior is unchanged.
- [x] **Step 2: Run** → FAIL (undefined NewCString).
- [x] **Step 3: Implement** — *(Scoping: the review's Q1 is perf, not ReDoS — Go regexp is RE2/linear. Surfacing bad patterns at parse time would need a `buildString` signature change rippling through `buildPrimitive`; deferred as not in the finding. Bad patterns still surface as `CodeInvalidValue` at Validate, unchanged.)* Added unexported `re` field + exported `NewCString` constructor (cross-package: `buildString` is in `template`, struct in `constraints`); `Validate` uses cached `re`, lazy local-compile fallback (no write-back → concurrency-safe).

```go
type CString struct {
	Pattern string
	List    []string
	Default string

	re *regexp.Regexp // compiled at parse time; nil → compile on first use
}
```

`buildString` compiles and sets `re` (error → strict/lenient handling). `Validate` uses `c.re` when non-nil, else falls back to today's compile path (keeps hand-constructed literals in tests working; note `CString` stays a value type — the pointer field is shared, which is fine since `*regexp.Regexp` is concurrency-safe).
- [x] **Step 4: Run** — `go test ./openehr/template/... ./openehr/validation/... -v` → PASS.
- [x] **Step 5: Commit** — `perf(template): compile C_STRING patterns once at parse time` *(d8e5d8e)*

---

## Phase 3 — Simplification & performance

**Definition of done:** Q2–Q8 closed; no behavior change except documented determinism fix (Q3); `make ci` green.

### Task 13: Deterministic, allocation-light BMM marshalling (Q2, Q3)

**Files:**
- Modify: `openehr/bmm/internal.go:40-…` (`marshalDiscriminated`, `marshalClassObject`), `openehr/bmm/function.go:120`
- Test: `openehr/bmm` round-trip tests; add a determinism test

**DESCOPED after investigation** (`b45e047`):

- **Q3 (determinism) — non-issue.** The premise ("`map[string]any` ordering is random") is wrong: Go's `encoding/json` **sorts string map keys**, so `Function.MarshalJSON` output is already deterministic. Verified empirically — byte-identical across 50×5 marshal runs. No determinism bug exists.
- **Real residue applied:** `Function.MarshalJSON` copied `Parameters` into an identical intermediate map for no reason. Removed it (direct assign) and documented the stable-ordering guarantee. Round-trip corpus green.
- **Q2 (round-trip removal) — deliberately NOT done.** `marshalDiscriminated`'s `json.Marshal`→`Unmarshal`→re-emit is *correct*. Every importer of `openehr/bmm` outside the package is build-time tooling (`bmmgen` generator, `bmmdiff` CLI) — there is **no** openEHR runtime/request path that marshals BMM, so the allocation cost is paid only by codegen and tests. The round-trip is improvable (the common struct's field names are statically known, so direct emission could skip it), but reproducing its exact sorted-key / empty-optional-skipping output across all property/parameter variants is fiddly and validated only by the model-level round-trip corpus (`roundtrip_test.go` asserts `reflect.DeepEqual` of the reloaded model, not byte-equality). Zero runtime payoff + corpus-only validation = poor risk/reward for a hardening pass. Descoped on risk/value grounds (revisit if BMM marshalling ever moves onto a runtime path).

- [x] Applied: `simplify(bmm): drop redundant Parameters map copy in Function.MarshalJSON` *(b45e047)*

### Task 14: Deduplicate `parseOAuth2Error` and `marshalAuditDetails` (Q4, Q5)

**Files:**
- Modify: `auth/clientcreds/clientcreds.go:318`, `auth/jwtbearer/jwtbearer.go:275` (delete local copies → call `auth.ParseOAuth2Error`)
- Modify: `openehr/client/ehr/composition/composition.go:289`, `…/directory/directory.go:260`, `…/ehrstatus/ehrstatus.go:178` → single helper in `openehr/client/ehr` (e.g. `audit_wire.go`)
- Test: existing suites cover both paths

- [x] **Step 1**: Pure refactor — both local `parseOAuth2Error` deleted → `auth.ParseOAuth2Error`; three local `marshalAuditDetails` deleted → new exported `ehr.MarshalAuditDetails` in `openehr/client/ehr/audit.go` (parent package; subpackages import it as `openehrclient`, no cycle). Behavior byte-identical; reviewer confirmed zero residual dupes.
- [x] **Step 2: Run** — `go build ./... && go test ./auth/... ./openehr/client/... -v` → PASS.
- [x] **Step 3: Commit** — `refactor(auth,ehr): deduplicate parseOAuth2Error and marshalAuditDetails` *(eb82f58)*

### Task 15: Cache `KnownRMTypes`; tidy generator hot spots (Q6, Q7)

**Files:**
- Modify: `openehr/rm/rminfo/lookup.go:123`, `internal/bmmgen/render_jsonmar.go:116`, `internal/bmmgen/render_jsonunmar.go:72`
- Test: existing suites

- [x] **Step 1**: `lookup.go` — `KnownRMTypes` memoized via `sync.Once` on the (immutable-after-construction) `lookup`, returns `slices.Clone` (works for Default + New, no value-copy since used only via `*lookup`). `render_jsonmar.go` — qualifier regex cached by qualifier string via `qualifierClassRE` (mutex+map; identical pattern; covers both renderer paths). `render_jsonunmar.go` — `strings.Join`+Contains replaced with loop-and-break (markers can't straddle whole-class chunks); dead `regexp` import removed.
- [x] **Step 2: Run** — `go test ./openehr/rm/rminfo/ ./internal/bmmgen/ && make codegen-verify` → PASS (codegen-verify: no `*_gen.go` diff).
- [x] **Step 3: Commit** — `perf(rminfo,bmmgen): cache known-type list and generator qualifier regex` *(f3b9e74 + 94167b8 gofmt)*

### Task 16: Naming, docs, and micro-cleanups (Q8)

**Files:**
- Rename: `transport/audit.go` → `transport/headers.go` (`git mv`; contents are `Prefer`/`CallerAttribution`/ETag helpers, not audit logic)
- Modify: `openehr/validation/walk_composition.go:175` (`formatAllowedTypes` → `strings.Join` over a pre-built slice)
- Modify: `openehr/client/query/execute.go:45` — doc warning on `ExecuteString`:

```go
// ExecuteString is an escape hatch for raw AQL. aqlText MUST be a
// static or programmatically validated statement; never interpolate
// caller-supplied values into it — pass them via params, which the
// CDR binds as named placeholders. String-built AQL is injectable.
```

- Modify: `auth/smart` — extract a private `randBase64URL(n int) (string, error)` helper in `pkce.go` and use it from both `BeginAuthorization` (`source.go`) and `NewPKCEPair`/state gen (dedupe the identical `rand.Read`+`base64URLEncode` block; carried over from Task 5 review).
- Modify: `auth/jwtbearer/assertion.go` — migrate `ClaimsSigner.jtiCounter uint64` to `atomic.Uint64` (`s.jtiCounter.Add(1)`), clearing the long-standing `atomictypes` lint and 32-bit alignment concern (carried over from Task 5 review).

- [x] **Step 1**: Applied all five (rename via `git mv`; `strings.Join`; ExecuteString warning; `randBase64URL` dedup; `jtiCounter` → `atomic.Uint64`). Behavior-neutral; reviewer confirmed byte-identical output, no value-copy, gofmt/vet clean.
- [x] **Step 2: Run** — `go build ./... && go test ./... ` → PASS.
- [x] **Step 3: Commit** — `refactor(transport,validation,query,auth): naming, docs, and micro-cleanups` *(c8a8faa)*

---

## Phase 4 — Tooling, scripts, CI

**Definition of done:** I1–I5 closed; release workflow and scripts pass a dry run; `make ci` green.

### Task 17: Scope release workflow permissions (I1)

**Files:**
- Modify: `.github/workflows/release.yml:26-27`

- [x] **Step 1**: Workflow token now defaults to `contents: read`; split into a read-only `verify` job (checkout, setup-go, `make ci`, build+upload release notes) and a `publish` job (`needs: verify`, `if: dry_run == false`, `permissions: contents: write`) that only downloads the notes artifact and runs `gh release create`. Dry-run path preserved. *(No `actionlint`/yaml parser in sandbox — validated structurally: no tabs, correct indentation, consistent `needs.verify.outputs.*` refs. **Pending Phase-4 review** — see status note.)*
- [x] **Step 2: Commit** — `ci(release): least-privilege permissions via verify/publish split` *(8c6cf1f)*

### Task 18: Script hardening (I2, I3)

**Files:**
- Modify: `scripts/spec-check.sh:46` (`grep -q` → `grep -qF`), same for any sibling interpolations in that script
- Modify: `scripts/ingest-robot-cassettes.sh:82-91` — `printf '%s' "$rel"` instead of `echo "$rel"`; allowlist `[[ "$base" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "skip: $base"; continue; }`

- [x] **Step 1**: Applied — `spec-check.sh` lines 46 + 122 use `grep -qF` (probe/REQ ids matched literally); `ingest-robot-cassettes.sh` uses `printf '%s'` for path munging and an allowlist `^[A-Za-z0-9._-]+$` on the external source filename. `bash -n` clean on both; `make spec-check` still OK. *(**Pending Phase-4 review** — see status note.)*
- [x] **Step 2: Commit** — `chore(scripts): fixed-string grep + filename allowlist` *(51fadfb)*

### Task 19: Pin builder image; assert generator output paths (I4, I5)

**Files:**
- Modify: `Dockerfile:14` — `ARG GO_VERSION=1.25.<current patch>` (match `go.mod` toolchain line; bump via PR like `LINT_IMAGE` in the Makefile)
- Modify: `internal/bmmgen/generate.go:389-405` (`writeAtomic` callers) — after building each output path:

```go
clean := filepath.Clean(path)
if rel, err := filepath.Rel(outDir, clean); err != nil || strings.HasPrefix(rel, "..") {
	return fmt.Errorf("bmmgen: generated path %q escapes output dir %q", path, outDir)
}
```

- [x] **Step 1: Failing test** — `confine_test.go`: `TestConfinePath` (escape→err, child→ok) + `TestSafeFileBase` (plain component ok; `..`, `a/b`, `dir/`, `a\b`, `openehr/rm` rejected).
- [x] **Step 2: Implement + run** — two source guards: `confinePath(opts.OutDir, outDir)` (covers BMM-derived `OutSubDir`) and `safeFileBase(f.FileBase)` (covers BMM-derived file base) — together confine every per-file path; fixed-component paths (`typereg_gen.go`, `rminfo/lookup_gen.go`) are inherently safe. `Dockerfile` `GO_VERSION` → `1.25.0`. `go test ./internal/bmmgen/` + `make codegen-verify` → PASS (no drift; guards accept real BMM). (Docker build not run in sandbox; tag form `golang:1.25.0-alpine` valid.)
- [x] **Step 3: Commit** — `chore(docker,bmmgen): pin Go patch version; confine generator output paths` *(001f443)*

**Phase-4 review (Tasks 17–19):** spec ✅ (all I1–I5 compliant; generator guards have no bypass — `FileBase` dot-traversal is neutralised by `.`→`_`, and raw separators are rejected by `safeFileBase`). Quality "yes, with fixes": applied the dead `verify.version` output removal (`b77abf1`); **rejected** dropping the `grep -qF "#### ${pr} "` trailing space (it prevents `PROBE-1`/`PROBE-10` prefix collisions). Noted as follow-ups (pre-existing / untestable here): release `--target "$GITHUB_SHA"` peeling for annotated tags, dead `ALPINE_VERSION` ARG, optional removal of the `publish`-job checkout (kept to preserve the original's known-good `gh`-in-checkout behavior), and extending the ingest allowlist to the other dynamic loops.

---

## Mapping to specs

- No existing REQ rows cover these findings. If maintainers adopt the flagged `REQ-candidate` items (bounded reads, PHI-free errors), add rows to [docs/specifications/REQ.md](../specifications/REQ.md) and `traceability.yaml` in the same PR as the implementing task, per the [header convention](README.md#header-convention-load-bearing).
- Phase 1 Task 3 interacts with REQ-09x observability work (error values feed `Observation`); cross-check the observer contract in [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) when landing.

## Suggested PR slicing

One PR per phase is reviewable (largest is Phase 1 at ~7 small diffs); Tasks 10 and 13 are the only ones touching generated output — keep each in its own commit with `make codegen` output separated from generator changes.
