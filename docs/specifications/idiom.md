---
kind: specification
---

# Go idiom

**Status:** Draft

The idiomatic Go surface rules every public package in this SDK adheres to.

The premise: **the SDK is idiomatic Go.**
Wire-level correctness is defined against the openEHR specifications and the conformance probes in [conformance.md](conformance.md), not by a particular source shape.
Java-style "every type is an object", Python-style keyword arguments, repository-graph ports from other languages — none of these constrain the Go API.

## Context propagation (REQ-020)

Every exported function or method that performs network I/O (HTTP, DNS, a discovery lookup) or waits on a deadline **MUST** take `context.Context` as its first parameter, and **MUST** honour that context's cancellation and deadline.
The local readers are the exception: `probe.ValidateHAR` reads a local file the way `os.ReadFile` does, with no context, and `bmm.LoadAll` calls its `Resolver` without one.

```go
// Correct
func (c *Client) GetComposition(ctx context.Context, id ObjectVersionID) (*rm.Composition, error)

// Wrong: no context, even with a sane default
func (c *Client) GetComposition(id ObjectVersionID) (*rm.Composition, error)

// Wrong: context not first
func (c *Client) GetComposition(id ObjectVersionID, ctx context.Context) (*rm.Composition, error)
```

`context.Context` is also the carrier for:

- **Request-scoped values** — primarily for OTel propagation.
  The SDK defines no context-value keys of its own for control flow, which would be an anti-pattern.
- **Per-request auth** — a server acting for many users, such as an MCP server, attaches with `auth.WithTokenSource(ctx, ts)` a source for a token obtained for the openEHR API on the user's behalf; it never forwards the token its own caller sent (see [auth.md § Per-request TokenSource](auth.md#per-request-tokensource)).

Pure functions (codecs, validators with no I/O, AQL string builders) **SHOULD NOT** take `context.Context`, because a context they never consult is noise at every call site.

## HTTP client injection (REQ-021)

All outgoing network I/O **MUST** go through the `*http.Client` the caller injects, its `Transport`, `Jar` and `Timeout` included, and a constructor given no client refuses with an error rather than fall back to a client of its own.
Constructors accept the client via functional option or wrapper:

```go
// Explicit injection
client, err := transport.New(catalog,
    transport.WithHTTPClient(httpClient),
    transport.WithTokenSource(ts),
)
```

The SDK allocates no `*http.Client` of its own.
Three shallow copies of the injected client are the exceptions, each changing only its redirect policy, and each **MUST** leave the injected client unmodified:

