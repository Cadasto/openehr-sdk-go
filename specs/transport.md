# Transport layer

**Status:** Draft

Normative contract for `transport/` — HTTP wrapping around an injected `*http.Client`, cross-cutting wire hygiene, and REST binding helpers shared by all `openehr/client/*` leaf packages. Covers REQ-090 through REQ-094.

openEHR resource semantics (Compositions, AQL, canonical codecs) live in [wire.md](wire.md). Service catalog resolution lives in [service-discovery.md](service-discovery.md).

---

## REQ-090 — OpenTelemetry hooks

`transport/` **MUST** expose OpenTelemetry hooks:

- **Spans.** Every outgoing request opens an OTel span named `<METHOD> <route_template>` (e.g. `GET /ehr/{ehr_id}`). The span **MUST** carry attributes: HTTP method, URL (sanitised — no tokens), status code, response size, `openehr.spec_version`, `openehr.resource_type` (where applicable).
- **Propagation.** Trace context **MUST** be propagated outbound via the standard W3C `traceparent` / `tracestate` headers (using the OTel `propagation` API).
- **No-op safety.** The absence of a `TracerProvider` in the context **MUST** be a silent no-op — the SDK **MUST NOT** require an OTel setup to function.

Metrics and logs **MAY** be added later once the OTel SDK stabilises a metrics surface and the SDK has a benchmarked basis for which metrics to emit.

- **Lives in:** [`transport/`](../transport/)
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

- **Lives in:** [`transport/`](../transport/)

---

## REQ-092 — TLS posture

The SDK does not allocate its own `*http.Client` (REQ-021), so TLS configuration is the consumer's responsibility. However, the SDK **SHOULD**:

- Emit a warning when a `ServiceCatalog` entry's `BaseURL` uses plaintext `http://` and the entry is not explicitly marked insecure.
- Emit a warning when the SMART discovery document is fetched over `http://`.
- Default the opt-in discovery fetcher to refusing plaintext URLs unless `discovery.WithAllowInsecure()` is set.

The SDK **MUST NOT** silently override or relax the consumer's `*http.Client` TLS config.

- **Lives in:** [`transport/`](../transport/), [`smart/discovery/`](../smart/discovery/)

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
- Map the HTTP status to typed sentinels per [idiom.md § Errors](idiom.md#errors-req-025): `ErrNotFound` (404), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrVersionConflict` (409), `ErrPreconditionFailed` (412), `ErrPreconditionRequired` (428).
- Preserve the raw response body on `WireError.RawBody`.

- **Lives in:** [`transport/`](../transport/)

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

- **Lives in:** [`transport/`](../transport/), [`openehr/client/ehr/`](../openehr/client/ehr/)

---

## Coverage

| REQ | Package |
|---|---|
| REQ-090 | `transport/` |
| REQ-091 | `transport/` |
| REQ-092 | `transport/`, `smart/discovery/` |
| REQ-093 | `transport/` |
| REQ-094 | `transport/`, `openehr/client/*` |
