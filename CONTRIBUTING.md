# Contributing

`openehr-sdk-go` is the first-party Go SDK for openEHR. Contributions are welcome. This file is the short version; how work actually flows here (the spec-first ladder and its gates) is in [`docs/development-process.md`](docs/development-process.md), and [`AGENTS.md`](AGENTS.md) is the entry point shared with coding agents.

## Before you start

1. **Read [`AGENTS.md`](AGENTS.md)**, the 1-page entry point for every contributor and AI agent. It covers the spec-driven workflow, the building-block independence rule, and where the normative contract lives.
2. **Check [`docs/specifications/`](docs/specifications/)**, the source of truth for what the SDK must do. Expect RFC-2119 keywords, REQ/PROBE/STRAND identifiers, and traceability.
3. **Read [`docs/roadmap.md`](docs/roadmap.md)** to see what has landed and what is planned. If your contribution overlaps with a plan in [`docs/plans/`](docs/plans/), coordinate via the issue tracker first.

## How to contribute

### Reporting a bug

- Open a [bug report or feature request](https://github.com/Cadasto/openehr-sdk-go/issues/new/choose) (`.github/ISSUE_TEMPLATE/`).
- Include: Go version (`go version`), OS, the smallest reproducing snippet, the expected vs actual behaviour. If the bug touches the wire (REST clients, canjson, validation against an OPT), include the OPT or composition shape that triggers it.
- **Security bugs**: do NOT open a public issue. Follow [`SECURITY.md`](SECURITY.md) instead.

### Proposing a feature

1. Check [`docs/roadmap.md`](docs/roadmap.md) and [`docs/plans/`](docs/plans/). Your feature may already be planned or explicitly deferred.
2. If the feature touches normative wire behaviour, propose a REQ in [`docs/specifications/`](docs/specifications/) FIRST. The spec comes first and the code follows it.
3. Open an issue describing the feature and how it interacts with the existing REQ catalog. For non-trivial scope, write a plan in [`docs/plans/`](docs/plans/) before any code (copy [`_template.md`](docs/plans/_template.md)).

### Sending a pull request

1. Fork and branch from `main`. Name the branch after what it does: `feat/req-N-short-name`, `fix/short-name`, `docs/<area>`, etc.
2. **Run `make ci` locally** before opening the PR. CI replicates the gate ([`docs/ci.md`](docs/ci.md)).
3. Leave [`CHANGELOG.md`](CHANGELOG.md) to the maintainers. They curate it on request or when a release is cut, not in each feature PR ([AGENTS.md](AGENTS.md#code-style-and-conventions)). Call out any consumer-visible change in the PR description so it can be folded in.
4. If you add or change a REQ-marked behaviour, update [`docs/specifications/traceability.yaml`](docs/specifications/traceability.yaml) in the same PR (`make spec-check` enforces this).
5. Cite REQ-NNN / PROBE-NNN in commit messages and doc comments. REQs are stable identifiers, so never renumber one.
6. **New or changed `cmd/examples/`**: update [`docs/examples.md`](docs/examples.md) and [`cmd/examples/doc.go`](cmd/examples/doc.go) in the same PR; touch [`docs/quick-start.md`](docs/quick-start.md) when the onboarding path changes. See [ai-workflow.md § Examples](docs/ai-workflow.md#examples).
7. Keep PRs **scoped to one logical change**. Small PRs are easier to review and get merged sooner.

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/) style:

```
feat(transport): add Idempotency-Key header support
fix(canjson): handle nil interface in polymorphic decode
docs(plans): track REQ-110 follow-up
```

Write the subject in the imperative mood. The body explains *why*; the diff already shows the *what*. Reference REQ-NNN / PROBE-NNN / issue numbers when relevant. If an AI tool helped write the change, add an `Assisted-by:` trailer. See [AI-assisted contributions](#ai-assisted-contributions).

## AI-assisted contributions

AI coding assistants are a normal part of how this project is built. The tooling is described in [`docs/ai-workflow.md`](docs/ai-workflow.md). Using one is fine. The rules:

- **You are the author.** An assistant is a tool, not a co-author. You are accountable for every line you submit and should be able to explain any of it in review. A change you can't explain isn't ready.
- **Say so.** Add an `Assisted-by:` git trailer to each commit an assistant helped with (format below).
- **Same bar, no exceptions.** A REQ in the spec first for new behaviour, tests that fail if the guard is removed, `make ci` green, review.
- **Look it up; don't take the model's word.** For openEHR facts (RM paths, terminology codes, wire shapes) use the ground-truth lookups in [`docs/ai-workflow.md` § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills). An assistant's recollection of a spec is not a source.
- **Keep private things out of the prompt.** No credentials, tokens, patient or personal data, or content from private repositories in an assistant's context.
- **Licence provenance is on you.** Don't accept generated code that reproduces third-party code under a licence incompatible with MIT. In-tree third-party material is inventoried in [`docs/licensing.md`](docs/licensing.md).

The trailer names the tool, and the model when you know it. Trailers go at the end of the commit message: a blank line after the body, then one line per tool.

```
Assisted-by: Claude Code (claude-opus-4-8)
Assisted-by: Cursor
```

Use `Assisted-by:`, not `Co-authored-by:`. A `Co-authored-by:` line names a person and carries authorship. If a tool inserts a `Co-authored-by:` line for itself, change it to `Assisted-by:`. Reviewers may use assistants as well; a review is still a maintainer's call.

## Local development

The three targets you will run most:

```bash
make ci      # the full PR gate; run it before opening a PR
make test    # unit tests (includes the codegen drift check)
make fmt     # gofumpt + goimports
```

`make help` lists every target, grouped, and [`docs/ci.md`](docs/ci.md) explains what the gate checks. The Docker fallback when you have no host Go is in [`docs/quick-start.md`](docs/quick-start.md#if-you-dont-have-host-go).

### Hooks and IDE integration

Nothing runs on commit, so run `make lint` yourself before opening a PR. If you use Claude Code, its format-on-save hook is described in [`.claude/CLAUDE.md`](.claude/CLAUDE.md).

## Code style

The detailed, normative idiom spec is [`docs/specifications/idiom.md`](docs/specifications/idiom.md). It covers context propagation, `*http.Client` injection, functional options, generics-no-reflection, error wrapping / typed errors, concurrency, imports & naming, and public-API stability. Formatting, lint, and commit conventions are in [AGENTS.md § Code style and conventions](AGENTS.md#code-style-and-conventions). Read those first. These are the points that trip up most PRs:

- **Building-block independence (REQ-013)**: the openEHR building-block packages and the AQL blocks MUST be usable standalone, with no `transport/` or `auth/` import. The exact package set and the per-package import guards are in [AGENTS.md § Code style and conventions](AGENTS.md#code-style-and-conventions) and [`docs/specifications/module-layout.md`](docs/specifications/module-layout.md).
- **No reflection** (REQ-024): RM polymorphism uses closed type-switches only. Generics are fine; `reflect.Value` is not.
- **Strict-encode / permissive-decode** numerics per [ADR 0004](docs/adr/0004-numeric-wire-tolerance.md).
- **Comments**: explain WHY, not WHAT; identifiers carry the WHAT. Cite REQ-NNN / PROBE-NNN where relevant; do NOT cite issue numbers or commit SHAs (those rot). One short line per non-obvious choice; no multi-paragraph docstrings except package-level `doc.go`.
- **Test contexts**: in the I/O-bearing test packages (`transport/`, `auth/`, `smart/`, `openehr/client/*`) use `t.Context()` (Go 1.24+) for request-scoped contexts. It is cancelled at test cleanup, so leaks surface. Don't reintroduce `context.Background()` there; derive timeouts/cancellation from `t.Context()`. Pure-compute test packages are unaffected.

## Releases

See [`docs/releases.md`](docs/releases.md). Maintainers cut tags; contributors do not need to tag.

## Code of conduct

The standard one: be respectful, focus on the technical issue, no harassment. Maintainers will moderate as needed.

## Questions

Open an [issue](https://github.com/Cadasto/openehr-sdk-go/issues/new/choose). A blank issue is fine for a question or design feedback.