- the copy that [auth.md § Credential-bearing requests](auth.md#credential-bearing-requests) prescribes for token, refresh and revocation requests;
- the copy the discovery resolver fetches with, to refuse a redirect to a URL that is not https ([auth.md § REQ-072](auth.md#trust-posture) *Trust posture*);
- the copy the transport sends a request carrying a credential header (`Authorization`, `Proxy-Authorization`, `Cookie` or `Cookie2`) with, to refuse a redirect from https to a URL that is not https ([transport.md § REQ-092](transport.md#req-092--tls-posture)).

Each copy keeps the injected client's `Transport`, `Jar` and `Timeout`.

Rationale:

- Connection-pool sizing, timeouts, TLS roots, proxy config belong to the consumer's runtime.
- The federator use case constructs many clients with independent transport configs.
- Benchmarks need precise control over transport tuning (e.g. `MaxIdleConnsPerHost`).

The package documentation of `transport/` says that the injected client governs all outgoing network I/O.

## API conventions

These conventions shape the public surface and the library code behind it.
They are informative, apart from the `Repository` rule of [§ Surface shape](#surface-shape): the code follows them, and the code can change them without an amendment here.

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
  The auth providers (`auth/smart`, `auth/clientcreds`, `auth/jwtbearer`) are the exception: their options act on an exported `Config`, which also serves as the serialised form below.
- A public `Config` struct for serialised configuration (YAML, env) can exist, and the constructor routes it through the same option chain.

The SDK does not use:

- Setter methods after construction (`client.SetHTTPClient(...)`) — mutable after construction implies a hidden lock or a data race.
- Builder-pattern intermediate types (`ehr.NewBuilder().WithX(...).Build()`) — verbose and harder to compose than functional options.
- The "options bag" pattern (`transport.New(catalog, transport.Options{HTTPClient: ..., TokenSource: ...})`) — a second configuration style beside functional options.
  A keyed literal keeps compiling when a field is added, and [module-layout.md § Versioning](module-layout.md#versioning) does not class that as a breaking change.

### Surface shape

The primary call-site surface is **package-level functions**:

```go
// Idiomatic
import "github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"

saved, meta, err := composition.Save(ctx, client, ehrID, comp, composition.WithPrefer(transport.PreferRepresentation))
```

A repository-style struct surface can be offered alongside as an injection-seam convenience:

```go
// Allowed seam for tests / DI
repo := composition.NewRepository(client)
saved, meta, err := repo.Save(ctx, ehrID, comp)
```

Such a repository is an interface (e.g. `composition.Repository`), so consumers can mock it directly rather than wrap the package functions, and the package-level functions stay present beside it.
When a leaf gains an operation or an option, its package's `Repository` interface grows with it.
That is no break for a caller of the package functions, but it breaks an implementer of the interface at compile time ([module-layout.md § REQ-004](module-layout.md#req-004--semantic-versioning)).

### Generics and reflection

Go generics are used where they remove a reflection hop, an `interface{}` cast, or a type assertion, never decoratively.

```go
// Idiomatic — the caller names the type, no reflection (openehr/rm/typereg)
func DecodeAs[T any](data []byte) (T, error)

// Not used — a decoder that returns any for a polymorphic _type
func Decode(data []byte) (any, error) // forces type assertions everywhere
```

Library code (non-test code outside `cmd/`) uses no reflection, with two exceptions: ordinary field mapping over struct tags (it is what `encoding/json` does), and uses that never choose a Go type from `_type`.
The second exception has four classes: a typed-nil or zero-value check, a value comparison (`reflect.DeepEqual`), a type name in an error message, and an addressable copy that reaches pointer-receiver methods.
The type registry ([reference-model.md § Type registry (REQ-040)](reference-model.md#type-registry-req-040)) is the only mechanism that projects the `_type` discriminator onto a concrete Go type.
A guard test still checks this convention: it holds every library file that imports `reflect` to a reviewed list naming the class of each use.

- A constraint is a closed type set, such as `contribution.Versionable` (`rm.Composition | rm.EHRStatus | rm.Folder | rm.EHRAccess`), or a marker method plus the necessary method set — the same rule as for abstract categories.
- A constraint appears only in the type parameter list of a function signature: `contribution.Creation[T Versionable](data *T, …)` keeps the caller's type, where a parameter of an interface type would erase it.

If a generic API is harder to read than a `T`-specific one for the most common call site, the generic is wrong — drop it.

### String iteration

A check defined over **bytes** — a wire-format guard, a byte-exact scanner — iterates `for i := range len(s)`, never `for i := range s`.
Ranging a string yields rune-start indices and skips UTF-8 continuation bytes, so a byte loop rewritten that way stops inspecting every byte.
`for i := range s` is correct only where runes are genuinely the unit.

`go fix` deliberately declines to rewrite `for i := 0; i < len(s); i++` on a string for exactly this reason — it applies the `rangeint` fixer to slices only.
A `range s` over a byte-oriented check is therefore always hand-written, and always worth a second look in review.

### Imports and naming

- **Import groups:** standard library; third-party; module-internal — separated by blank lines.
  `gofumpt` + `goimports` (run via `golangci-lint fmt` / `make fmt`) handle ordering and enforce the group separation.
- **Package names:** short, lowercase, no underscores, no plurals (`composition`, not `compositions`).
  Avoid stuttering — `composition.Composition` is allowed if the type is the package's primary export; `composition.Service` and `composition.Repository` are preferred for non-primary types.
- **Exported names:** `CamelCase`.
  Acronyms preserve case (`HTTPClient`, `JWKS`, `EHRID`).
- **Unexported names:** `camelCase` with leading lowercase.
- **Constructor names:** `New<Thing>` returns `*Thing` or `Thing`; `New<Thing>(opts ...Option)` returns `(*Thing, error)` when configuration validation can fail.

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

A wire failure is a `*transport.WireError` matched by a status sentinel such as `transport.ErrPreconditionFailed` ([transport.md § REQ-093](transport.md#req-093--openehr-error-envelope-mapping)).
Consumers detect classes with `errors.Is(err, transport.ErrPreconditionFailed)` and read the details with `errors.AsType[*transport.WireError]`.
Discovery, parse, and auth errors **MUST** have their own sentinel or typed error, in their own package (such as `*discovery.DiscoveryError` and `*auth.ExchangeError`), so they are distinguishable from wire errors.

To extract a typed error's fields — not just match a sentinel — SDK code uses the Go 1.26+ `errors.AsType[E]` form, which the module's minimum Go version ([releases.md § Minimum Go version](../releases.md#minimum-go-version)) always makes available.
The older `errors.As(err, &target)` out-parameter remains where that shape is itself the subject, as in the nil-receiver guard in `openehr/client/query`:

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

Sentinel matching with `errors.Is` is unchanged.
Neither form is nil-proof: a failed match leaves a nil typed pointer that a caller can box into a non-nil `error`, which is why the no-panic rule below covers the nil receiver.

### No panics

Library code **MUST NOT** panic on wire input or on caller input: it returns an error or fails closed instead.
In particular:

- Wire input (malformed JSON, unexpected `_type`): a typed error.
- Caller input (nil maps, empty strings, invalid IDs, a nil argument, a zero-value struct whose interface fields are nil): a typed error, or a fail-closed answer.
  An exported constructor whose signature returns no error fails closed by returning a value that a documented validation or marshalling path rejects with an error.
- A nil variadic option: skipped rather than invoked.
  `f(..., opts ...Option)` invites a caller to build one conditionally, so a nil element is caller input, not a programmer error.
- A nil receiver on a decode method, whether every generated `UnmarshalJSONFrom` or the hand-written primitive codecs' `UnmarshalJSON` / `UnmarshalText` (`rm.Real`, `rm.Integer`, `rm.Integer64`, `rm.Character`): an error carrying `typereg.ErrNilReceiver`.
  A nil decoder handed to a generated `UnmarshalJSONFrom`: an error carrying `typereg.ErrNilArgument`.
  A census over the type registry checks every registered library type for both guards.
- A nil decoder or target handed to `typereg.DecodeInto`, `typereg.DecodePolymorphic` or `typereg.DecodeTypeMember`, or a nil `gotType` handed to `DecodeInto`: a `typereg.DecodeError` carrying `typereg.ErrNilArgument`.
  A nil encoder or decoder handed to `canxml.EncodePoly`, `EncodeString`, `EncodeText`, `DecodeAs` or `DecodeAsOrDefault`: an error carrying `typereg.ErrNilArgument`, inside a `canxml.DecodeError` from the two decoders.
  A typed nil inside a non-nil target is not caught up front; it reaches the decode and fails there.
- A nil receiver on an exported method of an SDK error type (`Error`, `Unwrap` and any accessor) — the typed nil a failed `errors.As` / `errors.AsType` match leaves behind — and on the accessors of `transport.Client` (`Do`, `Catalog`, `HTTPClient`): an answer.
  An error type answers with the text its zero value produces unless its godoc documents a different nil-receiver text.
- Authentication failures: `ErrUnauthorized` or a wrapped equivalent.

The one exception is the Go `Must`-prefix convention (`regexp.MustCompile`, `template.Must`): a constructor whose name begins with `Must` **MAY** panic on invalid input, because the prefix is the documented signal that it does.
A non-panicking alternative **MUST** be available to callers — a sibling in the same package, or the standard-library call it wraps — so the panic is always the caller's explicit opt-in, and the godoc of the `Must` function says that it panics.

Except for `Must`-prefix constructors, panics are reserved for **programmer errors** the consumer cannot trigger via documented APIs (nil dereference of an unexported struct, broken invariant in the type registry).

## Concurrency (REQ-026)

All public clients **MUST** be safe for concurrent use by multiple goroutines without external synchronisation.

Practical implications:

- Mutable state inside a client is guarded by a mutex or a `sync.Map`, or moved into a per-call context.
- The auth refresh path coalesces concurrent refresh attempts (one goroutine refreshes; the others wait), and the discovery cache coalesces concurrent resolution attempts.
- Connection pooling is handled by the injected `*http.Client`; the SDK does not need to do its own.

A caller waiting on a coalesced refresh or resolution **MUST NOT** be handed the outcome of an attempt that failed only because the running caller's own context ended (its cancellation, deadline or cause), or that panicked; such an attempt instead:

- releases every waiter;
- lets a waiter whose own context is still live try again, running a new attempt or joining a newer one;
- lets the panic continue unchanged to the caller that ran the attempt.

A caller whose own context ends before the coalesced attempt it started or joined has answered **MUST** get an error that `errors.Is` matches to its context's error.
These rules hold for the token sources of `auth/smart`, `auth/clientcreds` and `auth/jwtbearer`, for the key-set (JWKS) refresh of `auth/smart`, and for discovery resolution, whose cache entry such a failure leaves in place ([auth.md § Refresh API](auth.md#refresh-api)).
The token refresh of `auth/smart` runs on a goroutine of its own that the starting caller's context does not end ([auth.md § REQ-063](auth.md#req-063--token-refresh-and-revocation)), so it never fails only because that context ended.
Its panic goes to the caller that started it while that caller waits; once that caller has returned, the panic surfaces on the refresh's goroutine after the waiters are released, as any unrecovered panic in a goroutine does.

Documented exceptions:

- Builders (`aql.NewBuilder()`, `composition.NewBuilder(ctx, compiled)`, `contribution.NewBuilder()`) are **not** goroutine-safe: they are construct-then-finalise types, not long-lived clients, so a caller does not share one across goroutines.
- The Cassette `probe.Recorder` and `probe.Replayer` (REQ-082) guard their captured and remaining exchanges with a mutex, so both are safe to share across goroutines — no single-goroutine restriction applies.
