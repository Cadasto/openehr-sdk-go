# Transport layer

**Status:** Draft

Normative contract for `transport/` — HTTP wrapping around an injected `*http.Client`, cross-cutting wire hygiene, and REST binding helpers shared by all `openehr/client/*` leaf packages. Covers REQ-090 through REQ-094 plus the REQ-096..098 extension range.

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
- **Bounded response reads.** Every `Client.Do` **MUST** cap how many bytes are read from the response body. The default limit is `transport.DefaultMaxResponseBody` (64 MiB). `transport.WithMaxResponseBody(n)` overrides the cap: `0` means default, a positive value sets an explicit limit, a negative value disables the cap (documented escape hatch for trusted backends). Exceeding the limit **MUST** fail the request with an error mentioning the limit — not an unbounded `io.ReadAll`.

`WireError.Error()` values flow into REQ-098 observers and REQ-090 OTel span status; the PHI-safe default applies there too unless the consumer opts into raw bodies.

- **Lives in:** [`transport/`](../../transport)
- **Tests:** `transport/client_test.go` (`TestWireError*`, `TestMaxResponseBody*`)

---

## REQ-094 — `Prefer` response-shape negotiation

openEHR REST 1.1.0-development uses `Prefer: return=<mode>` on write paths. The SDK **MUST** support:

| Mode | Server response | SDK return |
|---|---|---|
| `minimal` | empty body; `Location` + `ETag` | metadata only (`*VersionMetadata`) |
| `identifier` | identifier body only | identifier slot populated |
| `representation` | full new resource | full typed return value |

Rules:

- Per-call option: `transport.WithPrefer(Prefer)` — typed enum, not a raw string.
- **Defaults:** writes default to `minimal`; reads default to `representation` (no Prefer header).
- The SDK **MUST NOT** silently downgrade `representation` when the server omits the body.

All three write-path modes are landed across `composition` / `directory` / `ehr_status`: `representation` decodes the bare resource (REQ-094) and returns [`transport.ErrInvalidShape`](../../transport/errors.go) on an empty body; `identifier` populates the `VersionMetadata` identifier slot from the ITS-REST `Identifier` body (`{"uid": …}`) via [`ehr.ResolveIdentifierBody`](../../openehr/client/ehr/identifier.go), with the `Location` header staying canonical; `minimal` returns metadata only. See the archived [follow-up plan](../plans/archive/2026-05-25-req094-prefer-followups.md). Deferred: the PROBE-065 `minimal`→GET identifier round-trip.

- **Lives in:** [`transport/`](../../transport), [`openehr/client/ehr/`](../../openehr/client/ehr)

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
