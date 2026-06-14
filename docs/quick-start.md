# Quick start

Get from zero to a working import in a few minutes. This guide targets **application developers** integrating the SDK — not contributors editing normative specs. For the full contract and package map, see [architecture.md](architecture.md) and [specifications/](specifications/).

> **Version:** the SDK is pre-1.0 — pin an exact tag; minors may break public API. See [releases.md](releases.md).

## Prerequisites

| Requirement | Notes |
|---|---|
| **Go 1.25.x** | Matches `go.mod`. Host Go is the fast path for `go run` and IDE tooling. |
| **Make** (optional) | Recommended for contributors and CI-parity checks (`make ci`). |
| **An openEHR backend** (optional) | Only needed for live REST calls. Most building-block examples run offline with vendored fixtures. |

## Install

Add the module to your project:

```bash
go get github.com/cadasto/openehr-sdk-go@latest   # pre-1.0: pin an exact tag (see releases.md)
```

Clone this repository if you want to run the bundled examples or contribute:

```bash
git clone https://github.com/cadasto/openehr-sdk-go.git
cd openehr-sdk-go
make doctor   # host Go vs Docker fallback
```

## Two integration paths

The SDK deliberately splits **clinical building blocks** from **HTTP clients**. You can import one package without pulling in auth or transport.

```text
Building blocks (no HTTP)          REST client path
─────────────────────────          ─────────────────
openehr/rm                         smart/discovery  →  service catalog
openehr/serialize/canjson          transport        →  injected *http.Client + auth
openehr/template                   openehr/client/* →  typed REST methods
openehr/validation
openehr/instance
```

**Pick building blocks** when you validate compositions in CI, parse OPT files, or transform canonical JSON — no CDR required.

**Pick the REST path** when you create EHRs, submit compositions, or run AQL against a live openEHR REST API.

Runnable walkthroughs for both paths live in [examples.md](examples.md).

---

## Path A — Building blocks (no network)

Decode canonical JSON into typed RM structs. This is the smallest useful program:

```go
package main

import (
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

func main() {
	body, err := os.ReadFile("composition.json")
	if err != nil {
		log.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		log.Fatal(err)
	}
	log.Printf("decoded %q with %d content item(s)", comp.Name.GetValue(), len(comp.Content))
}
```

From a clone of this repo, run the equivalent example (uses a vendored cassette — no file setup needed):

```bash
go run ./cmd/examples/canonical_json
```

Expected output includes the composition archetype id, language, and `OK: canonical-JSON Composition decoded`.

### Validate against a template

Typical CI pipeline: **bytes → RM → compiled OPT → validation issues**.

```bash
go run ./cmd/examples/validate-from-json
```

This decodes `testdata/minimal_blood_pressure.json`, compiles `vital_signs.opt`, and prints either `result : OK — JSON validates against OPT` or a list of constraint violations. See [examples.md](examples.md#validate-from-json) for flags and custom file paths.

---

## Path B — REST client (live or mocked backend)

Every REST call flows through three layers:

1. **Service catalog** — where the openEHR REST base URL lives (`smart/discovery`).
2. **Transport client** — injects your `*http.Client`, attaches auth, handles retries and OTel (`transport`).
3. **Leaf client** — typed methods per REST resource (`openehr/client/ehr`, `query`, `definition`, …).

### Minimal wiring (in-process mock)

The [`ehr_create`](../cmd/examples/ehr_create/main.go) example spins up an `httptest` server and calls `ehr.Create`:

```bash
go run ./cmd/examples/ehr_create
```

Core wiring (abbreviated):

```go
cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
	Issuer: "https://example.test",
	Services: map[string]discovery.ServiceEntry{
		discovery.ServiceIDOpenEHRRest: {
			BaseURL:     discovery.MustParseURL("https://cdr.example/openehr/v1"),
			SpecVersion: discovery.SpecVersionPin,
		},
	},
})
if err != nil {
	log.Fatal(err)
}

hc := &http.Client{Timeout: 30 * time.Second}
c, err := transport.New(cat, transport.WithHTTPClient(hc))
if err != nil {
	log.Fatal(err)
}

ehr, meta, err := openehrclient.Create(ctx, c)
```

### Pointing at a real CDR

Replace the static catalog URL with your deployment's openEHR REST base (usually ending in `/openehr/v1`). Inject auth when the backend requires it:

```go
import "github.com/cadasto/openehr-sdk-go/auth/clientcreds"

ts, err := clientcreds.New(
	os.Getenv("CLIENT_ID"),
	os.Getenv("CLIENT_SECRET"),
	"https://auth.example/oauth/token",
	clientcreds.WithHTTPClient(hc),
)
if err != nil {
	log.Fatal(err)
}

c, err := transport.New(cat,
	transport.WithHTTPClient(hc),
	transport.WithTokenSource(ts),
)
```

For SMART-on-openEHR launches, use `auth/smart` and the application-level helpers under `smart/`. Details: [specifications/auth.md](specifications/auth.md).

### Per-request auth (MCP, multi-tenant)

Attach a different token per call via context — useful when one process serves many users:

```go
ctx = auth.WithTokenSource(ctx, perRequestTokenSource)
ehr, meta, err := openehrclient.Create(ctx, c)
```

---

## Idioms to remember

These rules show up in every public API. Breaking them usually means fighting the SDK rather than using it.

| Rule | Why |
|---|---|
| `context.Context` is always the first parameter on I/O methods | Cancellation, deadlines, per-request auth. |
| Inject `*http.Client` — the SDK never allocates one | Connection pooling and TLS stay under your control. |
| Use functional options (`transport.WithHTTPClient`, …) | No giant config structs; options compose cleanly. |
| Prefer package-level functions over repository structs | Repositories exist as injection seams, not the primary surface. |
| Import building blocks without `transport/` when you can | Keeps CLI tools and validators lightweight (REQ-013). |

Full normative list: [specifications/idiom.md](specifications/idiom.md).

---

## Makefile essentials (contributors)

From the repo root:

```bash
make help     # discover targets
make fmt      # gofumpt + goimports
make test     # unit tests + codegen drift check
make lint     # golangci-lint (same config as CI)
make ci       # full PR gate — run before opening a PR
make build    # compile all packages including cmd/examples
```

If host Go is missing, `make image-dev` once, then Make transparently routes through Docker. See [ci.md](ci.md).

---

## What to read next

| Goal | Doc |
|---|---|
| Run and understand every bundled example | [examples.md](examples.md) |
| Package layout and dependency diagram | [architecture.md](architecture.md) |
| Auth providers and SMART launch | [specifications/auth.md](specifications/auth.md) |
| Wire formats (canonical JSON, REST envelopes) | [specifications/wire.md](specifications/wire.md) |
| Landed vs planned features | [roadmap.md](roadmap.md) |
| pkg.go.dev API reference | [pkg.go.dev/github.com/cadasto/openehr-sdk-go](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go) |
