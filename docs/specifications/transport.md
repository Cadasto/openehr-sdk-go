# Transport layer

**Status:** Draft

Normative contract for `transport/` — HTTP wrapping around an injected `*http.Client`, cross-cutting wire hygiene, and REST binding helpers shared by all `openehr/client/*` leaf packages. Covers REQ-090 through REQ-094, the REQ-096..098 extension range, REQ-150 (path-parameter segment validation), and REQ-151 (typed 2xx decode failure).

openEHR resource semantics (Compositions, AQL, canonical codecs) live in [wire.md](wire.md). Service catalog resolution lives in [service-discovery.md](service-discovery.md).

---

## REQ-090 — OpenTelemetry hooks

`transport/` **MUST** expose OpenTelemetry hooks:

- **Spans.** Every outgoing request opens an OTel span named `<METHOD> <route_template>` (e.g. `GET /ehr/{ehr_id}`). The span **MUST** carry attributes: HTTP method, URL (sanitised — no tokens), status code, response size, `openehr.spec_version`, `openehr.resource_type` (where applicable).
- **Propagation.** Trace context **MUST** be propagated outbound via the standard W3C `traceparent` / `tracestate` headers (using the OTel `propagation` API).
- **No-op safety.** The absence of a `TracerProvider` in the context **MUST** be a silent no-op — the SDK **MUST NOT** require an OTel setup to function.

Metrics and logs **MAY** be added later once the OTel SDK stabilises a metrics surface and the SDK has a benchmarked basis for which metrics to emit.

- **Lives in:** [`transport/`](../../transport)
- **Probes:** PROBE-050, PROBE-051

---

## REQ-091 — Retry policy

`transport/` **MUST** offer a default retry / backoff policy that is **off by default**. Enabling retries **MUST** be an explicit functional option:

```go
client, err := transport.New(catalog,
    transport.WithRetry(retry.Policy{
        MaxAttempts:     3,
        InitialBackoff:  100 * time.Millisecond,
        MaxBackoff:      5 * time.Second,
        Multiplier:      2.0,
        RetriableStatus: []int{502, 503, 504},
    }),
)
```

Rules:

- Retries **MUST NOT** be enabled by default for **any** method.
- Retries **MUST NOT** be applied to non-idempotent methods (POST, PATCH, DELETE-with-side-effects) unless the consumer explicitly opts in per call.
- Retries **MUST** respect `ctx` cancellation.
- The retry budget **MUST** be observable via the OTel span (`retry.attempt`, `retry.backoff_ms`).

- **Lives in:** [`transport/`](../../transport)

---

## REQ-092 — TLS posture

The SDK does not allocate its own `*http.Client` (REQ-021), so TLS configuration is the consumer's responsibility. However, the SDK **SHOULD**:

- Emit a warning when a `ServiceCatalog` entry's `BaseURL` uses plaintext `http://` and the entry is not explicitly marked insecure.
- Emit a warning when the SMART discovery document is fetched over `http://`.
- Default the opt-in discovery fetcher to refusing plaintext URLs unless `discovery.WithAllowInsecure()` is set.

The SDK **MUST NOT** silently override or relax the consumer's `*http.Client` TLS config.

- **Lives in:** [`transport/`](../../transport), [`smart/discovery/`](../../smart/discovery)

---

## REQ-093 — openEHR error envelope mapping

openEHR REST 1.1.0-development returns a structured JSON body on non-2xx responses:

```json
{
  "message": "Composition violates template constraints at /content[1]/...",
  "code": "VALIDATION_FAILED",
  "coded_text": [
    {"terminology_id": {"value": "openehr"}, "code_string": "..."}
  ]
}
```

The SDK **MUST**:

