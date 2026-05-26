# Contributing

`openehr-sdk-go` is the first-party Go SDK for openEHR. Contributions are welcome; this file is the short version. Long form: [`AGENTS.md`](AGENTS.md).

## Before you start

1. **Read [`AGENTS.md`](AGENTS.md)** — the 1-page entry point for every contributor and AI agent. Covers the spec-driven workflow, building-block independence rule, and where the normative contract lives.
2. **Check [`docs/specifications/`](docs/specifications/)** — the source of truth for what the SDK must do. RFC-2119 keywords, REQ/PROBE/STRAND identifiers, traceability.
3. **Read [`docs/roadmap.md`](docs/roadmap.md)** — landed vs planned. If your contribution overlaps with a planned plan in [`docs/plans/`](docs/plans/), coordinate via the issue tracker first.

## How to contribute

### Reporting a bug

- Open a [GitHub issue](https://github.com/Cadasto/openehr-sdk-go/issues/new).
- Include: Go version (`go version`), OS, the smallest reproducing snippet, the expected vs actual behaviour. If the bug touches the wire (REST clients, canjson, validation against an OPT), include the OPT or composition shape that triggers it.
- **Security bugs**: do NOT open a public issue — follow [`SECURITY.md`](SECURITY.md).

### Proposing a feature

1. Check [`docs/roadmap.md`](docs/roadmap.md) and [`docs/plans/`](docs/plans/) — your feature may already be planned or explicitly deferred.
2. If the feature touches normative wire behaviour, propose a REQ in [`docs/specifications/`](docs/specifications/) FIRST. Code follows spec, not the other way around.
3. Open an issue describing the feature and how it interacts with the existing REQ catalog. For non-trivial scope, a plan in [`docs/plans/`](docs/plans/) (copy [`_template.md`](docs/plans/_template.md)) is the right artefact before code.

### Sending a pull request

1. Fork + branch from `main`. Name the branch after what it does: `feat/req-N-short-name`, `fix/short-name`, `docs/<area>`, etc.
2. **Run `make ci` locally** before opening the PR. CI replicates the gate ([`docs/ci.md`](docs/ci.md)).
3. Update [`CHANGELOG.md`](CHANGELOG.md) under `## [Unreleased]` if your change is consumer-visible. Pre-1.0 we use **only `### Added`**; fold fix-ups into Added bullets — see the file's preamble.
4. If you add or change a REQ-marked behaviour, update [`docs/specifications/traceability.yaml`](docs/specifications/traceability.yaml) in the same PR (`make spec-check` enforces this).
5. Cite REQ-NNN / PROBE-NNN in commit messages and doc comments. REQs are stable identifiers — never renumber.
6. Keep PRs **scoped to one logical change**. The reviewer's job is easier; your change ships faster.

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/) style:

```
feat(transport): add Idempotency-Key header support
fix(canjson): handle nil interface in polymorphic decode
docs(plans): track REQ-110 follow-up
```

Imperative mood. Body explains *why* (the *what* is in the diff). Reference REQ-NNN / PROBE-NNN / issue numbers when relevant.

## Local development

```bash
make help        # grouped targets
make ci          # full PR gate
make test        # unit tests
make fmt         # gofmt -w -s
make vet
make lint
make codegen     # regenerate RM + AOM from resources/bmm/
make codegen-verify  # fail if codegen drifts
make spec-check  # validate docs/specifications/traceability.yaml
```

Go `1.25.x` on the host is the fast path; the Makefile transparently routes through a Docker dev image if host Go is missing. See [`docs/ci.md`](docs/ci.md).

### Hooks and IDE integration

- After Write/Edit on `*.go`, Claude Code runs `gofmt -w -s` on the touched file (see [`.claude/hooks/gofmt-on-save.sh`](.claude/hooks/gofmt-on-save.sh)).
- Pre-commit linters are not enforced by hook; run `make lint` before opening a PR.

## Code style

- **Building-block independence (REQ-013)**: `openehr/{rm,serialize,validation,template,instance,composition}/` and `openehr/aql/` (models only) MUST be usable standalone, with no imports of `transport/`, `auth/`, or `openehr/client/*`. Enforced by `TestXxxForbiddenImports` in each package. See [`docs/specifications/module-layout.md`](docs/specifications/module-layout.md).
- **No reflection** (REQ-024) — closed type-switches only on RM polymorphism. Generics are fine; `reflect.Value` is not.
- **Strict-encode / permissive-decode** numerics per [ADR 0004](docs/adr/0004-numeric-wire-tolerance.md).
- **Comments**: WHY, not WHAT. Identifiers carry the WHAT. Cite REQ-NNN / PROBE-NNN where relevant; do NOT cite issue numbers or commit SHAs (those rot).
- One short comment line per non-obvious choice. No multi-paragraph docstrings except on package-level `doc.go`.

## Releases

See [`docs/releases.md`](docs/releases.md). Maintainers cut tags; contributors do not need to tag.

## Code of conduct

The standard one: be respectful, focus on the technical issue, no harassment. Maintainers will moderate as needed.

## Questions

Open a [discussion](https://github.com/Cadasto/openehr-sdk-go/discussions) or an issue. For Cadasto-internal alignment, coordinate via your usual private channels — gap reports and consumer drafts are not linked from this public repository.
