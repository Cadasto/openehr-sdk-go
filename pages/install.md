---
description: >-
  Add the openEHR Go SDK to a Go module, pin a pre-1.0 tag, and choose between
  the building-block packages and the REST client. Requires Go 1.27 or newer.
---

# Install

Add the module, pin a tag, then pick a path.

## Prerequisites

| Requirement | Notes |
|---|---|
| **Go 1.27 or newer** | The floor in `go.mod` is `1.27.0`. |
| **A clinical data repository (CDR)** | Optional. Nothing on this page needs one: the building-block examples and the REST examples both run offline. |

## Add the module

The current release is `v0.27.0`. The SDK is pre-1.0, so pin an exact tag: a
minor release can change the public API. The version policy is
[docs/releases.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/releases.md).

```bash
go get github.com/cadasto/openehr-sdk-go@v0.27.0
```

To run the bundled examples, or to work on the SDK itself, clone the
repository instead:

```bash
git clone https://github.com/cadasto/openehr-sdk-go.git
cd openehr-sdk-go
```

[Contributing](contributing.md) covers the local checks and the toolchain the
Makefile picks.

## Two integration paths

**Building blocks** need no HTTP at all. Import `openehr/rm`,
`openehr/serialize/canjson`, `openehr/template`, `openehr/validation`,
`openehr/instance`, or `openehr/aql`. This is the path for validation in CI,
for parsing an operational template (OPT), and for canonical-JSON transforms.

**The REST client** adds three pieces: a service catalog (`smart/discovery`),
an injected `*http.Client` on `transport`, and the client packages under
`openehr/client/`. Take this path to create EHRs, commit Compositions, or run
AQL against an openEHR REST API.

The shortest program on each path is on [Examples](examples.md). The longer
walkthrough in the repository, which also shows how to wire authentication for
a live CDR, is
[docs/quick-start.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/quick-start.md).
[Workflow](workflow.md#which-cdr) lists the CDRs this client has been run
against.

The full import map is on [Packages](packages.md) — including the `cadasto/`
packages, which ship in the same module but are not part of the openEHR REST
surface. Package-level API detail is on
[pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go).
