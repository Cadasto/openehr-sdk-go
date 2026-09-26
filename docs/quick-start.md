# Quick start

Get from zero to a working import in a few minutes. This guide is for application developers integrating the SDK. Contributors editing the normative specs should start with [CONTRIBUTING.md](../CONTRIBUTING.md). For the full contract and package map, see [architecture.md](architecture.md) and [specifications/](specifications/).

> **Version:** the SDK is pre-1.0. Pin an exact tag, because a minor release may break the public API. See [releases.md](releases.md).

## Prerequisites

| Requirement | Notes |
|---|---|
| **Go 1.27.x** | Matches `go.mod` (minimum `1.27.0`). Host Go is the fast path for `go run` and IDE tooling. |
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

The SDK keeps clinical building blocks separate from HTTP clients, so you can import one package without pulling in auth or transport.

```text
Building blocks (no HTTP)          REST client path
─────────────────────────          ─────────────────
openehr/rm                         smart/discovery  →  service catalog
openehr/serialize/canjson          transport        →  injected *http.Client + auth
openehr/template                   openehr/client/* →  typed REST methods
openehr/validation
openehr/instance
```

Pick building blocks when you validate compositions in CI, parse OPT files, or transform canonical JSON. You do not need a clinical data repository (CDR).

Pick the REST path when you create EHRs, submit compositions, or run AQL against a live openEHR REST API.

Runnable walkthroughs for both paths live in [examples.md](examples.md).

---

## Path A — Building blocks (no network)

This is the smallest useful program. It decodes canonical JSON into typed Reference Model (RM) structs:

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

From a clone of this repo, run the equivalent example. It reads a vendored cassette, so there is no file to set up:

```bash
go run ./cmd/examples/canonical_json
```

Expected output includes the composition archetype id, language, and `OK: canonical-JSON Composition decoded`.

### Validate against a template

A typical CI pipeline runs bytes → RM → compiled OPT → validation issues.

```bash
go run ./cmd/examples/validate-from-json
```

This decodes `testdata/minimal_blood_pressure.json`, compiles `vital_signs.opt`, and prints either `result : OK — JSON validates against OPT` or a list of constraint violations. See [examples.md](examples.md#validate-from-json) for flags and custom file paths.

---

## Path B — REST client (live or mocked backend)

Every REST call flows through three layers:

1. **Service catalog** (`smart/discovery`) says where the openEHR REST base URL lives.
2. **Transport client** (`transport`) injects your `*http.Client`, attaches auth, and handles retries and OpenTelemetry (OTel) tracing.
3. **Leaf client** gives typed methods per REST resource (`openehr/client/ehr`, `query`, `definition`, …).

### Minimal wiring (in-process sandbox)

[`sandbox.Backend`](../sandbox/doc.go) is an in-memory openEHR REST backend that implements `http.RoundTripper`. It needs no listener and no credentials. Inject it as the client's Transport:

```go
b := sandbox.New()
cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
	Issuer: "https://sandbox.local",
	Services: map[string]discovery.ServiceEntry{
		discovery.ServiceIDOpenEHRRest: {
			BaseURL:     discovery.MustParseURL("https://sandbox.local/openehr/v1"),
			SpecVersion: discovery.SpecVersionPin,
		},
	},
})
if err != nil {
	log.Fatal(err)
}
c, err := transport.New(cat, transport.WithHTTPClient(b.HTTPClient()))
if err != nil {
	log.Fatal(err)
}
created, meta, err := ehr.Create(ctx, c)
```

`ehr` is the leaf client `openehr/client/ehr`. `created` is the decoded `*rm.EHR`, and `meta` carries the response headers such as `Location`.

To run the same call without importing `sandbox/`, use the [`ehr_create`](../cmd/examples/ehr_create/main.go) example. It targets a throwaway handler:

```bash
go run ./cmd/examples/ehr_create
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

When one process serves many users, attach a different token to each call through the context:

```go
ctx = auth.WithTokenSource(ctx, perRequestTokenSource)
created, meta, err := ehr.Create(ctx, c)
```

---

## Idioms to remember

Every public API follows these rules, and code that breaks them usually ends up working against the SDK.

| Rule | Why |
|---|---|
| `context.Context` is always the first parameter on I/O methods | Cancellation, deadlines, per-request auth. |
| Inject `*http.Client`; the SDK never allocates one | Connection pooling and TLS stay under your control. |
| Use functional options (`transport.WithHTTPClient`, …) | No large config structs, and options combine freely. |
| Prefer package-level functions over repository structs | Repositories exist as injection seams, not the primary surface. |
| Import building blocks without `transport/` when you can | Keeps CLI tools and validators lightweight. |

Full normative list: [specifications/idiom.md](specifications/idiom.md).

---

## If you don't have host Go

When host Go 1.27.x is missing, the Makefile routes every target through a Docker dev image. Run `make image-dev` once, then use `make` as normal. `make doctor` tells you which toolchain is active. [ci.md](ci.md) and [CONTRIBUTING.md](../CONTRIBUTING.md) cover the contributor targets and the PR gate.

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