- Decode this envelope on every non-2xx response (best-effort; missing fields default to zero values).
- Attach the parsed envelope to `transport.WireError` as `OpenEHRErrorDetail{Message, Code, CodedText}`.
- Map the HTTP status to typed sentinels per [idiom.md § Errors](idiom.md#errors-req-025): `ErrNotFound` (404), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrVersionConflict` (409), `ErrPreconditionFailed` (412), `ErrUnprocessable` (422), `ErrPreconditionRequired` (428). The status set follows the openEHR contract: a **422** signals a well-formed-but-semantically-invalid request (validation / template failure — `resources/its-rest/overview-validation.openapi.yaml` line 400; used on EHR and demographic writes) and **MUST** map to `ErrUnprocessable`. A **409** spans both an optimistic-concurrency conflict and a resource-already-exists collision; the openEHR error `code` on `OpenEHRErrorDetail` disambiguates and `ErrVersionConflict` remains the sentinel for both. openEHR signals a stale `If-Match` as **412** and a missing-but-expected `If-Match` as **400** (overview line 370) — **not** 428; `ErrPreconditionRequired` (428) is retained only as a defensive mapping for non-conformant servers and is **not** an openEHR-canonical status. A **400** with no more specific sentinel surfaces as a bare `WireError` whose openEHR `code` remains reachable via `errors.As`.
- **PHI-safe error surfaces by default.** openEHR error bodies routinely carry patient identifiers and clinical detail. `WireError.Error()` **MUST NOT** interpolate `OpenEHR.Message` or the raw response bytes. It **MUST** carry HTTP status, route template, and the openEHR error *code* (codes are not PHI). Callers that need the full server payload for diagnostics **MUST** opt in via `transport.WithRawErrorBodies(true)`; only then are `OpenEHR.Message` and `WireError.RawBody` populated. `OpenEHR.Code` and `OpenEHR.CodedText` remain available via `errors.As` regardless of the opt-in (structured codes, not free-text clinical narrative).
- **Unrouted requests render a placeholder.** Where a diagnostic string names the request's route, it **MUST** carry the caller's route template, or — when the request carried none — the stable placeholder `(unrouted)`; it **MUST NOT** fall back to the expanded request path or the resolved URL, either of which may hold a caller-supplied identifier. The placeholder's exact text is part of this contract, not an implementation detail: a caller may match on it. The obligation reaches inside a wrapped error: where a network failure arrives as a net/http `*url.Error`, whose own text prints the resolved URL, the SDK **MUST** substitute the same route slot for that URL. That substitution **MUST** leave the error's type, its `Op`, and its cause unchanged, so `errors.AsType[*url.Error]`, `Timeout()`, `Temporary()` and `errors.Is` against the cause all answer as before. The telemetry surfaces resolve the route their own way (REQ-090 span naming and the `http.route` attribute, REQ-098 `Observation.Route`) and this clause does not bind them.
- **Bounded response reads.** Every `Client.Do` **MUST** cap how many bytes are read from the response body. The default limit is `transport.DefaultMaxResponseBody` (64 MiB). `transport.WithMaxResponseBody(n)` overrides the cap: `0` means default, a positive value sets an explicit limit, a negative value disables the cap (documented escape hatch for trusted backends). Exceeding the limit **MUST** fail the request with an error mentioning the limit — not an unbounded `io.ReadAll`.

`WireError.Error()` values flow into REQ-098 observers and REQ-090 OTel span status; the PHI-safe default applies there too unless the consumer opts into raw bodies.

- **Lives in:** [`transport/`](../../transport)
- **Tests:** `transport/client_test.go` (`TestWireError*`, `TestMaxResponseBody*`), `transport/errors_test.go` (`TestErrorStringsValueFreeWhenRouteUnset`, `TestNetworkFailureSanitisesWrappedURL`, `TestUnroutedObservationKeepsTheResolvedPath`)

---

## REQ-094 — `Prefer` response-shape negotiation

openEHR REST 1.1.0-development uses `Prefer: return=<mode>` on write paths. The SDK **MUST** support:

| Mode | Server response | SDK return |
|---|---|---|
| `minimal` | empty body; `Location` + `ETag` | metadata only (`*VersionMetadata`) |
| `identifier` | identifier body only | identifier slot populated |
| `representation` | full new resource | full typed return value |

Rules:

- Per-call option: each write leaf exposes `WithPrefer(transport.Prefer)` (e.g. `composition.WithPrefer`) — typed enum, not a raw string.
- **Defaults:** writes default to `minimal`; reads default to `representation` (no Prefer header).
- The SDK **MUST NOT** silently downgrade `representation` when the server omits the body.

All three write-path modes are landed for the shared `WriteResult` family (`composition` / `directory` / `ehr_status` / `demographic`): `representation` decodes the bare resource; `identifier` populates the `VersionMetadata` identifier slot from the ITS-REST `Identifier` body (`{"uid": …}`) via `ehr.ResolveIdentifierBody`, with the `Location` header staying canonical; `minimal` returns metadata only. `contribution.Commit` (its own `WithPrefer`) carries `minimal` and the `representation` decode only: the `identifier` slot is never populated from the body (deferred; recorded in the archived [write-result plan](../plans/archive/2026-08-18-write-result-contract.md) Defers). Identifier-slot population for the WriteResult family landed in the archived [follow-up plan](../plans/archive/2026-05-25-req094-prefer-followups.md). Deferred: the PROBE-065 `minimal`→GET identifier round-trip.

**Absent resource on a successful write.** `minimal` and `identifier` **MUST** return a nil error and a zero resource. For a pointer resource type the zero value is a typed nil — `== nil` is the correct test for a concrete `*T` return, but an interface return (`rm.Party`) can hold a typed-nil pointer, for which `== nil` is the wrong test. The SDK **MUST** expose a reflection-free `HasResource` helper scoped to registered RM types: it reports false for a typed-nil pointer to a registered RM type, a bare-nil interface, and an interface holding a typed-nil registered RM pointer, and true for any populated resource. A typed nil of a type outside the RM registry is out of scope. Write-path documentation **MUST** name the typed-nil trap and point at that helper (and at `rm.IsTypedNil` for callers already on the RM type).

**Committed write, unusable representation.** After a successful HTTP response (2xx), when `Prefer: return=representation` was sent and the body is empty or does not decode as the expected resource, the write **MUST** be reported as a typed `NoRepresentationError`. A whitespace-only body and the JSON `null` literal — which unmarshals into a struct as a nil-error no-op — **MUST** classify as empty; they carry no representation. The error:

- carries the version metadata that proves the commit (including `VersionUID` when the server supplied it);
- wraps the cause (`transport.ErrInvalidShape` for an empty body; the decoder's error otherwise);
- is distinguishable with `errors.As` alone, with no reference to `WireError` and no correlation against a separately-returned metadata value;
- keeps its `Error()` string value-free in the `WireError.Error()` discipline (REQ-093): the classification only, **never** the cause text or any payload-derived value — the cause stays reachable through unwrapping.

A wire failure **MUST** remain a `*transport.WireError`. The SDK **MUST NOT** return `NoRepresentationError` for a non-2xx response. The existing `(resource, metadata, error)` triple **MUST** still populate metadata on this path so current callers do not break.

The same empty-body and decode-failure rules **MUST** apply to `contribution.Commit` when `Prefer: return=representation` was sent. An empty representation body **MUST NOT** be a silent success.

This contract binds the versioned-write `WriteResult` family, `contribution.Commit`, and EHR creation. `ehr.Create` (a non-versioned write) **MUST** return a typed `NoRepresentationError`, carrying the commit metadata as above, when a 2xx body is empty, whitespace-only, or the JSON `null` literal. A 2xx body that is present but does not decode is `ehr.Create`'s *decode-failure* arm, typed by [§ REQ-151](#req-151--typed-2xx-decode-failure) as a `*transport.DecodeError`. Until 2026-09-02 the empty/null-body arm was a keyed exception returning the bare `transport.ErrInvalidShape`; it was closed by the archived [`ehr.Create` empty-2xx typing plan](../plans/archive/2026-09-01-ehr-create-empty-2xx-typing.md).

- **Lives in:** [`transport/`](../../transport), [`openehr/client/ehr/`](../../openehr/client/ehr) (composition / directory / ehrstatus / contribution), [`openehr/client/demographic/`](../../openehr/client/demographic)

---

## REQ-096 — Unambiguous "disable retry"

`transport.RetryPolicy` **MUST** distinguish "no retries" from "use the package default" so consumers can opt out of retry behaviour unambiguously at construction time. The contract:

| `RetryPolicy` value | Behaviour |
|---|---|
| `RetryPolicy{}` (zero value) | Disabled — one attempt. Equivalent to today's default. |
| `RetryPolicy{Disabled: true, ...}` | Disabled regardless of `MaxAttempts`. |
| `transport.NoRetry` | Canonical sentinel for the above. |
| `RetryPolicy{MaxAttempts: 0, ...}` | Disabled (use package default; documented). |
| `RetryPolicy{MaxAttempts: 1, ...}` | Exactly one attempt. |
| `RetryPolicy{MaxAttempts: N, ...}` for N ≥ 2 | Up to N total attempts. |

Rationale: benchmark / load-tool consumers that measure server-observed latency **MUST** be able to express "no retries" at construction without reading the implementation. This clarification is non-breaking — callers that previously passed `MaxAttempts: N` for N ≥ 2 see no behavioural change.

- **Lives in:** [`transport/`](../../transport)
- **Probes:** unit test `TestRetryNoRetrySentinel` in `transport/client_test.go`

---

## REQ-097 — First-class `Idempotency-Key` (deprecated)

**Status: Deprecated (2026-05).** Removal target: **v1.0.0** (first tagged release). Cadasto openEHR services no longer accept the `Idempotency-Key` HTTP header. Until removal, the SDK **MUST NOT** expose first-class `Idempotency-Key` support on `transport.Request` or emit the header on outgoing requests.

The original REQ-097 design (first-class field, verbatim header, OTel attribute) is superseded by this deprecation. The identifier is retained for traceability.

---

## REQ-098 — Request-level observer hook

`transport.Client` **MUST** expose a structured observer hook that fires once per request lifecycle, independent of OTel:

```go
type Observation struct {
    Method     string
    Route      string
    URL        string
    StatusCode int
    Duration   time.Duration
    Attempts   int
    Err        error
    Tags       map[string]any
}

type Observer interface {
    OnRequest(Observation)
}

// Option:
transport.WithObserver(o Observer) Option
// Context tag plumbing:
transport.WithObservationTag(ctx, k, v) context.Context
```

Rules:

- The observer **MUST** fire exactly once per logical `Client.Do` call — after retries settle — with retry-aware `Attempts` and total wall-clock `Duration`.
- `WithObserver(nil)` **MUST** be a safe no-op.
- A panicking observer **MUST NOT** break the request lifecycle. The transport **MUST** recover the panic and log via the configured `slog.Logger`.
- `Observation.Tags` **MUST** be a defensive copy of any context-attached tags — observers **MUST NOT** be able to mutate the caller's context.
- The hook is **additive** to REQ-090 OTel — not a substitute. Consumers that want both keep both.

Out of scope:
- Per-observer filtering / sampling (composition concern).
- Body-level observation (PII risk; wrap the injected `*http.Client` if needed).

- **Lives in:** [`transport/`](../../transport)
- **Probes:** unit tests `TestObserver*` and `TestObservation*` in `transport/observer_test.go`

---

## REQ-150 — Path-parameter segment validation

`REQ-095` makes the transport the single canonical path *encoder*. This requirement makes it the single path-*segment validator* as well. Encoding without validation leaves every caller owning a class of request-URI mutation that the type system cannot see.

Before issuing an HTTP request, the transport **MUST** validate the decoded `Request.Path` (not the service base URL). The path is split on `/`; the leading empty segment of an absolute path is ignored. A `Request.Path` of exactly `"/"` — the service root, the System API's only operation (`OPTIONS /`, sent by the landed `system.Capabilities` / `Version` / `Health`) — carries no segments and **MUST** pass. Every remaining segment of any other path **MUST** be non-empty and **MUST NOT**:

- be `.` or `..`;
- contain `\`;
- contain a control character (bytes `< 0x20` or `DEL` `0x7F`).

A space or any other percent-encodable octet **MUST** be accepted — encoding remains the transport's job (REQ-095). Validation **MUST** run on the decoded path the transport already requires. A literal `%2e%2e` that was not decoded is an ordinary segment, not a separator.

The transport **MUST NOT** honour a caller-populated `url.URL.RawPath` when building the request URI: the encoded form is derived from the validated decoded path alone (REQ-095), so a pre-encoded spelling cannot bypass validation.

A violation **MUST**:

- return `ErrInvalidPathSegment` wrapped with `ErrInvalidConfig` (so `errors.Is(err, ErrInvalidConfig)` still means the request never left the process);
- issue **no** HTTP request.

The SDK **MUST** export the same checks as package functions so a caller constructing a raw `Request` can preflight:

- `ValidatePathSegment(s string) error` — one interpolated identifier (a `/` in `s` is a violation);
- `ValidateRequestPath(path string) error` — the whole decoded `Request.Path`; the transport's join step **MUST** use this.

(The `Validate*`-returns-`error` spelling follows the repo's own precedent — `template.ValidatePath`, `aql.ValidateIdentifier` — where `Valid*` names return `bool`, as in `rm.TimeDefinitions.ValidDay`.)

`ValidateRequestPath` splits on `/`, so an interpolated parameter that *contains* `/` produces well-formed segments that segment inspection alone cannot catch. When `Request.Route` is set, the transport **MUST** therefore also refuse a decoded `Request.Path` whose segment count differs from the route template's — a smuggled separator changes the count and mutates the request URI onto a different route. Path-interpolating leaf requests **MUST** set `Route`, and that sentence is load-bearing rather than observability: the arity rule runs only when `Route` is set, so an unset `Route` silently disables the smuggling defence. The check **MUST** key on the `Route` field itself being non-empty — never on a helper that falls back to `Path` (comparing a path's arity against itself never fires) — and the SDK **MUST** carry a tripwire test asserting that every `transport.Request` literal under `openehr/client/` sets `Route` (every current leaf does; the same field feeds REQ-090 span naming).

Leaf clients **MUST NOT** re-implement the check. Stated as behaviour: a parameter that is a well-formed openEHR identifier satisfies every rule above and passes; a parameter carrying a `/` is refused as a smuggled separator; and validation **MUST** run unconditionally — never skipped on the expectation that openEHR identifiers carry no separator, which is exactly the expectation a hostile identifier exploits.

Out of scope: a breaking `PathSegment` named type on every leaf; validating the service base URL; the `cadasto/admin` health-probe paths (`WithLivePath` / `WithReadyPath`), which bypass `transport.Do` by design and keep their own non-empty / leading-`/` guard.

- **Lives in:** [`transport/`](../../transport)
- **Probes:** PROBE-091 (Implemented — Sandbox)

---

## REQ-151 — Typed 2xx decode failure

A 2xx response whose body cannot be decoded as the requested representation is a distinct failure
from a wire failure and from an absent body. The SDK **MUST** surface it as such, and **MUST NOT**
discard the bytes the server delivered.

**The typed error.** After a 2xx, when the response body cannot be decoded as the requested
representation, the SDK **MUST** return a `*transport.DecodeError`. That error **MUST**:

- be recoverable from the returned error with `errors.AsType[*transport.DecodeError]` — the
  REQ-025 preferred matcher; `errors.As` reaches it identically — through any `fmt.Errorf`
  operation-name wrapping a leaf package adds, so a leaf's wrap is presentation, never a barrier;
- carry the raw response bytes in a `Body` field, populated unconditionally (no opt-in gates it;
  [ADR 0018](../adr/0018-raw-bytes-on-decode-error.md) is the decision of record), bounded by
  whatever ceiling `WithMaxResponseBody` imposes on that client — the 64 MiB default, an explicit
  positive limit, or **no ceiling at all** where the caller has taken REQ-093's documented escape
  hatch and disabled the cap with a negative value. `Body` inherits the caller's configured
  ceiling; it does not add one of its own;
- wrap the decoder's error and expose it through `Unwrap`, so `errors.Is` and `errors.AsType`
  still reach the codec's own typed diagnostics (path, type, offset) unchanged;
- carry the request's HTTP method and route template, so the failure is attributable without
  re-deriving it from the call site.

**`Error()` stays value-free.** `DecodeError.Error()` **MUST** carry the HTTP method, the route
template and the classification only, in the REQ-093 discipline. It **MUST NOT** interpolate the
body, and **MUST NOT** interpolate the wrapped decoder's text — codec errors embed offending
values in `parse %q`-style messages, so echoing the cause would leak through the string surface
what the field deliberately gates. Callers that need the diagnostics unwrap or read `Body`.

**Metadata still arrives.** The `(*T, *Metadata, error)` triple the leaf packages return **MUST**
still populate `*Metadata` on this path. A decode failure does not cost the caller the response
headers — `ETag`, `Location` and the rest remain available beside the error.

**A non-2xx is never this error.** A wire failure **MUST** remain a `*transport.WireError`, and
the SDK **MUST NOT** return a `*transport.DecodeError` for a non-2xx response. The two
classifications are disjoint: recovering one **MUST NOT** recover the other.

**An empty 2xx body keeps its existing per-surface contract.** This requirement re-types no
empty-body arm. Each arm **MUST** keep the contract its own surface already had, and no arm is
unified under `*transport.DecodeError`: an empty body has no representation to decode and no
bytes to hand back, so where it is a failure at all it is an *absent* body rather than an unusable
one. The arms take three shapes, named below.

**The refusal arms.** A read that expected a representation and received an empty 2xx body
**MUST** keep today's `transport.ErrInvalidShape` behaviour, so callers already keying on
`errors.Is(err, transport.ErrInvalidShape)` keep working unchanged. This is the shared
`transport.Decode` arm and the hand-rolled reads that mirror it — `system.Capabilities`,
`composition.Get`, and the Demographic party and version reads. Where such a leaf carves out
`204 No Content` as a typed success ahead of the refusal — `composition.ErrDeletedAtTime`, the
Demographic reads' nil-for-no-matching-version — that carve-out is the leaf's own contract and
stands unchanged; the refusal covers every remaining empty-body 2xx.

**Keyed exclusion — the Definition list leaves.** For the Definition **list** operations, an empty
2xx body is a *successful empty catalog*, not a failed representation decode. It **MUST NOT**
produce `transport.ErrInvalidShape` and **MUST NOT** produce a `*transport.DecodeError`. What
slice value that success returns is the Definition list contract's business, not this
requirement's; this § only points there and holds either way. Only a **non-empty** list body that
fails to decode — for example a JSON object where an array is expected — is REQ-151's.

**Keyed exclusion — the synthesized-metadata arms.** Four Definition leaves answer an empty 2xx
body with a metadata value they synthesize themselves and a nil error: `GetStoredQuery`,
`PutStoredQuery` and `PutStoredQueryVersion` — three stored-query surfaces owned by
[wire.md § REQ-057](wire.md#req-057) — and `UploadTemplate`, owned by its template leaf contract.
What each synthesized value carries is the owning contract's business, not this requirement's; as
with the list leaves, this § only points there. Each is deliberate deployment tolerance that
predates this requirement — a deployment may legally answer these calls `200` or `204` with no
body — and each **MUST NOT** be re-typed by it: no `transport.ErrInvalidShape`, and no
`*transport.DecodeError`. As with the list leaves, only a **non-empty** body that fails to decode
is REQ-151's on those routes.

**Scope.** This requirement binds every 2xx **response-body decode** performed on the
`transport.Client` stack that is not owned by the REQ-094 write-result contract. Scope follows
the *decode*, not the request's verb: the Definition upload and PUT leaves are in scope, because
their requests are writes but their 2xx metadata responses are decoded exactly as any read
response is. Every call site reaching `transport.Decode` inherits the contract by construction;
hand-rolled response decodes on the same stack **MUST** satisfy it identically, so which
implementation route a leaf took stays invisible to the caller.

**Keyed exclusions, by owning contract.** Three surfaces keep their own error taxonomy and
**MUST NOT** be re-typed by this requirement:

- the write-result funnel — the `ehr.WriteResult` callbacks, `contribution.Commit`, and
  `ehr.Create`'s empty/null-body arm — whose committed-but-unusable classification is
  `*ehr.NoRepresentationError` ([§ REQ-094](#req-094--prefer-response-shape-negotiation));
- the `Prefer: return=identifier` arm (`ehr.ResolveIdentifierBody`), likewise REQ-094's
  negotiation surface, which keeps its `transport.ErrInvalidShape` wrap;
- the empty-body arms named above (the refusal arms, the Definition list leaves, and the
  synthesized-metadata arms).

Everything a keyed exclusion covers stays covered by the requirement that owns it; nothing here
loosens those contracts.

**`Body` is PHI-bearing by design.** The field carries the response as the injected
`http.Client` delivered it, which for a
clinical resource means patient data. It **MUST** be documented as such on the exported type, in
the treatment `ehr.NoRepresentationError.Cause` already receives: the value-free `Error()` string
is the boundary-safe surface, and reading `Body` is a deliberate act by a caller who has decided
the diagnostics are worth the exposure. Why those bytes are attached unconditionally, rather than
gated the way REQ-093 gates non-2xx error bodies, is settled in
[ADR 0018](../adr/0018-raw-bytes-on-decode-error.md), which is the authority for that asymmetry.

- **Lives in:** [`transport/`](../../transport)
- **Probes:** PROBE-101 (Implemented — Sandbox)

---

## Coverage

| REQ | Package |
|---|---|
| REQ-090 | `transport/` |
| REQ-091 | `transport/` |
| REQ-092 | `transport/`, `smart/discovery/` |
| REQ-093 | `transport/` |
| REQ-094 | `transport/`, `openehr/client/*` |
| REQ-096 | `transport/` |
| REQ-098 | `transport/` |
| REQ-150 | `transport/`, `openehr/client/*` |
| REQ-151 | `transport/`, `openehr/client/*` |
