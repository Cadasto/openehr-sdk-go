---
description: >-
  How to build, test, and propose a change to the openEHR Go SDK — Makefile
  targets, the specification-driven loop, and where to open an issue.
---

# Contributing

This website is built from the same repository as the SDK
([cadasto/openehr-sdk-go](https://github.com/cadasto/openehr-sdk-go)). Issues
and pull requests for code, specs, and these pages all go there.

The contributor contract is
[CONTRIBUTING.md](https://github.com/cadasto/openehr-sdk-go/blob/main/CONTRIBUTING.md).
This page is the short path.

## Local checks

```bash
make doctor
make ci
```

`make ci` is the pull-request gate: format, `go mod tidy`, vet, unit tests,
lint, spec traceability, fixture integrity, and a compile-all. It needs Docker
for AQL parser codegen verify. `make help` lists the grouped targets.

## Specification-driven changes

New behaviour is specified first. The loop:

1. `make spec-context REQ=NNN` — registry row, traceability, canonical excerpt.
2. Change the topic spec under `docs/specifications/`, then the code and tests.
3. Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`; update
   `docs/specifications/traceability.yaml` in the same change.
4. `make spec-check` (included in `make ci`).

When code and a spec disagree, the spec wins. Open research questions stay in
`docs/specifications/research-strands.md` until an ADR lands.

## This documentation site

```bash
make docs-serve
```

serves the site on `http://127.0.0.1:8000`. `make docs-check` is the strict
build plus the output assertions CI runs before publishing to GitHub Pages.
Brand files come from [Cadasto/docs-theme](https://github.com/Cadasto/docs-theme)
at the commit pinned in `sources.json` — do not edit the fetched CSS or
`overrides/home.html`.

## Security

Vulnerability reports go through
[SECURITY.md](https://github.com/cadasto/openehr-sdk-go/blob/main/SECURITY.md),
not a public issue.
