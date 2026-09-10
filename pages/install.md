---
description: >-
  Add the openEHR Go SDK to a module, pin a pre-1.0 tag, and choose the
  building-block path or the REST client path. Host requirement is Go 1.27.x.
---

# Install

Add the module, pin a tag, then pick a path. The walkthrough that lives in the
repository — toolchain notes, both paths, and live-CDR wiring — is
[docs/quick-start.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/quick-start.md).

## Prerequisites

| Requirement | Notes |
|---|---|
| **Go 1.27.x** | Matches the `go 1.27.0` module floor. Host Go is the fast path for `go run` and IDE tooling. |
| **An openEHR backend** | Optional. Building-block examples and the REST mocks run offline. |

## Add the module

The latest tagged release at the time this page was written is `v0.27.0`. The
SDK is pre-1.0: pin that tag (or whichever tag `go get @latest` resolved),
because a minor release can change the public API. Version policy:
[docs/releases.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/releases.md).

```bash
go get github.com/cadasto/openehr-sdk-go@v0.27.0
```

To run the bundled examples or contribute, clone the repository:

```bash
git clone https://github.com/cadasto/openehr-sdk-go.git
cd openehr-sdk-go
make doctor
```

`make doctor` reports whether the Makefile will use host Go 1.27.x or the
Docker fallback.

## Two integration paths

**Building blocks** — no HTTP. Import `openehr/rm`, `openehr/serialize/canjson`,
`openehr/template`, `openehr/validation`, `openehr/instance`, `openehr/aql`.
Use this for CI validation, OPT parse, and canonical-JSON transforms.

**REST client** — a service catalog (`smart/discovery`), an injected
`*http.Client` on `transport`, and typed leaves under `openehr/client/`. Use
this to create EHRs, commit compositions, or run AQL against an openEHR REST
API.

The shortest program on each path is on [Examples](examples.md). The
repository copies, with live-CDR auth wiring, are in
[quick-start.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/quick-start.md).

## What this page does not cover

Cadasto-platform extras under `cadasto/` (Datamap, MPI, Extra API, Care) are
**planned** or **partial**. They ship in the same module behind a cut line so
they can be extracted later. Do not treat them as the openEHR REST surface.

Package-level API detail is on [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
and in [Packages](packages.md).
