---
kind: specification
---

# Transport layer

**Status:** Draft

Normative contract for `transport/` — HTTP wrapping around an injected `*http.Client`, cross-cutting wire hygiene, and REST binding helpers shared by all `openehr/client/*` leaf packages. Covers REQ-090 through REQ-094, REQ-098, REQ-150 (path-parameter segment validation), REQ-151 (typed 2xx failures), and REQ-166 (Bearer challenge on 401 and 403). REQ-096, REQ-097 and REQ-172 are retired.

openEHR resource semantics (Compositions, AQL, canonical codecs) live in [wire.md](wire.md). Service catalog resolution lives in [service-discovery.md](service-discovery.md).

---

## REQ-090 — OpenTelemetry hooks

`transport/` **MUST** expose OpenTelemetry hooks:

- **Spans.** Every outgoing request opens an OTel span named `<METHOD> <route_template>` (e.g. `GET /ehr/{ehr_id}`). The span **SHOULD** carry as attributes the HTTP method, the URL, the status code, the response size and `openehr.spec_version`, so a trace says what the request was without its body. The URL on the span **MUST** be sanitised: it carries no token and none of the request URL's userinfo (a user name or password).
- **Propagation.** Trace context **MUST** be propagated outbound via the standard W3C `traceparent` / `tracestate` headers (using the OTel `propagation` API).
- **No-op safety.** The absence of a `TracerProvider` in the context **MUST** be a silent no-op: the SDK works with no OTel setup.

Metrics and logs are not emitted: they wait for the OTel SDK to stabilise a metrics surface and for a benchmarked basis for which metrics to emit.

- **Probes:** PROBE-050, PROBE-051

---

## REQ-091 — Retry policy

`transport/` **MUST** offer a retry and backoff policy that is off by default for every method and is enabled only through an explicit functional option:

```go
client, err := transport.New(catalog,
    transport.WithRetry(transport.RetryPolicy{
        MaxAttempts:     3,
        InitialBackoff:  100 * time.Millisecond,
        MaxBackoff:      5 * time.Second,
        Multiplier:      2.0,
        RetriableStatus: []int{502, 503, 504},
    }),
)
```

**Disabling retries.** `transport.RetryPolicy` **MUST** tell "no retries" apart from "use the package default", so a consumer can turn retries off at construction without reading the implementation, as a benchmark that measures server-observed latency does:

| `RetryPolicy` value | Behaviour |
|---|---|
| `RetryPolicy{}` (zero value) | Disabled: one attempt, the package default. |
| `RetryPolicy{Disabled: true, ...}` | Disabled, whatever `MaxAttempts` says. |
| `transport.NoRetry` | The canonical form of the row above. |
| `RetryPolicy{MaxAttempts: 0, ...}` | The package default, which is disabled. |
| `RetryPolicy{MaxAttempts: 1, ...}` | Exactly one attempt. |
| `RetryPolicy{MaxAttempts: N, ...}` for N ≥ 2 | Up to N attempts in all. |

Rules:

