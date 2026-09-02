# Plan — Read-path decode taxonomy

**Date:** 2026-08-30
**Status:** Done (2026-08-30) — all seven phases executed; [REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure) `landed` with [PROBE-101](../../specifications/conformance.md#probe-101--2xx-decode-failure-is-surfaced-not-swallowed) Implemented (Sandbox), and the REQ-052 encode-sentinel rider landed with it.
**Owner:** SDK maintainers
**Covers:** REQ-151 (new — typed 2xx decode failure), REQ-052 (amend — encode-side sentinel), REQ-094 (amend — cross-reference only), ADR 0018 (new — raw bytes on the typed decode error)
**Probes:** PROBE-101 (new, Sandbox)
**Implementation:** landed
**Depends on:** nothing. Independent of the [Definition metadata decoding plan](2026-08-30-definition-metadata-decoding.md); the two touch the same Definition list functions textually, so whichever lands second rebases, but they share no types or contract, and the § REQ-151 empty-list-body exclusion is self-contained (an empty list body is success, whoever defines its slice shape), so neither plan orders the other.
**Defers:** lifting REQ-094's keyed exception for `ehr.Create`'s **empty-body** 2xx arm (stays bare `transport.ErrInvalidShape`; typing it means routing EHR creation through the write-result contract — a separate REQ-094 amendment); changing `Prefer` defaults; typing the `Prefer=identifier` decode arm (`openehr/client/ehr/identifier.go` — REQ-094 negotiation surface, already typed as `ErrInvalidShape`); the auth/SMART JSON decodes (`auth/*`, `smart/*` — separate `net/http` stacks, not the `transport.Client` read path); aligning `canxml`'s single-sentinel-both-directions shape with the new `canjson` encode sentinel; the optional PROBE-038 `CONTRIBUTION.versions` missing-`_type` row (regression pin on already-landed REQ-052 behaviour, unrelated to this taxonomy); the **decode-side producer for REQ-052's § Floating-point precision MUST** — its open arm is mantissa precision loss, which still rounds silently (`TestUnmarshalMantissaPrecisionLossIsSilent` pins today's behaviour and must be inverted when the producer lands) — a `canjson` decode-path change, not an extension of the encode-side sentinel this plan ships.

> **Closed:** the `ehr.Create` empty-body deferral above is closed by [2026-09-01-ehr-create-empty-2xx-typing.md](2026-09-01-ehr-create-empty-2xx-typing.md) (2026-09-02) — that arm is now a typed `*ehr.NoRepresentationError`, and `ehr.Create` no longer routes through `transport.Decode` at all. Every other deferral above stands.

## Goal

When a 2xx response body cannot be decoded as the requested type, the SDK today returns a wrapped error and drops the bytes — the payload the server already delivered is unrecoverable (`transport/client.go:589-591`; `Metadata` is headers-only). The archived [write-result plan](2026-08-18-write-result-contract.md) deferred typing `ehr.Create`'s committed-but-unusable arm *because* it decodes through this shared path — this plan ships the missing primitive: a `transport.DecodeError` that carries the raw body, used by `transport.Decode` and by every hand-rolled 2xx response-body decode outside the write-result funnel ("read path" in this plan means the response-decode path — it includes the Definition upload/PUT leaves, whose requests are writes but whose 2xx metadata responses are decoded the same way). As a rider it closes the second deferral from the same plan: a typed sentinel for `canjson` marshal failures.

## Definition of Ready

- **Covers:** REQ-151, REQ-052, REQ-094, ADR 0018 — all listed above with canonical links planned below.
- Canonical normative prose for REQ-151 is written in Phase 0 (`transport.md`), the REQ-052 clause likewise; both land ahead of the code in this plan's own commits. Until Phase 0 lands, the spec links in Mapping to specs are forward references — nothing (code comments, traceability, other docs) cites REQ-151 before its § exists. REQ-151 is **reserved** in [REQ.md § Numbering policy](../../specifications/REQ.md#numbering-policy) as of this plan's merge.
- The irreversible fork (raw bytes attached to the error **by default**) is settled by ADR 0018, Accepted in Phase 0 before any code. Rationale: once consumers rely on the populated field, gating it later is a break; that is the same one-way door REQ-093 walked through in the opposite direction.
- The seed's refuse paths are likewise citations of the future §, not contracts this plan carries on its own: a non-2xx stays `*transport.WireError`; an empty representation-expecting 2xx body stays `transport.ErrInvalidShape`; the keyed exclusions keep their owning contracts (REQ-094). None is citable from code or traceability until the Phase 0 § exists.
- Phases name their verification: `make spec-check`, `make ci` (or the documented no-Docker fallback: `fmt-check`, `vet`, `spec-check`, `build`, `go test ./... -count=1`).

## Definition of Done

- Code and tests land with `// REQ-151` / `// PROBE-101` citations; guard tests are named so removing a guard fails a named test.
- `traceability.yaml` gains the REQ-151 entry; REQ-052 and REQ-094 entries gain this plan in `plans:` plus a one-sentence `notes:` amendment record; REQ.md `Impl.` column agrees.
- The two indexes `make spec-check` cannot see: a `docs/roadmap.md` row for REQ-151 (model: the REQ-150 row) and a touch on the transport row (line ~110); the REQ.md band-prose reservation flipped to "taken", with the band-table headroom cell updated.
- `transport.md` line 5 (Covers sentence) and the Coverage table gain REQ-151; `conformance.md` PROBE-101 entry reaches `Status: Implemented` and the Coverage matrix gains its row.
- `make spec-check` and `make ci` pass.
- Plan archived under `docs/plans/archive/`.

## Implementation checklist

| Step | Status |
|---|---|
| Spec/registry updated (transport.md § REQ-151, wire.md REQ-052 clause, REQ.md row + band prose, ADR 0018, PROBE-101 catalog entry) | ☑ Phase 0 landed 2026-08-30 (`5ea4975`, `045a40e`, `aaf11f1`); the REQ-052 clause reworded to landed behaviour in Phase 5 (`7d8bc38`) |
| Indexes spec-check misses (roadmap.md rows, REQ.md band prose) | ☑ band prose + band-table headroom in Phase 0; `roadmap.md` REQ-151 row and the transport row's error-taxonomy note in Phase 6 |
| Code (`transport/errors.go`, `transport/client.go`, six leaf files, `canjson/marshal.go`) | ☑ `01ef1cc` (transport), `585dea6` (five Definition leaves), `c7b6093` / `5591750` / `f5d4bd5` (system, demographic, composition), `14e7f94` (canjson) |
| Tests with `// REQ-` / `// PROBE-` comments | ☑ per-leaf `decode_error_test.go` files plus `79fd41c`, which pins the keyed exclusions negatively |
| `make spec-check` | ☑ OK at every commit, including after the archive move |
| `make ci` | ☑ Docker unavailable on the host, so the documented fallback ran instead: `go vet -unreachable=false ./...` (the `vet` target's own form) clean, `golangci-lint run` **0 issues** on the touched packages, `go test ./... -count=1` green full-tree, `gofmt -l .` reporting only the three pre-existing generated ANTLR files the lint config treats as lax. The containerised gate stays PR CI's |

## Phases

### Phase 0 — Decision and spec (ADR Accepted before code)

**Tasks:**

- [x] Write **ADR 0018 — Raw response bytes on the typed 2xx decode error** (`docs/adr/0018-raw-bytes-on-decode-error.md`, heading form `# ADR 0018 — …`, metadata bullets per ADR 0017's richer form: Status / `Strand: none — direct decision (this plan's Phase 0); no prior open research strand` (the ADR 0011/0012 spelling) / Introduces REQ-151 / Amends REQ-052, REQ-094 / Plan link / Related ADR 0004, REQ-093). Decision: the `Body` field on `transport.DecodeError` is **always populated** on a 2xx decode failure — no opt-in knob. The ADR must answer, in Consequences, why 2xx decode-failure bytes differ from the non-2xx bodies REQ-093 gates behind `WithRawErrorBodies`: a 2xx body is the very representation the caller requested and would have received fully decoded had it parsed — the caller is already entitled to those bytes; a non-2xx error body is diagnostic content the caller never asked for and routinely carries server-side clinical narrative. Supporting points: the bytes have already crossed the wire into the process; `WithMaxResponseBody` (default 64 MiB, transport.md § REQ-093) bounds them; `Error()` stays value-free so log surfaces are unchanged; the *field* is documented as potentially PHI-bearing, exactly how `NoRepresentationError.Cause` is treated. Alternatives considered: reuse `WithRawErrorBodies` (default still loses the payload — the defect survives), a new opt-in `WithRawDecodeBodies` (same problem unless default-on, in which case it is a no-op knob; ADR 0004's consequence line — no second strict-mode knob in v1 — is the precedent against it).
- [x] Update `docs/adr/README.md` table with the 0018 row (Accepted, 2026-08-30).
- [x] Write **`transport.md` § REQ-151** as `## REQ-151 — Typed 2xx decode failure`, inserted after the REQ-150 block (~line 239), before `## Coverage`. The bullets below are the `sdd-specify` seed — the § authored in Phase 0 is the only normative home, and its RFC-2119 wording is written there, not copied from here:
  - After a 2xx, when the body cannot be decoded as the requested representation, the returned error is a `*transport.DecodeError` recoverable via `errors.AsType` (the REQ-025 preferred matcher; `errors.As` reaches it identically), carrying the raw response bytes in `Body`, wrapping the decoder's error (`Unwrap`), with a value-free `Error()` in the REQ-093 discipline (method + route template + classification; never the body, never interpolated cause text).
  - The existing `(*T, *Metadata, error)` triple still populates `*Metadata` on this path.
  - A non-2xx stays `*transport.WireError` and is never a `DecodeError`.
  - An **empty** 2xx body on a read that expected a representation keeps today's `transport.ErrInvalidShape` contract — deliberately not unified under the new type (state this explicitly). Keyed exclusion, self-contained: the Definition **list** leaves, where an empty 2xx body is a successful empty catalog, not a failed representation decode — never `ErrInvalidShape`, never a `DecodeError`. The slice shape of that success is the Definition list contract's business (REQ-144 once its [plan](2026-08-30-definition-metadata-decoding.md) lands; today's landed behaviour is already success), so this § only points there and holds with or without it. Only a non-empty list body that fails to decode (e.g. an object where an array is expected) is REQ-151's.
  - Scope, stated precisely (the § does not use the ambiguous term "read path"): every 2xx **response-body decode** on the `transport.Client` stack that is not owned by the REQ-094 write-result contract — regardless of the request's verb, so the Definition upload/PUT leaves are in scope: their requests are writes, but their 2xx metadata responses are decoded like any read. The ten `transport.Decode` call sites inherit it; the hand-rolled sites are enumerated in Phases 2–3. Keyed exclusions, named in the §: the write-result funnel (`ehr.WriteResult` callbacks and `contribution.Commit`, owned by REQ-094 / `NoRepresentationError`) and the `Prefer=identifier` arm (`ehr.ResolveIdentifierBody`, REQ-094).
  - The `Body` field is PHI-bearing by design; ADR 0018 is the authority.
  - Footer: `- **Lives in:** [`transport/`](../../transport)` + `- **Probes:** PROBE-101 (Implemented — Sandbox)`.
- [x] Amend `transport.md` line 5 Covers sentence and add the `| REQ-151 | transport/ … |` Coverage row.
- [x] Amend **§ REQ-094** (line ~127): one sentence noting the *decode-failure* arm of `ehr.Create` is now typed by REQ-151 while the empty-body keyed exception stands unchanged.
- [x] Amend **`wire.md` § REQ-052**: a new clause under Canonical JSON — a `canjson.Marshal` / `MarshalIndent` failure wraps the new encode-only sentinel `canjson.ErrInvalidValue`, `errors.Is`-distinguishable from the decode-side `canjson.ErrInvalidShape` and from `transport.ErrInvalidShape` (three distinct sentinel values). Note neutrally that `canxml` reuses one sentinel across both directions (`canxml/marshal.go`); aligning it is out of scope.
- [x] Add the **PROBE-101 catalog entry** to `conformance.md` (before implementation — ordering is normative): Title "2xx decode failure is surfaced, not swallowed"; Preconditions: a sandbox server that can serve a 200 whose JSON body does not match the requested type, a 404, and a list route answering a JSON object where an array is expected; Wire assertion — kept at observable-behaviour level, since a probe must not assert on error types (REQ-080): a 200 with an undecodable body fails the call with a non-nil error and never a silently zero-valued result; the 404 arm fails with a non-nil error; the list leaf served an object fails with a non-nil error. Error-type identity, `Body` recovery, and value-free `Error()` are pinned by `transport/` and `openehr/client/definition/` unit tests, not by this probe — the PROBE-093 sentence, per REQ-080. Effect: read-only. Modes: Sandbox. Status: Draft (flipped in Phase 4).
- [x] REQ.md: registry row for REQ-151 (after the REQ-150 row, line ~112) with `Impl.` `planned`, flipped to `landed` at close-out; flip the existing band-prose **reservation** (landed with this plan's PR, 114/115 precedent) to "taken" ("**151** was reserved by this plan and taken YYYY-MM-DD by REQ-151 …, leaving 152–159 free"), and update the band-table headroom cell.
- [x] `traceability.yaml`: new REQ-151 block (`canonical: docs/specifications/transport.md#req-151--typed-2xx-decode-failure`, `status: draft`, `implementation: planned` until code lands in this same PR, `packages: [transport, openehr/client/definition, openehr/client/system, openehr/client/demographic, openehr/client/ehr/composition]`, `adrs: [docs/adr/0018-raw-bytes-on-decode-error.md]`, `plans:` this file). Do **not** list `probes: [PROBE-101]` while the catalog entry is Draft and the REQ is landed/partial — either flip both in the same commit (Phase 4) or keep `implementation: planned` until then; spec-check fails a landed REQ citing a Draft probe. Append this plan + a one-line `notes:` amendment record to the REQ-052 and REQ-094 blocks.

**Definition of done:** ADR 0018 Accepted; `make spec-check` passes with the REQ-151 entry present (`implementation: planned`).

### Phase 1 — `transport.DecodeError` and `Decode`

**Tasks:**

- [x] Failing tests first in `transport/client_test.go` (external `_test` package, stdlib `testing` only):
  - `TestDecodeTypedErrorCarriesBody` — httptest 200 with body `[1, 2, 3]` (a top-level JSON array cannot decode into any struct type, so the failure is guaranteed; an object with unknown keys such as `{"unexpected":"shape"}` decodes cleanly — unknown fields are ignored and no required-field validation runs) decoded as an RM type; assert `errors.AsType[*transport.DecodeError]` (REQ-025's preferred matcher since PR #135) recovers the typed error, `Body` equals the served bytes, `Unwrap` reaches the decoder error, metadata on the triple is non-nil.
  - `TestDecodeErrorStringIsValueFree` — `Error()` contains the method and route but no substring of the body **and no substring of the wrapped decoder error's text** (the decoder's `parse %q`-style messages embed payload values — the leak this guard exists to block). Removing the value-free guard must fail this named test.
  - `TestDecodeNon2xxStaysWireError` — 404: `errors.AsType[*transport.WireError]` matches, `errors.AsType[*transport.DecodeError]` does not.
  - `TestDecodeEmptyBodyStaysInvalidShape` — 200 empty body: `errors.Is(err, transport.ErrInvalidShape)` true, not a `DecodeError`.
  - `TestDecodeErrorNilReceiver` — a typed-nil `*transport.DecodeError` answers `Error()` and `Unwrap()` without panicking (REQ-025 nil-receiver axis; the `NoRepresentationError` and `nilaqlerror_test.go` precedent). Removing either nil guard must fail this named test.
- [x] Implement in `transport/errors.go`, next to `WireError`:

```go
// DecodeError reports a 2xx response whose body could not be decoded as the
// requested type. Body carries the raw response bytes (bounded by
// WithMaxResponseBody) so callers can recover the payload the server already
// delivered; the field may contain PHI — Error() never includes it (ADR 0018).
type DecodeError struct {
	Method string // HTTP method of the request
	Route  string // route template, not the expanded URL
	Body   []byte // raw response bytes; PHI-bearing by design
	Inner  error  // the decoder's error
}

// Error answers on a nil receiver instead of panicking: a typed-nil
// *DecodeError is caller-constructible input (REQ-025 § No panics,
// nil-receiver axis — the NoRepresentationError / nilaqlerror_test.go shape).
func (e *DecodeError) Error() string {
	if e == nil {
		return "transport: decode: response body does not match the requested type"
	}
	return fmt.Sprintf("transport: decode %s %s: response body does not match the requested type", e.Method, e.Route)
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}
```

- [x] Replace the decode arm in `transport/client.go` (`Decode`, ~line 589): return `nil, resp.Metadata, &DecodeError{Method: req.effectiveMethod(), Route: req.effectiveRoute(), Body: resp.Body, Inner: err}`. The empty-body arm above it is untouched.
- [x] Run the four tests; then the package suite.

**Definition of done:** the four named tests pass; `go test ./transport/... -count=1` green. The ten existing `transport.Decode` call sites (ehr, directory, ehrstatus, contribution.Get, demographic versioned, template ADL2, query execute) now inherit the typed error — `openehr/client/query/errors.go` `mapQueryError` passes non-`WireError` values through unchanged, so no query-layer change is needed (verified 2026-08-30).

### Phase 2 — Definition-client leaves

The five hand-rolled 2xx decodes in `openehr/client/definition/`. Each keeps its operation-name wrapper and swaps the inner error for the typed one, filling `Method`/`Route` from the request the leaf already built, e.g.:

```go
if err := json.Unmarshal(resp.Body, &out); err != nil {
	return nil, resp.Metadata, fmt.Errorf("definition.ListTemplates: %w",
		&transport.DecodeError{Method: http.MethodGet, Route: routeTemplateList, Body: resp.Body, Inner: err})
}
```

(`errors.AsType` traverses the `fmt.Errorf` wrap, so acceptance is unchanged; reuse each leaf's existing method/route values rather than new constants.)

**Tasks:**

- [x] Failing tests first, per leaf: a 200 whose JSON is an object where an array is expected (`ListTemplates`, `ListStoredQueries` — the object-body arm is PROBE-101's third assertion) and a 200 with a non-conforming metadata body (`UploadTemplate`; `PutStoredQuery` and `PutStoredQueryVersion`, which share the decode site; `GetStoredQuery`); assert the typed error and body recovery. `ListStoredQueries` currently has zero tests — add a happy-path list decode test alongside.
- [x] Convert the five sites: `template.go:219` (UploadTemplate), `template.go:294` (ListTemplates), `stored_query.go:161` (inside the unexported `putStoredQuery` helper, the one decode serving both `PutStoredQuery` and `PutStoredQueryVersion`), `stored_query.go:247` (GetStoredQuery), `stored_query.go:281` (ListStoredQueries). Empty-body arms in all of them are untouched (list arms are the other plan's business; metadata arms keep their Location/synthesis behaviour).
- [x] `go test ./openehr/client/definition/... -count=1`.

**Definition of done:** all five leaves return the typed error on decode failure with the body recoverable; existing behaviour tests unchanged.

### Phase 3 — Remaining response-decode leaves

The in-scope decodes outside `transport.Decode` and outside the Definition client, from the 2026-08-30 census: `system.Capabilities` (`openehr/client/system/system.go:160`), `demographic` `getVersion` — which has **two** decode arms, the `rm.OriginalVersion[json.RawMessage]` envelope (`versioned.go:176`) and the polymorphic version data (`typereg.DecodeAs[rm.Party]` on `env.Data`, `versioned.go:187`) — and `getParty` (`party.go:310`), `composition.get` (`openehr/client/ehr/composition/composition.go:71`). Explicitly **not** converted (keyed exclusions in the §): the `WriteResult` callback decodes (`party.go:320`, `composition.go:332`, `directory.go:303`, `ehrstatus.go:183` — funnel to `NoRepresentationError`), `contribution.Commit` (`contribution.go:108`, same), `ResolveIdentifierBody` (`identifier.go:45`, REQ-094 `Prefer=identifier` arm).

**Tasks:**

- [x] Failing test then conversion, one leaf at a time, same wrapper-plus-typed-inner shape as Phase 2. The demographic and composition leaves decode via `typereg.DecodeAs` / `canjson.Unmarshal` — the typed error's `Inner` wraps whatever those return (including `canjson.DecodeError`), preserving today's `errors.AsType` reach into path/type diagnostics.
- [x] `getVersion` gets one failing test **per arm**: an undecodable `ORIGINAL_VERSION` envelope, and a well-formed envelope whose `data` fails the polymorphic `rm.Party` decode. Both arms return the typed error with `Body` carrying the **full response bytes** (the payload the server delivered — not the `data` sub-slice), so the consumer's recovery story is the same either way.
- [x] `go test ./openehr/client/... -count=1`.

**Definition of done:** every in-scope 2xx response-decode failure surfaces as `*transport.DecodeError`; **each** excluded arm is covered by a named test asserting it is **not** the new type — `contribution.Commit` decode failure stays `*ehr.NoRepresentationError`; a `WriteResult` callback decode failure (e.g. `composition.Update`) stays `*ehr.NoRepresentationError`; `ResolveIdentifierBody` decode failure keeps its `transport.ErrInvalidShape` wrap.

### Phase 4 — PROBE-101

**Tasks:**

- [x] Implement `testkit/probes/transport/probe_101_decode_failure_surfaced.go` (the transport probe dir exists — PROBE-091 is the model) plus its case in `probes_test.go`, driving an httptest-style sandbox server through the three wire-level catalog assertions (200 undecodable body → non-nil error, no zero-valued success; 404 → non-nil error; list leaf served an object → non-nil error). No error-type assertions in the probe (REQ-080) — type, `Body`, and `Error()` identity are already pinned by the Phase 1–2 unit tests.
- [x] Flip the `conformance.md` entry to `Status: Implemented`, add the Coverage-matrix row, and in the same commit set `traceability.yaml` REQ-151 to `implementation: landed` with `probes: [PROBE-101]` and the test files listed; flip the REQ.md `Impl.` cell to `landed`.
- [x] `make probe-status` (informational — inline probes print MISSING by filename heuristic; this one matches the `probe_NNN_*.go` pattern so it should list), then `make spec-check`.

**Definition of done:** probe green in Sandbox mode; spec-check passes with the landed entry.

### Phase 5 — `canjson` marshal sentinel (rider)

**Tasks:**

- [x] Failing test in `openehr/serialize/canjson`: `canjson.Marshal(make(chan int))` satisfies `errors.Is(err, canjson.ErrInvalidValue)` and does **not** satisfy `errors.Is` for `canjson.ErrInvalidShape` nor `transport.ErrInvalidShape`; the underlying `*json.UnsupportedTypeError` stays reachable via `errors.AsType`. A sibling case asserts `canjson.MarshalIndent(make(chan int), "", "  ")` carries the same sentinel.
- [x] Implement in `openehr/serialize/canjson/marshal.go`:

```go
// ErrInvalidValue marks a value the canonical-JSON encoder refuses
// (encode-side counterpart of ErrInvalidShape, which stays decode-only).
var ErrInvalidValue = errors.New("canjson: value cannot be encoded")

func Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}
```

  (`MarshalIndent` gets the same wrap.)
- [x] `go test ./openehr/serialize/... -count=1`. The leaf packages that call `canjson.Marshal` and wrap with their own `fmt.Errorf` chains are unaffected — the sentinel travels inside their wraps.

**Definition of done:** sentinel distinguishability test passes; REQ-052 clause and the traceability `notes:` line from Phase 0 now describe landed behaviour.

### Phase 6 — Close-out

**Tasks:**

- [x] Full-tree verification: `make spec-check`; `make ci` (or the documented no-Docker fallback plus PR CI).
- [x] Roadmap rows (REQ-151 row modeled on REQ-150's; transport summary row); confirm the REQ.md band prose from Phase 0 survived review edits.
- [x] Flip this plan's **Status:** to done and `git mv` it to `docs/plans/archive/` inside the implementing PR (sdd-archive).

**Definition of done:** the Implementation checklist above is all ☑.

## Mapping to specs

- [transport.md § REQ-151](../../specifications/transport.md#req-151--typed-2xx-decode-failure) (new, Phase 0) — canonical home of the typed-decode contract.
- [transport.md § REQ-093](../../specifications/transport.md#req-093--openehr-error-envelope-mapping) — the value-free `Error()` discipline the new type inherits; untouched.
- [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) — cross-reference amendment only; the `ehr.Create` empty-body keyed exception stood at the time of this plan, and was closed 2026-09-02 by [2026-09-01-ehr-create-empty-2xx-typing.md](2026-09-01-ehr-create-empty-2xx-typing.md).
- [wire.md § REQ-052](../../specifications/wire.md#req-052) — encode-sentinel clause (Phase 0 / Phase 5).
- [ADR 0018](../../adr/0018-raw-bytes-on-decode-error.md) (new, Phase 0) — the default-on raw-bytes decision.
- [REQ.md registry](../../specifications/REQ.md) rows REQ-151 / REQ-052 / REQ-094.
