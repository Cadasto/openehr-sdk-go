---
description: >-
  How to build, test, and propose a change to the openEHR Go SDK — Makefile
  targets, the specification-driven loop, and where to open an issue.
---

# Contributing

This website is built from the same repository as the SDK
([cadasto/openehr-sdk-go](https://github.com/cadasto/openehr-sdk-go)), so
issues and pull requests for code, specifications, and these pages all go to
the same place.

The full contributor contract is
[CONTRIBUTING.md](https://github.com/cadasto/openehr-sdk-go/blob/main/CONTRIBUTING.md).
This page is the short path.

## Local checks

```bash
make doctor
make ci
```

`make doctor` reports which toolchain the Makefile will use: Go 1.27.x on the
host when you have it, the Docker dev image otherwise. `make ci` is the
pull-request gate, and runs formatting, `go mod tidy`, vet, unit tests, lint,
spec traceability, fixture integrity for the FLAT corpus and the terminology
tables, and a compile-all. It needs Docker for the AQL parser codegen check.
`make help` lists the grouped targets.

## Specification-driven changes

New behaviour is specified before it is written. The loop:

1. Run `make spec-context REQ=NNN` for the registry row, the traceability
   block, and the canonical excerpt.
2. Change the topic specification under `docs/specifications/`, then the code
   and the tests.
3. Cite `REQ-NNN` and `PROBE-NNN` in the tests and in `doc.go`, and update
   `docs/specifications/traceability.yaml` in the same change.
4. Run `make spec-check`, which `make ci` includes anyway.

When the code and a specification disagree, the specification wins. Open
research questions stay in `docs/specifications/research-strands.md` until an
ADR settles them.

## This documentation site

`make docs-serve` previews the site on `http://127.0.0.1:8000`.
`make docs-check` runs the strict build plus the output assertions, and is the
gate CI applies before publishing to GitHub Pages.

```bash
make docs-serve
make docs-check
```

Brand files come from
[Cadasto/docs-theme](https://github.com/Cadasto/docs-theme) at the commit named
in `sources.json`. Do not edit the fetched CSS or `overrides/home.html`.

## Security

Vulnerability reports go through
[SECURITY.md](https://github.com/cadasto/openehr-sdk-go/blob/main/SECURITY.md),
not a public issue.