- Retries **MUST NOT** be applied to a method that is not idempotent (RFC 9110 §9.2.2), such as `POST` or `PATCH`, unless the policy sets `RetryNonIdempotent`. The SDK checks the method before it looks at the error or the status, so a token or network failure on such a request is not retried either.
- Retries **MUST** respect `ctx` cancellation: no attempt starts once `ctx` has ended.
- The retry budget **MUST** be observable via the OTel span (`retry.attempt`, `retry.backoff_ms`).
- Two requests **MUST NOT** be sent again, since every attempt would fail the same way: a request `Client.Do` refuses with `transport.ErrInsecureRedirect` ([§ REQ-092](#req-092--tls-posture) *No downgrade with a credential*), and a request that cannot be built (`http.NewRequestWithContext` refuses it).

**Retrying after a body read failure.** A request whose 2xx body could not be read ([§ REQ-151](#req-151--typed-2xx-decode-failure) *A body that cannot be read*) **MUST NOT** be sent again unless its method is `GET`, `HEAD` or `OPTIONS`, which RFC 9110 §9.2.1 defines as safe, even when `RetryNonIdempotent` is set: the server has acted on it, and a second `PUT` carrying the same `If-Match` would fail as a stale precondition and hide the version the first one made. The non-2xx arm is retried by its status, as any `*WireError` is. A request whose body could not be read, on either arm, **MUST NOT** be sent again once its context has ended, and when the context ends during the wait before a retry, `Client.Do` **MUST** still return that arm's error and `*Response`, with the context's error wrapped beside it.

**Acceptance (body read failures).** With retries enabled, a `GET` whose 2xx body is cut is sent again. A `PUT`, a `DELETE`, and a `POST` under `RetryNonIdempotent`, each with its 2xx body cut, are sent once.

---

## REQ-092 — TLS posture

The SDK does not allocate its own `*http.Client` (REQ-021), so TLS configuration is the consumer's responsibility. However, the SDK **SHOULD**:

- Emit a warning when a `ServiceCatalog` entry's `BaseURL` uses plaintext `http://` and the entry is not explicitly marked insecure.
- Emit a warning when the SMART discovery document is fetched over `http://`.

The resolver's refusal of plaintext discovery URLs, and the `discovery.WithAllowInsecure()` opt-out, are specified in [service-discovery.md § REQ-073](service-discovery.md#req-073--discovery-trust-posture).

The SDK **MUST NOT** silently override or relax the consumer's `*http.Client` TLS config.

**No downgrade with a credential.** A request that `Client.Do` sends and that carries a credential header, `Authorization`, `Proxy-Authorization`, `Cookie` or `Cookie2`, **MUST NOT** follow a redirect from an `https` URL to a URL whose scheme is not `https`: `Client.Do` **MUST** fail with an error that `errors.Is` matches to `transport.ErrInsecureRedirect`, without sending the request to the redirect target, and the retry policy does not send it again ([REQ-091](#req-091--retry-policy)). A request sent without any of these headers, and every redirect other than that downgrade, **MUST** follow the injected client's own redirect policy (its `CheckRedirect`, or net/http's limit of 10 redirects when it has none), and the transport leaves the injected client unmodified ([REQ-021](idiom.md#http-client-injection-req-021)), so a change the caller makes to it later reaches the next request. The transport does this with a shallow copy of the injected client, taken for each request, whose redirect policy refuses that redirect and otherwise defers to the injected client's own. [REQ-021](idiom.md#http-client-injection-req-021) names this copy as an exception to its rule that the SDK does not allocate its own client.

net/http forwards these four headers to a redirect target on the same host, or a subdomain of it, whatever the target's scheme, and drops them only for another domain, so without this rule a 3xx to `http://` on the same host would carry a token or a session cookie unencrypted. A cookie the injected client's `Jar` adds is the jar's to scope, by its `Secure` attribute.

---

## REQ-093 — openEHR error envelope mapping

openEHR REST 1.1.0 returns a structured JSON body on non-2xx responses:

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
- Accept the envelope's `code` as a JSON string or a JSON number (the overview's own example is `"code": 90000`), keeping the text form (`"90000"`) in `OpenEHRErrorDetail.Code`, and decode the `validationErrors` list the ITS-REST `Error` schema (the `400` bodies) defines beside `message`, so neither a numeric `code` nor a body that carries `validationErrors` with no `code` or no `message` leaves the envelope undecoded.
- Attach the parsed envelope to `transport.WireError` as `OpenEHRErrorDetail{Message, Code, CodedText, ValidationErrors}`.
- Map the HTTP status to typed sentinels per [idiom.md § Errors](idiom.md#errors-req-025): `ErrNotFound` (404), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrVersionConflict` (409), `ErrPreconditionFailed` (412), `ErrUnprocessable` (422), `ErrPreconditionRequired` (428). The status set follows the openEHR contract: a **422** signals a well-formed-but-semantically-invalid request (validation / template failure — `resources/its-rest/overview-validation.openapi.yaml` line 400; used on EHR and demographic writes) and **MUST** map to `ErrUnprocessable`. A **409** spans a conflict with the resource's current state (the Composition delete of a version that is not the latest) and a resource-already-exists collision; the openEHR error `code` on `OpenEHRErrorDetail` disambiguates and `ErrVersionConflict` remains the sentinel for both. openEHR signals a stale `If-Match` as **412** and a missing-but-expected `If-Match` as **400** (overview line 370) — **not** 428; `ErrPreconditionRequired` (428) is retained only as a defensive mapping for non-conformant servers and is **not** an openEHR-canonical status. A **400** with no more specific sentinel surfaces as a bare `WireError` whose openEHR `code` remains reachable via `errors.As`.
- **PHI-safe error surfaces by default.** openEHR error bodies routinely carry patient identifiers and clinical detail. `WireError.Error()` **MUST NOT** interpolate `OpenEHR.Message` or the raw response bytes. It **MUST** carry HTTP status, route template, and the openEHR error *code* (codes are not PHI). Callers that need the full server payload for diagnostics **MUST** opt in via `transport.WithRawErrorBodies(true)`; only then are `OpenEHR.Message`, `OpenEHR.ValidationErrors` (free text, like the message) and `WireError.RawBody` populated. `OpenEHR.Code` and `OpenEHR.CodedText` remain available via `errors.As` regardless of the opt-in (structured codes, not free-text clinical narrative).
- **Unrouted requests render a placeholder.** Where a diagnostic string names the request's route, it **MUST** carry the caller's route template, or — when the request carried none — the stable placeholder `(unrouted)`; it **MUST NOT** fall back to the expanded request path or the resolved URL, either of which may hold a caller-supplied identifier. The placeholder's exact text is part of this contract, not an implementation detail: a caller may match on it. The obligation reaches inside a wrapped error: where a network failure arrives as a net/http `*url.Error`, whose own text prints the resolved URL, the SDK **MUST** substitute the same route slot for that URL and leave the error's type, its `Op` and its cause as they were, so `errors.AsType[*url.Error]`, `Timeout()`, `Temporary()` and `errors.Is` against the cause all answer as before. The telemetry surfaces resolve the route their own way (REQ-090 span naming and the `http.route` attribute, REQ-098 `Observation.Route`) and this clause does not bind them.
- **Bounded response reads.** Every `Client.Do` **MUST** cap how many bytes are read from the response body. The default limit is `transport.DefaultMaxResponseBody` (64 MiB). `transport.WithMaxResponseBody(n)` overrides the cap: `0` means default, a positive value sets an explicit limit, a negative value disables the cap (documented escape hatch for trusted backends). Exceeding the limit **MUST** fail the request with an error that `errors.Is` matches to `transport.ErrResponseTooLarge` and that names the limit, never with an unbounded `io.ReadAll`. What that failure returns beside the error is [§ REQ-151](#req-151--typed-2xx-decode-failure)'s.

`WireError.Error()` values flow into REQ-098 observers and REQ-090 OTel span status; the PHI-safe default applies there too unless the consumer opts into raw bodies.

---

## REQ-094 — `Prefer` response-shape negotiation

openEHR REST 1.1.0 uses `Prefer: return=<mode>` on write paths. The SDK **MUST** support:

| Mode | Server response | SDK return |
|---|---|---|
| `minimal` | empty body; `Location` + `ETag` | metadata only (`*VersionMetadata`) |
| `identifier` | identifier body only | identifier slot populated |
| `representation` | full new resource | full typed return value |

Rules:

- Per-call option: each write leaf exposes `WithPrefer(transport.Prefer)` (e.g. `composition.WithPrefer`) — typed enum, not a raw string.
- **Defaults:** writes default to `minimal`; reads default to `representation` (no Prefer header).
- The SDK **MUST NOT** silently downgrade `representation` when the server omits the body.

For the `WriteResult` family (`composition`, `directory`, `ehr_status`, `demographic`), `identifier` fills the identifier slot of `VersionMetadata` from the ITS-REST `Identifier` body (`{"uid": …}`) through `(*ehr.VersionMetadata).ResolveIdentifierBody`, after the version identifier [REQ-054](wire.md#req-054) selects. `contribution.Commit` decodes a `representation` body and never fills the identifier slot from a body.

**Absent resource on a successful write.** `minimal` and `identifier` **MUST** return a nil error and a zero resource. For a pointer resource type the zero value is a typed nil: `== nil` is the right test for a concrete `*T` return, but an interface return (`rm.Party`) can hold a typed-nil pointer, for which `== nil` is the wrong test. The SDK **MUST** expose a `HasResource` helper scoped to registered RM types: it reports false for a typed-nil pointer to a registered RM type, a bare-nil interface, and an interface holding a typed-nil registered RM pointer, and true for any populated resource. A typed nil of a type outside the RM registry is out of scope. The helper uses no reflection, and the write-path documentation names the typed-nil trap and points at it, and at `rm.IsTypedNil` for callers already on the RM type.

**A failed body after a committed write.** A write whose `representation` body is empty or does not decode, and a write in any `Prefer` mode whose body could not be read, has still committed: [§ REQ-151](#req-151--typed-2xx-decode-failure) names the error it returns beside its version metadata.

---

## REQ-096 — Unambiguous "disable retry"

**Retired.** The rule that tells "no retries" apart from "use the package default" moved to [§ REQ-091](#req-091--retry-policy) *Disabling retries*. The identifier is never reused.

---

## REQ-097 — First-class `Idempotency-Key` (deprecated)

**Retired.** Cadasto openEHR services no longer accept the `Idempotency-Key` header, so the SDK does not send it. The identifier is never reused.

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

- The observer **MUST** fire exactly once per logical `Client.Do` call — after retries settle — with retry-aware `Attempts` and total wall-clock `Duration`. A call `Client.Do` refuses before any attempt (a nil `Request`, a context already ended, a service missing from the catalog, a path that fails validation) **MUST** be observed too, with `Attempts` 0 and an empty `URL`.
- `WithObserver(nil)` **MUST** be a safe no-op.
- A panicking observer **MUST NOT** break the request lifecycle: the transport recovers the panic and logs it through the configured `slog.Logger`.
- `Observation.Tags` **MUST** be a copy of the context-attached tags, so an observer cannot change the caller's context.
- The hook is **additive** to REQ-090 OTel — not a substitute. Consumers that want both keep both.

Out of scope:
- Per-observer filtering / sampling (composition concern).
- Body-level observation (PII risk; wrap the injected `*http.Client` if needed).

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

- `ValidatePathSegment(s string) error`: one interpolated identifier (a `/` in `s` is a violation);
- `ValidateRequestPath(path string) error`: the whole decoded `Request.Path`, the same check the transport runs.

(The `Validate*`-returns-`error` spelling follows the repo's own precedent — `template.ValidatePath`, `aql.ValidateIdentifier` — where `Valid*` names return `bool`, as in `rm.TimeDefinitions.ValidDay`.)

`ValidateRequestPath` splits on `/`, so an interpolated parameter that *contains* `/` produces well-formed segments that segment inspection alone cannot catch. When `Request.Route` is set, the transport **MUST** therefore also refuse a decoded `Request.Path` whose segment count differs from the route template's: a smuggled separator changes the count and moves the request onto a different route. The check runs only when `Route` is set, so every path-interpolating leaf request sets it, and the check reads the `Route` field itself, never a helper that falls back to `Path` (a path's arity compared against itself never fires). A tripwire test holds every `transport.Request` literal under `openehr/client/` to setting `Route`, the field that also names the REQ-090 span.

Leaf clients do not re-implement the check. On every `openehr/client/*` operation, a path parameter that contains `/` **MUST** be refused, with an error wrapping `transport.ErrInvalidPathSegment` and `transport.ErrInvalidConfig`, and no request issued: a parameter that is a well-formed openEHR identifier satisfies every rule above and passes, and a `/` is refused as a smuggled separator, never let through on the expectation that openEHR identifiers carry no separator, which is exactly the expectation a hostile identifier exploits.

Out of scope: a breaking `PathSegment` named type on every leaf; validating the service base URL; the `cadasto/admin` health-probe paths (`WithLivePath` / `WithReadyPath`), which bypass `transport.Do` by design and keep their own non-empty / leading-`/` guard.

- **Probes:** PROBE-091 (Implemented — Sandbox)

---

## REQ-151 — Typed 2xx decode failure

A 2xx status says the server did what was asked. On a write, the status and its `ETag` and `Location` are the proof that the server committed and the name of the version it made. A body that then cannot be used is therefore a different failure from a wire failure, and the SDK keeps what the server sent: the status, the headers and, when the body was read, its bytes.

**The taxonomy.** When the body of a 2xx response cannot be used, the SDK **MUST** report it with the error this table names for the case:

| After a 2xx | Error |
|---|---|
| The body could not be read: the connection dropped mid-body, the body did not match its `Content-Encoding`, or it was longer than the [§ REQ-093](#req-093--openehr-error-envelope-mapping) *Bounded response reads* ceiling | `*transport.BodyReadError`, in every `Prefer` mode |
| A write that sent `Prefer: return=representation`, through the `ehr.WriteResult` family or `contribution.Commit`, got an empty body or one that does not decode as the expected resource; or `ehr.Create` got an empty body | `*ehr.NoRepresentationError`, wrapping `transport.ErrInvalidShape` for an empty body and the decoder's error otherwise |
| Any other body that does not decode as the requested representation, `ehr.Create`'s included | `*transport.DecodeError` |
| A read that expected a representation got an empty body: `transport.Decode` and the reads that mirror it | an error wrapping `transport.ErrInvalidShape` |

A body that could not be read is never decoded, so the first row takes precedence over the others. *Empty* means zero bytes, whitespace only, or the JSON `null` literal, decided on the raw bytes before any decode by `transport.IsNoRepresentationBody`: a `null` body decodes into a struct as a no-op with no error, so a decoder that tested only the length would return an all-zero value that looks populated.

These outcomes are their own, and the table **MUST NOT** override them:

- The `Prefer: return=identifier` arm (`(*ehr.VersionMetadata).ResolveIdentifierBody`, [§ REQ-094](#req-094--prefer-response-shape-negotiation)) reports a body it cannot read as an identifier with an error wrapping `transport.ErrInvalidShape`, not a `*transport.DecodeError`.
- On the Definition list operations, and on `GetStoredQuery`, `PutStoredQuery`, `PutStoredQueryVersion` and `UploadTemplate`, an empty body is a success: the list operations return an empty catalog ([wire.md § REQ-144](wire.md#req-144--definition-metadata-decoding) *Empty list bodies*), and the other four return metadata they build themselves with a nil error ([wire.md § REQ-057](wire.md#req-057), [§ REQ-144](wire.md#req-144--definition-metadata-decoding)). A non-empty body there that does not decode is still a `*transport.DecodeError`.
- A read that answers `204 No Content` with an outcome of its own before the empty-body refusal keeps it: `composition.ErrDeletedAtTime`, and the demographic reads' nil result for no matching version.

**What each typed error carries.** Each typed error in the table **MUST** be recoverable from the returned error with `errors.AsType` through any `fmt.Errorf` operation-name wrap a leaf package adds, and **MUST** wrap its cause, exposing it through `Unwrap`, so `errors.Is` and `errors.AsType` still reach the cause's own diagnostics. Beneath a read failure that is `io.ErrUnexpectedEOF` for a connection cut mid-body, the content-coding error (such as `gzip.ErrHeader`) for a body that does not match its encoding, or `transport.ErrResponseTooLarge` for a body over the ceiling; beneath a decode failure, the codec's own typed errors (path, type, offset).

**`Error()` stays value-free.** The `Error()` of each typed error in the table **MUST** follow the [REQ-093](#req-093--openehr-error-envelope-mapping) discipline: it names the classification and the request fields the error holds, the method and route template of a `DecodeError` and the status too for a `BodyReadError`, with the route under REQ-093's *Unrouted requests render a placeholder* rule, and it never names a header value, a body byte or the cause's own text. That text can carry the offending value or bytes the server sent: a `*strconv.NumError` beneath a decode failure always quotes its input, as a `*json.SemanticError` can beneath a `canjson.DecodeError` whose own text carries none ([wire.md § REQ-052](wire.md#req-052)), and net/http quotes a malformed chunked trailer in a read failure. The cause is reachable only by unwrapping. For a body over the ceiling the message names the limit, as REQ-093 requires; the limit is the caller's own setting, not a payload value.

**Metadata still arrives.** The `(*T, *Metadata, error)` triples the leaf packages return **MUST** populate `*Metadata` from the response beside each typed error in the table, and beside the `*transport.WireError` of a non-2xx response whose body could not be read, so the `ETag`, `Location` and the other headers stay available. A `*ehr.NoRepresentationError` **MUST** also carry that version metadata itself, `VersionUID` included when the server supplied it, so a caller needs no correlation against a separately returned value. A leaf whose return has no metadata slot returns the error alone.

**A non-2xx is never this error.** A response whose status is not 2xx **MUST** be reported as a `*transport.WireError`, never as an error in the table, and recovering one of `*transport.WireError`, `*transport.BodyReadError`, `*transport.DecodeError` and `*ehr.NoRepresentationError` **MUST NOT** recover another.

**A body that cannot be read.** On a 2xx, `Client.Do` **MUST** return the `*transport.BodyReadError` beside a non-nil `*transport.Response` that carries `StatusCode`, `Header` and `Metadata` as for any 2xx, and an empty `Body`; the error **MUST** carry the request's method, its route template and the status, and no byte read before the failure, since a partial body is not the representation. On any other status whose body cannot be read, `Client.Do` **MUST** return a non-nil `*Response` whose `Body` is empty, beside an error from which `errors.AsType` recovers the `*transport.WireError` the status and headers classify (its status sentinel, its route and, on a 401 or 403, its Bearer challenge under [§ REQ-166](#req-166--bearer-challenge-on-401-and-403)), with no openEHR envelope and no `RawBody`. That error **MUST** also wrap the read failure, so `errors.Is` reaches the same causes as on a 2xx, and the text it adds to the `WireError`'s own **MUST** stay value-free under the rule above. A body that cannot be read leaves the failure classified by its status, as a body that is not an error envelope already does. The [§ REQ-098](#req-098--request-level-observer-hook) observation of such a call carries the response status beside the error, and [§ REQ-091](#req-091--retry-policy) *Retrying after a body read failure* says when it is sent again.

**A body that does not decode.** A `*transport.DecodeError` **MUST** carry the request's method and route template, and the raw response bytes in `Body` whatever the client's options ([ADR 0018](../adr/0018-raw-bytes-on-decode-error.md)), bounded only by the client's `WithMaxResponseBody` ceiling: the 64 MiB default, an explicit positive limit, or none where the caller disabled the cap with a negative value (*Bounded response reads*, [§ REQ-093](#req-093--openehr-error-envelope-mapping)). `Body` holds patient data for a clinical resource, so reading it is a deliberate act: `Error()` never includes it, and the type's documentation says so, as it does for `ehr.NoRepresentationError.Cause`. ADR 0018 settles why these bytes are attached unconditionally while REQ-093 gates the bodies of non-2xx responses.

**Scope.** The taxonomy binds every 2xx response body read on the `transport.Client` stack, through `transport.Decode` or a decode a leaf writes by hand, so which route a leaf took is invisible to the caller. Scope follows the decode, not the verb: the Definition upload and PUT leaves are in scope, since their 2xx metadata responses are decoded as any read response is.

**Acceptance.**

- A `201` with an `ETag` whose body is cut mid-read returns a `*Response` with status `201`, the headers, the parsed `Metadata` and an empty `Body`. Beside it is an error from which `errors.AsType[*transport.BodyReadError]` recovers status `201`, and `errors.Is(err, io.ErrUnexpectedEOF)` holds.
- A `Content-Encoding: gzip` header over a plain body gives the same shape with `errors.Is(err, gzip.ErrHeader)`. A body over `WithMaxResponseBody` gives it with `errors.Is(err, transport.ErrResponseTooLarge)` and a message that names the limit.
- `composition.Save` against such a `201` returns no composition, the version metadata the `ETag` names, and the `*BodyReadError`.
- A `409` whose body is cut matches `transport.ErrVersionConflict`; `errors.AsType[*transport.WireError]` recovers status `409`, and `errors.Is(err, io.ErrUnexpectedEOF)` holds. `errors.AsType[*transport.BodyReadError]` does not match.
- A value placed in a response header and in the bytes read before the cut appears nowhere in `BodyReadError.Error()`, and a value the server sends as a malformed chunked trailer appears nowhere in the error text on either arm.

- **Probes:** PROBE-101 (Implemented — Sandbox)

---

## REQ-166 — Bearer challenge on 401 and 403

A resource server that refuses a bearer token says why in a `WWW-Authenticate: Bearer` challenge (RFC 6750 §3), and the openEHR REST overview requires services to use that header on 401 and 403. A client needs the reason to tell an expired or revoked token, which a refresh can fix, from a token that lacks a scope, which only a new authorization can fix.

On a 401 or 403 response the transport **MUST** parse the `Bearer` challenge, matching the scheme name case-insensitively, from every `WWW-Authenticate` header line and from a line that lists several challenges (RFC 9110 §11.6.1), and **MUST** attach it to the `*WireError` as `Challenge *BearerChallenge`:

```go
type BearerChallenge struct {
    Realm            string
    Error            string            // "invalid_token", "insufficient_scope", "invalid_request", …
    ErrorDescription string
    ErrorURI         string
    Scope            string            // the scope the resource server says the request needs
    Params           map[string]string // every other auth-param, names in lower case, e.g. resource_metadata (RFC 9728)
}
```

- The parser **MUST** match auth-param names in any letter case (`Error`, `SCOPE` and `Resource_Metadata` are read as `error`, `scope` and `resource_metadata`), keeping every other auth-param in `Params` under its lower-case name, so a caller reads `Params["resource_metadata"]` whatever case the server wrote.
- A response without a `Bearer` challenge **MUST** leave `Challenge` nil; a challenge the transport cannot parse **MUST** leave it nil too, and the response **MUST** still map to its status sentinel. The parser **MUST** ignore empty list elements (RFC 9110 §5.6.1.2).
- The work spent on a challenge **MUST** stay bounded: a `WWW-Authenticate` field line whose value is longer than the parser's limit, 8 KiB, is treated as unparseable without being parsed, while the other lines are still read (RFC 9110 §5.4 lets a recipient ignore a field line it does not wish to process; real Bearer challenges are far shorter).
- `WireError.Error()` **MUST NOT** include challenge values; they stay reachable through `errors.As`, under the same discipline as the openEHR error envelope (REQ-093): `Error`, `Scope`, `Realm`, `ErrorURI` and `Params` are always populated, while `ErrorDescription`, free text from the server, **MUST** be populated only when the client is built with `WithRawErrorBodies(true)`.
- The opt-in 401 safety net (REQ-063) **MUST** call `Reauth` only for a 401 whose challenge is absent, carries no `error`, or carries `error="invalid_token"` (RFC 6750 §3.1: the token is expired, revoked or malformed, and a new one may succeed). A 401 whose challenge names any other error, `insufficient_scope` included, **MUST** be surfaced without calling `Reauth`, since a fresh token with the same grant cannot fix it.

---

## REQ-172 — Typed 2xx body-read failure

**Retired.** Its rules moved to [§ REQ-151](#req-151--typed-2xx-decode-failure), and its retry rule to [§ REQ-091](#req-091--retry-policy). The identifier is never reused.
