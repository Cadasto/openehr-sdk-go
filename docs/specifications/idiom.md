---
kind: specification
---

# Go idiom

**Status:** Draft

The idiomatic Go surface rules every public package in this SDK adheres to. Covers REQ-020, REQ-021, REQ-025 and REQ-026; the conventions of the retired REQ-022, REQ-023 and REQ-024 are [§ API conventions](#api-conventions).

The premise: **the SDK is idiomatic Go.** Wire-level correctness is defined against the openEHR specifications and the conformance probes in [conformance.md](conformance.md), not by a particular source shape. Java-style "every type is an object", Python-style keyword arguments, repository-graph ports from other languages — none of these constrain the Go API.

## Context propagation (REQ-020)

Every exported function or method that performs network I/O (HTTP, DNS, a discovery lookup) or waits on a deadline **MUST** take `context.Context` as its first parameter, and **MUST** honour that context's cancellation and deadline. The local readers are the exception: `template.ParseFile`, `template.ParseFileStrict` and `probe.ValidateHAR` read a local file the way `os.ReadFile` does, with no context, and `bmm.LoadAll` calls its `Resolver` without one.

```go
// Correct
func (c *Client) GetComposition(ctx context.Context, id ObjectVersionID) (*rm.Composition, error)

// Wrong: no context, even with a sane default
func (c *Client) GetComposition(id ObjectVersionID) (*rm.Composition, error)

// Wrong: context not first
func (c *Client) GetComposition(id ObjectVersionID, ctx context.Context) (*rm.Composition, error)
```

`context.Context` is also the carrier for:

- **Request-scoped values** — primarily for OTel propagation. The SDK defines no context-value keys of its own for control flow, which would be an anti-pattern.
- **Per-request auth** — the MCP server use case forwards an incoming token via `auth.WithTokenSource(ctx, ts)` (see [auth.md § Per-request TokenSource](auth.md#per-request-tokensource)).

Pure functions (codecs, validators with no I/O, AQL string builders) **SHOULD NOT** take `context.Context`, because a context they never consult is noise at every call site.

**In tests**, the same pure-vs-I/O split applies. Test code that drives an I/O-bearing call (the `transport`, `auth`, `smart`, and `openehr/client` packages, all `httptest`-backed) uses `t.Context()` rather than `context.Background()` or `context.TODO()`, so cancellation and deadlines reach the call under test and are released at test cleanup. Pure-compute test packages (codec, validator and builder tests, such as `internal/bmmgen` and `openehr/instance`) can keep `context.Background()`, where `t.Context()` is cosmetic, and the conformance probes under `testkit/probes/` keep it too. The rule of thumb: if the production method takes `ctx` to do real I/O, its test passes `t.Context()`.

## HTTP client injection (REQ-021)

All outgoing network I/O **MUST** go through the `*http.Client` the caller injects, its `Transport`, `Jar` and `Timeout` included, and a constructor given no client refuses with an error rather than fall back to a client of its own. Constructors accept the client via functional option or wrapper:

```go
// Explicit injection
client, err := transport.New(catalog,
    transport.WithHTTPClient(httpClient),
    transport.WithTokenSource(ts),
)
```

The SDK allocates no `*http.Client` of its own. Three shallow copies of the injected client are the exceptions, each changing only its redirect policy, and each **MUST** leave the injected client unmodified: the copy that [auth.md § Credential-bearing requests](auth.md#credential-bearing-requests) prescribes for token, refresh and revocation requests; the copy the discovery resolver fetches with, to refuse a redirect to a URL that is not https ([service-discovery.md § REQ-073](service-discovery.md#req-073--discovery-trust-posture)); and the copy the transport sends a request carrying a credential header (`Authorization`, `Proxy-Authorization`, `Cookie` or `Cookie2`) with, to refuse a redirect from https to a URL that is not https ([transport.md § REQ-092](transport.md#req-092--tls-posture)). Each copy keeps the injected client's `Transport`, `Jar` and `Timeout`.

Rationale:

- Connection-pool sizing, timeouts, TLS roots, proxy config belong to the consumer's runtime.
- The federator use case constructs many clients with independent transport configs.
- Benchmarks need precise control over transport tuning (e.g. `MaxIdleConnsPerHost`).

The package documentation of `transport/` says that the injected client governs all outgoing network I/O.

## Functional options (REQ-022)

**Retired.** Its conventions are now prose in [§ API conventions](#api-conventions). The identifier is never reused.

## Surface shape (REQ-023)

**Retired.** Its conventions are now prose in [§ API conventions](#api-conventions). The identifier is never reused.

## Generics policy (REQ-024)

**Retired.** Its conventions, the no-reflection rule included, are now prose in [§ API conventions](#api-conventions). The identifier is never reused.

## API conventions

These conventions shape the public surface and the library code behind it. They are informative: the code follows them, and the code can change them without an amendment here.

### Functional options

SDK constructors take their configuration as functional options:

```go
client, err := transport.New(catalog,
    transport.WithHTTPClient(httpClient),
    transport.WithTokenSource(ts),
    transport.WithSpecVersion("1.1.0"),
    transport.WithUserAgent("my-app/1.0"),
    transport.WithRetry(transport.RetryPolicy{...}),
)
```

- An option is a value of one small interface type, typically `func(*config) error` or `func(*config)`.
- Options are additive: applying two options of the same family has a well-defined result, typically "last write wins", with any exception for a slice-accumulator option documented on it.
- The `config` struct is **unexported**; consumers configure it only through `With*` options.
- A public `Config` struct for serialised configuration (YAML, env) can exist, and the constructor routes it through the same option chain.

The SDK does not use:

- Setter methods after construction (`client.SetHTTPClient(...)`) — mutable after construction implies a hidden lock or a data race.
- Builder-pattern intermediate types (`ehr.NewBuilder().WithX(...).Build()`) — verbose and harder to compose than functional options.
- The "options bag" pattern (`transport.New(catalog, transport.Options{HTTPClient: ..., TokenSource: ...})`) — a second configuration style beside functional options. A keyed literal keeps compiling when a field is added, and [module-layout.md § Versioning](module-layout.md#versioning) does not class that as a breaking change.

### Surface shape

The primary call-site surface is **package-level functions**:

```go
// Idiomatic
import "github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"

err := composition.Save(ctx, ehrID, comp, write.Options{...})
```

A repository-style struct surface can be offered alongside as an injection-seam convenience:

```go
// Allowed seam for tests / DI
repo := composition.NewRepository(httpClient, ts, catalog)
err := repo.Save(ctx, ehrID, comp, write.Options{...})
```

Such a repository is an interface (e.g. `composition.Repository`), so consumers can mock it directly rather than wrap the package functions, and the package-level functions stay present beside it.

### Generics and reflection

Go generics are used where they remove a reflection hop, an `interface{}` cast, or a type assertion, never decoratively.

```go
// Idiomatic — typed response, no reflection
func Get[T rm.Resource](ctx context.Context, c *Client, id string) (T, error)

// Not used — reflection-based decoder for polymorphic _type
func Get(ctx context.Context, c *Client, id string) (any, error) // forces type assertions everywhere
```

Library code (non-test code outside `cmd/`) uses no reflection, with two exceptions: ordinary field mapping over struct tags (it is what `encoding/json` does), and uses that never choose a Go type from `_type`. The second exception has four classes: a typed-nil or zero-value check, a value comparison (`reflect.DeepEqual`), a type name in an error message, and an addressable copy that reaches pointer-receiver methods. The type registry ([rm-modeling.md § Type registry (REQ-040)](rm-modeling.md#type-registry-req-040)) is the only mechanism that projects the `_type` discriminator onto a concrete Go type. A guard test still checks this convention: it holds every library file that imports `reflect` to a reviewed list naming the class of each use.

- A constraint interface is a marker (`isComposition()`) plus the necessary method set — the same rule as for abstract categories.
- A constraint interface appears only in the type parameter list of a function signature: `Get[T rm.CompositionLike](...) (T, error)` keeps the runtime type, while `func Get(...) (rm.CompositionLike, error)` erases it.

If a generic API is harder to read than a `T`-specific one for the most common call site, the generic is wrong — drop it.

### String iteration

A check defined over **bytes** — a wire-format guard, a byte-exact scanner — iterates `for i := range len(s)`, never `for i := range s`. Ranging a string yields rune-start indices and skips UTF-8 continuation bytes, so a byte loop rewritten that way stops inspecting every byte. `for i := range s` is correct only where runes are genuinely the unit.

`go fix` deliberately declines to rewrite `for i := 0; i < len(s); i++` on a string for exactly this reason — it applies the `rangeint` fixer to slices only. A `range s` over a byte-oriented check is therefore always hand-written, and always worth a second look in review.

### Imports and naming

- **Import groups:** standard library; third-party; module-internal — separated by blank lines. `gofumpt` + `goimports` (run via `golangci-lint fmt` / `make fmt`) handle ordering and enforce the group separation.
- **Package names:** short, lowercase, no underscores, no plurals (`composition`, not `compositions`). Avoid stuttering — `composition.Composition` is allowed if the type is the package's primary export; `composition.Service` and `composition.Repository` are preferred for non-primary types.
- **Exported names:** `CamelCase`. Acronyms preserve case (`HTTPClient`, `JWKS`, `EHRID`).
- **Unexported names:** `camelCase` with leading lowercase.
- **Constructor names:** `New<Thing>` returns `*Thing` or `Thing`; `New<Thing>(opts ...Option)` returns `(*Thing, error)` when configuration validation can fail.

## Substitution slots and the `*Like` interfaces

The openEHR RM permits Liskov substitution at every property slot (per AOM `valid_value` semantics). The Go SDK surfaces this on two distinct surfaces, and the call pattern differs between them, so it helps consumers to know which is which.

**Concrete-with-subtypes parents → narrow `<Parent>Like` interfaces.** Where the BMM declares a property with a concrete parent class that has registered subtypes (`LOCATABLE.name DV_TEXT`, `EVENT_CONTEXT.health_care_facility PARTY_IDENTIFIED`, audit envelopes on Versions, `OBJECT_REF`-typed slots, `DV_URI`-typed slots), the generated Go field type is a narrow interface — `DVTextLike`, `PartyIdentifiedLike`, `AuditDetailsLike`, `ObjectRefLike`, `DVURILike`. The interface declares Get-prefixed accessor methods that work uniformly across the parent and every registered subtype:

```go
// Idiomatic — direct method call, no helper
name := c.Name.GetValue()
if code, ok := c.Name.GetDefiningCode(); ok {
    // c.Name was a DV_CODED_TEXT on the wire; code is the terminology binding
}
```

The Get-prefix is mechanical, not stylistic — BMM property names like `value` and `defining_code` are already field identifiers on the concrete structs, and Go forbids a method and a field with the same name on a single type. The closed type-switch helpers in `openehr/rm` (`AsDVText`, `AuditDetailsBase`, …) are compat shims for callers consuming the parent struct value directly; prefer the methods when reading scalar fields.

**Nil interface values:** calling `name.GetValue()` on a `nil` `DVTextLike` (e.g. an optional `*Like`-typed field left unset by the caller) panics like any other nil-interface method call. The compat helpers (`rm.DVTextValueOf(nil) → ""`, `rm.AsDVText(nil) → (zero, false)`) absorb the nil case for callers that want defensive defaults; for required-by-spec slots, prefer the direct method and let the panic surface the missing field early.

**Abstract RM categories → existing marker interfaces.** `DataValue`, `Item`, `ContentItem`, `UIDBasedID`, `PartyProxy`, `DVOrdered`, `ItemStructure`, etc. stay as marker-only interfaces (REQ-040). Reach concrete fields via type assertion:

```go
// Idiomatic — type assert on a marker-only abstract interface
if q, ok := el.Value.(*rm.DVQuantity); ok {
    use(q.Magnitude, q.Units)
}
```

Quick decision matrix:

| RM slot shape | Go field type | Read scalars by | Read full payload by |
|---|---|---|---|
| Concrete parent with subtypes (DV_TEXT, AUDIT_DETAILS, …) | `<Parent>Like` interface | `f.GetValue()` (etc.) — methods | `rm.AsDVText(f)` / `rm.AuditDetailsBase(f)` — helpers |
| Abstract RM category (DATA_VALUE, ITEM_STRUCTURE, …) | abstract Go interface | type assert | type assert |
| Concrete parent without subtypes | concrete struct | direct field access | direct field access |

Adding a new subtype on a BMM bump is documented in [ADR-0001 § Procedure step 10](../adr/0001-bmm-version-bump-runbook.md).

## Errors (REQ-025)

### Wrapping

Errors crossing package boundaries are wrapped with context:

```go
if err := tr.do(ctx, req, &out); err != nil {
    return fmt.Errorf("composition.Save ehr=%s: %w", ehrID, err)
}
```

A wrapped error **MUST** stay reachable with `errors.Is` and `errors.As`, and a custom error type that wraps an inner error **MUST** implement `Unwrap() error`.

### Typed errors

The SDK defines a typed error hierarchy in `transport/` (and re-exported from leaf packages where useful):

```go
type WireError struct {
    StatusCode int
    OpenEHR    *OpenEHRErrorDetail  // ITS-REST error envelope, if parseable
    Inner      error
}

var (
    ErrPreconditionFailed   = errors.New("precondition failed")    // 412
    ErrUnprocessable        = errors.New("unprocessable entity")   // 422
    ErrPreconditionRequired = errors.New("precondition required")  // 428 (defensive; not openEHR-canonical)
    ErrVersionConflict      = errors.New("version conflict")       // 409
    ErrNotFound             = errors.New("not found")              // 404
    ErrUnauthorized         = errors.New("unauthorized")           // 401
    ErrForbidden            = errors.New("forbidden")              // 403
    ErrDiscovery            = errors.New("service catalog error")
)
```

Consumers detect classes with `errors.Is(err, transport.ErrPreconditionFailed)`. Discovery, parse, and auth errors **MUST** have their own sentinel or typed error so they are distinguishable from wire errors.

To extract a typed error's fields — not just match a sentinel — SDK code uses the Go 1.26+ `errors.AsType[E]` form, which the module's minimum Go version ([releases.md § Minimum Go version](../releases.md#minimum-go-version)) always makes available. The older `errors.As(err, &target)` out-parameter remains where that shape is itself the subject, as in the nil-receiver guard in `openehr/client/query`:

```go
// Preferred — Go 1.26+ generic extraction
if we, ok := errors.AsType[*transport.WireError](err); ok {
    use(we.StatusCode, we.OpenEHR)
}

// Also works — the pre-1.26 out-parameter shape
var we *transport.WireError
if errors.As(err, &we) {
    use(we.StatusCode, we.OpenEHR)
}
```

Sentinel matching with `errors.Is` is unchanged. Neither form is nil-proof: a failed match leaves a nil typed pointer that a caller can box into a non-nil `error`, which is why the no-panic rule below covers the nil receiver.

### No panics

Library code **MUST NOT** panic on wire input or on caller input: it returns an error or fails closed instead. In particular:

- Wire input (malformed JSON, unexpected `_type`): a typed error.
- Caller input (nil maps, empty strings, invalid IDs, a nil argument, a zero-value struct whose interface fields are nil): a typed error, or a fail-closed answer. An exported constructor whose signature returns no error fails closed by returning a value that a documented validation or marshalling path rejects with an error.
- A nil variadic option: skipped rather than invoked. `f(..., opts ...Option)` invites a caller to build one conditionally, so a nil element is caller input, not a programmer error.
- A nil receiver on a decode method, whether every generated `UnmarshalJSONFrom` or the hand-written primitive codecs' `UnmarshalJSON` / `UnmarshalText` (`rm.Real`, `rm.Integer`, `rm.Integer64`, `rm.Character`): an error carrying `typereg.ErrNilReceiver`. A census over the type registry checks every registered library type for this guard.
- A nil decoder or target handed to `typereg.DecodeInto` or `typereg.DecodePolymorphic`, or a nil `gotType` handed to `DecodeInto`: a `typereg.DecodeError` carrying `typereg.ErrNilArgument`. A typed nil inside a non-nil target is not caught up front; it reaches the decode and fails there.
- A nil receiver on an exported method of an SDK error type (`Error`, `Unwrap` and any accessor) — the typed nil a failed `errors.As` / `errors.AsType` match leaves behind — and on the accessors of `transport.Client` (`Do`, `Catalog`, `HTTPClient`): an answer. An error type answers with the text its zero value produces unless its godoc documents a different nil-receiver text.
- Authentication failures: `ErrUnauthorized` or a wrapped equivalent.

The one exception is the Go `Must`-prefix convention (`regexp.MustCompile`, `template.Must`): a constructor whose name begins with `Must` **MAY** panic on invalid input, because the prefix is the documented signal that it does. A non-panicking alternative **MUST** be available to callers — a sibling in the same package, or the standard-library call it wraps — so the panic is always the caller's explicit opt-in, and the godoc of the `Must` function says that it panics.

Except for `Must`-prefix constructors, panics are reserved for **programmer errors** the consumer cannot trigger via documented APIs (nil dereference of an unexported struct, broken invariant in the type registry).

## Concurrency (REQ-026)

All public clients **MUST** be safe for concurrent use by multiple goroutines without external synchronisation.

Practical implications:

- Mutable state inside a client is guarded by a mutex or a `sync.Map`, or moved into a per-call context.
- The auth refresh path coalesces concurrent refresh attempts (one goroutine refreshes; the others wait), and the discovery cache coalesces concurrent resolution attempts.
- Connection pooling is handled by the injected `*http.Client`; the SDK does not need to do its own.

A caller waiting on a coalesced refresh or resolution **MUST NOT** be handed the outcome of an attempt that failed only because the running caller's own context ended (its cancellation, deadline or cause), or that panicked: such an attempt releases every waiter, and a waiter whose own context is still live tries again, running a new attempt or joining a newer one, while the panic continues to the caller that ran the attempt. This holds for the token sources of `auth/smart`, `auth/clientcreds` and `auth/jwtbearer`, for the key-set (JWKS) refresh of `auth/smart`, and for discovery resolution, whose cache entry such a failure leaves in place ([service-discovery.md § Refresh API](service-discovery.md#refresh-api)).

Documented exceptions:

- Builders (`aql.New().From(...)`, `composition.NewBuilder(opt)`) are **not** goroutine-safe: they are construct-then-finalise types, not long-lived clients, so a caller does not share one across goroutines, and each builder's package documentation says so.
- The Cassette `probe.Recorder` and `probe.Replayer` (REQ-082) guard their captured and remaining exchanges with a mutex, so both are safe to share across goroutines — no single-goroutine restriction applies.

## Public-API stability

Anything outside `internal/` (REQ-005) participates in semver (REQ-004). Implications:

- Adding to the public surface is a minor bump.
- Renaming or removing a public symbol is a major bump.
- Loosening a return type (e.g. `(*X, error)` → `(any, error)`) is a breaking change even if the new type is a superset.
- Adding a method to a public interface is **breaking** for consumers that implement it — prefer adding a new interface and a runtime type-assertion to introduce optional behaviour.
- Adding a struct field is not a breaking change per [module-layout.md § Versioning](module-layout.md#versioning). Positional struct literals are what break; struct literals **SHOULD** use field-by-name.
