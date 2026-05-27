# Go idiom

**Status:** Draft

The idiomatic Go surface rules every public package in this SDK adheres to. Covers REQ-020 through REQ-026.

The premise: **the SDK is idiomatic Go, not a Go transliteration of the PHP SDK.** Cross-SDK parity is enforced at the wire (REQ-081), not in source. Java-style "every type is an object", Python-style keyword arguments, PHP-style repository graphs — none of these constrain the Go API.

## Context propagation (REQ-020)

Every method, function, or constructor that performs I/O — HTTP, file system, network DNS, disk-cached lookup, OS clock if used for deadlines — **MUST** take `context.Context` as its first parameter.

```go
// MUST
func (c *Client) GetComposition(ctx context.Context, id ObjectVersionID) (*rm.Composition, error)

// MUST NOT — no context, even with a sane default
func (c *Client) GetComposition(id ObjectVersionID) (*rm.Composition, error)

// MUST NOT — context not first
func (c *Client) GetComposition(id ObjectVersionID, ctx context.Context) (*rm.Composition, error)
```

`context.Context` is also the carrier for:

- **Cancellation and deadlines** — every blocking call inside the SDK **MUST** respect `ctx.Done()`.
- **Request-scoped values** — primarily for OTel propagation; the SDK **MUST NOT** invent its own context-value keys for control-flow purposes (anti-pattern).
- **Per-request auth** — the MCP server use case forwards an incoming token via `auth.WithTokenSource(ctx, ts)` (see [auth.md § Per-request TokenSource](auth.md#per-request-tokensource)).

Pure functions (codecs, validators with no I/O, AQL string builders) **MUST NOT** take `context.Context` — adding it for "consistency" is noise.

## HTTP client injection (REQ-021)

The SDK **MUST NOT** allocate its own `*http.Client`. Constructors accept one via functional option or wrapper. Acceptable patterns:

```go
// MUST — explicit injection
client, err := ehr.New(catalog,
    transport.WithHTTPClient(httpClient),
    transport.WithTokenSource(ts),
)

// MAY — nil/zero means "use http.DefaultClient" if documented
client, err := ehr.New(catalog) // documented as: uses http.DefaultClient with no timeout — typically not what you want
```

Rationale:

- Connection-pool sizing, timeouts, TLS roots, proxy config belong to the consumer's runtime.
- The federator use case constructs many clients with independent transport configs.
- Benchmarks need precise control over transport tuning (e.g. `MaxIdleConnsPerHost`).

The SDK **MUST** document in `transport/`'s `doc.go` that the injected client governs all outgoing network I/O.

## Functional options (REQ-022)

SDK constructors **SHOULD** accept configuration via functional options:

```go
client, err := ehr.New(catalog,
    transport.WithHTTPClient(httpClient),
    transport.WithTokenSource(ts),
    transport.WithSpecVersion("1.1.0-development"),
    transport.WithUserAgent("my-app/1.0"),
    transport.WithRetry(retry.Policy{...}),
)
```

Rules:

- Options **MUST** be types implementing a single small interface (typically `func(*config) error` or `func(*config)`).
- Options **MUST** be additive — applying two options of the same family **MUST** result in well-defined merge behaviour (typically "last write wins" with a documented exception for slice-accumulator options).
- The `config` struct is **unexported**; consumers configure it only through `With*` options.
- A public `Config` struct **MAY** exist for serialised configuration (YAML, env), but the constructor **MUST** route it through the same option chain.

Antipatterns the SDK **MUST NOT** use:

- Setter methods after construction (`client.SetHTTPClient(...)`) — mutable after construction implies a hidden lock or a data race.
- Builder-pattern intermediate types (`ehr.NewBuilder().WithX(...).Build()`) — verbose and harder to compose than functional options.
- "Options bag" pattern (`ehr.New(catalog, ehr.Options{HTTPClient: ..., TokenSource: ...})`) — every new field is a breaking change to the struct literal.

## Surface shape (REQ-023)

The primary call-site surface **SHOULD** be **package-level functions**:

```go
// Idiomatic
import "github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"

err := composition.Save(ctx, ehrID, comp, write.Options{...})
```

Repository-style struct surfaces **MAY** be offered alongside as an injection-seam convenience:

```go
// Allowed seam for tests / DI
repo := composition.NewRepository(httpClient, ts, catalog)
err := repo.Save(ctx, ehrID, comp, write.Options{...})
```

But the repository **MUST** be defined as an interface (e.g. `composition.Repository`) so that consumers can mock it directly, not by wrapping the package functions. The package-level functions **MUST** be present even when a `Repository` is offered.

## Generics policy (REQ-024)

Use Go generics where they remove a reflection hop, an `interface{}` cast, or a type assertion. Do not use generics decoratively.

```go
// Idiomatic — typed response, no reflection
func Get[T rm.Resource](ctx context.Context, c *Client, id string) (T, error)

// MUST NOT — reflection-based decoder for polymorphic _type
func Get(ctx context.Context, c *Client, id string) (any, error) // forces type assertions everywhere
```

The type registry (REQ-040) is the **only** sanctioned mechanism for projecting the `_type` discriminator onto a concrete Go type. Reflection over struct tags is acceptable for ordinary JSON field mapping (it is what `encoding/json` does); reflection to dispatch on `_type` is not.

If a generic API is harder to read than a `T`-specific one for the most common call site, the generic is wrong — drop it.

## Substitution slots and the `*Like` interfaces

The openEHR RM permits Liskov substitution at every property slot (per AOM `valid_value` semantics). The Go SDK surfaces this on two distinct surfaces; the call pattern differs between them and consumers should know which is which.

**Concrete-with-subtypes parents → narrow `<Parent>Like` interfaces.** Where the BMM declares a property with a concrete parent class that has registered subtypes (`LOCATABLE.name DV_TEXT`, `EVENT_CONTEXT.health_care_facility PARTY_IDENTIFIED`, audit envelopes on Versions, `OBJECT_REF`-typed slots, `DV_URI`-typed slots), the generated Go field type is a narrow interface — [`DVTextLike`](../../openehr/rm/like_interfaces.go), `PartyIdentifiedLike`, `AuditDetailsLike`, `ObjectRefLike`, `DVURILike`. The interface declares Get-prefixed accessor methods that work uniformly across the parent and every registered subtype:

```go
// Idiomatic — direct method call, no helper
name := c.Name.GetValue()
if code, ok := c.Name.GetDefiningCode(); ok {
    // c.Name was a DV_CODED_TEXT on the wire; code is the terminology binding
}
```

The Get-prefix is mechanical, not stylistic — BMM property names like `value` and `defining_code` are already field identifiers on the concrete structs, and Go forbids a method and a field with the same name on a single type. The closed type-switch helpers in [`openehr/rm/like_accessors.go`](../../openehr/rm/like_accessors.go) (`AsDVText`, `AuditDetailsBase`, …) are compat shims for callers consuming the parent struct value directly; prefer the methods when reading scalar fields.

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

Errors crossing package boundaries **MUST** be wrapped with context:

```go
if err := tr.do(ctx, req, &out); err != nil {
    return fmt.Errorf("composition.Save ehr=%s: %w", ehrID, err)
}
```

The wrapped error **MUST** be unwrappable with `errors.Is` and `errors.As`. Custom error types **MUST** implement `Unwrap() error` when they wrap an inner error.

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
    ErrPreconditionRequired = errors.New("precondition required")  // 428
    ErrVersionConflict      = errors.New("version conflict")       // 409
    ErrNotFound             = errors.New("not found")              // 404
    ErrUnauthorized         = errors.New("unauthorized")           // 401
    ErrForbidden            = errors.New("forbidden")              // 403
    ErrDiscovery            = errors.New("service catalog error")
)
```

Consumers detect classes with `errors.Is(err, transport.ErrPreconditionFailed)`. Discovery, parse, and auth errors **MUST** have their own sentinel or typed error so they are distinguishable from wire errors.

### No panics

Library code **MUST NOT** panic on:

- Wire input (malformed JSON, unexpected `_type`) — return a typed error.
- Consumer input (nil maps, empty strings, invalid IDs) — return a typed error or document a documented preconditions.
- Authentication failures — return `ErrUnauthorized` or a wrapped equivalent.

Panics are reserved for **programmer errors** the consumer cannot trigger via documented APIs (nil dereference of an unexported struct, broken invariant in the type registry).

## Concurrency (REQ-026)

All public clients **MUST** be safe for concurrent use by multiple goroutines without external synchronisation.

Practical implications:

- Mutable state inside a client **MUST** be guarded by a mutex, a `sync.Map`, or moved into a per-call context.
- The auth refresh path **MUST** coalesce concurrent refresh attempts (one goroutine refreshes; the others wait).
- The discovery cache **MUST** coalesce concurrent resolution attempts.
- Connection pooling is handled by the injected `*http.Client`; the SDK does not need to do its own.

Documented exceptions:

- Builders (`aql.New().From(...)`, `composition.NewBuilder(opt)`) are **not** goroutine-safe and **MUST NOT** be shared across goroutines — they are construct-then-finalise types, not long-lived clients. Document this in each builder's `doc.go`.
- The recorder/replay transport in `sandbox/` **MAY** be single-goroutine in record mode if the consumer-side construction is single-threaded; if so it **MUST** be documented.

## Imports and naming

- **Import groups:** standard library; third-party; module-internal — separated by blank lines. `gofmt` handles ordering within groups; consumers **SHOULD** run `goimports` to enforce the group separation.
- **Package names:** short, lowercase, no underscores, no plurals (`composition`, not `compositions`). Avoid stuttering — `composition.Composition` is allowed if the type is the package's primary export; `composition.Service` and `composition.Repository` are preferred for non-primary types.
- **Exported names:** `CamelCase`. Acronyms preserve case (`HTTPClient`, `JWKS`, `EHRID`).
- **Unexported names:** `camelCase` with leading lowercase.
- **Constructor names:** `New<Thing>` returns `*Thing` or `Thing`; `New<Thing>(opts ...Option)` returns `(*Thing, error)` when configuration validation can fail.

## Public-API stability

Anything outside `internal/` (REQ-005) participates in semver (REQ-004). Implications:

- Adding to the public surface is a minor bump.
- Renaming or removing a public symbol is a major bump.
- Loosening a return type (e.g. `(*X, error)` → `(any, error)`) is a breaking change even if the new type is a superset.
- Adding a method to a public interface is **breaking** for consumers that implement it — prefer adding a new interface and a runtime type-assertion to introduce optional behaviour.
- Adding a struct field is **breaking** for consumers using positional construction (`Thing{a, b, c}`) — always document that struct literals **SHOULD** use field-by-name.
