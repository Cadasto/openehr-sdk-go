# Plan — AQL small corrections: 501 sentinel, stored-query prose, TOP+fetch lint, godoc home

**Date:** 2026-08-26
**Status:** Complete
**Owner:** SDK maintainers
**Covers:** no new REQ id — four independent implementation-aligned amendments to landed requirements, each extending the section that already owns its surface (the precedent set by the archived structured-node-predicates and value-free-diagnostics plans): [REQ-055](../../specifications/wire.md#req-055--wire-boundary) (§ AQL executor error taxonomy + two corrected sentences), [REQ-057](../../specifications/wire.md#req-057) (reserved-name guard), [REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text) + [REQ-109](../../specifications/clinical-modeling.md#req-109--aql-static-lint) (envelope-channel lint code — the code's catalogue home is REQ-109's Layer-2 table, which REQ-118 cross-references), [REQ-113](../../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (conformance fix — no prose change)
**Probes:** no new probe id — [PROBE-021](../../specifications/conformance.md#probe-021--aql-parse-error-mapping)'s wire assertion gains the 501 arm; PROBE-028's corpus is unaffected (its lint runs supply no `Options.Query`)
**Implementation:** landed
**Depends on:** nothing in the other three audit plans — every phase here is independent and separately committable
**Defers:** any message-text reading of engine errors (per-engine adapters / conformance tooling, per the maintainer's ruling below); pre-validating stored-query name *syntax* (REQ-057's deliberate choice, unchanged)

## Goal

Close audit findings **AQL-FIT-06, -07, -08, -10** (AQL alignment audit, 2026-08-26 —
maintainer's knowledge base, fit-gap report Part 2). Four small, unrelated defects, grouped only
because each is too small to carry a plan alone: a missing error sentinel, two wrong sentences in
the normative spec plus an unguarded routing collision, a Build/Lint asymmetry on a rule the SDK
already enforces on one side, and a documented caveat sitting where `go doc` cannot show it.

## Phase 1 — The 501 capability sentinel (AQL-FIT-06)

**The ruling this implements (maintainer, 2026-08-26).** The SDK follows the spec-correct
status contract, not any particular engine's: **501 means a capability gap** — the query is
valid AQL, this deployment does not implement the feature; **400 means bad AQL** — syntax,
semantically impossible containment, malformed path syntax. Engine compatibility is a goal, but
not at the cost of the wrong contract; message-text heuristics belong in a per-engine adapter,
never in the SDK's error taxonomy.

**Today:** `mapQueryError` ([`openehr/client/query/errors.go`](../../../openehr/client/query/errors.go))
wraps a `WireError` into `AQLError` on an openEHR error envelope or on status 400/408, and
classifies one sub-kind (`aql.ErrPathResolution`, deliberately narrow). **501 falls through the
switch entirely** and reaches the caller as a plain `transport.WireError` — the single most
useful status to branch on (retry-elsewhere / degrade-gracefully, not fix-your-query) is the one
with no typed handle.

**Tasks:**

- New sentinel in [`openehr/aql/errors.go`](../../../openehr/aql/errors.go) beside
  `ErrPathResolution`:
  `ErrEngineCapability = errors.New("aql: engine does not implement this AQL capability")`,
  godoc stating the ruling verbatim: *the query is valid; this deployment does not implement it.*
- `AQLError` gains an unexported `capability bool` mirror of `pathResolution`; `Is()` matches
  `aql.ErrEngineCapability` on it; `mapQueryError` sets it for `we.StatusCode == 501` (with or
  without an openEHR envelope — the status alone is the signal; **no message heuristics**).
- Spec amendment (same PR, implementation-aligned): the wire.md § *AQL executor* error bullet
  names the full taxonomy — 400/408 → `AQLError`; path-resolution envelope →
  `errors.Is(…, aql.ErrPathResolution)`; **501 → `errors.Is(…, aql.ErrEngineCapability)`**; and
  a sentence recording that 400 covers syntax, semantics and path form alike (the ruling's other
  half).
- PROBE-021's wire assertion gains the 501 arm (Sandbox — synthesised 501 response, envelope
  and bare); catalogue entry updated.
- Tests: 501 with envelope, 501 bare, 501 not double-classified as path resolution; existing
  400/408 mapping byte-stable.

**Definition of done:** `errors.Is(err, aql.ErrEngineCapability)` distinguishes a capability gap
from bad AQL in the executor's public contract; `make ci` green.

## Phase 2 — Stored-query prose and the reserved name (AQL-FIT-07)

**Today:** the **code is right** ([`execute.go`](../../../openehr/client/query/execute.go) posts
ad-hoc to `/query/aql`, builds `"/query/" + name [+ "/" + version]` for stored, routes
`/query/{qualified_query_name}[/{version}]`), and the vendored OAS
([`resources/its-rest/query-validation.openapi.yaml`](../../../resources/its-rest/query-validation.openapi.yaml))
declares exactly those three paths. The **normative prose is wrong twice** in
[wire.md § REQ-055 § AQL executor](../../specifications/wire.md#req-055--wire-boundary):

- line 296: *"(or `GET /query/aql/{queryId}` for stored queries)"* — there is no
  `/query/aql/{…}` route; the `aql` segment belongs to the ad-hoc route only. REQ-057, ten
  lines later in the same file, has it right.
- line 302 (EHR scoping): illustrated with the same non-existent path
  (`GET /query/aql/{qualified_query_name}`), and stated narrower than what the code implements.

Separately, `RunStored(ctx, c, "aql", …)` builds `/query/aql` — the ad-hoc route — and the
caller learns about the collision via the server's "missing q" 400, a confusing diagnostic for a
routing rule the SDK itself creates by concatenation.

**Tasks:**

- Correct line 296 to name both stored routes (`GET|POST /query/{qualified_query_name}[/{version}]`)
  and drop the phantom path.
- Restate the EHR-scoping rule as the verb rule it actually is (and the code already
  implements): **every GET** query operation — ad-hoc, stored, stored-versioned — carries
  `ehr_id` as a query parameter; **no POST** operation declares it and neither body schema
  carries it, so the `openehr-ehr-id` header is the only channel for both POST forms.
- Reserved-name guard in `runStoredAtVersion`: a trimmed qualified name equal to `aql` (exact,
  byte-level — the collision is byte-level path routing) fails before any request with
  `ErrInvalidConfig` and a message naming the collision (*"aql is the ad-hoc route
  `/query/aql`, not a stored-query name"*). REQ-057 gains one sentence scoping its
  no-pre-validation stance: name *syntax* stays the backend's; the `aql` routing collision the
  SDK creates itself is refused client-side.
- Tests: guard fires for `RunStored` and `RunStoredVersion`, GET and POST variants; `"aql "`
  (trimmed) covered; any other name still passes through verbatim.

**Definition of done:** the normative document agrees with its own vendored contract and its own
code; the guard is pinned; `make spec-check` and `make ci` green.

## Phase 3 — `TOP` versus the envelope `fetch`, on the read side (AQL-FIT-08)

**Today:** `Builder.Build()` refuses `Top(n)` with the in-text `LIMIT` **and** with the envelope
row limit, with distinct errors — exactly right. The linter flags only the in-text pairing
(`aql_top_with_limit`); given a `TOP` query and an `Options.Query` whose `Fetch` is set,
`lint.Lint` emits only `aql_deprecated_top`. The linter is the surface for AQL the SDK did *not*
author — imported queries, stored definitions, CI gates — precisely where an inherited `TOP`
meets a modern caller's `fetch`.

**Tasks:**

- New sibling code `aql_top_with_fetch` (Error — the OAS's common-parameters prose states
  `fetch` "cannot be combined with AQL-top", same force as the §4.4.3 pairing). A sibling
  rather than a widened `aql_top_with_limit`: the landed code's firing rule and Detail text
  stay byte-stable (the additivity discipline of REQ-161 § Additivity, applied to a REQ-118
  code).
- Fires from the TOP check group when `Options.Query != nil && Options.Query.Fetch > 0 &&
  Document.HasTop` — plumb `Options` to `topIssues`
  ([`lint.go`](../../../openehr/aql/lint/lint.go)); `Offset` alone does not fire (the OAS
  exclusion names the row *limit*).
- Spec amendment: the REQ-118 lint-codes prose gains the code; REQ-109's catalogue listing
  likewise.
- Tests: positive (TOP + Fetch), negatives (TOP + nil Query; TOP + zero Fetch; Fetch + no TOP);
  `aql_top_with_limit` behaviour byte-stable.

**Definition of done:** Build and Lint agree on the `TOP`+`LIMIT` and `TOP`+`fetch` pairings; the envelope-`Offset` arm `Build()` refuses stays undiagnosed by design — the OAS states no exclusion for `offset`; `make ci` green.

## Phase 4 — The `PredicateComparison` caveat moves to where callers read (AQL-FIT-10)

**Today:** `parse.ClassExpr.PredicateComparison` is populated on the Tier-2 structured AST and
left nil on the flat lint view (`parse.Parse` → `Document.Classes`) for the same source — a
deliberate, reasonable performance choice. But the field's own godoc lists four nil cases and
"read from `Document.Classes`" is not one of them; the caveat lives on the **unexported**
`EnterClassExpression` method ([`ast.go`](../../../openehr/aql/parse/ast.go)), where `go doc` will
not surface it. REQ-113 legislates against exactly this one level up: *"the classification MUST
be stated on the type a consumer holds rather than only in this spec."* This phase is
**conformance to landed REQ-113**, so no spec prose changes.

**Tasks:** add the fifth nil case to the exported `PredicateComparison` field godoc ("always nil
when read from the flat lint view, `Document.Classes` — populated only by the Tier-2 structured
extraction") and a matching sentence on `Document.Classes`; keep the lockstep note on
`EnterClassExpression` pointing at the field godoc as the canonical statement. Doc-only; no
behaviour change; verified by review and `make ci` (vet/lint clean).

**Definition of done:** `go doc parse.ClassExpr.PredicateComparison` states the flat-view nil;
`make ci` green.

## Definition of Ready

- Each phase is separately committable and none blocks another; sequencing above is by audit
  finding order, not dependency.
- The Phase-1 ruling and the Phase-3 sibling-code decision are recorded here and land in the
  spec prose with their phases (implementation-aligned amendments in the same PR as the code —
  the SDD exception for landed behaviour, applied as designed).

## Definition of Done

- Code and tests land with the owning `// REQ-` citations (055 / 057 / 118 / 113) and
  `// PROBE-021` where the probe grows.
- [`traceability.yaml`](../../specifications/traceability.yaml): five REQ entries — 055, 057, 109,
  113 and 118 — each gained this plan in their `plans:` list; no test list grew (the phases added
  no test file those entries did not already carry). No REQ.md registry changes (no new ids; Impl.
  columns already `landed`).
- **The index `make spec-check` cannot see:** no roadmap row needed (nothing new lands as a
  capability; these are corrections to landed rows) — recorded here so the omission is a
  decision, not a miss.
- `make spec-check` and `make ci` pass after each phase.
- Plan archived under [`docs/plans/archive/`](./).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 — `aql.ErrEngineCapability` + wire.md taxonomy + PROBE-021 arm | ✅ landed 2026-08-26 |
| Phase 2 — wire.md sentence fixes + reserved-name guard + REQ-057 sentence | ✅ landed 2026-08-26 |
| Phase 3 — `aql_top_with_fetch` + REQ-118/109 catalogue rows | ✅ landed 2026-08-26 |
| Phase 4 — godoc relocation (REQ-113 conformance) | ✅ landed 2026-08-26 |
| `make spec-check` / `make ci` per phase | ✅ `make spec-check` OK at every phase; the `make ci` constituents ran green on the host (`fmt-check`, `vet`, `go test ./...`, `golangci-lint run ./...`, `build`) |

[CHANGELOG.md](../../../CHANGELOG.md) is deliberately untouched: [AGENTS.md](../../../AGENTS.md) updates
it only on request or when cutting a release, and these four corrections carry no release of their
own.

## Mapping to specs

- [wire.md § REQ-055](../../specifications/wire.md#req-055--wire-boundary) — executor error taxonomy + the two corrected sentences
- [wire.md § REQ-057](../../specifications/wire.md#req-057) — reserved-name scope sentence
- [clinical-modeling.md § REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text) — the TOP lint family gaining the envelope arm
- [clinical-modeling.md § REQ-113](../../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) — the "stated on the type a consumer holds" clause Phase 4 conforms to
- [conformance.md § PROBE-021](../../specifications/conformance.md#probe-021--aql-parse-error-mapping) — error-mapping probe gaining the 501 arm
